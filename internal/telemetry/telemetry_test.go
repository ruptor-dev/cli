package telemetry

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_DisabledReturnsNoop(t *testing.T) {
	p, err := Init(Config{Enabled: false}, zerolog.Nop())
	require.NoError(t, err)
	require.NotNil(t, p)

	// Noop tracer's spans must not record.
	tr := p.Tracer("test")
	_, span := tr.Start(context.Background(), "op")
	defer span.End()
	assert.False(t, span.IsRecording(), "disabled telemetry must produce non-recording spans")

	require.NoError(t, p.Shutdown(context.Background()))
}

func TestInit_EnabledWithoutEndpointReturnsNoop(t *testing.T) {
	// Enabled with no Endpoint is the default shape for users who set
	// RUPTOR_TELEMETRY_ENABLED in anticipation of an exporter: the
	// provider degrades to noop rather than accumulating spans in memory
	// with nowhere to export. Parity with the Enabled=false path is the
	// contract — verified via span recording.
	resetDegradedWarnOnceForTest(t)

	p, err := Init(Config{Enabled: true, Endpoint: "", ServiceVersion: "test"}, zerolog.Nop())
	require.NoError(t, err)
	require.NotNil(t, p)

	tr := p.Tracer("test")
	_, span := tr.Start(context.Background(), "op")
	defer span.End()
	assert.False(t, span.IsRecording(), "enabled without endpoint must degrade to noop spans")

	// Shutdown on the noop path is a no-op and must not error.
	require.NoError(t, p.Shutdown(context.Background()))
}

func TestInit_EnabledWithEndpointReturnsSDK(t *testing.T) {
	// Enabled + Endpoint set = the real SDK provider. The OTLP exporter
	// wiring is deferred to ADR-010; the provider still records spans.
	p, err := Init(Config{Enabled: true, Endpoint: "http://localhost:4318", ServiceVersion: "test"}, zerolog.Nop())
	require.NoError(t, err)
	require.NotNil(t, p)

	tr := p.Tracer("test")
	_, span := tr.Start(context.Background(), "op")
	assert.True(t, span.IsRecording(), "enabled with endpoint must produce recording spans")
	span.End()

	require.NoError(t, p.Shutdown(context.Background()))
}

func TestInit_WarnFiresOnce(t *testing.T) {
	// Repeated Init calls with Enabled=true, Endpoint="" must emit the
	// degraded-state Warn exactly once per process. The sync.Once guard
	// protects against a future config-reload loop spamming the warning.
	resetDegradedWarnOnceForTest(t)

	var buf bytes.Buffer
	logger := zerolog.New(&buf).Level(zerolog.WarnLevel)

	for range 3 {
		p, err := Init(Config{Enabled: true, Endpoint: "", ServiceVersion: "test"}, logger)
		require.NoError(t, err)
		require.NotNil(t, p)
	}

	count := strings.Count(buf.String(), "telemetry enabled but no exporter endpoint configured")
	assert.Equal(t, 1, count, "degraded-state Warn must fire exactly once across repeated Init calls")
}

func TestRecordRun_DoesNotPanicOnDisabledSpan(t *testing.T) {
	p, err := Init(Config{Enabled: false}, zerolog.Nop())
	require.NoError(t, err)

	_, span := p.Tracer("x").Start(context.Background(), "op")
	defer span.End()

	// Must not panic — no-op spans are allowed.
	RecordRun(span, "run", []string{"tool_timeout"}, 1.23, "0.1.0")
}

// resetDegradedWarnOnceForTest resets the package-level sync.Once so
// tests can exercise the first-fire branch deterministically regardless
// of test ordering. Tests that do not care about warn firing can omit
// the call.
func resetDegradedWarnOnceForTest(t *testing.T) {
	t.Helper()
	degradedWarnOnce = sync.Once{}
}
