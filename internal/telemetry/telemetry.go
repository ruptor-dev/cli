// Package telemetry wires ruptor's opt-in OpenTelemetry tracing.
//
// Security rule: telemetry is DISABLED by default. It is enabled only after
// explicit `ruptor auth login` (user knowingly connects to cloud) or by
// setting RUPTOR_TELEMETRY_ENABLED=true. The telemetry payload contains
// ONLY: command name, fault types used, run duration, ruptor version.
// NEVER: tool response content, agent output, file paths, user data, IP.
//
// Init resolves to one of three states (see ADR-010):
//
//   - Disabled (cfg.Enabled == false): return a noop provider. Shutdown
//     is a no-op.
//   - Enabled without endpoint (cfg.Enabled == true && cfg.Endpoint == ""):
//     return a noop provider and emit a single Warn log line through the
//     caller-supplied logger. The flag parses so RUPTOR_TELEMETRY_ENABLED
//     set in anticipation of an exporter does not break startup; spans
//     simply do not record until an endpoint is configured.
//   - Enabled with endpoint (both set): return an SDK TracerProvider
//     carrying the ruptor resource. Wiring the OTLP exporter onto this
//     provider is tracked by ADR-010.
//
// The span payload must stay within the existing allowlist enforced by
// RecordRun: command name, fault types, run duration, ruptor version.
// Tool response bodies, agent output, file paths, user data, and IP
// addresses are not eligible and the function signature is the contract.
package telemetry

import (
	"context"
	"fmt"
	"sync"

	"github.com/rs/zerolog"
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
	// Endpoint is the OTLP collector URL. Empty means no exporter is
	// configured; Init degrades to a noop provider even when Enabled is
	// true (see ADR-010).
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

// degradedWarnOnce guards the "enabled without endpoint" Warn so repeated
// Init calls (e.g. from a future config-reload loop) fire the warning
// exactly once per process.
var degradedWarnOnce sync.Once

// Init constructs a Provider per cfg and sets it as the global otel
// TracerProvider so callers that reach for otel.Tracer("...") see the
// same configuration. The supplied logger receives the degraded-state
// warning; pass the already-configured CLI logger (typically the one
// returned by ui.NewLogger) so the message renders through the same
// writer as the rest of the terminal output. Tests pass zerolog.Nop().
//
// Behaviour depends on the (Enabled, Endpoint) pair:
//
//   - Enabled == false: noop provider, Shutdown is a no-op.
//   - Enabled == true, Endpoint == "": noop provider plus one Warn log
//     line pointing at ADR-010, emitted at most once per process
//     regardless of how many times Init is called.
//   - Enabled == true, Endpoint != "": SDK TracerProvider carrying the
//     ruptor resource. The OTLP exporter wiring lands with ADR-010.
//
// Init returns an error only when building the SDK resource fails; the
// degraded-state branch never errors so a user who set the flag in
// anticipation of an exporter does not fail to start.
func Init(cfg Config, logger zerolog.Logger) (*Provider, error) {
	if !cfg.Enabled {
		return newNoopProvider(), nil
	}

	if cfg.Endpoint == "" {
		degradedWarnOnce.Do(func() {
			logger.Warn().Msg("telemetry enabled but no exporter endpoint configured (ADR-010); spans will not be exported")
		})
		return newNoopProvider(), nil
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

func newNoopProvider() *Provider {
	tp := noop.NewTracerProvider()
	otel.SetTracerProvider(tp)
	return &Provider{tp: tp}
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
