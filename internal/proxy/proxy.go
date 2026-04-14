package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"sync"
	"time"

	"github.com/faultforge/faultforge/internal/config"
	"github.com/faultforge/faultforge/internal/proxy/faults"
)

const defaultTimeout = 30 * time.Second

// ProxyOption configures optional Proxy parameters.
type ProxyOption func(*Proxy)

// WithLogger sets a structured logger for the proxy.
func WithLogger(l *slog.Logger) ProxyOption {
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

// Proxy is a fault-injecting HTTP reverse proxy that sits between an AI agent
// and its tool APIs.
type Proxy struct {
	server   *http.Server
	registry *faults.FaultRegistry
	cfg      *config.ProxyConfig
	tests    []config.TestConfig
	logger   *slog.Logger
	timeout  time.Duration

	mu       sync.Mutex
	listener net.Listener
	rp       *httputil.ReverseProxy
}

// NewProxy constructs a Proxy with the given configuration, test cases,
// fault registry, and optional settings.
func NewProxy(cfg *config.ProxyConfig, tests []config.TestConfig, registry *faults.FaultRegistry, opts ...ProxyOption) *Proxy {
	p := &Proxy{
		cfg:      cfg,
		tests:    tests,
		registry: registry,
		logger:   slog.Default(),
		timeout:  defaultTimeout,
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
func (p *Proxy) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", p.cfg.Port)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("proxy: listen on %s: %w", addr, err)
	}

	p.mu.Lock()
	p.listener = ln
	p.mu.Unlock()

	p.logger.Info("proxy started",
		slog.String("addr", ln.Addr().String()),
		slog.String("passthrough_url", p.cfg.PassthroughURL),
	)

	errCh := make(chan error, 1)
	go func() {
		if err := p.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("proxy: serve: %w", err)
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := p.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("proxy: shutdown: %w", err)
		}

		p.logger.Info("proxy stopped",
			slog.String("addr", ln.Addr().String()),
		)
		return nil

	case err := <-errCh:
		return err
	}
}

// Stop performs a graceful shutdown of the proxy server.
func (p *Proxy) Stop(ctx context.Context) error {
	p.logger.Info("proxy stopping")

	if err := p.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("proxy: stop: %w", err)
	}

	p.logger.Info("proxy stopped")
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
