// Package ui is the terminal-output boundary for ruptor.
//
// CLAUDE.md forbids fmt.Println / fmt.Printf / log.Printf outside this
// package — every user-visible line in cmd/ and the rest of internal/
// must go through a helper defined here. The v1 implementation is
// deliberately minimal: a writer-backed Println. The bubbletea + lipgloss
// work lands in a later PR and replaces the internals without changing
// this API.
package ui

import (
	"fmt"
	"io"
	"os"
)

// out carries data (pipeable); errOut carries diagnostics (warnings, errors).
var (
	out    io.Writer = os.Stdout
	errOut io.Writer = os.Stderr
)

// SetWriter overrides the data-stream writer (stdout). Intended for tests
// that capture pipeable output.
func SetWriter(w io.Writer) { out = w }

// SetErrWriter overrides the diagnostic-stream writer (stderr). Intended
// for tests that capture warnings, errors, and other diagnostic output.
func SetErrWriter(w io.Writer) { errOut = w }

// Writer returns the current data-stream writer.
func Writer() io.Writer { return out }

// ErrWriter returns the current diagnostic-stream writer.
func ErrWriter() io.Writer { return errOut }

// Println writes a line of plain text followed by a newline to stdout.
func Println(s string) {
	fmt.Fprintln(out, s)
}

// Printf writes formatted text with no trailing newline to stdout.
func Printf(format string, a ...any) {
	fmt.Fprintf(out, format, a...)
}
