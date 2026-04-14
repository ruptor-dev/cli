package proxy

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/ruptor-dev/cli/internal/config"
	"github.com/ruptor-dev/cli/internal/proxy/faults"
)

const (
	defaultTimeout         = 30 * time.Second
	defaultShutdownTimeout = 5 * time.Second
	// portSearchWindow caps how many ports above the requested one we scan
	// when the requested port is already in use. Prevents unbounded probing.
	portSearchWindow = 100
)

// ProxyOption configures optional Proxy parameters.
type ProxyOption func(*Proxy)

// WithLogger sets a structured logger for the proxy.
func WithLogger(l zerolog.Logger) ProxyOption {
	return func(p *Proxy) {
		p.logger = l
	}
}

// WithTimeout sets the HTTP request timeout for proxied requests.
func WithTimeout(d time.Duration) ProxyOption {
	return func(p *Proxy) {
		p.timeout = d
	}
}

// WithRandSource sets a deterministic random source for fault injection probability rolls.
func WithRandSource(src rand.Source) ProxyOption {
	return func(p *Proxy) {
		p.rng = rand.New(src)
	}
}

// Proxy is a fault-injecting HTTP reverse proxy that sits between an AI agent
// and its tool APIs.
type Proxy struct {
	server   *http.Server
	registry *faults.FaultRegistry
	cfg      *config.ProxyConfig
	tests    []config.TestConfig
	logger   zerolog.Logger
	timeout  time.Duration
	rng      *rand.Rand

	mu       sync.Mutex
	listener net.Listener
	rp       *httputil.ReverseProxy
	obs      map[string]*Observation
}

// NewProxy constructs a Proxy with the given configuration, test cases,
// fault registry, and optional settings.
func NewProxy(cfg *config.ProxyConfig, tests []config.TestConfig, registry *faults.FaultRegistry, opts ...ProxyOption) *Proxy {
	p := &Proxy{
		cfg:      cfg,
		tests:    tests,
		registry: registry,
		logger:   zerolog.Nop(),
		timeout:  defaultTimeout,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}

	for _, opt := range opts {
		opt(p)
	}

	p.server = &http.Server{
		Handler:      p,
		ReadTimeout:  p.timeout,
		WriteTimeout: p.timeout,
	}

	return p
}

// Start begins listening and serving HTTP traffic. It blocks until ctx is
// cancelled or an unrecoverable error occurs.
//
// Port selection follows ADR-008: the requested port is tried first; if it
// is occupied, Start increments by one until it finds a free port or
// exhausts portSearchWindow attempts. The chosen port is emitted via the
// "proxy started" log line so the user always sees the real address.
func (p *Proxy) Start(ctx context.Context) error {
	ln, err := listenWithFallback(p.cfg.Port, p.logger)
	if err != nil {
		return err
	}

	p.mu.Lock()
	p.listener = ln
	p.mu.Unlock()

	p.logger.Info().
		Str("addr", ln.Addr().String()).
		Str("passthrough_url", p.cfg.PassthroughURL).
		Msg("proxy started")

	errCh := make(chan error, 1)
	go func() {
		if err := p.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("proxy: serve: %w", err)
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), defaultShutdownTimeout)
		defer cancel()

		if err := p.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("proxy: shutdown: %w", err)
		}

		p.logger.Info().
			Str("addr", ln.Addr().String()).
			Msg("proxy stopped")
		return nil

	case err := <-errCh:
		return err
	}
}

// Stop performs a graceful shutdown of the proxy server.
func (p *Proxy) Stop(ctx context.Context) error {
	p.logger.Info().Msg("proxy stopping")

	if err := p.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("proxy: stop: %w", err)
	}

	p.logger.Info().Msg("proxy stopped")
	return nil
}

// Addr returns the network address the proxy is listening on. This is
// especially useful when the proxy was started on port 0 for testing.
func (p *Proxy) Addr() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.listener == nil {
		return ""
	}
	return p.listener.Addr().String()
}

// listenWithFallback opens a TCP listener, auto-incrementing the port when
// the requested one is already in use. A port of 0 is passed through
// unchanged (OS-assigned). See ADR-008.
func listenWithFallback(startPort int, logger zerolog.Logger) (net.Listener, error) {
	if startPort == 0 {
		ln, err := net.Listen("tcp", ":0")
		if err != nil {
			return nil, fmt.Errorf("proxy: listen on :0: %w", err)
		}
		return ln, nil
	}

	var lastErr error
	for offset := 0; offset < portSearchWindow; offset++ {
		port := startPort + offset
		addr := fmt.Sprintf(":%d", port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			if offset > 0 {
				logger.Warn().
					Int("requested", startPort).
					Int("bound", port).
					Msg("proxy: requested port busy, using fallback")
			}
			return ln, nil
		}
		if !isAddrInUse(err) {
			return nil, fmt.Errorf("proxy: listen on %s: %w", addr, err)
		}
		lastErr = err
	}
	return nil, fmt.Errorf("proxy: no free port in [%d,%d): %w",
		startPort, startPort+portSearchWindow, lastErr)
}

// isAddrInUse returns true when err is an "address already in use" error.
// Matches the error string across platforms rather than switching on
// syscall.EADDRINUSE to avoid GOOS-specific build tags.
func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "address already in use") ||
		strings.Contains(msg, "Only one usage of each socket address")
}
