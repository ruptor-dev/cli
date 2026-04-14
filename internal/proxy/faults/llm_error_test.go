package faults_test

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/proxy"
	"github.com/ruptor-dev/cli/internal/proxy/faults"
	"github.com/ruptor-dev/cli/internal/ui"
	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- unit tests ---

func TestLLMErrorFault_Inject_Defaults(t *testing.T) {
	f, err := faults.NewLLMErrorFault(faults.FaultConfig{})
	require.NoError(t, err)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	require.NoError(t, f.Inject(rec, req))

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	// Default body must parse as an OpenAI-shaped error envelope so
	// clients that speak that protocol hit their normal error path.
	var body struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.NotEmpty(t, body.Error.Message)
	assert.NotEmpty(t, body.Error.Code)
}

func TestLLMErrorFault_Inject_TableDriven(t *testing.T) {
	tests := []struct {
		name       string
		cfg        faults.FaultConfig
		wantStatus int
		wantBody   string
	}{
		{
			name:       "custom 500 with empty body uses default body",
			cfg:        faults.FaultConfig{StatusCode: 500},
			wantStatus: 500,
			wantBody:   `{"error":{"message":"service unavailable","type":"server_error","code":"service_unavailable"}}`,
		},
		{
			name: "custom 502 with custom body preserves both",
			cfg: faults.FaultConfig{
				StatusCode: 502,
				Body:       `{"error":{"message":"upstream gateway","type":"upstream_error"}}`,
			},
			wantStatus: 502,
			wantBody:   `{"error":{"message":"upstream gateway","type":"upstream_error"}}`,
		},
		{
			name:       "zero status falls back to 503",
			cfg:        faults.FaultConfig{StatusCode: 0, Body: ""},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"error":{"message":"service unavailable","type":"server_error","code":"service_unavailable"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := faults.NewLLMErrorFault(tt.cfg)
			require.NoError(t, err)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			require.NoError(t, f.Inject(rec, req))

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantBody, rec.Body.String())
		})
	}
}

func TestLLMErrorFault_Type(t *testing.T) {
	f, err := faults.NewLLMErrorFault(faults.FaultConfig{})
	require.NoError(t, err)
	assert.Equal(t, types.FaultLLMError, f.Type())
}

func TestLLMErrorFault_RegistryBuilds(t *testing.T) {
	r := faults.NewFaultRegistry()
	f, err := r.Build(types.FaultLLMError, faults.FaultConfig{})
	require.NoError(t, err)
	assert.Equal(t, types.FaultLLMError, f.Type())
}

// --- integration test: fault fires through the real proxy + real backend ---

func TestLLMErrorFault_EndToEndThroughProxy(t *testing.T) {
	// Backend we must never reach — the fault should short-circuit.
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
			ID:          "llm_error_on_chat",
			Tool:        "/v1/chat/completions",
			Fault:       types.FaultLLMError,
			Probability: 1.0,
			StatusCode:  502,
			Body:        `{"error":{"message":"llm gateway unreachable","type":"gateway_error"}}`,
		},
	}

	p := proxy.NewProxy(cfg, tests, registry,
		proxy.WithLogger(ui.SilentLogger()),
		proxy.WithRandSource(rand.NewSource(1)),
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	p.ServeHTTP(rec, req)

	assert.Equal(t, 502, rec.Code)
	assert.Contains(t, rec.Body.String(), "llm gateway unreachable")
	assert.Equal(t, 0, backendHit, "fault must short-circuit; backend must not be called")

	body, _ := io.ReadAll(rec.Body)
	_ = body

	// Observation must record the fault as an error for the evaluator.
	obs := p.Observations()["llm_error_on_chat"]
	assert.Equal(t, 1, obs.Hits)
	assert.Equal(t, 1, obs.FaultsInjected)
	assert.True(t, obs.HadError)
	assert.Equal(t, 502, obs.LastStatusCode)
}
