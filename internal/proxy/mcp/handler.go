// Package mcp implements a fault-injecting reverse proxy for the Model
// Context Protocol (MCP). It intercepts JSON-RPC 2.0 tools/call requests,
// applies faults from the shared fault registry, and forwards everything
// else to the upstream MCP server unmodified.
package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/proxy/faults"
	"github.com/ruptor-dev/cli/pkg/types"
)

// MaxBodySize is the maximum request body size the proxy will buffer.
// Requests larger than 10 MB are rejected with HTTP 413 Request Entity Too
// Large to prevent OOM from untrusted clients. The oversized body is never
// forwarded upstream and does not count toward any test's observation stats.
const MaxBodySize = 10 << 20 // 10 MB

// Handler is the MCP-aware HTTP handler that sits in front of a JSON-RPC
// 2.0 MCP server. It intercepts tools/call requests for fault injection
// and passes everything else through. Per-test observations are handed to
// the owning proxy via the types.ObservationSink interface.
type Handler struct {
	tests    []config.TestConfig
	registry *faults.FaultRegistry
	target   *url.URL
	logger   zerolog.Logger
	rng      *rand.Rand
	sink     types.ObservationSink

	// mu guards activeTest only. Observation state lives on the sink.
	mu         sync.Mutex
	activeTest string

	rp *httputil.ReverseProxy
}

// NewHandler constructs an MCP handler. The sink receives per-test
// observations — typically the surrounding *proxy.Proxy, so MCP and HTTP
// paths share a single observation map.
func NewHandler(
	target *url.URL,
	tests []config.TestConfig,
	registry *faults.FaultRegistry,
	logger zerolog.Logger,
	rng *rand.Rand,
	sink types.ObservationSink,
) *Handler {
	h := &Handler{
		tests:    tests,
		registry: registry,
		target:   target,
		logger:   logger,
		rng:      rng,
		sink:     sink,
	}
	h.rp = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
		},
	}
	return h
}

// SetActiveTest pins the test whose observations should receive matching
// hits. Empty restores first-match dispatch.
func (h *Handler) SetActiveTest(id string) {
	h.mu.Lock()
	h.activeTest = id
	h.mu.Unlock()
}

// ServeHTTP implements http.Handler. It peeks at the request body to
// determine if this is a JSON-RPC tools/call; if so, applies fault
// injection. Everything else passes through unmodified.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only POST carries JSON-RPC requests.
	if r.Method != http.MethodPost {
		h.passthrough(w, r, "")
		return
	}

	// Buffer the body so we can peek and still forward it. MaxBytesReader
	// enforces the size cap: if the client sends more than MaxBodySize, the
	// read returns *http.MaxBytesError and the underlying body is closed,
	// so no truncated payload ever reaches the parser or the upstream.
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodySize)
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			h.logger.Warn().
				Int64("limit", maxErr.Limit).
				Msg("mcp: request body exceeds MaxBodySize")
			http.Error(w,
				fmt.Sprintf("request body exceeds %d bytes", MaxBodySize),
				http.StatusRequestEntityTooLarge)
			return
		}
		h.logger.Error().Err(err).Msg("mcp: read body")
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var rpcReq types.JSONRPCRequest
	if err := json.Unmarshal(body, &rpcReq); err != nil || rpcReq.JSONRPC != "2.0" {
		// Not valid JSON-RPC — pass through as-is.
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		h.passthrough(w, r, "")
		return
	}

	// Only intercept tools/call.
	if rpcReq.Method != "tools/call" {
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		h.passthrough(w, r, "")
		return
	}

	// Extract tool name from params.
	toolName := extractToolName(rpcReq.Params)
	if toolName == "" {
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		h.passthrough(w, r, "")
		return
	}

	test, ok := h.matchTest(toolName)
	if !ok {
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		h.passthrough(w, r, "")
		return
	}

	if h.shouldInject(test.Probability) {
		h.injectFault(w, r, rpcReq, test)
		return
	}

	// Probability miss — passthrough but record the hit.
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))
	h.passthrough(w, r, test.ID)
}

// matchTest finds the TestConfig for the given tool name.
func (h *Handler) matchTest(toolName string) (config.TestConfig, bool) {
	h.mu.Lock()
	active := h.activeTest
	h.mu.Unlock()

	if active != "" {
		for _, t := range h.tests {
			if t.ID == active && t.Tool == toolName {
				return t, true
			}
		}
	}
	for _, t := range h.tests {
		if t.Tool == toolName {
			return t, true
		}
	}
	return config.TestConfig{}, false
}

func (h *Handler) shouldInject(probability float64) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.rng.Float64() < probability
}

