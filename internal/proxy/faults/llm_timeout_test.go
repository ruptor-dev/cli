package faults_test

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/proxy"
	"github.com/ruptor-dev/cli/internal/proxy/faults"
	"github.com/ruptor-dev/cli/internal/ui"
	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- unit tests ---

func TestLLMTimeoutFault_Inject_WritesGatewayTimeoutAfterDelay(t *testing.T) {
	f, err := faults.NewLLMTimeoutFault(faults.FaultConfig{DelayMs: 50})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	start := time.Now()
	require.NoError(t, f.Inject(rec, req))
	elapsed := time.Since(start)

	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
	assert.GreaterOrEqual(t, elapsed, 50*time.Millisecond, "must honour delay_ms")
	assert.Less(t, elapsed, 500*time.Millisecond, "must not hang much longer than delay_ms")
}

func TestLLMTimeoutFault_Inject_RespectsContextCancellation(t *testing.T) {
	// Big delay. Context cancellation must short-circuit it so tests
	// never block on the full duration.
	f, err := faults.NewLLMTimeoutFault(faults.FaultConfig{DelayMs: 60_000})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)

	errCh := make(chan error, 1)
	go func() {
		errCh <- f.Inject(rec, req)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("fault did not honour context cancellation")
	}
}

func TestLLMTimeoutFault_ZeroDelayUsesDefault(t *testing.T) {
	// Construction with zero delay must not crash, and must apply the
	// default. We don't wait 30 s — just verify the struct builds and
	// type is correct.
	f, err := faults.NewLLMTimeoutFault(faults.FaultConfig{})
	require.NoError(t, err)
	assert.Equal(t, types.FaultLLMTimeout, f.Type())
}

func TestLLMTimeoutFault_RegistryBuilds(t *testing.T) {
	r := faults.NewFaultRegistry()
	f, err := r.Build(types.FaultLLMTimeout, faults.FaultConfig{DelayMs: 10})
	require.NoError(t, err)
	assert.Equal(t, types.FaultLLMTimeout, f.Type())
}

// --- integration test: fault fires through the real proxy ---

func TestLLMTimeoutFault_EndToEndThroughProxy(t *testing.T) {
	backendHit := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendHit++
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "backend OK")
	}))
	defer backend.Close()

	registry := faults.NewFaultRegistry()
	cfg := &config.ProxyConfig{Port: 0, PassthroughURL: backend.URL}
	tests := []config.TestConfig{
		{
			ID:          "llm_timeout_on_chat",
			Tool:        "/v1/chat/completions",
			Fault:       types.FaultLLMTimeout,
			DelayMS:     40,
			Probability: 1.0,
		},
	}

	p := proxy.NewProxy(cfg, tests, registry,
		proxy.WithLogger(ui.SilentLogger()),
		proxy.WithRandSource(rand.NewSource(1)),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	p.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
	assert.Equal(t, 0, backendHit, "fault must short-circuit; backend must not be called")

	obs := p.Observations()["llm_timeout_on_chat"]
	assert.Equal(t, 1, obs.Hits)
	assert.Equal(t, 1, obs.FaultsInjected)
	assert.True(t, obs.HadError, "504 is an error from the agent's perspective")
	assert.Equal(t, http.StatusGatewayTimeout, obs.LastStatusCode)
}
