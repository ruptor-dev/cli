// Package ui is the terminal-output boundary for ruptor.
//
// docs/engineering.md forbids fmt.Println / fmt.Printf / log.Printf outside this
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

var out io.Writer = os.Stdout

// SetWriter overrides the destination writer. Intended for tests.
func SetWriter(w io.Writer) { out = w }

// Writer returns the current destination writer.
func Writer() io.Writer { return out }

// Println writes a line of plain text followed by a newline.
func Println(s string) {
	fmt.Fprintln(out, s)
}

// Printf writes formatted text with no trailing newline.
func Printf(format string, a ...any) {
	fmt.Fprintf(out, format, a...)
}
