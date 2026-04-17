package proxy_test

import (
	"bytes"
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
	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMCPBackend returns an httptest.Server that echoes valid JSON-RPC
// results for any tools/call it receives. Used only to stand in as the
// upstream MCP server in these integration tests.
func mockMCPBackend() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()

		var req types.JSONRPCRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		resp := types.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: types.ToolCallResult{
				Content: []types.ToolContent{
					{Type: "text", Text: fmt.Sprintf("result from %s", req.Method)},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func mcpToolsCallBody(t *testing.T, tool string, id int) []byte {
	t.Helper()
	req := types.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/call",
		Params: map[string]interface{}{
			"name":      tool,
			"arguments": map[string]string{"query": "hello"},
		},
	}
	data, err := json.Marshal(req)
	require.NoError(t, err)
	return data
}

// setupMCPProxyForSinkTest builds a *proxy.Proxy in MCP mode that
// forwards to a fresh mock backend. The backend is cleaned up via
// t.Cleanup so callers never need to defer-close it.
func setupMCPProxyForSinkTest(t *testing.T, tests []config.TestConfig) (*proxy.Proxy, *httptest.Server) {
	t.Helper()
	backend := mockMCPBackend()
	t.Cleanup(backend.Close)
	cfg := &config.ProxyConfig{
		Port:           0,
		PassthroughURL: backend.URL,
		Mode:           config.ProxyModeMCP,
	}
	p := proxy.NewProxy(cfg, tests, faults.NewFaultRegistry(),
		proxy.WithLogger(newTestLogger()),
		proxy.WithRandSource(rand.NewSource(1)),
	)
	return p, backend
}

// TestMCPHandler_ToolsCall_ObservationsReachProxy proves that MCP
// observations land in the owning *proxy.Proxy's map — the same map the
// evaluator reads via p.Observations(). Regression test for the "MCP
// scores as not exercised" bug tracked in
// docs/specs/backlog/mcp-observations-evaluator.md.
func TestMCPHandler_ToolsCall_ObservationsReachProxy(t *testing.T) {
	cases := []struct {
		name           string
		probability    float64
		wantFaults     int
		wantHadError   bool
		faultInjection bool
	}{
		{name: "fault_injected", probability: 1.0, wantFaults: 1, wantHadError: true, faultInjection: true},
		{name: "passthrough", probability: 0.0, wantFaults: 0, wantHadError: false, faultInjection: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testID := "mcp-" + tc.name
			tests := []config.TestConfig{{
				ID:          testID,
				Tool:        "search",
				Fault:       types.FaultToolError,
				Probability: tc.probability,
				Body:        "search unavailable",
			}}

			p, _ := setupMCPProxyForSinkTest(t, tests)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/",
				bytes.NewReader(mcpToolsCallBody(t, "search", 1)))
			req.Header.Set("Content-Type", "application/json")
			p.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code,
				"MCP faults and passthroughs both ride on HTTP 200")

			obs := p.Observations()
			require.Contains(t, obs, testID,
				"MCP observation must reach *proxy.Proxy.Observations()")
			got := obs[testID]
			assert.Equal(t, 1, got.Hits)
			assert.Equal(t, tc.wantFaults, got.FaultsInjected)
			assert.Equal(t, tc.wantHadError, got.HadError)
		})
	}
}
