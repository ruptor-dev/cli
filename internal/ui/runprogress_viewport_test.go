package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seededModel returns a runModel whose log viewport is already sized
// and holds `n` lines of buffered content. Centralises the setup the
// viewport tests would otherwise repeat.
func seededModel(t *testing.T, n int) runModel {
	t.Helper()
	lines := make([]string, n)
	for i := range lines {
		lines[i] = "line-" + strings.Repeat("x", 1) + rune2str(i)
	}
	reader := &fakeLogReader{lines: lines}
	m := runModel{ctx: RunContext{LogReader: reader}, sticky: true}
	return m.initLogViewport(80)
}

// rune2str stringifies a small int without pulling strconv in — keeps
// test helpers trivial.
func rune2str(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return string(rune('a' + i%26))
}

// pressKey synthesises a tea.KeyPressMsg matching the viewport /
// runModel key matchers (they use msg.String()).
func pressKey(s string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: keyCodeFor(s)}
}

// keyCodeFor maps the named scroll keys to their rune codes. Tests
// exercise the same key strings the real TUI forwards.
func keyCodeFor(s string) rune {
	switch s {
	case "up":
		return tea.KeyUp
	case "down":
		return tea.KeyDown
	case "pgup":
		return tea.KeyPgUp
	case "pgdown":
		return tea.KeyPgDown
	case "home":
		return tea.KeyHome
	case "end":
		return tea.KeyEnd
	case "s":
		return 's'
	}
	return 0
}

// TestLogPanel_ScrollUp_ChangesOffset seeds a 50-line buffer, presses
// ↑, and asserts the viewport offset decreased while sticky flipped
// to false (manual scroll pauses auto-tail).
func TestLogPanel_ScrollUp_ChangesOffset(t *testing.T) {
	m := seededModel(t, 50)
	require.True(t, m.sticky, "model starts auto-tailing")
	before := m.vp.YOffset()
	require.Positive(t, before, "seeded model must have scrolled to the bottom so ↑ has room")

	next, _ := m.Update(pressKey("up"))
	got := next.(runModel)

	assert.Less(t, got.vp.YOffset(), before, "↑ must reduce YOffset")
	assert.False(t, got.sticky, "manual scroll must pause auto-tail")
}

// TestLogPanel_StickyToggle_JumpsToBottom scrolls up, toggles with `s`
// (sticky→paused), toggles again (paused→sticky), and asserts the
// final offset is at the bottom.
func TestLogPanel_StickyToggle_JumpsToBottom(t *testing.T) {
	m := seededModel(t, 50)
	m.vp.GotoTop()
	m.sticky = false

	// paused → sticky (must jump to bottom)
	next, _ := m.Update(pressKey("s"))
	got := next.(runModel)

	assert.True(t, got.sticky, "s must re-enable auto-tail")
	assert.True(t, got.vp.AtBottom(), "re-enabling sticky must jump the viewport to the bottom")
}

// TestLogPanel_NewLineWhilePaused_DoesNotJump confirms that with
// sticky=false, a fresh log line arriving on the next tick does NOT
// move the viewport's YOffset — the operator stays put.
func TestLogPanel_NewLineWhilePaused_DoesNotJump(t *testing.T) {
	m := seededModel(t, 50)
	m.vp.GotoTop()
	m.sticky = false
	offsetBefore := m.vp.YOffset()

	// simulate a new log line landing in the buffer between ticks
	reader := m.ctx.LogReader.(*fakeLogReader)
	reader.lines = append(reader.lines, "INF new line during pause")

	m = m.refreshLogViewport()

	assert.Equal(t, offsetBefore, m.vp.YOffset(), "paused viewport must not jump when new lines arrive")
	assert.False(t, m.sticky, "paused flag must remain paused")
}

// TestLogPanel_DefaultSticky_FollowsTail confirms the default — a
// fresh model with no manual scroll — keeps the viewport at the
// bottom as new lines arrive.
func TestLogPanel_DefaultSticky_FollowsTail(t *testing.T) {
	m := seededModel(t, 20)
	require.True(t, m.sticky, "default must be sticky")
	require.True(t, m.vp.AtBottom(), "seeded sticky model starts at the bottom")

	reader := m.ctx.LogReader.(*fakeLogReader)
	reader.lines = append(reader.lines, "INF tail line wins")
	m = m.refreshLogViewport()

	assert.True(t, m.vp.AtBottom(), "sticky must keep the viewport pinned to the bottom")
	assert.Contains(t, m.vp.GetContent(), "tail line wins", "latest line must make it into the viewport content")
}

// TestLogPanel_PauseIndicatorRenders pins the title contract: `tail`
// when sticky, `paused (s)` when frozen.
func TestLogPanel_PauseIndicatorRenders(t *testing.T) {
	m := seededModel(t, 10)
	stickyOut := stripANSI(m.renderLogPanel())
	assert.Contains(t, stickyOut, "tail", "sticky title must advertise tail mode")
	assert.NotContains(t, stickyOut, "paused", "sticky title must not show paused")

	m.sticky = false
	pausedOut := stripANSI(m.renderLogPanel())
	assert.Contains(t, pausedOut, "paused (s)", "paused title must call out the `s` shortcut")
}

// TestLogPanel_NoViewportWhenReaderAbsent confirms Layer 1 preservation:
// when LogReader is nil, neither the panel title nor the viewport
// render in the final View, and scroll keys fall through to the
// existing q/r shortcuts (here: q aborts).
func TestLogPanel_NoViewportWhenReaderAbsent(t *testing.T) {
	m := runModel{ctx: RunContext{ConfigFile: "chaos.yaml"}, sticky: true}
	out := stripANSI(mustString(m.View()))

	assert.NotContains(t, out, "Logs", "no Logs panel when LogReader is nil")

	next, _ := m.Update(pressKey("up"))
	got := next.(runModel)
	assert.False(t, got.aborted, "up without a log panel must be a no-op, not abort")
}
