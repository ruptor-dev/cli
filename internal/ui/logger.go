package ui

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

// LogLevel mirrors zerolog levels for callers that don't want to import
// zerolog directly. Strings match zerolog's own level names.
type LogLevel string

const (
	LogDebug LogLevel = "debug"
	LogInfo  LogLevel = "info"
	LogWarn  LogLevel = "warn"
	LogError LogLevel = "error"
)

// NewLogger returns a zerolog.Logger writing to stderr in human-friendly
// console format. The chosen format matches the rest of the UI styling
// rather than the structured-JSON default — operator-facing output, not
// pipeline-ingest output. JSON output is reserved for telemetry.
func NewLogger(level LogLevel) zerolog.Logger {
	return NewLoggerTo(os.Stderr, level)
}

// NewLoggerTo writes to an arbitrary writer. Tests use this with
// io.Discard to silence output.
func NewLoggerTo(w io.Writer, level LogLevel) zerolog.Logger {
	zl, err := zerolog.ParseLevel(string(level))
	if err != nil {
		zl = zerolog.InfoLevel
	}
	cw := zerolog.ConsoleWriter{Out: w, TimeFormat: time.RFC3339}
	return zerolog.New(cw).Level(zl).With().Timestamp().Logger()
}

// SilentLogger returns a logger that drops every event. Used by tests
// that need to construct a Proxy / Simulator / Evaluator without noise.
func SilentLogger() zerolog.Logger {
	return zerolog.New(io.Discard).Level(zerolog.Disabled)
}
