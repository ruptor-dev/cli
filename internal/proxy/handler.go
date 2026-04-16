package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/proxy/faults"
	"github.com/ruptor-dev/cli/internal/proxy/mcp"
)

// ServeHTTP implements http.Handler. It routes requests based on the
// configured proxy mode:
//   - "mcp": all traffic goes through the MCP JSON-RPC handler
//   - "auto": inspects the request body; JSON-RPC 2.0 traffic goes to
//     the MCP handler, everything else to the HTTP handler
//   - "http" or empty: existing HTTP fault-injection path
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mode := strings.ToLower(string(p.cfg.Mode))

	switch mode {
	case "mcp":
		if p.mcpHandler != nil {
			p.mcpHandler.ServeHTTP(w, r)
			return
		}
		// Fallback if handler wasn't initialised (bad passthrough URL).

	case "auto":
		if p.mcpHandler != nil && r.Method == http.MethodPost {
			body, err := io.ReadAll(io.LimitReader(r.Body, mcp.MaxBodySize))
			r.Body.Close()
			if err == nil && mcp.IsMCPRequest(body) {
				r.Body = io.NopCloser(bytes.NewReader(body))
				r.ContentLength = int64(len(body))
				p.mcpHandler.ServeHTTP(w, r)
				return
			}
			// Not MCP — restore body and fall through to HTTP handler.
			r.Body = io.NopCloser(bytes.NewReader(body))
			r.ContentLength = int64(len(body))
		}
	}

	// Default HTTP handler path.
	p.serveHTTP(w, r)
}

// serveHTTP is the original HTTP fault-injection handler.
func (p *Proxy) serveHTTP(w http.ResponseWriter, r *http.Request) {
	test, ok := p.matchTest(r.URL.Path)
	if !ok {
		p.passthrough(w, r, "")
		return
	}

	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	if p.shouldInject(test.Probability) {
		p.injectFault(rec, r, test)
		p.RecordFault(test.ID, rec.status)
		return
	}

	p.passthrough(rec, r, test.ID)
}

// matchTest finds the TestConfig to apply for the given request path.
// When the orchestrator has pinned an "active" test via SetActiveTest,
// that test wins if its Tool matches the path — this prevents the
// first-match dispatch from routing every request for a shared tool
// onto a single test's observations across the entire run. With no
// active test set, fall back to first-match over p.tests.
//
// Matching is exact (not prefix/wildcard): "/search" matches "/search"
// but not "/search?q=test" or "/search/order/123".
func (p *Proxy) matchTest(path string) (config.TestConfig, bool) {
	p.mu.Lock()
	active := p.activeTest
	p.mu.Unlock()
	if active != "" {
		for _, t := range p.tests {
			if t.ID == active && t.Tool == path {
				return t, true
			}
		}
	}
	for _, t := range p.tests {
		if t.Tool == path {
			return t, true
		}
	}
	return config.TestConfig{}, false
}

// SetActiveTest pins the test whose observations should receive matching
// hits during the current experiment. Empty string restores first-match
// dispatch. Goroutine-safe.
func (p *Proxy) SetActiveTest(id string) {
	p.mu.Lock()
	p.activeTest = id
	p.mu.Unlock()
}

// shouldInject rolls a random number and returns true if the fault should fire.
func (p *Proxy) shouldInject(probability float64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rng.Float64() < probability
}

// buildFaultConfig maps TestConfig fields to faults.FaultConfig.
func buildFaultConfig(t config.TestConfig) faults.FaultConfig {
	return faults.FaultConfig{
		Type:        t.Fault,
		DelayMs:     t.DelayMS,
		StatusCode:  t.StatusCode,
		Body:        t.Body,
		Payload:     t.Payload,
		Probability: t.Probability,
		RetryAfterS: t.RetryAfterS,
	}
}

// injectFault builds and fires the configured fault for the matched test.
func (p *Proxy) injectFault(w http.ResponseWriter, r *http.Request, t config.TestConfig) {
	cfg := buildFaultConfig(t)

	fault, err := p.registry.Build(t.Fault, cfg)
	if err != nil {
		p.logger.Error().
			Str("fault_type", string(t.Fault)).
			Str("tool", t.Tool).
			Str("test_id", t.ID).
			Err(err).
			Msg("proxy: build fault")
		http.Error(w, "internal proxy error", http.StatusInternalServerError)
		return
	}

	p.logger.Info().
		Str("fault_type", string(t.Fault)).
		Str("tool", t.Tool).
		Str("test_id", t.ID).
		Msg("proxy: injecting fault")

	if err := fault.Inject(w, r); err != nil {
		p.logger.Error().
			Str("fault_type", string(t.Fault)).
			Str("tool", t.Tool).
			Str("test_id", t.ID).
			Err(err).
			Msg("proxy: inject fault")
	}
}

// reverseProxy returns or lazily initialises a cached httputil.ReverseProxy
// for the configured passthrough URL. Creating a new proxy per request is
// wasteful — the Director closure only depends on the static target URL.
func (p *Proxy) reverseProxy() (*httputil.ReverseProxy, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.rp != nil {
		return p.rp, nil
	}

	target, err := url.Parse(p.cfg.PassthroughURL)
	if err != nil {
		return nil, fmt.Errorf("proxy: parse passthrough URL %q: %w", p.cfg.PassthroughURL, err)
	}

	p.rp = &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.Host = target.Host
		},
	}
	return p.rp, nil
}

// passthrough reverse-proxies the request to the configured backend. When
// testID is non-empty, the request matched a configured test whose
// probability did not fire; we still record the hit so the evaluator sees
// the path was exercised.
func (p *Proxy) passthrough(w http.ResponseWriter, r *http.Request, testID string) {
	rp, err := p.reverseProxy()
	if err != nil {
		p.logger.Error().Err(err).Msg("proxy: passthrough setup")
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	p.logger.Info().Str("path", r.URL.Path).Msg("proxy: passthrough")

	rp.ServeHTTP(w, r)

	if testID != "" {
		if rec, ok := w.(*statusRecorder); ok {
			p.RecordPassthrough(testID, rec.status)
		}
	}
}

// statusRecorder captures the HTTP status code a handler writes so the
// proxy can surface it in Observations without re-reading the response.
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
