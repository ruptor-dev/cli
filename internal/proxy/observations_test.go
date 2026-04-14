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
