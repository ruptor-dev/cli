package llmjudge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/faultforge/faultforge/pkg/types"
)

const (
	openaiAPIURL = "https://api.openai.com/v1/chat/completions"
	defaultModel = "gpt-4o-mini"
)

// OpenAIJudge evaluates agent behavior using the OpenAI chat completions API.
type OpenAIJudge struct {
	apiKey string
	model  string
	client *http.Client
	logger *slog.Logger
}

// NewOpenAIJudge creates a new OpenAI-backed judge.
// If model is empty, defaults to "gpt-4o-mini".
func NewOpenAIJudge(apiKey, model string, logger *slog.Logger) *OpenAIJudge {
	if model == "" {
		model = defaultModel
	}
	return &OpenAIJudge{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{},
		logger: logger,
	}
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (j *OpenAIJudge) EvaluateChaos(ctx context.Context, prompt, agentBehavior string) (string, string, error) {
	j.logger.Debug("evaluating chaos behavior with LLM judge",
		slog.String("model", j.model),
	)

	systemMsg := "You are an AI reliability evaluator. Analyze the agent's behavior and respond with exactly two lines:\nLine 1: VERDICT: PASS or FAIL or PARTIAL\nLine 2: REASON: <your explanation>"
	if prompt != "" {
		systemMsg = prompt
	}

	userMsg := fmt.Sprintf("Agent behavior during fault injection:\n\n%s", agentBehavior)

	content, err := j.complete(ctx, []chatMessage{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userMsg},
	})
	if err != nil {
		return "", "", fmt.Errorf("llmjudge: evaluating chaos: %w", err)
	}

	verdict, reason := parseChaosResponse(content)
	return verdict, reason, nil
}

func (j *OpenAIJudge) EvaluateConversation(ctx context.Context, prompt string, history *types.ConversationHistory) (int, []string, error) {
	j.logger.Debug("evaluating conversation with LLM judge",
		slog.String("model", j.model),
		slog.Int("turns", len(history.Turns)),
	)

	systemMsg := `You are a conversation quality evaluator. Analyze the conversation and respond with valid JSON:
{"score": <0-100>, "issues": ["issue1", "issue2"]}`
	if prompt != "" {
		systemMsg = prompt
	}

	convBytes, err := json.Marshal(history.ToOpenAIMessages())
	if err != nil {
		return 0, nil, fmt.Errorf("llmjudge: marshaling conversation: %w", err)
	}

	userMsg := fmt.Sprintf("Evaluate this conversation:\n\n%s", string(convBytes))

	content, err := j.complete(ctx, []chatMessage{
		{Role: "system", Content: systemMsg},
		{Role: "user", Content: userMsg},
	})
	if err != nil {
		return 0, nil, fmt.Errorf("llmjudge: evaluating conversation: %w", err)
	}

	score, issues, err := parseConversationResponse(content)
	if err != nil {
		return 0, nil, fmt.Errorf("llmjudge: parsing conversation response: %w", err)
	}

	return score, issues, nil
}

func (j *OpenAIJudge) complete(ctx context.Context, messages []chatMessage) (string, error) {
	reqBody := chatRequest{
		Model:    j.model,
		Messages: messages,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, openaiAPIURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+j.apiKey)

	resp, err := j.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", fmt.Errorf("parsing response: %w", err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("API error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, nil
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
