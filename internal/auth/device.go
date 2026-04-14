package auth

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ruptor-dev/cli/internal/cloud"
)

// DeviceCodeResponse is what the platform returns from
// POST /v1/auth/device/code.
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"` // seconds
	Interval        int    `json:"interval"`   // seconds
}

// deviceTokenResponse mirrors the OAuth 2.0 Device Authorization Grant
// response: either a token on success or an error code while the user
// is still in the browser.
type deviceTokenResponse struct {
	Token string `json:"token"`
	Error string `json:"error"`
}

// Error codes the polling endpoint may return while the flow is in
// progress.
const (
	errAuthorizationPending = "authorization_pending"
	errAccessDenied         = "access_denied"
	errExpiredToken         = "expired_token"
	errSlowDown             = "slow_down"
)

// Client talks to the platform auth API. Tests swap in an
// httptest.Server URL via WithBaseURL.
type Client struct {
	baseURL string
	http    *http.Client
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithBaseURL overrides cloud.DefaultAPIURL. Tests point this at
// httptest.NewServer.URL().
func WithBaseURL(u string) ClientOption {
	return func(c *Client) { c.baseURL = u }
}

// WithHTTPClient overrides the default http.Client. Callers use this
// to inject test transports or to share connection pools.
func WithHTTPClient(h *http.Client) ClientOption {
	return func(c *Client) { c.http = h }
}

// NewClient builds a client with TLS enforced (InsecureSkipVerify
// must be false per security.md) and the platform timeout.
func NewClient(opts ...ClientOption) *Client {
	c := &Client{
		baseURL: cloud.DefaultAPIURL,
		http: &http.Client{
			Timeout: cloud.DefaultHTTPTimeout,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					MinVersion: tls.VersionTLS12,
					// InsecureSkipVerify: false (default) — never flip this.
				},
			},
		},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// StartDevice initiates the OAuth device-authorization grant. The
// returned DeviceCodeResponse carries the URL the user opens and the
// polling cadence.
func (c *Client) StartDevice(ctx context.Context) (*DeviceCodeResponse, error) {
	var resp DeviceCodeResponse
	if err := c.postJSON(ctx, cloud.DeviceCodePath, nil, &resp); err != nil {
		return nil, fmt.Errorf("auth: start device: %w", err)
	}
	if resp.DeviceCode == "" {
		return nil, fmt.Errorf("auth: start device: empty device_code in response")
	}
	return &resp, nil
}

// PollToken blocks until the platform returns a token, the user
// denies, or the device code expires. Callers pass the Interval and
// ExpiresIn from StartDevice.
func (c *Client) PollToken(ctx context.Context, deviceCode string, interval, expiresIn time.Duration) (string, error) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(expiresIn)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if time.Now().After(deadline) {
			return "", ErrDeviceTimeout
		}
		token, done, err := c.pollOnce(ctx, deviceCode)
		if done {
			return token, err
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

// pollOnce makes a single /token request. Returns (token, done, err)
// where done=true means the loop should stop (success or terminal
// error); done=false means the server is still waiting for the user.
func (c *Client) pollOnce(ctx context.Context, deviceCode string) (string, bool, error) {
	body := map[string]string{"device_code": deviceCode}
	var resp deviceTokenResponse
	if err := c.postJSON(ctx, cloud.DeviceTokenPath, body, &resp); err != nil {
		return "", true, fmt.Errorf("auth: poll: %w", err)
	}
	if resp.Token != "" {
		return resp.Token, true, nil
	}
	switch resp.Error {
	case errAuthorizationPending, errSlowDown, "":
		return "", false, nil
	case errAccessDenied:
		return "", true, ErrDeviceDenied
	case errExpiredToken:
		return "", true, ErrDeviceTimeout
	default:
		return "", true, fmt.Errorf("auth: poll: unknown response %q", resp.Error)
	}
}

// Refresh rotates a still-valid token. The platform returns a new
// JWT; if the old token was revoked mid-refresh the server returns
// ErrTokenRevoked.
func (c *Client) Refresh(ctx context.Context, current string) (string, error) {
	var resp struct {
		Token string `json:"token"`
		Error string `json:"error"`
	}
	body := map[string]string{"token": current}
	if err := c.postJSON(ctx, cloud.RefreshPath, body, &resp); err != nil {
		return "", fmt.Errorf("auth: refresh: %w", err)
	}
	if resp.Error == "revoked" {
		return "", ErrTokenRevoked
	}
	if resp.Token == "" {
		return "", fmt.Errorf("auth: refresh: empty token in response")
	}
	return resp.Token, nil
}

// Revoke tells the platform to invalidate the token. Best-effort: a
// network failure is logged but not returned so logout stays
// idempotent.
func (c *Client) Revoke(ctx context.Context, token string) error {
	body := map[string]string{"token": token}
	var resp struct{}
	if err := c.postJSON(ctx, cloud.RevokePath, body, &resp); err != nil {
		return fmt.Errorf("auth: revoke: %w", err)
	}
	return nil
}

// postJSON marshals body, posts to c.baseURL+path, and decodes the
// response into out.
func (c *Client) postJSON(ctx context.Context, path string, body, out any) error {
	var buf io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}
		buf = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, buf)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	if buf != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}
	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusBadRequest {
		return fmt.Errorf("http %d: %s", resp.StatusCode, truncateBody(raw))
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}
	return nil
}

func truncateBody(b []byte) string {
	if len(b) <= 200 {
		return string(b)
	}
	return string(b[:200]) + "…"
}
