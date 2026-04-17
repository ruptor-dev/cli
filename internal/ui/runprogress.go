// Package ui — runprogress.go: the live Bubbletea program shown during
// `ruptor run`. Layout per docs/specs/ui.md: header box, experiment list,
// robustness score progress bar, keyboard shortcut hint.
//
// The model refreshes every tickInterval by pulling a snapshot from the
// Observations callback supplied by the caller (see cmd/ruptor). This is
// the 100ms tick-poller mentioned in AUDIT.md §10 — clean but not
// push-based. A future PR will replace the poll with a channel fed by
// the proxy.
package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const tickInterval = 100 * time.Millisecond

// ExperimentState is the snapshot of one configured test as the TUI
// understands it. Caller builds these from proxy.Observations().
type ExperimentState struct {
	ID       string
	Status   ExperimentStatus
	Duration time.Duration
}

// ExperimentStatus is a coarse enum for the TUI list icons.
type ExperimentStatus int

const (
	StatusPending ExperimentStatus = iota
	StatusRunning
	StatusPassed
	StatusFailed
)

// RunSnapshot is the TUI's view of the world on one tick.
type RunSnapshot struct {
	Experiments []ExperimentState
	// ScorePercent is 0..100.
	ScorePercent int
	// Done marks the snapshot as terminal; the program quits.
	Done bool
}

// SnapshotFn returns a new RunSnapshot. Called on every tick. Keep it
// fast — runs on the UI goroutine.
type SnapshotFn func() RunSnapshot

// RunContext configures the header of the TUI.
type RunContext struct {
	ConfigFile string
	AgentName  string
	Port       int
	Entrypoint string
	Snapshot   SnapshotFn
	// LogReader, when non-nil, mounts a log panel between the
	// Experiments box and the Robustness bar. Per docs/specs/ui.md
	// (`Run progress TUI with --verbose`). Layer 1 contract: tail
	// only, fixed height, no scroll. Pass *LogBuffer or any other
	// LogReader implementation.
	LogReader LogReader
}

// LogPanelHeight is the visible-row count of the verbose-mode log
// panel. Box border + title add 2 lines on top.
const LogPanelHeight = 12

// LogPanelMaxLineLen caps each rendered log line so a stray multi-KB
// trace does not blow the panel's width. Suggested cap when wiring
// the LogBuffer; the panel itself does NOT re-truncate (the buffer
// owns truncation so multiple readers see the same string).
const LogPanelMaxLineLen = 240

// runModel is the Bubbletea model driving the live TUI.
type runModel struct {
	ctx      RunContext
	snap     RunSnapshot
	quitting bool
	aborted  bool
	// vp is the scrollable viewport backing the verbose-mode log
	// panel (Layer 2, `charm.land/bubbles/v2`). Only activated when
	// ctx.LogReader is non-nil; unused in non-verbose runs.
	vp viewport.Model
	// vpInit tracks whether the viewport has been sized yet. First
	// tick with a LogReader sizes it to LogPanelHeight rows.
	vpInit bool
	// sticky is true when the panel auto-tails (default). `s` toggles
	// to paused; any manual scroll clears the flag. Re-enabling sticky
	// jumps to the bottom.
	sticky bool
}

// Program wraps the Bubbletea program so callers outside the ui
// package do not import bubbletea directly. Thin forwarder: Run blocks
// until the user quits or the snapshot returns Done=true; Send and
// Quit are goroutine-safe passthroughs.
type Program struct {
	prog *tea.Program
}

// NewRunProgress builds a Bubbletea program for the live run view.
func NewRunProgress(ctx RunContext) *Program {
	m := runModel{ctx: ctx, sticky: true}
	if ctx.LogReader != nil {
		m.vp = newLogViewport()
		m.vpInit = true
	}
	return &Program{prog: tea.NewProgram(m)}
}

// newLogViewport constructs the bubbles/v2 viewport sized to the log
// panel's fixed height. Width is left to the first WindowSizeMsg.
func newLogViewport() viewport.Model {
	vp := viewport.New(viewport.WithHeight(LogPanelHeight))
	vp.MouseWheelEnabled = true
	vp.SoftWrap = false
	vp.FillHeight = true
	return vp
}

// Run blocks and returns the final model.
func (p *Program) Run() (tea.Model, error) {
	return p.prog.Run()
}

