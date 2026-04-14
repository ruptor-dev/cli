package ui

import (
	"os"

	"github.com/mattn/go-isatty"
)

// IsInteractive reports whether stdout is a real terminal. When it
// isn't (CI pipelines, pipe into a file, test harness), the TUI opts
// out and the caller falls back to plain context.Done() blocking.
func IsInteractive() bool {
	fd := os.Stdout.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