// injectFault translates the configured fault type into a JSON-RPC 2.0
// error or garbage response. Instead of using the HTTP-level Fault.Inject
// (which writes HTTP status codes), we map each fault type to the
// appropriate JSON-RPC error shape.
func (h *Handler) injectFault(w http.ResponseWriter, r *http.Request, rpcReq types.JSONRPCRequest, t config.TestConfig) {
	h.logger.Info().
		Str("fault_type", string(t.Fault)).
		Str("tool", t.Tool).
		Str("test_id", t.ID).
		Msg("mcp: injecting fault")

	// Handle delay-based faults first.
	if t.DelayMS > 0 {
		timer := time.NewTimer(time.Duration(t.DelayMS) * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-r.Context().Done():
			h.sink.RecordFault(t.ID, 0)
			return
		}
	}

	var resp types.JSONRPCResponse
	resp.JSONRPC = "2.0"
	resp.ID = rpcReq.ID

	// All MCP fault responses are HTTP 200 — JSON-RPC carries its own
	// error envelope in the body. Fault-specific status codes belong on
	// the HTTP proxy path, not here.
	const statusCode = http.StatusOK

	switch t.Fault {
	case types.FaultToolError, types.FaultLLMError:
		resp.Error = &types.JSONRPCError{
			Code:    -32603, // Internal error
			Message: "internal error",
		}
		if t.Body != "" {
			resp.Error.Message = t.Body
		}

	case types.FaultToolTimeout, types.FaultLLMTimeout:
		resp.Error = &types.JSONRPCError{
			Code:    -32603,
			Message: "timeout",
		}

	case types.FaultInvalidJSON:
		// Write garbage instead of valid JSON-RPC.
		payload := `{"jsonrpc": "2.0", "result": INVALID, "data": [1, 2,}`
		if t.Payload != "" {
			payload = t.Payload
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if _, err := w.Write([]byte(payload)); err != nil {
			h.logger.Debug().Err(err).Msg("mcp: write invalid-json payload")
		}
		h.sink.RecordFault(t.ID, statusCode)
		return

	case types.FaultEmptyResponse:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		h.sink.RecordFault(t.ID, statusCode)
		return

	case types.FaultRateLimit:
		resp.Error = &types.JSONRPCError{
			Code:    -32000, // Server error
			Message: "rate limited",
		}
		if t.RetryAfterS > 0 {
			resp.Error.Data = map[string]int{"retry_after": t.RetryAfterS}
		}

	case types.FaultSlowResponse:
		// Delay already handled above. Return a successful empty result
		// to simulate a slow-but-OK response.
		resp.Result = types.ToolCallResult{
			Content: []types.ToolContent{{Type: "text", Text: ""}},
		}

	default:
		// Unknown fault — map to generic JSON-RPC internal error.
		resp.Error = &types.JSONRPCError{
			Code:    -32603,
			Message: fmt.Sprintf("injected fault: %s", t.Fault),
		}
	}

	data, err := json.Marshal(resp)
	if err != nil {
		h.logger.Error().Err(err).Msg("mcp: marshal fault response")
		http.Error(w, "internal error", http.StatusInternalServerError)
		h.sink.RecordFault(t.ID, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if _, err := w.Write(data); err != nil {
		h.logger.Debug().Err(err).Msg("mcp: write fault response")
	}
	h.sink.RecordFault(t.ID, statusCode)
}

// passthrough forwards the request to the upstream MCP server.
func (h *Handler) passthrough(w http.ResponseWriter, r *http.Request, testID string) {
	h.logger.Debug().
		Str("path", r.URL.Path).
		Str("method", r.Method).
		Msg("mcp: passthrough")

	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	h.rp.ServeHTTP(rec, r)

	if testID != "" {
		h.sink.RecordPassthrough(testID, rec.status)
	}
}

// extractToolName pulls the tool name from the params field of a
// tools/call JSON-RPC request.
func extractToolName(params interface{}) string {
	if params == nil {
		return ""
	}

	// params may already be a map from JSON unmarshal.
	switch p := params.(type) {
	case map[string]interface{}:
		if name, ok := p["name"].(string); ok {
			return name
		}
	}

	// Try marshaling and re-parsing if it's some other type.
	data, err := json.Marshal(params)
	if err != nil {
		return ""
	}
	var tc types.ToolCallParams
	if err := json.Unmarshal(data, &tc); err != nil {
		return ""
	}
	return tc.Name
}

// statusRecorder captures the HTTP status code written by the reverse proxy.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteStatus bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wroteStatus {
		s.status = code
		s.wroteStatus = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wroteStatus {
		s.status = http.StatusOK
		s.wroteStatus = true
	}
	return s.ResponseWriter.Write(b)
}

// IsMCPRequest returns true if the request body looks like a JSON-RPC 2.0
// MCP request (has jsonrpc "2.0" and a method field). Used by auto-detection.
func IsMCPRequest(body []byte) bool {
	var rpcReq types.JSONRPCRequest
	if err := json.Unmarshal(body, &rpcReq); err != nil {
		return false
	}
	return rpcReq.JSONRPC == "2.0" && rpcReq.Method != ""
}
