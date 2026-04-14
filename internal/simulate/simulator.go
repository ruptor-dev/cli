package simulate

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/faultforge/faultforge/internal/config"
	"github.com/faultforge/faultforge/pkg/types"
)

// LLMClient abstracts the language model used for generating simulated user
// messages and agent responses.
type LLMClient interface {
	Complete(ctx context.Context, messages []map[string]string, model string) (string, error)
}

// Simulator orchestrates simulated conversations between a persona-driven user
// and an AI agent under test.
type Simulator struct {
	llmClient  LLMClient
	httpClient *http.Client
	logger     *slog.Logger
}

// NewSimulator creates a Simulator with the provided dependencies.
func NewSimulator(llmClient LLMClient, httpClient *http.Client, logger *slog.Logger) *Simulator {
	return &Simulator{
		llmClient:  llmClient,
		httpClient: httpClient,
		logger:     logger,
	}
}

// Run executes a single simulation scenario, returning the result with timing
// and goal-reached status.
func (s *Simulator) Run(ctx context.Context, sim config.Simulation) (*types.SimulationResult, error) {
	start := time.Now()

	builder := &PersonaPromptBuilder{}
	systemPrompt := builder.Build(sim.Persona, sim.Goal, sim.SuccessCriteria)

	history := &types.ConversationHistory{}
	history.Add("system", systemPrompt)

	var goalReached bool
	var turnCount int

	for turn := 0; turn < sim.MaxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("simulate: running %s: %w", sim.ID, err)
		}

		// Generate simulated user message.
		userMsg, err := s.generateUserMessage(ctx, history)
		if err != nil {
			return nil, fmt.Errorf("simulate: running %s: generating user message: %w", sim.ID, err)
		}
		history.Add("user", userMsg)

		s.logger.Info("simulated user message",
			slog.String("simulation_id", sim.ID),
			slog.Int("turn", turn+1),
			slog.String("message", userMsg),
		)

		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("simulate: running %s: %w", sim.ID, err)
		}

		// Generate agent response.
		agentResp, err := s.generateAgentResponse(ctx, history)
		if err != nil {
			return nil, fmt.Errorf("simulate: running %s: generating agent response: %w", sim.ID, err)
		}
		history.Add("assistant", agentResp)

		s.logger.Info("agent response",
			slog.String("simulation_id", sim.ID),
			slog.Int("turn", turn+1),
			slog.String("message", agentResp),
		)

		turnCount = turn + 1

		if checkGoalReached(agentResp, sim.SuccessCriteria) {
			goalReached = true
			break
		}
	}

	duration := time.Since(start)

	return &types.SimulationResult{
		SimulationID: sim.ID,
		Persona:      sim.Persona,
		Goal:         sim.Goal,
		GoalReached:  goalReached,
		TurnCount:    turnCount,
		MaxTurns:     sim.MaxTurns,
		DurationMs:   duration.Milliseconds(),
	}, nil
}

// generateUserMessage calls the LLM with the conversation history to produce
// the next simulated user message.
func (s *Simulator) generateUserMessage(ctx context.Context, history *types.ConversationHistory) (string, error) {
	resp, err := s.llmClient.Complete(ctx, history.ToOpenAIMessages(), "")
	if err != nil {
		return "", fmt.Errorf("simulate: llm complete: %w", err)
	}
	return resp, nil
}

// generateAgentResponse calls the LLM acting as the agent under test.
func (s *Simulator) generateAgentResponse(ctx context.Context, history *types.ConversationHistory) (string, error) {
	// Build agent-perspective messages: strip the persona system prompt and
	// present the conversation from the agent's point of view.
	msgs := history.ToOpenAIMessages()
	agentMsgs := make([]map[string]string, 0, len(msgs))
	for _, m := range msgs {
		if m["role"] == "system" {
			continue
		}
		agentMsgs = append(agentMsgs, m)
	}

	resp, err := s.llmClient.Complete(ctx, agentMsgs, "")
	if err != nil {
		return "", fmt.Errorf("simulate: llm complete: %w", err)
	}
	return resp, nil
}

// checkGoalReached determines whether the agent response satisfies the success
// criteria. An empty criteria string is never matched.
func checkGoalReached(agentResponse, successCriteria string) bool {
	if successCriteria == "" {
		return false
	}
	return strings.Contains(
		strings.ToLower(agentResponse),
		strings.ToLower(successCriteria),
	)
}
