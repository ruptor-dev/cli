package simulate

import (
	"context"
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

func TestSimulatorRun(t *testing.T) {
	logger := slog.Default()

	tests := []struct {
		name        string
		sim         config.Simulation
		responses   []string
		wantReached bool
		wantTurns   int
		wantErr     bool
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
			// Turn 1: user message, then agent response containing criteria.
			responses:   []string{"I want a refund for my order", "Your refund approved. It will be processed in 3-5 days."},
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
			// Turn 1: user msg, agent (no criteria). Turn 2: user msg, agent (criteria).
			responses: []string{
				"I forgot my password",
				"Could you provide your email?",
				"My email is user@example.com",
				"Your password has been reset. Check your inbox.",
			},
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
			// Responses never contain "discount granted".
			responses:   []string{"Give me a discount", "I understand your concern, but I cannot offer that."},
			wantReached: false,
			wantTurns:   3,
		},
		{
			name: "empty persona and goal still works",
			sim: config.Simulation{
				ID:              "test-4",
				Persona:         "",
				Goal:            "",
				MaxTurns:        2,
				SuccessCriteria: "done",
			},
			responses:   []string{"hello", "done"},
			wantReached: true,
			wantTurns:   1,
		},
		{
			name: "empty success criteria never matches",
			sim: config.Simulation{
				ID:              "test-5",
				Persona:         "tester",
				Goal:            "test empty criteria",
				MaxTurns:        2,
				SuccessCriteria: "",
			},
			responses:   []string{"hi", "sure thing"},
			wantReached: false,
			wantTurns:   2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := &stubLLMClient{responses: tt.responses}
			sim := NewSimulator(stub, http.DefaultClient, logger, "", 30)

			result, err := sim.Run(context.Background(), tt.sim)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.sim.ID, result.SimulationID)
			assert.Equal(t, tt.sim.Persona, result.Persona)
			assert.Equal(t, tt.sim.Goal, result.Goal)
			assert.Equal(t, tt.wantReached, result.GoalReached)
			assert.Equal(t, tt.wantTurns, result.TurnCount)
			assert.Equal(t, tt.sim.MaxTurns, result.MaxTurns)
			assert.Greater(t, result.DurationMs, int64(-1))
		})
	}
}

func TestSimulatorRun_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	stub := &stubLLMClient{responses: []string{"hello", "world"}}
	sim := NewSimulator(stub, http.DefaultClient, slog.Default(), "http://localhost:3000", 30)

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

func TestCheckGoalReached(t *testing.T) {
	tests := []struct {
		name     string
		response string
		criteria string
		want     bool
	}{
		{"exact match", "refund approved", "refund approved", true},
		{"case insensitive", "Refund APPROVED", "refund approved", true},
		{"substring match", "Your refund approved today.", "refund approved", true},
		{"no match", "I cannot help with that", "refund approved", false},
		{"empty criteria", "anything here", "", false},
		{"empty response", "", "refund approved", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, checkGoalReached(tt.response, tt.criteria))
		})
	}
}
