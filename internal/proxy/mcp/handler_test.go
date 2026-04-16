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
		json.NewEncoder(w).Encode(resp)
	}))
}

func newHandler(t *testing.T, backend *httptest.Server, tests []config.TestConfig) *mcp.Handler {
	t.Helper()
	target, err := url.Parse(backend.URL)
	require.NoError(t, err)

	logger := zerolog.Nop()
	rng := rand.New(rand.NewSource(42))
	return mcp.NewHandler(target, tests, faults.NewFaultRegistry(), logger, rng)
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

	h := newHandler(t, backend, tests)
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

	// Check observation was recorded.
	obs := h.Observations()
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

	h := newHandler(t, backend, tests)
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

	// Check observation — hit recorded but no fault.
	obs := h.Observations()
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

	h := newHandler(t, backend, tests)
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
	obs := h.Observations()
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

	h := newHandler(t, backend, tests)
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

	h := newHandler(t, backend, tests)
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

	h := newHandler(t, backend, tests)
	body := jsonRPCBody("tools/call", 1, map[string]interface{}{"name": "search"})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
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
