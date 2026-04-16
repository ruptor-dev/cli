package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeLogReader is an in-test LogReader so the tests can assert
// exactly what the panel receives, independent of LogBuffer logic.
type fakeLogReader struct {
	lines []string
}

func (f *fakeLogReader) Lines(n int) []string {
	if n <= 0 || n > len(f.lines) {
		return append([]string(nil), f.lines...)
	}
	return append([]string(nil), f.lines[len(f.lines)-n:]...)
}

// stripANSI removes the escape sequences lipgloss embeds so the
// assertions can match on the visible text. This is a visual-test
// helper — the real TUI ships the ANSI so the styles render.
func stripANSI(s string) string {
	// Naive state machine: everything from ESC until the terminating
	// byte ('m' for SGR, 'J' for clear, etc) is an escape. Good enough
	// for lipgloss SGR codes which end in 'm'.
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			for i += 2; i < len(s); i++ {
				c := s[i]
				if c >= '@' && c <= '~' {
					break
				}
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// visibleLineCount returns how many newline-separated rows the
// rendered string produces. Used to pin panel height stability.
func visibleLineCount(s string) int {
	return strings.Count(s, "\n") + 1
}

// TestRenderLogPanel_ShowsRecentLines is the baseline visual
// assertion: panel title present, every provided log line reaches
// the rendered output.
func TestRenderLogPanel_ShowsRecentLines(t *testing.T) {
	reader := &fakeLogReader{lines: []string{
		"INF proxy started addr=:8080",
		"INF runner: agent started pid=42",
		"ERR proxy: inject fault error=\"context canceled\"",
	}}
	m := runModel{ctx: RunContext{LogReader: reader}}

	rendered := stripANSI(m.renderLogPanel())

	assert.Contains(t, rendered, "Logs", "panel must carry the Logs title so operators know what they're looking at")
	assert.Contains(t, rendered, "proxy started", "first log line must reach the render")
	assert.Contains(t, rendered, "agent started", "middle log line must reach the render")
	assert.Contains(t, rendered, "context canceled", "error line must reach the render")
}

// TestRenderLogPanel_HeightIsStable covers the rendering invariant
// the spec promises: the panel's row count does NOT change as the
// buffer fills. A bouncing height would push the Robustness bar up
// and down each tick.
func TestRenderLogPanel_HeightIsStable(t *testing.T) {
	empty := &fakeLogReader{lines: nil}
	partial := &fakeLogReader{lines: []string{"one", "two", "three"}}
	fullSlice := make([]string, LogPanelHeight)
	for i := range fullSlice {
		fullSlice[i] = "line"
	}
	full := &fakeLogReader{lines: fullSlice}

	heights := map[string]int{
		"empty":   visibleLineCount(runModel{ctx: RunContext{LogReader: empty}}.renderLogPanel()),
		"partial": visibleLineCount(runModel{ctx: RunContext{LogReader: partial}}.renderLogPanel()),
		"full":    visibleLineCount(runModel{ctx: RunContext{LogReader: full}}.renderLogPanel()),
	}

	assert.Equal(t, heights["empty"], heights["partial"], "height must not change as lines trickle in")
	assert.Equal(t, heights["partial"], heights["full"], "height must not change when the buffer saturates")
}

// TestRenderLogPanel_TailsAtPanelHeight confirms the panel only
// pulls LogPanelHeight lines regardless of buffer size.
func TestRenderLogPanel_TailsAtPanelHeight(t *testing.T) {
	// Load 2× the panel height so tail behaviour matters.
	lines := make([]string, LogPanelHeight*2)
	for i := range lines {
		lines[i] = "line-" + string(rune('a'+i%26))
	}
	reader := &fakeLogReader{lines: lines}
	m := runModel{ctx: RunContext{LogReader: reader}}

	rendered := stripANSI(m.renderLogPanel())

	// The first half (oldest) must be absent; the second half present.
	first := lines[0]
	last := lines[len(lines)-1]
	assert.NotContains(t, rendered, first, "oldest line must have tailed off once we exceed panel height")
	assert.Contains(t, rendered, last, "most recent line must always be visible")
}

// TestView_MountsLogPanelOnlyWhenReaderPresent asserts the zero-impact
// promise of the feature: runs WITHOUT --verbose must render exactly
// the same View as before the panel existed.
func TestView_MountsLogPanelOnlyWhenReaderPresent(t *testing.T) {
	snap := RunSnapshot{
		Experiments: []ExperimentState{{ID: "test_a", Status: StatusPending}},
	}

	noReader := runModel{
		ctx:  RunContext{ConfigFile: "chaos.yaml", Snapshot: nil},
		snap: snap,
	}
	withReader := runModel{
		ctx: RunContext{
			ConfigFile: "chaos.yaml",
			LogReader:  &fakeLogReader{lines: []string{"INF proxy started"}},
		},
		snap: snap,
	}

	noReaderOut := stripANSI(mustString(noReader.View()))
	withReaderOut := stripANSI(mustString(withReader.View()))

	assert.NotContains(t, noReaderOut, "Logs",
		"the Logs panel title must not appear when the caller did not pass a LogReader (non-verbose run)")
	assert.Contains(t, withReaderOut, "Logs",
		"the Logs panel title must appear once a LogReader is wired (verbose run)")
	assert.Contains(t, withReaderOut, "INF proxy started",
		"the panel must surface the buffered log line when mounted")
}

// TestView_LogPanelSitsBetweenExperimentsAndScore pins the layout
// contract from docs/specs/ui.md: Experiments → Logs → Robustness.
func TestView_LogPanelSitsBetweenExperimentsAndScore(t *testing.T) {
	snap := RunSnapshot{
		Experiments:  []ExperimentState{{ID: "one", Status: StatusRunning}},
		ScorePercent: 50,
	}
	m := runModel{
		ctx: RunContext{
			ConfigFile: "chaos.yaml",
			LogReader:  &fakeLogReader{lines: []string{"INF a single line"}},
		},
		snap: snap,
	}
	out := stripANSI(mustString(m.View()))

	experimentsIdx := strings.Index(out, "Experiments")
	logsIdx := strings.Index(out, "Logs")
	robustnessIdx := strings.Index(out, "Robustness")

	require.NotEqual(t, -1, experimentsIdx, "Experiments box must render")
	require.NotEqual(t, -1, logsIdx, "Logs panel must render")
	require.NotEqual(t, -1, robustnessIdx, "Robustness bar must render")

	assert.Less(t, experimentsIdx, logsIdx, "Experiments must precede Logs — the panel sits below the experiment list")
	assert.Less(t, logsIdx, robustnessIdx, "Logs must precede Robustness — the score bar is still the closing element")
}

// mustString extracts the rendered content from a tea.View. Helper
// for terse assertions; the test fails loudly if the shape changes.
func mustString(v tea.View) string {
	return v.Content
}
