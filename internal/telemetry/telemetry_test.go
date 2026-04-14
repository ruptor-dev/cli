package telemetry

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_DisabledReturnsNoop(t *testing.T) {
	p, err := Init(Config{Enabled: false})
	require.NoError(t, err)
	require.NotNil(t, p)

	// Noop tracer's spans must not record.
	tr := p.Tracer("test")
	_, span := tr.Start(context.Background(), "op")
	defer span.End()
	assert.False(t, span.IsRecording(), "disabled telemetry must produce non-recording spans")

	require.NoError(t, p.Shutdown(context.Background()))
}

func TestInit_EnabledWithoutEndpoint(t *testing.T) {
	// Enabled but no Endpoint = the span recorder records locally but
	// nothing exports. This is the common test-environment shape.
	p, err := Init(Config{Enabled: true, ServiceVersion: "test"})
	require.NoError(t, err)
	require.NotNil(t, p)

	tr := p.Tracer("test")
	_, span := tr.Start(context.Background(), "op")
	assert.True(t, span.IsRecording())
	span.End()

	require.NoError(t, p.Shutdown(context.Background()))
}

func TestRecordRun_DoesNotPanicOnDisabledSpan(t *testing.T) {
	p, err := Init(Config{Enabled: false})
	require.NoError(t, err)

	_, span := p.Tracer("x").Start(context.Background(), "op")
	defer span.End()

	// Must not panic — no-op spans are allowed.
	RecordRun(span, "run", []string{"tool_timeout"}, 1.23, "0.1.0")
}
