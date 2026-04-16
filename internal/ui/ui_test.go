package ui

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
)

func TestProgressBar_Widths(t *testing.T) {
	tests := []struct {
		percent int
		width   int
		wantPct string
	}{
		{0, 10, "░░░░░░░░░░"},
		{50, 10, "█████░░░░░"},
		{100, 10, "██████████"},
	}
	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := ProgressBar(tt.percent, tt.width)
			// Strip ANSI escape sequences for plain comparison.
			plain := stripAnsi(got)
			assert.Equal(t, tt.wantPct, plain)
		})
	}
}

func TestProgressBar_ClampsOutOfRange(t *testing.T) {
	lo := stripAnsi(ProgressBar(-50, 10))
	hi := stripAnsi(ProgressBar(150, 10))
	assert.Equal(t, "░░░░░░░░░░", lo)
	assert.Equal(t, "██████████", hi)
}

func TestRenderCompletion_ContainsSummary(t *testing.T) {
	s := RenderCompletion(CompletionSummary{
		ScorePercent: 82,
		Passed:       6,
		Failed:       2,
		Errored:      0,
		ReportPaths:  []string{"./reports/run.html"},
	})
	plain := stripAnsi(s)
	assert.Contains(t, plain, "Robustness Score")
	assert.Contains(t, plain, "82%")
	assert.Contains(t, plain, "6 passed")
	assert.Contains(t, plain, "2 failed")
	assert.Contains(t, plain, "./reports/run.html")
}

func TestCountDone(t *testing.T) {
	exps := []ExperimentState{
		{Status: StatusPassed},
		{Status: StatusFailed},
		{Status: StatusPending},
		{Status: StatusRunning},
	}
	assert.Equal(t, 2, countDone(exps))
}

func TestScoreColor_Thresholds(t *testing.T) {
	// Same pointer identity means we hit the intended branch.
	assert.Equal(t, Theme.Success, scoreColor(80))
	assert.Equal(t, Theme.Warning, scoreColor(50))
	assert.Equal(t, Theme.Failure, scoreColor(0))
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "—"},
		{250 * time.Millisecond, "250ms"},
		{1500 * time.Millisecond, "1.5s"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, formatDuration(tt.d))
	}
}

func TestOutputHelpers_WriteThroughErrWriter(t *testing.T) {
	// Success/Error/Info/Warning/Dim are diagnostic helpers routed to
	// stderr — capture via SetErrWriter, not SetWriter. This guards the
	// stdout-vs-stderr split: any helper that regressed to stdout would
	// silently drop from the buffer and fail the assertion below.
	var buf bytes.Buffer
	prev := ErrWriter()
	SetErrWriter(&buf)
	defer SetErrWriter(prev)

	Success("saved")
	Error("bad")
	Info("here")
	Warning("watch")
	Dim("idle")

	plain := stripAnsi(buf.String())
	for _, want := range []string{"saved", "bad", "here", "watch", "idle"} {
		assert.Contains(t, plain, want)
	}
}

func TestPlainHelpers_WriteThroughWriter(t *testing.T) {
	// Println/Printf carry pipeable data and must stay on stdout.
	var buf bytes.Buffer
	prev := Writer()
	SetWriter(&buf)
	defer SetWriter(prev)

	Println("line")
	Printf("formatted %d\n", 7)

	got := buf.String()
	assert.Contains(t, got, "line")
	assert.Contains(t, got, "formatted 7")
}

func TestIconFor(t *testing.T) {
	tests := []struct {
		status ExperimentStatus
		want   string
	}{
		{StatusPassed, IconPassed},
		{StatusFailed, IconFailed},
		{StatusRunning, IconRunning},
		{StatusPending, IconPending},
	}
	for _, tt := range tests {
		icon, _ := iconFor(tt.status)
		assert.Equal(t, tt.want, icon)
	}
}

// stripAnsi removes ANSI SGR sequences so tests can assert on visible
// characters without fighting lipgloss colour codes.
func stripAnsi(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Exercise the runModel update loop in isolation so the bubbletea
// wiring is covered without needing a real terminal.
func TestRunModel_QOnPressQuits(t *testing.T) {
	m := runModel{ctx: RunContext{Snapshot: func() RunSnapshot { return RunSnapshot{} }}}

	// Simulate 'q'. KeyPressMsg embeds Key; we construct it via the
	// bubbletea constructor path to avoid coupling to internals.
	key := tea.KeyPressMsg{Text: "q", Code: 'q'}
	next, cmd := m.Update(key)
	assert.NotNil(t, cmd, "quit key must emit a Quit command")
	nm, ok := next.(runModel)
	if ok {
		assert.True(t, nm.aborted)
	}
}
