package simulate

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"testing"

	"github.com/ruptor-dev/cli/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubLLMClient struct {
	responses []string
	callCount int
}

func (s *stubLLMClient) Complete(_ context.Context, _ []map[string]string, _ string) (string, error) {
	resp := s.responses[s.callCount%len(s.responses)]
	s.callCount++
	return resp, nil
}

// scriptedGoalChecker returns YES on a specific turn and NO otherwise,
// letting tests pin goal-reached behavior without touching the LLM.
type scriptedGoalChecker struct {
	reachOnCall int // 1-based. 0 disables.
	calls       int
	err         error
}

func (g *scriptedGoalChecker) Check(_ context.Context, _, _, _ string) (bool, error) {
	g.calls++
	if g.err != nil {
		return false, g.err
	}
	return g.reachOnCall > 0 && g.calls == g.reachOnCall, nil
}

func TestSimulatorRun(t *testing.T) {
	logger := slog.Default()

	tests := []struct {
		name         string
		sim          config.Simulation
		responses    []string
		reachOnCall  int
		wantReached  bool
		wantTurns    int
	}{
		{
			name: "goal reached on first turn",
			sim: config.Simulation{
				ID:              "test-1",
				Persona:         "impatient customer",
				Goal:            "get a refund",
				MaxTurns:        5,
				SuccessCriteria: "refund approved",
			},
			responses:   []string{"I want a refund", "Your refund is approved."},
			reachOnCall: 1,
			wantReached: true,
			wantTurns:   1,
		},
		{
			name: "goal reached on second turn",
			sim: config.Simulation{
				ID:              "test-2",
				Persona:         "confused user",
				Goal:            "reset password",
				MaxTurns:        5,
				SuccessCriteria: "password has been reset",
			},
			responses: []string{
				"I forgot my password",
				"Could you provide your email?",
				"My email is user@example.com",
				"Your password has been reset.",
			},
			reachOnCall: 2,
			wantReached: true,
			wantTurns:   2,
		},
		{
			name: "max turns reached without goal",
			sim: config.Simulation{
				ID:              "test-3",
				Persona:         "stubborn user",
				Goal:            "get a discount",
				MaxTurns:        3,
				SuccessCriteria: "discount granted",
			},
			responses:   []string{"Give me a discount", "I cannot help with that."},
			reachOnCall: 0,
			wantReached: false,
			wantTurns:   3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubLLMClient{responses: tt.responses}
			goal := &scriptedGoalChecker{reachOnCall: tt.reachOnCall}
			sim := NewSimulatorFromBaseURL(stub, http.DefaultClient, logger, "", 30).
				WithGoalChecker(goal)

			result, err := sim.Run(context.Background(), tt.sim)
			require.NoError(t, err)
			assert.Equal(t, tt.sim.ID, result.SimulationID)
			assert.Equal(t, tt.sim.Persona, result.Persona)
			assert.Equal(t, tt.sim.Goal, result.Goal)
			assert.Equal(t, tt.wantReached, result.GoalReached)
			assert.Equal(t, tt.wantTurns, result.TurnCount)
			assert.Equal(t, tt.sim.MaxTurns, result.MaxTurns)
		})
	}
}

func TestSimulatorRun_GoalCheckErrorDoesNotAbort(t *testing.T) {
	// A transient goal-checker failure must not fail the run. The turn
	// continues, logging a warning; the run ends naturally at max_turns.
	stub := &stubLLMClient{responses: []string{"hello", "response"}}
	goal := &scriptedGoalChecker{err: errors.New("llm down")}
	sim := NewSimulatorFromBaseURL(stub, http.DefaultClient, slog.Default(), "", 30).
		WithGoalChecker(goal)

	result, err := sim.Run(context.Background(), config.Simulation{
		ID:              "err-test",
		Persona:         "user",
		Goal:            "whatever",
		MaxTurns:        2,
		SuccessCriteria: "done",
	})
	require.NoError(t, err)
	assert.False(t, result.GoalReached)
	assert.Equal(t, 2, result.TurnCount)
	assert.Equal(t, 2, goal.calls)
}

func TestSimulatorRun_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	stub := &stubLLMClient{responses: []string{"hello", "world"}}
	sim := NewSimulatorFromBaseURL(stub, http.DefaultClient, slog.Default(), "http://localhost:3000", 30)

	result, err := sim.Run(ctx, config.Simulation{
		ID:              "cancel-test",
		Persona:         "user",
		Goal:            "test",
		MaxTurns:        5,
		SuccessCriteria: "done",
	})

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "simulate: running cancel-test")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestPersonaPromptBuilder(t *testing.T) {
	builder := &PersonaPromptBuilder{}

	prompt := builder.Build("angry customer", "get a refund", "refund processed")

	assert.Contains(t, prompt, "angry customer")
	assert.Contains(t, prompt, "get a refund")
	assert.Contains(t, prompt, "refund processed")
	assert.Contains(t, prompt, "Stay in character")
}

func TestLLMGoalChecker(t *testing.T) {
	tests := []struct {
		name     string
		reply    string
		criteria string
		want     bool
	}{
		{"yes answer", "YES", "password reset", true},
		{"lowercase yes", "yes", "password reset", true},
		{"yes with whitespace", " YES ", "password reset", true},
		{"no answer", "NO", "password reset", false},
		{"paragraph answer is not yes", "The assistant mostly did what was asked.", "password reset", false},
		{"empty criteria short-circuits", "YES", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &LLMGoalChecker{Client: &stubLLMClient{responses: []string{tt.reply}}}
			got, err := c.Check(context.Background(), "some agent message", "some goal", tt.criteria)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLLMGoalChecker_NilClient(t *testing.T) {
	c := &LLMGoalChecker{Client: nil}
	got, err := c.Check(context.Background(), "agent reply", "goal", "criteria")
	require.NoError(t, err)
	assert.False(t, got)
}
