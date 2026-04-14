package simulate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/pkg/types"
)

// LLMClient abstracts the language model used for generating simulated user
// messages and agent responses.
type LLMClient interface {
	Complete(ctx context.Context, messages []map[string]string, model string) (string, error)
}

// Simulator orchestrates simulated conversations between a persona-driven user
// and an AI agent under test.
type Simulator struct {
	llmClient      LLMClient
	httpClient     *http.Client
	logger         *slog.Logger
	agentBaseURL   string
	agentTimeoutS  int
}

// NewSimulator creates a Simulator with the provided dependencies.
func NewSimulator(llmClient LLMClient, httpClient *http.Client, logger *slog.Logger, agentBaseURL string, agentTimeoutS int) *Simulator {
	return &Simulator{
		llmClient:     llmClient,
		httpClient:    httpClient,
		logger:        logger,
		agentBaseURL:  agentBaseURL,
		agentTimeoutS: agentTimeoutS,
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
		History:      history,
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

// generateAgentResponse calls the agent under test via HTTP, or falls back to LLMClient if no baseURL configured.
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

	// If no agent baseURL configured, use LLMClient (for testing or when not available).
	if s.agentBaseURL == "" {
		resp, err := s.llmClient.Complete(ctx, agentMsgs, "")
		if err != nil {
			return "", fmt.Errorf("simulate: llm complete: %w", err)
		}
		return resp, nil
	}

	// Call agent under test via HTTP.
	reqBody := map[string]interface{}{
		"messages": agentMsgs,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("simulate: marshaling request: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, time.Duration(s.agentTimeoutS)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "POST", s.agentBaseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("simulate: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("simulate: calling agent: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("simulate: agent returned HTTP %d", resp.StatusCode)
	}

	var respData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		return "", fmt.Errorf("simulate: decoding response: %w", err)
	}

	respStr, ok := respData["response"].(string)
	if !ok {
		respStr, ok = respData["message"].(string)
	}
	if !ok {
		return "", fmt.Errorf("simulate: response missing 'response' or 'message' field")
	}

	return respStr, nil
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
