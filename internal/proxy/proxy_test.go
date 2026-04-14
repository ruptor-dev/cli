package proxy_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/proxy"
	"github.com/ruptor-dev/cli/internal/proxy/faults"
	"github.com/ruptor-dev/cli/internal/ui"
	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger() zerolog.Logger {
	return ui.SilentLogger()
}

func TestFaultInjection(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "backend response")
	}))
	defer backend.Close()

	registry := faults.NewFaultRegistry()
	cfg := &config.ProxyConfig{
		Port:           0,
		PassthroughURL: backend.URL,
	}
	tests := []config.TestConfig{
		{
			ID:          "test-timeout",
			Tool:        "/search",
			Fault:       types.FaultToolTimeout,
			Probability: 1.0,
		},
	}

	p := proxy.NewProxy(cfg, tests, registry,
		proxy.WithLogger(newTestLogger()),
		proxy.WithTimeout(5*time.Second),
	)

	// Use httptest to exercise the handler directly.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/search", nil)
	p.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
}

func TestPassthrough(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "backend OK")
	}))
	defer backend.Close()

	registry := faults.NewFaultRegistry()
	cfg := &config.ProxyConfig{
		Port:           0,
		PassthroughURL: backend.URL,
	}

	// No tests configured, so every request should passthrough.
	p := proxy.NewProxy(cfg, nil, registry,
		proxy.WithLogger(newTestLogger()),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	p.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "backend OK")
}

func TestProbabilityZero(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "passthrough response")
	}))
	defer backend.Close()

	registry := faults.NewFaultRegistry()
	cfg := &config.ProxyConfig{
		Port:           0,
		PassthroughURL: backend.URL,
	}
	tests := []config.TestConfig{
		{
			ID:          "test-never-fire",
			Tool:        "/search",
			Fault:       types.FaultToolError,
			Probability: 0.0,
			StatusCode:  500,
			Body:        "error",
		},
	}

	p := proxy.NewProxy(cfg, tests, registry,
		proxy.WithLogger(newTestLogger()),
	)

	// With probability 0.0, rand.Float64() is always >= 0.0, so this should
	// always passthrough.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/search", nil)
	p.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "passthrough response")
}

func TestToolErrorInjection(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "should not reach here")
	}))
	defer backend.Close()

	registry := faults.NewFaultRegistry()
	cfg := &config.ProxyConfig{
		Port:           0,
		PassthroughURL: backend.URL,
	}
	tests := []config.TestConfig{
		{
			ID:          "test-tool-error",
			Tool:        "/api/call",
			Fault:       types.FaultToolError,
			Probability: 1.0,
			StatusCode:  503,
			Body:        "service unavailable",
		},
	}

	p := proxy.NewProxy(cfg, tests, registry,
		proxy.WithLogger(newTestLogger()),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/call", nil)
	p.ServeHTTP(rec, req)

	assert.Equal(t, 503, rec.Code)
	assert.Contains(t, rec.Body.String(), "service unavailable")
}

func TestStartStop(t *testing.T) {
	registry := faults.NewFaultRegistry()
	cfg := &config.ProxyConfig{
		Port:           0,
		PassthroughURL: "http://localhost:9999",
	}

	p := proxy.NewProxy(cfg, nil, registry,
		proxy.WithLogger(newTestLogger()),
		proxy.WithTimeout(5*time.Second),
	)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- p.Start(ctx)
	}()

	// Wait briefly for the server to bind.
	time.Sleep(50 * time.Millisecond)

	addr := p.Addr()
	require.NotEmpty(t, addr, "proxy should report a listening address")

	// Verify it accepts connections.
	resp, err := http.Get(fmt.Sprintf("http://%s/healthcheck", addr))
	require.NoError(t, err)
	resp.Body.Close()

	// Cancel context to trigger shutdown.
	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("proxy did not stop within timeout")
	}
}

func TestUnmatchedPathPassthrough(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "hello from backend")
	}))
	defer backend.Close()

	registry := faults.NewFaultRegistry()
	cfg := &config.ProxyConfig{
		Port:           0,
		PassthroughURL: backend.URL,
	}
	tests := []config.TestConfig{
		{
			ID:          "test-search",
			Tool:        "/search",
			Fault:       types.FaultToolTimeout,
			Probability: 1.0,
		},
	}

	p := proxy.NewProxy(cfg, tests, registry,
		proxy.WithLogger(newTestLogger()),
	)

	// Request to a path that does NOT match any test tool.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/other", nil)
	p.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "hello from backend")
}
