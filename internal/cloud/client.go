// Cloud client for run-report ingestion. Stays inert until
// CloudReportingEnabled flips to true at the launch tag — until then
// nothing in this file is reachable from a real run, but `ruptor sync`
// uses it against the test harness so the wire format does not bit-rot
// between releases.
package cloud

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/rs/zerolog"
)

// IngestPath is the platform endpoint that accepts a single report
// JSON payload. The request body is the raw bytes loaded from
// ~/.ruptor/pending/<file>.json — the CLI does not re-marshal so a
// future schema change requires only a server-side read.
const IngestPath = "/v1/runs"

// retryPolicy mirrors the CLAUDE.md backoff envelope used by the LLM
// client. Kept here as a package-level value so callers can override
// for tests without touching the public API.
type retryPolicy struct {
	initialInterval time.Duration
	multiplier      float64
	randomization   float64
	maxInterval     time.Duration
	maxElapsed      time.Duration
}

func defaultRetryPolicy() retryPolicy {
	return retryPolicy{
		initialInterval: 500 * time.Millisecond,
		multiplier:      2.0,
		randomization:   0.5,
		maxInterval:     30 * time.Second,
		maxElapsed:      5 * time.Minute,
	}
}

// Client uploads run reports to the platform. It is safe for
// concurrent use; sync calls Upload sequentially today but a future
// parallel-flush PR can reuse the same client.
type Client struct {
	baseURL string
	http    *http.Client
	retry   retryPolicy
	logger  zerolog.Logger
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithBaseURL overrides DefaultAPIURL — tests inject httptest.NewServer.URL().
func WithBaseURL(u string) ClientOption { return func(c *Client) { c.baseURL = u } }

// WithHTTPClient swaps the underlying http.Client. Tests pass the
// httptest server's Client so TLS config is shared.
func WithHTTPClient(h *http.Client) ClientOption { return func(c *Client) { c.http = h } }

// WithLogger wires zerolog. The logger is only used for transient-
// failure noise; a Disabled logger is a valid no-op.
func WithLogger(l zerolog.Logger) ClientOption { return func(c *Client) { c.logger = l } }

// WithRetryPolicy overrides the default backoff. Tests shrink the
// envelope so a deliberate failure does not stall the suite.
func WithRetryPolicy(initial, max, maxElapsed time.Duration) ClientOption {
	return func(c *Client) {
		c.retry.initialInterval = initial
		c.retry.maxInterval = max
		c.retry.maxElapsed = maxElapsed
	}
}

// NewClient builds a TLS-enforced client with the default retry policy.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		baseURL: DefaultAPIURL,
		http: &http.Client{
			Timeout: DefaultHTTPTimeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		},
		retry:  defaultRetryPolicy(),
		logger: zerolog.Nop(),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// UploadReport POSTs body to IngestPath with exponential backoff.
// 5xx and network errors are retried; 4xx (other than 408 / 429) are
// permanent and surface immediately so a malformed report never
// burns the full retry budget.
func (c *Client) UploadReport(ctx context.Context, body []byte) error {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = c.retry.initialInterval
	b.Multiplier = c.retry.multiplier
	b.RandomizationFactor = c.retry.randomization
	b.MaxInterval = c.retry.maxInterval
	b.MaxElapsedTime = c.retry.maxElapsed

	op := func() error {
		err := c.uploadOnce(ctx, body)
		if err == nil {
			return nil
		}
		var perm permanentError
		if errors.As(err, &perm) {
			return backoff.Permanent(perm.err)
		}
		c.logger.Warn().Err(err).Msg("cloud: transient upload failure, retrying")
		return err
	}
	return backoff.Retry(op, backoff.WithContext(b, ctx))
}

// permanentError signals a non-retriable failure to the backoff loop.
type permanentError struct{ err error }

func (e permanentError) Error() string { return e.err.Error() }

func (c *Client) uploadOnce(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+IngestPath, bytes.NewReader(body))
	if err != nil {
		return permanentError{err: fmt.Errorf("cloud: build request: %w", err)}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cloud: upload: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	wrapped := fmt.Errorf("cloud: http %d: %s", resp.StatusCode, raw)
	if isPermanentStatus(resp.StatusCode) {
		return permanentError{err: wrapped}
	}
	return wrapped
}

// isPermanentStatus distinguishes "your request is broken" from
// "the platform is having a moment". 408 and 429 stay retriable;
// the rest of 4xx is the user's problem and short-circuits the loop.
func isPermanentStatus(code int) bool {
	if code < 400 || code >= 500 {
		return false
	}
	return code != http.StatusRequestTimeout && code != http.StatusTooManyRequests
}
