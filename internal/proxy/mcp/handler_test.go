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
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/proxy/faults"
	"github.com/ruptor-dev/cli/internal/proxy/mcp"
	"github.com/ruptor-dev/cli/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// obsRecord mirrors the fields the handler used to track internally. It
// lives in the test package so the handler can stay ignorant of how its
// observations are stored.
type obsRecord struct {
	Hits           int
	FaultsInjected int
	LastStatusCode int
	HadError       bool
}

// fakeSink implements mcp.ObservationSink for unit tests. It keeps a
// local map so assertions remain independent of the real *proxy.Proxy.
type fakeSink struct {
	mu  sync.Mutex
	obs map[string]*obsRecord
}

func newFakeSink() *fakeSink {
	return &fakeSink{obs: map[string]*obsRecord{}}
}

func (f *fakeSink) ensure(id string) *obsRecord {
	o, ok := f.obs[id]
	if !ok {
		o = &obsRecord{}
		f.obs[id] = o
	}
	return o
}

func (f *fakeSink) RecordFault(id string, sc int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o := f.ensure(id)
	o.Hits++
	o.FaultsInjected++
	o.LastStatusCode = sc
	o.HadError = true
}

func (f *fakeSink) RecordPassthrough(id string, sc int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	o := f.ensure(id)
	o.Hits++
	o.LastStatusCode = sc
}

func (f *fakeSink) snapshot(id string) obsRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	if o, ok := f.obs[id]; ok {
		return *o
	}
	return obsRecord{}
}

func (f *fakeSink) contains(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.obs[id]
	return ok
}

func (f *fakeSink) empty() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.obs) == 0
}

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

func newHandler(t *testing.T, backend *httptest.Server, tests []config.TestConfig) (*mcp.Handler, *fakeSink) {
	t.Helper()
	target, err := url.Parse(backend.URL)
	require.NoError(t, err)

	logger := zerolog.Nop()
	rng := rand.New(rand.NewSource(42))
	sink := newFakeSink()
	return mcp.NewHandler(target, tests, faults.NewFaultRegistry(), logger, rng, sink), sink
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

	h, sink := newHandler(t, backend, tests)
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

	// Check observation was recorded via the sink.
	require.True(t, sink.contains("test-search-error"))
	obs := sink.snapshot("test-search-error")
	assert.Equal(t, 1, obs.Hits)
	assert.Equal(t, 1, obs.FaultsInjected)
	assert.True(t, obs.HadError)
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

	h, sink := newHandler(t, backend, tests)
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
	require.True(t, sink.contains("test-search-pass"))
	obs := sink.snapshot("test-search-pass")
	assert.Equal(t, 1, obs.Hits)
	assert.Equal(t, 0, obs.FaultsInjected)
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

	h, sink := newHandler(t, backend, tests)
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
	assert.True(t, sink.empty())
}

// assertFaultObservation verifies the sink recorded a single fault hit
// for the given test ID. Used by fault-type-specific tests below to
// guard the P2 regression where rate_limit / invalid_json /
// empty_response stopped asserting observations.
func assertFaultObservation(t *testing.T, sink *fakeSink, testID string) {
	t.Helper()
	require.True(t, sink.contains(testID), "sink must record observation for %q", testID)
	obs := sink.snapshot(testID)
	assert.Equal(t, 1, obs.Hits)
	assert.Equal(t, 1, obs.FaultsInjected)
	assert.True(t, obs.HadError, "MCP fault responses ride on HTTP 200; HadError must be unconditional")
}

func TestMCPRateLimitFault(t *testing.T) {
	backend := mockMCPServer()
	defer backend.Close()

	tests := []config.TestConfig{{
		ID: "test-rate-limit", Tool: "search",
		Fault: types.FaultRateLimit, Probability: 1.0, RetryAfterS: 5,
	}}
	h, sink := newHandler(t, backend, tests)
	body := jsonRPCBody("tools/call", 1, map[string]interface{}{"name": "search"})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp types.JSONRPCResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, -32000, resp.Error.Code)
	assert.Equal(t, "rate limited", resp.Error.Message)
	assertFaultObservation(t, sink, "test-rate-limit")
}

func TestMCPInvalidJSONFault(t *testing.T) {
	backend := mockMCPServer()
	defer backend.Close()

	tests := []config.TestConfig{{
		ID: "test-garbage", Tool: "search",
		Fault: types.FaultInvalidJSON, Probability: 1.0,
	}}
	h, sink := newHandler(t, backend, tests)
	body := jsonRPCBody("tools/call", 1, map[string]interface{}{"name": "search"})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp types.JSONRPCResponse
	assert.Error(t, json.Unmarshal(rec.Body.Bytes(), &resp), "expected garbage response")
	assertFaultObservation(t, sink, "test-garbage")
}

func TestMCPEmptyResponseFault(t *testing.T) {
	backend := mockMCPServer()
	defer backend.Close()

	tests := []config.TestConfig{{
		ID: "test-empty", Tool: "search",
		Fault: types.FaultEmptyResponse, Probability: 1.0,
	}}
	h, sink := newHandler(t, backend, tests)
	body := jsonRPCBody("tools/call", 1, map[string]interface{}{"name": "search"})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body)))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Empty(t, rec.Body.String())
	assertFaultObservation(t, sink, "test-empty")
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
	h, sink := newHandler(t, backend, tests)

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

	// Overflow must not pollute hit counts.
	assert.True(t, sink.empty(), "over-limit request must not record observations")
}

// TestServeHTTP_BodyExactlyMaxSize_NotRejected confirms the edge case: a
// body of exactly MaxBodySize bytes is NOT rejected by the size check. It
// may still fail JSON parsing (padding isn't valid JSON-RPC) and fall
// through to passthrough — that's fine; the point is the 413 path doesn't
// fire.
func TestServeHTTP_BodyExactlyMaxSize_NotRejected(t *testing.T) {
	backend := mockMCPServer()
	defer backend.Close()

	h, _ := newHandler(t, backend, nil)

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
