package mcp_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/rs/zerolog"
	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/proxy"
	"github.com/ruptor-dev/cli/internal/proxy/faults"
	"github.com/ruptor-dev/cli/internal/proxy/mcp"
	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockMCPServer returns an httptest.Server that responds to JSON-RPC 2.0
// requests with valid tool call results.
func mockMCPServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body.Close()

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

// newProxy constructs a Proxy configured against the backend. It serves
// as the ObservationSink for the MCP handler under test; its HTTP
// behaviour is unused in most MCP tests.
func newProxy(t *testing.T, backend *httptest.Server, tests []config.TestConfig) *proxy.Proxy {
	t.Helper()
	cfg := &config.ProxyConfig{Port: 0, PassthroughURL: backend.URL}
	return proxy.NewProxy(cfg, tests, faults.NewFaultRegistry(),
		proxy.WithLogger(zerolog.Nop()),
	)
}

func newHandler(t *testing.T, backend *httptest.Server, tests []config.TestConfig, sink proxy.ObservationSink) *mcp.Handler {
	t.Helper()
	target, err := url.Parse(backend.URL)
	require.NoError(t, err)

	logger := zerolog.Nop()
	rng := rand.New(rand.NewSource(42))
	return mcp.NewHandler(target, tests, faults.NewFaultRegistry(), logger, rng, sink)
}

func jsonRPCBody(method string, id interface{}, params interface{}) []byte {
	req := types.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	data, _ := json.Marshal(req)
	return data
}

