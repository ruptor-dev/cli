//go:build tuismoke

// Package main_test — TUI smoke test gated behind the `tuismoke` build
// tag. It exec's the compiled binary inside a pty so bubbletea enters
// interactive mode (otherwise `ui.IsInteractive()` returns false and
// the TUI opts out entirely). The assertions pin what the operator
// actually sees when running `ruptor run chaos.yaml -v`: the log
// panel title, AltScreen entry, and mouse-cell-motion enable. These
// are the byte sequences bubbletea emits regardless of whether the
// pty consumer can reconstruct the alt-screen frame, so the smoke
// test catches both "panel never mounted" and "wheel events silently
// dropped" regressions without a human at a TTY.
//
// Run with:
//
//	go test -tags=tuismoke ./cmd/ruptor/... -count=1 -timeout 60s
package main_test

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSmokeChaos lays down a self-contained chaos.yaml that starts a
// 2-second agent (`sh -c 'sleep 2'`) and a throwaway passthrough. No
// python, no network. Fast enough to smoke, long enough to observe
// at least one render tick.
func writeSmokeChaos(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "chaos.yaml")
	body := `version: "1"
agent:
  name: smoke_agent
  entrypoint: sh -c 'sleep 2'
proxy:
  port: 18765
  passthrough_url: http://127.0.0.1:1
  request_timeout_s: 2
tests:
  - id: smoke
    tool: /x
    fault: tool_timeout
    delay_ms: 10
    probability: 1.0
evaluation:
  max_iterations: 1
  timeout_s: 2
  llm_judge: false
output:
  format: stdout
`
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

// runInPTY exec's the binary inside a pty and returns everything the
// program writes before it exits or deadline fires. TERM=xterm-256color
// so bubbletea does not fall back to the dumb renderer.
func runInPTY(t *testing.T, deadline time.Duration, args ...string) string {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"TERM=xterm-256color",
	)
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	require.NoError(t, err, "pty start")
	t.Cleanup(func() { _ = ptmx.Close() })

	done := make(chan []byte, 1)
	go func() {
		b, _ := io.ReadAll(ptmx)
		done <- b
	}()

	select {
	case buf := <-done:
		return string(buf)
	case <-time.After(deadline):
		_ = cmd.Process.Kill()
		return string(<-done)
	}
}

// TestTUI_VerboseRun_MountsLogPanel is the Layer 2 smoke assertion:
// with --verbose the program must enter alt-screen, enable mouse
// cell motion, and emit the Logs panel title. If any of those are
// missing the Layer 2 log panel is broken.
func TestTUI_VerboseRun_MountsLogPanel(t *testing.T) {
	cfg := writeSmokeChaos(t)
	out := runInPTY(t, 8*time.Second, "run", cfg, "-v")

	// AltScreen and mouse-cell-motion are must-haves for the log panel
	// to render cleanly (inline mode leaves artefacts when row widths
	// change) and for the wheel handler to actually receive events.
	assert.Contains(t, out, "\x1b[?1049h",
		"verbose TUI must enter AltScreen (\\e[?1049h) so the log panel frames render cleanly")
	assert.Contains(t, out, "\x1b[?1002h",
		"verbose TUI must request mouse cell motion (\\e[?1002h) so viewport.MouseWheelEnabled works")
	assert.Contains(t, out, "Logs",
		"verbose TUI must emit the Logs panel title")
	// Tail is the default mode; the literal title fragment catches a
	// silent rename of the panel header.
	assert.Contains(t, out, "tail",
		"verbose TUI must advertise `tail` in the panel title")
}

// TestTUI_NonVerboseRun_SkipsAltScreen pins the opt-out: a run without
// --verbose must not enable AltScreen or mouse cell motion. This
// guards against someone turning those on unconditionally and breaking
// scripts that pipe ruptor output into other tools.
func TestTUI_NonVerboseRun_SkipsAltScreen(t *testing.T) {
	cfg := writeSmokeChaos(t)
	out := runInPTY(t, 8*time.Second, "run", cfg)

	assert.NotContains(t, out, "\x1b[?1049h",
		"non-verbose run must not enter AltScreen — breaks pipes and scrollback")
	assert.False(t, strings.Contains(out, "Logs · tail"),
		"non-verbose run must not emit the log panel title")
}
