// Package llmclient provides HTTP clients for language-model APIs used by both
// the LLM judge (evaluator) and the user simulator.
//
// The shared OpenAIClient consolidates request construction, authentication,
// and error handling so that both callers get the same behavior: HTTP status
// checks before JSON decode, structured logging of the target model, and
// uniform error wrapping.
package llmclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

const (
	// DefaultOpenAIURL is the Chat Completions endpoint.
	DefaultOpenAIURL = "https://api.openai.com/v1/chat/completions"
	// DefaultModel is the model used when a caller passes an empty model name.
	DefaultModel = "gpt-4o-mini"
)

// Message is a single chat message. Exposed so callers can build messages
// without marshaling through map[string]string.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OpenAIClient speaks the OpenAI Chat Completions API. It is safe for
// concurrent use by multiple goroutines.
type OpenAIClient struct {
	apiKey string
	model  string
	url    string
	http   *http.Client
	logger *slog.Logger
}

// Option configures an OpenAIClient at construction time.
type Option func(*OpenAIClient)

// WithHTTPClient overrides the default HTTP client. Use this to set a shared
// timeout, transport, or to inject a test double.
func WithHTTPClient(c *http.Client) Option {
	return func(o *OpenAIClient) { o.http = c }
}

// WithURL overrides the API endpoint. Useful for Azure OpenAI deployments
// or for pointing tests at an httptest.Server.
func WithURL(url string) Option {
	return func(o *OpenAIClient) { o.url = url }
}

// WithLogger attaches a logger for request-level diagnostics.
func WithLogger(l *slog.Logger) Option {
	return func(o *OpenAIClient) { o.logger = l }
}

// New constructs an OpenAIClient. apiKey is required; model defaults to
// DefaultModel if empty.
func New(apiKey, model string, opts ...Option) *OpenAIClient {
	if model == "" {
		model = DefaultModel
	}
	c := &OpenAIClient{
		apiKey: apiKey,
		model:  model,
		url:    DefaultOpenAIURL,
		http:   &http.Client{},
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Model returns the configured model name.
func (c *OpenAIClient) Model() string { return c.model }

// Complete sends the given messages to the chat completions endpoint and
// returns the first choice's content. If model is empty the client-level
// default is used.
//
// Errors returned include:
//   - context cancellation / deadline
//   - non-2xx HTTP status (body included in message)
//   - malformed JSON response
//   - API-reported error
//   - empty choices array
func (c *OpenAIClient) Complete(ctx context.Context, messages []Message, model string) (string, error) {
	if model == "" {
		model = c.model
	}

	reqBody := struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
	}{Model: model, Messages: messages}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("llmclient: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("llmclient: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	c.logger.Debug("llmclient: request", slog.String("model", model), slog.Int("messages", len(messages)))

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("llmclient: sending request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("llmclient: reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("llmclient: openai HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		return "", fmt.Errorf("llmclient: parsing response: %w", err)
	}
	if chatResp.Error != nil {
		return "", fmt.Errorf("llmclient: api error: %s", chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("llmclient: no choices in response")
	}
	return chatResp.Choices[0].Message.Content, nil
}

// CompleteMap is a convenience wrapper for callers that already hold messages
// as []map[string]string (e.g. simulate.LLMClient). It performs the
// conversion and delegates to Complete.
func (c *OpenAIClient) CompleteMap(ctx context.Context, messages []map[string]string, model string) (string, error) {
	msgs := make([]Message, 0, len(messages))
	for _, m := range messages {
		msgs = append(msgs, Message{Role: m["role"], Content: m["content"]})
	}
	return c.Complete(ctx, msgs, model)
}