func TestMCPToolsCall_FaultInjection(t *testing.T) {
	backend := mockMCPServer()
	defer backend.Close()

	tests := []config.TestConfig{
		{
			ID:          "test-search-error",
			Tool:        "search",
			Fault:       types.FaultToolError,
			Probability: 1.0,
			Body:        "search service unavailable",
		},
	}

	p := newProxy(t, backend, tests)
	h := newHandler(t, backend, tests, p)
	body := jsonRPCBody("tools/call", 1, map[string]interface{}{
		"name":      "search",
		"arguments": map[string]string{"query": "test"},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp types.JSONRPCResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "2.0", resp.JSONRPC)
	assert.NotNil(t, resp.Error, "expected JSON-RPC error response")
	assert.Equal(t, -32603, resp.Error.Code)
	assert.Equal(t, "search service unavailable", resp.Error.Message)

	obs := p.Observations()
	require.Contains(t, obs, "test-search-error")
	assert.Equal(t, 1, obs["test-search-error"].Hits)
	assert.Equal(t, 1, obs["test-search-error"].FaultsInjected)
	assert.True(t, obs["test-search-error"].HadError)
}

func TestMCPToolsCall_Passthrough(t *testing.T) {
	backend := mockMCPServer()
	defer backend.Close()

	tests := []config.TestConfig{
		{
			ID:          "test-search-pass",
			Tool:        "search",
			Fault:       types.FaultToolError,
			Probability: 0.0, // Never fire.
		},
	}

	p := newProxy(t, backend, tests)
	h := newHandler(t, backend, tests, p)
	body := jsonRPCBody("tools/call", 42, map[string]interface{}{
		"name":      "search",
		"arguments": map[string]string{"query": "hello"},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp types.JSONRPCResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "2.0", resp.JSONRPC)
	assert.Nil(t, resp.Error, "expected no JSON-RPC error")
	assert.NotNil(t, resp.Result, "expected result from backend")

	obs := p.Observations()
	require.Contains(t, obs, "test-search-pass")
	assert.Equal(t, 1, obs["test-search-pass"].Hits)
	assert.Equal(t, 0, obs["test-search-pass"].FaultsInjected)
}

func TestMCPNonToolCall_Passthrough(t *testing.T) {
	backend := mockMCPServer()
	defer backend.Close()

	tests := []config.TestConfig{
		{
			ID:          "test-search",
			Tool:        "search",
			Fault:       types.FaultToolError,
			Probability: 1.0,
		},
	}

	p := newProxy(t, backend, tests)
	h := newHandler(t, backend, tests, p)
	// Send an "initialize" request — should pass through unmodified.
	body := jsonRPCBody("initialize", 1, map[string]interface{}{
		"protocolVersion": "2025-03-26",
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp types.JSONRPCResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	// Should get the backend's response, not an error.
	assert.Nil(t, resp.Error)
	assert.NotNil(t, resp.Result)

	// No observations should be recorded for non-tool-call methods.
	obs := p.Observations()
	assert.Empty(t, obs)
}

func TestMCPRateLimitFault(t *testing.T) {
	backend := mockMCPServer()
	defer backend.Close()

	tests := []config.TestConfig{
		{
			ID:          "test-rate-limit",
			Tool:        "search",
			Fault:       types.FaultRateLimit,
			Probability: 1.0,
			RetryAfterS: 5,
		},
	}

	p := newProxy(t, backend, tests)
	h := newHandler(t, backend, tests, p)
	body := jsonRPCBody("tools/call", 1, map[string]interface{}{"name": "search"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp types.JSONRPCResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, -32000, resp.Error.Code)
	assert.Equal(t, "rate limited", resp.Error.Message)
}

func TestMCPInvalidJSONFault(t *testing.T) {
	backend := mockMCPServer()
	defer backend.Close()

	tests := []config.TestConfig{
		{
			ID:          "test-garbage",
			Tool:        "search",
			Fault:       types.FaultInvalidJSON,
			Probability: 1.0,
		},
	}

	p := newProxy(t, backend, tests)
	h := newHandler(t, backend, tests, p)
	body := jsonRPCBody("tools/call", 1, map[string]interface{}{"name": "search"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	// The body should NOT be valid JSON.
	var resp types.JSONRPCResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	assert.Error(t, err, "expected garbage response to be invalid JSON")
}

func TestMCPEmptyResponseFault(t *testing.T) {
	backend := mockMCPServer()
	defer backend.Close()

	tests := []config.TestConfig{
		{
			ID:          "test-empty",
			Tool:        "search",
			Fault:       types.FaultEmptyResponse,
			Probability: 1.0,
		},
	}

	p := newProxy(t, backend, tests)
	h := newHandler(t, backend, tests, p)
	body := jsonRPCBody("tools/call", 1, map[string]interface{}{"name": "search"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
}

// TestServeHTTP_BodyOverflow_Returns413 ensures that a request body larger
// than MaxBodySize is rejected with 413 and never forwarded upstream. The
// oversized request must not be counted as a hit against any test.
func TestServeHTTP_BodyOverflow_Returns413(t *testing.T) {
	// Backend must never be called for an over-limit request. If it is,
	// the test will detect the bad-passthrough via the flag.
	var upstreamHit bool
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	tests := []config.TestConfig{
		{
			ID:          "test-search",
			Tool:        "search",
			Fault:       types.FaultToolError,
			Probability: 1.0,
		},
	}
	p := newProxy(t, backend, tests)
	h := newHandler(t, backend, tests, p)

	// Build a body that starts as JSON-RPC-ish but is MaxBodySize+1 bytes.
	// Content doesn't matter: the read must fail before Unmarshal.
	oversized := make([]byte, mcp.MaxBodySize+1)
	prefix := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"pad":"`)
	copy(oversized, prefix)
	for i := len(prefix); i < len(oversized)-2; i++ {
		oversized[i] = 'a'
	}
	oversized[len(oversized)-2] = '"'
	oversized[len(oversized)-1] = '}'

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(oversized))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code,
		"over-limit body must be rejected with 413")
	assert.False(t, upstreamHit,
		"upstream must never receive a truncated over-limit body")

	// Overflow must not pollute hit counts on the sink.
	assert.Empty(t, p.Observations(), "over-limit request must not record observations")
}

// TestServeHTTP_BodyExactlyMaxSize_NotRejected confirms the edge case: a
// body of exactly MaxBodySize bytes is NOT rejected by the size check. It
// may still fail JSON parsing (padding isn't valid JSON-RPC) and fall
// through to passthrough — that's fine; the point is the 413 path doesn't
// fire.
func TestServeHTTP_BodyExactlyMaxSize_NotRejected(t *testing.T) {
	backend := mockMCPServer()
	defer backend.Close()

	p := newProxy(t, backend, nil)
	h := newHandler(t, backend, nil, p)

	// Exactly MaxBodySize bytes. Use a padded valid JSON-RPC so the
	// handler parses it and passes through cleanly.
	body := make([]byte, mcp.MaxBodySize)
	prefix := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"pad":"`)
	copy(body, prefix)
	for i := len(prefix); i < len(body)-2; i++ {
		body[i] = 'a'
	}
	body[len(body)-2] = `"`[0]
	body[len(body)-1] = '}'

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)

	assert.NotEqual(t, http.StatusRequestEntityTooLarge, rec.Code,
		"body of exactly MaxBodySize must not trigger 413")
}

func TestMCPIsMCPRequest(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		expect bool
	}{
		{
			name:   "valid tools/call",
			body:   `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search"}}`,
			expect: true,
		},
		{
			name:   "valid initialize",
			body:   `{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
			expect: true,
		},
		{
			name:   "REST body",
			body:   `{"query":"test","model":"gpt-4"}`,
			expect: false,
		},
		{
			name:   "invalid JSON",
			body:   `not json at all`,
			expect: false,
		},
		{
			name:   "wrong jsonrpc version",
			body:   `{"jsonrpc":"1.0","id":1,"method":"tools/call"}`,
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mcp.IsMCPRequest([]byte(tt.body))
			assert.Equal(t, tt.expect, got)
		})
	}
}

// TestMCPObservationsFlowToProxy drives a fault-injecting tools/call
// through the MCP handler and asserts the owning Proxy's observations
// reflect the hit + fault — the end-to-end wire the evaluator reads.
func TestMCPObservationsFlowToProxy(t *testing.T) {
	backend := mockMCPServer()
	defer backend.Close()

	tests := []config.TestConfig{
		{
			ID:          "mcp-search-timeout",
			Tool:        "search",
			Fault:       types.FaultToolTimeout,
			Probability: 1.0,
		},
	}

	p := newProxy(t, backend, tests)
	h := newHandler(t, backend, tests, p)
	body := jsonRPCBody("tools/call", 7, map[string]interface{}{
		"name":      "search",
		"arguments": map[string]string{"q": "mcp"},
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	obs := p.Observations()
	require.Contains(t, obs, "mcp-search-timeout",
		"MCP handler fault must flow to the Proxy's observation store")

	got := obs["mcp-search-timeout"]
	assert.Equal(t, 1, got.Hits)
	assert.Equal(t, 1, got.FaultsInjected)
	assert.True(t, got.HadError, "JSON-RPC timeout fault must set HadError")
}

// TestMCPObservations_Passthrough confirms a non-tool-call method passes
// through and records nothing — the sink is only written for intercepted
// tool calls that match a configured test.
func TestMCPObservations_Passthrough(t *testing.T) {
	backend := mockMCPServer()
	defer backend.Close()

	tests := []config.TestConfig{
		{
			ID:          "mcp-search-pass",
			Tool:        "search",
			Fault:       types.FaultToolError,
			Probability: 1.0,
		},
	}

	p := newProxy(t, backend, tests)
	h := newHandler(t, backend, tests, p)
	// initialize is not tools/call — must not write to the sink.
	body := jsonRPCBody("initialize", 1, map[string]interface{}{})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	obs := p.Observations()
	assert.Empty(t, obs,
		"non-tool-call passthrough must not record observations")
}

// TestMCPObservations_PreservesExistingHTTPObservations verifies the MCP
// handler's writes do not clobber HTTP-mode observations on the same
// Proxy — both paths compose into a single observation map.
func TestMCPObservations_PreservesExistingHTTPObservations(t *testing.T) {
	backend := mockMCPServer()
	defer backend.Close()

	tests := []config.TestConfig{
		{
			ID:          "http-api-error",
			Tool:        "/api/call",
			Fault:       types.FaultToolError,
			Probability: 1.0,
			StatusCode:  503,
			Body:        "boom",
		},
		{
			ID:          "mcp-search-error",
			Tool:        "search",
			Fault:       types.FaultToolError,
			Probability: 1.0,
		},
	}

	p := newProxy(t, backend, tests)

	// First drive the HTTP handler to populate an HTTP observation.
	httpRec := httptest.NewRecorder()
	httpReq := httptest.NewRequest(http.MethodPost, "/api/call", nil)
	p.ServeHTTP(httpRec, httpReq)
	require.Equal(t, 503, httpRec.Code)

	// Then drive the MCP handler, sharing the same Proxy as sink.
	h := newHandler(t, backend, tests, p)
	body := jsonRPCBody("tools/call", 1, map[string]interface{}{
		"name": "search",
	})
	mcpRec := httptest.NewRecorder()
	mcpReq := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	mcpReq.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(mcpRec, mcpReq)
	require.Equal(t, http.StatusOK, mcpRec.Code)

	obs := p.Observations()

	httpObs, ok := obs["http-api-error"]
	require.True(t, ok, "HTTP observation must still be present after MCP traffic")
	assert.Equal(t, 1, httpObs.Hits)
	assert.Equal(t, 1, httpObs.FaultsInjected)
	assert.Equal(t, 503, httpObs.LastStatusCode)

	mcpObs, ok := obs["mcp-search-error"]
	require.True(t, ok, "MCP observation must be recorded")
	assert.Equal(t, 1, mcpObs.Hits)
	assert.Equal(t, 1, mcpObs.FaultsInjected)
	assert.True(t, mcpObs.HadError)
}
