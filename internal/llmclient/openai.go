// Package llmclient provides HTTP clients for language-model APIs used by
// both the LLM judge (evaluator) and the user simulator.
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
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/rs/zerolog"
)

const (
	// DefaultOpenAIURL is the Chat Completions endpoint.
	DefaultOpenAIURL = "https://api.openai.com/v1/chat/completions"
	// DefaultModel is the model used when a caller passes an empty model name.
	DefaultModel = "gpt-4o-mini"
)

// Message is a single chat message.
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
	logger zerolog.Logger
	// retry is nil when retry is disabled.
	retry *retryPolicy
}

// retryPolicy holds the backoff configuration for transient failures.
type retryPolicy struct {
	initialInterval time.Duration
	maxInterval     time.Duration
	maxElapsed      time.Duration
	randomization   float64
	multiplier      float64
}

// Option configures an OpenAIClient at construction time.
type Option func(*OpenAIClient)

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(o *OpenAIClient) { o.http = c }
}

// WithURL overrides the API endpoint.
func WithURL(url string) Option {
	return func(o *OpenAIClient) { o.url = url }
}

// WithLogger attaches a logger for request-level diagnostics.
func WithLogger(l zerolog.Logger) Option {
	return func(o *OpenAIClient) { o.logger = l }
}

// WithRetry enables exponential backoff retry with jitter for transient
// failures (HTTP 429, 5xx, network errors). Disabled by default because
// tests want deterministic single-shot calls.
func WithRetry() Option {
	return func(o *OpenAIClient) {
		o.retry = defaultRetry()
	}
}

func defaultRetry() *retryPolicy {
	return &retryPolicy{
		initialInterval: 500 * time.Millisecond,
		maxInterval:     30 * time.Second,
		maxElapsed:      5 * time.Minute,
		randomization:   0.5,
		multiplier:      2.0,
	}
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
		logger: zerolog.Nop(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Model returns the configured model name.
func (c *OpenAIClient) Model() string { return c.model }

// Complete sends the given messages to the chat completions endpoint and
// returns the first choice's content. When WithRetry is enabled, transient
// HTTP errors (429, 5xx) and network errors are retried with exponential
// backoff plus jitter; permanent errors (4xx other than 429) short-circuit.
func (c *OpenAIClient) Complete(ctx context.Context, messages []Message, model string) (string, error) {
	if model == "" {
		model = c.model
	}

	bodyBytes, err := c.marshalRequest(model, messages)
	if err != nil {
		return "", err
	}

	if c.retry == nil {
		return c.doOnce(ctx, bodyBytes, model, messages)
	}
	return c.doWithRetry(ctx, bodyBytes, model, messages)
}

func (c *OpenAIClient) marshalRequest(model string, messages []Message) ([]byte, error) {
	reqBody := struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
	}{Model: model, Messages: messages}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("llmclient: marshaling request: %w", err)
	}
	return b, nil
}

func (c *OpenAIClient) doOnce(ctx context.Context, bodyBytes []byte, model string, messages []Message) (string, error) {
	content, _, err := c.send(ctx, bodyBytes, model, messages)
	return content, err
}

func (c *OpenAIClient) doWithRetry(ctx context.Context, bodyBytes []byte, model string, messages []Message) (string, error) {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = c.retry.initialInterval
	b.Multiplier = c.retry.multiplier
	b.RandomizationFactor = c.retry.randomization
	b.MaxInterval = c.retry.maxInterval
	b.MaxElapsedTime = c.retry.maxElapsed

	var last string
	op := func() error {
		content, transient, err := c.send(ctx, bodyBytes, model, messages)
		if err == nil {
			last = content
			return nil
		}
		if !transient {
			return backoff.Permanent(err)
		}
		c.logger.Warn().Err(err).Msg("llmclient: transient failure, retrying")
		return err
	}
	if err := backoff.Retry(op, backoff.WithContext(b, ctx)); err != nil {
		return "", err
	}
	return last, nil
}

// send performs a single HTTP round-trip. The bool indicates whether the
// error (if any) is transient and eligible for retry.
func (c *OpenAIClient) send(ctx context.Context, bodyBytes []byte, model string, messages []Message) (string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", false, fmt.Errorf("llmclient: creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	c.logger.Debug().Str("model", model).Int("messages", len(messages)).Msg("llmclient: request")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", true, fmt.Errorf("llmclient: sending request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", true, fmt.Errorf("llmclient: reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		transient := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return "", transient, fmt.Errorf("llmclient: openai HTTP %d: %s", resp.StatusCode, string(respBytes))
	}

	return parseChatResponse(respBytes)
}

func parseChatResponse(respBytes []byte) (string, bool, error) {
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
		return "", false, fmt.Errorf("llmclient: parsing response: %w", err)
	}
	if chatResp.Error != nil {
		return "", false, errors.New("llmclient: api error: " + chatResp.Error.Message)
	}
	if len(chatResp.Choices) == 0 {
		return "", false, errors.New("llmclient: no choices in response")
	}
	return chatResp.Choices[0].Message.Content, false, nil
}

// CompleteMap is a convenience wrapper for callers that hold messages as
// []map[string]string (e.g. simulate.LLMClient). It performs the conversion
// and delegates to Complete.
func (c *OpenAIClient) CompleteMap(ctx context.Context, messages []map[string]string, model string) (string, error) {
	msgs := make([]Message, 0, len(messages))
	for _, m := range messages {
		msgs = append(msgs, Message{Role: m["role"], Content: m["content"]})
	}
	return c.Complete(ctx, msgs, model)
}
