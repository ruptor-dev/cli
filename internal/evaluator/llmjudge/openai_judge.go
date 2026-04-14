package llmjudge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/ruptor-dev/cli/internal/evaluator/llmjudge/prompts"
	"github.com/ruptor-dev/cli/internal/llmclient"
	"github.com/ruptor-dev/cli/pkg/types"
)

// OpenAIJudge evaluates agent behavior using the OpenAI chat completions API.
// Transport, auth, and HTTP error handling are delegated to llmclient.OpenAIClient.
type OpenAIJudge struct {
	client *llmclient.OpenAIClient
	logger *slog.Logger
}

// NewOpenAIJudge creates a new OpenAI-backed judge.
// If model is empty, defaults to llmclient.DefaultModel.
func NewOpenAIJudge(apiKey, model string, logger *slog.Logger) *OpenAIJudge {
	return &OpenAIJudge{
		client: llmclient.New(apiKey, model, llmclient.WithLogger(logger)),
		logger: logger,
	}
}

// NewOpenAIJudgeWithClient wires a judge to a caller-provided client. Useful
// when the caller wants to share a single client across multiple components.
func NewOpenAIJudgeWithClient(client *llmclient.OpenAIClient, logger *slog.Logger) *OpenAIJudge {
	return &OpenAIJudge{client: client, logger: logger}
}

func (j *OpenAIJudge) EvaluateChaos(ctx context.Context, prompt, agentBehavior string) (string, string, error) {
	j.logger.Debug("evaluating chaos behavior with LLM judge",
		slog.String("model", j.client.Model()),
	)

	systemMsg := prompts.ChaosSystem
	if prompt != "" {
		systemMsg = prompt
	}

	userMsg := fmt.Sprintf("Agent behavior during fault injection:\n\n%s", agentBehavior)

	content, err := j.client.Complete(ctx, []llmclient.Message{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userMsg},
	}, "")
	if err != nil {
		return "", "", fmt.Errorf("llmjudge: evaluating chaos: %w", err)
	}

	verdict, reason := parseChaosResponse(content)
	return verdict, reason, nil
}

func (j *OpenAIJudge) EvaluateConversation(ctx context.Context, prompt string, history *types.ConversationHistory) (int, []string, error) {
	j.logger.Debug("evaluating conversation with LLM judge",
		slog.String("model", j.client.Model()),
		slog.Int("turns", len(history.Turns)),
	)

	systemMsg := prompts.ConversationSystem
	if prompt != "" {
		systemMsg = prompt
	}

	convBytes, err := json.Marshal(history.ToOpenAIMessages())
	if err != nil {
		return 0, nil, fmt.Errorf("llmjudge: marshaling conversation: %w", err)
	}

	userMsg := fmt.Sprintf("Evaluate this conversation:\n\n%s", string(convBytes))

	content, err := j.client.Complete(ctx, []llmclient.Message{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userMsg},
	}, "")
	if err != nil {
		return 0, nil, fmt.Errorf("llmjudge: evaluating conversation: %w", err)
	}

	score, issues, err := parseConversationResponse(content)
	if err != nil {
		return 0, nil, fmt.Errorf("llmjudge: parsing conversation response: %w", err)
	}

	return score, issues, nil
}

// parseChaosResponse extracts verdict and reason from the LLM response.
func parseChaosResponse(content string) (string, string) {
	verdict := "FAIL"
	reason := content

	lines := strings.Split(strings.TrimSpace(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		upper := strings.ToUpper(line)
		if strings.HasPrefix(upper, "VERDICT:") {
			v := strings.TrimSpace(strings.TrimPrefix(upper, "VERDICT:"))
			switch {
			case strings.HasPrefix(v, "PASS"):
				verdict = "PASS"
			case strings.HasPrefix(v, "PARTIAL"):
				verdict = "PARTIAL"
			default:
				verdict = "FAIL"
			}
		}
		if strings.HasPrefix(upper, "REASON:") {
			reason = strings.TrimSpace(line[len("REASON:"):])
		}
	}

	return verdict, reason
}

// parseConversationResponse extracts score and issues from the LLM JSON response.
func parseConversationResponse(content string) (int, []string, error) {
	var result struct {
		Score  int      `json:"score"`
		Issues []string `json:"issues"`
	}

	// Try to find JSON in the response (it might be wrapped in markdown code blocks).
	jsonStr := content
	if idx := strings.Index(content, "{"); idx >= 0 {
		if end := strings.LastIndex(content, "}"); end > idx {
			jsonStr = content[idx : end+1]
		}
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return 0, nil, fmt.Errorf("invalid JSON response: %w", err)
	}

	return result.Score, result.Issues, nil
}