// Send delivers a message to the program's Update loop. Safe from
// any goroutine. Used to force a snapshot refresh from the
// orchestrator after the last experiment completes.
func (p *Program) Send(msg tea.Msg) {
	p.prog.Send(msg)
}

// Quit asks the program to exit. Safe from any goroutine.
func (p *Program) Quit() {
	p.prog.Quit()
}

// Aborted reports whether the final model exited because the user
// pressed 'q' rather than a natural completion.
func Aborted(finalModel tea.Model) bool {
	if m, ok := finalModel.(runModel); ok {
		return m.aborted
	}
	return false
}

type tickMsg time.Time
type finalTickMsg time.Time

// ForceSnapshotMsg asks the program to take a snapshot on the current
// goroutine instead of waiting for the next tick. Callers send it via
// prog.Send when they need to flush a state change into the view
// before ctx is cancelled.
type ForceSnapshotMsg struct{}

func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m runModel) Init() tea.Cmd {
	return tick()
}

// initLogViewport is the test-time shortcut for what would otherwise
// happen on the first WindowSizeMsg + tick: size the viewport and
// seed content from the reader. Tests can call this so assertions
// against YOffset after scrolls are meaningful without driving the
// full bubbletea event loop.
func (m runModel) initLogViewport(width int) runModel {
	if m.ctx.LogReader == nil {
		return m
	}
	if !m.vpInit {
		m.vp = newLogViewport()
	}
	if width <= 0 {
		width = 80
	}
	m.vp.SetWidth(width)
	m.vp.SetHeight(LogPanelHeight)
	m.vpInit = true
	return m.refreshLogViewport()
}

