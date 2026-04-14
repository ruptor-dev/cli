package proxy

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/faultforge/faultforge/internal/config"
	"github.com/faultforge/faultforge/internal/proxy/faults"
)

// ServeHTTP implements http.Handler. It matches the request path against
// configured test cases, rolls against the probability, and either injects a
// fault or reverse-proxies the request to the passthrough backend.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	test, ok := p.matchTest(r.URL.Path)
	if ok && p.shouldInject(test.Probability) {
		p.injectFault(w, r, test)
		return
	}

	p.passthrough(w, r)
}

// matchTest finds the first TestConfig whose Tool matches the request path.
// Note: matching is exact (not prefix/wildcard). Tool paths must match request paths exactly.
// For example, "/search" matches "/search" but not "/search?q=test" or "/search/order/123".
func (p *Proxy) matchTest(path string) (config.TestConfig, bool) {
	for _, t := range p.tests {
		if t.Tool == path {
			return t, true
		}
	}
	return config.TestConfig{}, false
}

// shouldInject rolls a random number and returns true if the fault should fire.
func (p *Proxy) shouldInject(probability float64) bool {
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
		p.logger.Error("proxy: build fault",
			slog.String("fault_type", string(t.Fault)),
			slog.String("tool", t.Tool),
			slog.String("test_id", t.ID),
			slog.String("error", err.Error()),
		)
		http.Error(w, "internal proxy error", http.StatusInternalServerError)
		return
	}

	p.logger.Info("proxy: injecting fault",
		slog.String("fault_type", string(t.Fault)),
		slog.String("tool", t.Tool),
		slog.String("test_id", t.ID),
	)

	if err := fault.Inject(w, r); err != nil {
		p.logger.Error("proxy: inject fault",
			slog.String("fault_type", string(t.Fault)),
			slog.String("tool", t.Tool),
			slog.String("test_id", t.ID),
			slog.String("error", err.Error()),
		)
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

// passthrough reverse-proxies the request to the configured backend.
func (p *Proxy) passthrough(w http.ResponseWriter, r *http.Request) {
	rp, err := p.reverseProxy()
	if err != nil {
		p.logger.Error("proxy: passthrough setup",
			slog.String("error", err.Error()),
		)
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	p.logger.Info("proxy: passthrough",
		slog.String("path", r.URL.Path),
	)

	rp.ServeHTTP(w, r)
}
