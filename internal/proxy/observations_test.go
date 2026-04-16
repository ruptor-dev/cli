package proxy_test

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/proxy"
	"github.com/ruptor-dev/cli/internal/proxy/faults"
	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObservations_RecordsFaultHits(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	}))
	defer backend.Close()

	registry := faults.NewFaultRegistry()
	cfg := &config.ProxyConfig{Port: 0, PassthroughURL: backend.URL}
	tests := []config.TestConfig{
		{ID: "t1", Tool: "/search", Fault: types.FaultToolTimeout, Probability: 1.0},
	}

	p := proxy.NewProxy(cfg, tests, registry,
		proxy.WithLogger(newTestLogger()),
	)

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/search", nil)
		p.ServeHTTP(rec, req)
	}

	obs := p.Observations()
	require.Contains(t, obs, "t1")
	t1 := obs["t1"]
	assert.Equal(t, 3, t1.Hits)
	assert.Equal(t, 3, t1.FaultsInjected)
	assert.True(t, t1.HadError)
	assert.Equal(t, http.StatusGatewayTimeout, t1.LastStatusCode)
}

func TestObservations_RecordsPassthroughWhenProbabilityZero(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	}))
	defer backend.Close()

	registry := faults.NewFaultRegistry()
	cfg := &config.ProxyConfig{Port: 0, PassthroughURL: backend.URL}
	tests := []config.TestConfig{
		{ID: "t1", Tool: "/api", Fault: types.FaultToolError, Probability: 0.0, StatusCode: 500, Body: "x"},
	}

	// Deterministic rng — won't matter since probability is 0.
	p := proxy.NewProxy(cfg, tests, registry,
		proxy.WithLogger(newTestLogger()),
		proxy.WithRandSource(rand.NewSource(1)),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	p.ServeHTTP(rec, req)

	obs := p.Observations()
	require.Contains(t, obs, "t1")
	t1 := obs["t1"]
	assert.Equal(t, 1, t1.Hits)
	assert.Equal(t, 0, t1.FaultsInjected)
	assert.False(t, t1.HadError)
	assert.Equal(t, http.StatusOK, t1.LastStatusCode)
}

func TestObservation_Recovered(t *testing.T) {
	tests := []struct {
		name string
		obs  proxy.Observation
		want bool
	}{
		{
			name: "no fault means nothing to recover from",
			obs:  proxy.Observation{HadError: false},
			want: false,
		},
		{
			name: "faulted once no retry",
			obs:  proxy.Observation{HadError: true, Hits: 1, FaultsInjected: 1, LastStatusCode: 500},
			want: false,
		},
		{
			name: "retried and succeeded",
			obs:  proxy.Observation{HadError: true, Hits: 3, FaultsInjected: 1, LastStatusCode: 200},
			want: true,
		},
		{
			name: "retried but final still failed",
			obs:  proxy.Observation{HadError: true, Hits: 3, FaultsInjected: 1, LastStatusCode: 500},
			want: false,
		},
		{
			name: "all hits faulted even with OK last status",
			obs:  proxy.Observation{HadError: true, Hits: 2, FaultsInjected: 2, LastStatusCode: 200},
			want: false,
		},
		{
			name: "redirect counts as success",
			obs:  proxy.Observation{HadError: true, Hits: 5, FaultsInjected: 2, LastStatusCode: 301},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.obs.Recovered())
		})
	}
}

// TestObservations_RecoverySignalFlipsOnRetry drives the handler
// end-to-end with a deterministic RNG seeded so the fault fires on
// the first hit and passes through on the second. That shape — fault
// then successful retry of the same tool path — is the canonical
// "recovered" signal the evaluator consumes. Asserts that:
//  1. After the first hit, Recovered() is false (faulted, no success).
//  2. After the second hit, Recovered() flips to true.
func TestObservations_RecoverySignalFlipsOnRetry(t *testing.T) {
	// Backend always responds 200. The fault is applied by the proxy
	// before the request is forwarded, so we control the signal by
	// toggling whether the proxy fires.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	}))
	defer backend.Close()

	registry := faults.NewFaultRegistry()
	cfg := &config.ProxyConfig{Port: 0, PassthroughURL: backend.URL}
	// Probability 0.2 with rand seed 2 yields 0.17 (< 0.2 → fire)
	// then 0.27 (> 0.2 → passthrough). See package tests for the
	// seed probe; these two draws give the exact fault-then-recover
	// sequence we need.
	tests := []config.TestConfig{
		{
			ID:          "t1",
			Tool:        "/search",
			Fault:       types.FaultToolError,
			Probability: 0.2,
			StatusCode:  500,
			Body:        "boom",
		},
	}

	p := proxy.NewProxy(cfg, tests, registry,
		proxy.WithLogger(newTestLogger()),
		proxy.WithRandSource(rand.NewSource(2)),
	)

	// First hit: fault fires.
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/search", nil)
	p.ServeHTTP(rec1, req1)
	require.Equal(t, 500, rec1.Code, "first hit must be faulted (500)")

	obs1 := p.Observations()["t1"]
	assert.True(t, obs1.HadError, "faulted hit must mark HadError")
	assert.Equal(t, 1, obs1.Hits)
	assert.Equal(t, 1, obs1.FaultsInjected)
	assert.False(t, obs1.Recovered(), "one faulted hit is not recovery")

	// Second hit: passthrough, backend returns 200.
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/search", nil)
	p.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code, "second hit must passthrough")

	obs2 := p.Observations()["t1"]
	assert.Equal(t, 2, obs2.Hits)
	assert.Equal(t, 1, obs2.FaultsInjected, "probability miss must not count as injection")
	assert.Equal(t, http.StatusOK, obs2.LastStatusCode)
	assert.True(t, obs2.Recovered(),
		"retry-after-fault with successful final hit must set Recovered()=true")
}

func TestObservations_UnmatchedPathDoesNotRecord(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	registry := faults.NewFaultRegistry()
	cfg := &config.ProxyConfig{Port: 0, PassthroughURL: backend.URL}
	tests := []config.TestConfig{
		{ID: "t1", Tool: "/search", Fault: types.FaultToolTimeout, Probability: 1.0},
	}

	p := proxy.NewProxy(cfg, tests, registry, proxy.WithLogger(newTestLogger()))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/other", nil)
	p.ServeHTTP(rec, req)

	obs := p.Observations()
	assert.Empty(t, obs, "unmatched path must not produce observations")
}