func (m runModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tea.MouseWheelMsg:
		return m.handleMouseWheel(msg)
	case tea.WindowSizeMsg:
		return m.handleWindowSize(msg), nil
	case tickMsg:
		return m.handleTick()
	case ForceSnapshotMsg:
		if m.ctx.Snapshot != nil {
			m.snap = m.ctx.Snapshot()
		}
		return m, nil
	case finalTickMsg:
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

// handleWindowSize sizes the log viewport to fill the panel box. Width
// stays wide enough for the operator's terminal; height is pinned to
// LogPanelHeight so the panel does not grow on resize.
func (m runModel) handleWindowSize(msg tea.WindowSizeMsg) runModel {
	if m.ctx.LogReader == nil {
		return m
	}
	w := msg.Width - 6 // border + padding
	if w < 10 {
		w = 10
	}
	m.vp.SetWidth(w)
	m.vp.SetHeight(LogPanelHeight)
	m.vpInit = true
	return m
}

func (m runModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.ctx.LogReader != nil {
		if handled, next, cmd := m.routeLogKey(msg); handled {
			return next, cmd
		}
	}
	switch msg.String() {
	case "q", "ctrl+c":
		m.aborted = true
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

// routeLogKey gives the log viewport first shot at the scroll keys.
// Returns handled=true when the key belongs to the panel so the outer
// switch does not also try to match it. Any manual scroll clears the
// sticky flag; `s` toggles sticky ↔ paused.
func (m runModel) routeLogKey(msg tea.KeyPressMsg) (bool, runModel, tea.Cmd) {
	switch msg.String() {
	case "s":
		m = m.toggleSticky()
		return true, m, nil
	case "home":
		m.vp.GotoTop()
		m.sticky = false
		return true, m, nil
	case "end":
		m.vp.GotoBottom()
		m.sticky = true
		return true, m, nil
	case "up", "down", "pgup", "pgdown":
		next, cmd := m.scrollViewport(msg)
		return true, next, cmd
	}
	return false, m, nil
}

// scrollViewport forwards a scroll key to the viewport and clears the
// sticky flag if the offset actually moved. Offset-unchanged keys
// (e.g. `up` at top) leave sticky as-is so a paused operator staying
// put does not accidentally re-enter auto-tail. Propagates any tea.Cmd
// the viewport returns so future bubbles/v2 animations (scrollbar
// fade, etc.) are not silently dropped.
func (m runModel) scrollViewport(msg tea.KeyPressMsg) (runModel, tea.Cmd) {
	before := m.vp.YOffset()
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	if m.vp.YOffset() != before {
		m.sticky = false
	}
	return m, cmd
}

// toggleSticky flips auto-tail. paused → sticky jumps to the bottom so
// the operator catches up to the latest line in one keypress.
func (m runModel) toggleSticky() runModel {
	m.sticky = !m.sticky
	if m.sticky {
		m.vp.GotoBottom()
	}
	return m
}

// handleMouseWheel forwards wheel events to the viewport when the log
// panel is mounted. Scrolling clears sticky so new lines do not yank
// the operator back to the bottom.
func (m runModel) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.ctx.LogReader == nil {
		return m, nil
	}
	before := m.vp.YOffset()
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	if m.vp.YOffset() != before {
		m.sticky = false
	}
	return m, cmd
}

func (m runModel) handleTick() (tea.Model, tea.Cmd) {
	if m.ctx.Snapshot != nil {
		m.snap = m.ctx.Snapshot()
	}
	m = m.refreshLogViewport()
	if m.snap.Done {
		// Render the terminal state (X/X → %) once, then quit on the
		// next tick. Calling tea.Quit here would kill the render
		// loop before View() could produce the final frame.
		return m, tea.Tick(tickInterval, func(t time.Time) tea.Msg { return finalTickMsg(t) })
	}
	return m, tick()
}

// refreshLogViewport copies the latest ring-buffer contents into the
// viewport. When sticky (auto-tail on), we jump to the bottom after
// SetContent so the newest line is always visible. When paused, the
// operator's YOffset is preserved.
func (m runModel) refreshLogViewport() runModel {
	if m.ctx.LogReader == nil {
		return m
	}
	lines := m.ctx.LogReader.Lines(0)
	m.vp.SetContent(strings.Join(lines, "\n"))
	if m.sticky {
		m.vp.GotoBottom()
	}
	return m
}

func (m runModel) View() tea.View {
	// On user abort, blank the TUI and let completion (or lack
	// thereof) take over. On natural completion, keep rendering the
	// final frame so the user sees the terminal X/X + % snapshot
	// before the completion summary prints below it.
	if m.quitting && m.aborted {
		return tea.NewView("")
	}
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")
	b.WriteString(m.renderStartup())
	b.WriteString("\n")
	b.WriteString(m.renderExperiments())
	b.WriteString("\n")
	if m.ctx.LogReader != nil {
		b.WriteString(m.renderLogPanel())
		b.WriteString("\n")
	}
	b.WriteString(m.renderScore())
	b.WriteString("\n\n")
	b.WriteString(m.renderFooter())
	v := tea.NewView(b.String())
	// Verbose mode mounts the scrollable log panel. Inline rendering
	// cannot cleanly overwrite a frame whose row widths change as the
	// ring buffer fills — empty rows are narrow, log-line rows are
	// wide, and bubbletea's inline diff leaves the leftover glyphs in
	// place. Alt-screen guarantees a full clear between frames so the
	// panel, experiments box, and score bar stay aligned. Enable
	// mouse cell motion at the same time so `viewport.MouseWheelEnabled`
	// actually receives wheel events.
	if m.ctx.LogReader != nil {
		v.AltScreen = true
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

func (m runModel) renderHeader() string {
	line := fmt.Sprintf("ruptor  run  •  %s", m.ctx.ConfigFile)
	return styleBox.Render(styleHeader.Render(line))
}

func (m runModel) renderStartup() string {
	var b strings.Builder
	if m.ctx.Port != 0 {
		b.WriteString(styleSubtle.Render("▸ "))
		b.WriteString(styleInfo.Render(fmt.Sprintf("Proxy started on :%d", m.ctx.Port)))
		b.WriteString("\n")
	}
	if m.ctx.Entrypoint != "" {
		b.WriteString(styleSubtle.Render("▸ "))
		b.WriteString(styleInfo.Render("Agent: " + m.ctx.Entrypoint))
		b.WriteString("\n")
	}
	return b.String()
}

func (m runModel) renderExperiments() string {
	done := countDone(m.snap.Experiments)
	total := len(m.snap.Experiments)
	label := fmt.Sprintf(" Experiments %d/%d ", done, total)

	var lines []string
	for _, e := range m.snap.Experiments {
		lines = append(lines, renderExperimentLine(e))
	}

	body := strings.Join(lines, "\n")
	title := lipgloss.NewStyle().Foreground(Theme.Muted).Render(label)

	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(Theme.Border).
		Padding(0, 2)

	return box.Render(title + "\n" + body)
}

// renderLogPanel mounts the verbose-mode log panel between the
// Experiments box and the Robustness bar. Layer 2 (this file): the
// body is a bubbles/v2 viewport with scroll + pause-tail support.
// The bordered box still matches the Experiments style so the layout
// is stable.
func (m runModel) renderLogPanel() string {
	title := lipgloss.NewStyle().Foreground(Theme.Muted).Render(m.logPanelTitle())
	body := m.logPanelBody()
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(Theme.Border).
		Padding(0, 2)
	return box.Render(title + "\n" + body)
}

// logPanelTitle reflects the current auto-tail state so the operator
// always knows whether new lines are pushing the view.
func (m runModel) logPanelTitle() string {
	if m.sticky {
		return " Logs · tail · -v "
	}
	return " Logs · paused (s) · -v "
}

// logPanelBody returns the pre-viewport-initialised fallback (a
// padded tail render) or the viewport's own View once it has been
// sized. The fallback keeps the first-frame render legible even
// before the first WindowSizeMsg arrives.
func (m runModel) logPanelBody() string {
	if !m.vpInit {
		return paddedTail(m.ctx.LogReader.Lines(LogPanelHeight), LogPanelHeight)
	}
	return m.vp.View()
}

// paddedTail joins `lines` with newlines and tops up to height rows
// so the box height is stable before the viewport owns rendering.
func paddedTail(lines []string, height int) string {
	body := make([]string, 0, height)
	body = append(body, lines...)
	for len(body) < height {
		body = append(body, "")
	}
	return strings.Join(body, "\n")
}

func renderExperimentLine(e ExperimentState) string {
	icon, col := iconFor(e.Status)
	status := iconStyle(col).Render(icon + " ")
	id := styleInfo.Render(padRight(e.ID, 24))
	dur := styleMuted.Render(padRight(formatDuration(e.Duration), 8))
	state := styleStatusLabel(e.Status).Render(statusLabel(e.Status))
	return status + id + dur + state
}

func iconFor(s ExperimentStatus) (string, color.Color) {
	switch s {
	case StatusPassed:
		return IconPassed, Theme.Success
	case StatusFailed:
		return IconFailed, Theme.Failure
	case StatusRunning:
		return IconRunning, Theme.Warning
	default:
		return IconPending, Theme.Muted
	}
}

func iconStyle(c color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c).Bold(true)
}

func statusLabel(s ExperimentStatus) string {
	switch s {
	case StatusPassed:
		return "PASSED"
	case StatusFailed:
		return "FAILED"
	case StatusRunning:
		return "running..."
	default:
		return "pending"
	}
}

func styleStatusLabel(s ExperimentStatus) lipgloss.Style {
	switch s {
	case StatusPassed:
		return styleSuccess
	case StatusFailed:
		return styleFailure
	case StatusRunning:
		return styleWarning
	default:
		return styleMuted
	}
}

func countDone(exps []ExperimentState) int {
	n := 0
	for _, e := range exps {
		if e.Status == StatusPassed || e.Status == StatusFailed {
			n++
		}
	}
	return n
}

func (m runModel) renderScore() string {
	label := styleInfo.Render("Robustness ")
	bar := ProgressBar(m.snap.ScorePercent, 20)
	total := len(m.snap.Experiments)
	done := countDone(m.snap.Experiments)
	if total > 0 && done < total {
		// Mid-run: a percentage derived from a partial tally is
		// misleading. Show progress as X/Y instead, reserving the
		// percentage for the final frame.
		progress := lipgloss.NewStyle().
			Foreground(Theme.Muted).
			Render(fmt.Sprintf(" %d/%d complete", done, total))
		return "  " + label + bar + progress
	}
	pct := lipgloss.NewStyle().
		Foreground(scoreColor(m.snap.ScorePercent)).
		Bold(true).
		Render(fmt.Sprintf(" %d%%", m.snap.ScorePercent))
	return "  " + label + bar + pct
}

func (m runModel) renderFooter() string {
	return styleMuted.Render("  Press q to abort  •  r to retry failed")
}

// ProgressBar renders a unicode bar. Exported so the completion screen
// can reuse it.
func ProgressBar(percent, width int) string {
	if width <= 0 {
		width = 20
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := (percent * width) / 100
	full := strings.Repeat("█", filled)
	empty := strings.Repeat("░", width-filled)
	col := scoreColor(percent)
	filledStyle := lipgloss.NewStyle().Foreground(col)
	emptyStyle := lipgloss.NewStyle().Foreground(Theme.Border)
	return filledStyle.Render(full) + emptyStyle.Render(empty)
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func formatDuration(d time.Duration) string {
	if d == 0 {
		return "—"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}
