// Package ui — runprogress.go: the live Bubbletea program shown during
// `ruptor run`. Layout per SKILL-ui.md: header box, experiment list,
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
}

// runModel is the Bubbletea model driving the live TUI.
type runModel struct {
	ctx      RunContext
	snap     RunSnapshot
	quitting bool
	aborted  bool
}

// NewRunProgress builds a Bubbletea program for the live run view.
// Caller invokes Run() to block until the user quits or the snapshot
// returns Done=true.
func NewRunProgress(ctx RunContext) *tea.Program {
	m := runModel{ctx: ctx}
	return tea.NewProgram(m)
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

func tick() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m runModel) Init() tea.Cmd {
	return tick()
}

func (m runModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case tickMsg:
		return m.handleTick()
	}
	return m, nil
}

func (m runModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		m.aborted = true
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m runModel) handleTick() (tea.Model, tea.Cmd) {
	if m.ctx.Snapshot != nil {
		m.snap = m.ctx.Snapshot()
	}
	if m.snap.Done {
		m.quitting = true
		return m, tea.Quit
	}
	return m, tick()
}

func (m runModel) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	var b strings.Builder
	b.WriteString(m.renderHeader())
	b.WriteString("\n\n")
	b.WriteString(m.renderStartup())
	b.WriteString("\n")
	b.WriteString(m.renderExperiments())
	b.WriteString("\n")
	b.WriteString(m.renderScore())
	b.WriteString("\n\n")
	b.WriteString(m.renderFooter())
	return tea.NewView(b.String())
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
