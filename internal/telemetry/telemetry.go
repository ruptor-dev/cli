// Package telemetry holds ruptor's OpenTelemetry tracer plumbing.
//
// Deferred to v2 (see ADR-010). Init intentionally constructs a
// TracerProvider with no exporter: the collector endpoint
// (telemetry.ruptor.dev) does not exist yet, and shipping a live
// TracerProvider that silently drops spans would lie about what v1
// does. The package is kept so v2 can plug an OTLP exporter in one
// place without relocating every span call site.
//
// When the exporter lands, the span payload must stay within the
// existing allowlist enforced by RecordRun: command name, fault
// types, run duration, ruptor version. Tool response bodies, agent
// output, file paths, user data, and IP addresses are not eligible
// and the function signature is the contract.
package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

// Config controls telemetry startup.
type Config struct {
	Enabled        bool
	ServiceVersion string
	// Endpoint is the OTLP collector URL. Empty disables the exporter
	// even when Enabled is true — useful for tests that want spans
	// but no network traffic.
	Endpoint string
}

// Provider wraps the tracer provider so callers can Shutdown cleanly
// without importing otel/sdk directly.
type Provider struct {
	tp       trace.TracerProvider
	shutdown func(context.Context) error
}

// Shutdown flushes pending spans and releases exporter resources.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p.shutdown == nil {
		return nil
	}
	return p.shutdown(ctx)
}

// Tracer returns a named tracer. Safe to call even when telemetry is
// disabled — it will return a no-op tracer.
func (p *Provider) Tracer(name string) trace.Tracer {
	return p.tp.Tracer(name)
}

// Init constructs a Provider per cfg. When cfg.Enabled is false, a
// noop provider is returned and Shutdown is a no-op. The global otel
// TracerProvider is set to the new provider so callers that reach for
// otel.Tracer("...") see the same configuration.
//
// The enabled branch builds an SDK TracerProvider with no exporter —
// spans record but never leave the process. This is intentional until
// v2 wires an OTLP exporter (see ADR-010); do not "fix" by adding a
// batcher without also solving the collector endpoint and auth story.
func Init(cfg Config) (*Provider, error) {
	if !cfg.Enabled {
		tp := noop.NewTracerProvider()
		otel.SetTracerProvider(tp)
		return &Provider{tp: tp}, nil
	}

	res, err := newResource(cfg.ServiceVersion)
	if err != nil {
		return nil, err
	}

	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res))
	otel.SetTracerProvider(tp)

	return &Provider{
		tp:       tp,
		shutdown: tp.Shutdown,
	}, nil
}

func newResource(serviceVersion string) (*sdkresource.Resource, error) {
	if serviceVersion == "" {
		serviceVersion = "dev"
	}
	r, err := sdkresource.Merge(
		sdkresource.Default(),
		sdkresource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("ruptor"),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: building resource: %w", err)
	}
	return r, nil
}

// RecordRun annotates a span with the minimum fields the security model
// allows: command, fault types, duration seconds, version. Callers
// pass only non-sensitive values — the allowlist here is enforced by
// function signature.
func RecordRun(span trace.Span, command string, faultTypes []string, durationS float64, version string) {
	if !span.IsRecording() {
		return
	}
	span.SetAttributes(
		attribute.String("ruptor.command", command),
		attribute.StringSlice("ruptor.fault_types", faultTypes),
		attribute.Float64("ruptor.duration_s", durationS),
		attribute.String("ruptor.version", version),
	)
}
