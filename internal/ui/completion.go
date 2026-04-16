package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// CompletionSummary is the data driving the completion screen.
type CompletionSummary struct {
	ScorePercent int
	Passed       int
	Failed       int
	Errored      int
	ReportPaths  []string
	// AgentLogDir is the directory holding per-experiment agent
	// stdout/stderr logs. Rendered at the bottom of the completion
	// screen so users can find the forensics artefacts after a run.
	// Empty when no runner was used.
	AgentLogDir string
	// LogPath is the structured ruptor.log written during the run.
	// zerolog is redirected there while the TUI owns the terminal so
	// events do not corrupt the live render. Shown after TUI exit so
	// users know where to look for proxy/orchestrator log events.
	LogPath string
}

// RenderCompletion builds the boxed completion screen from docs/specs/ui.md.
// Returned as a string so the caller decides where to write it (test or
// real stdout).
// RenderCompletion produces the three-line post-run summary:
//
//	✓ Score: X% (P passed · F failed)
//	✓ Report → <path>
//	▸ Run log → <path>
//
// Kept intentionally terse. The HTML report is the source of truth
// for per-experiment details (behaviors, durations, judge output).
func RenderCompletion(s CompletionSummary) string {
	scoreStyle := lipgloss.NewStyle().Foreground(scoreColor(s.ScorePercent)).Bold(true)

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("  " + styleSuccess.Render("✓ ") + styleInfo.Render("Score: "))
	b.WriteString(scoreStyle.Render(fmt.Sprintf("%d%%", s.ScorePercent)))
	b.WriteString(styleMuted.Render(fmt.Sprintf("  (%d passed · %d failed)", s.Passed, s.Failed)))
	b.WriteString("\n")
	for _, p := range s.ReportPaths {
		b.WriteString("  " + styleSuccess.Render("✓ ") + styleInfo.Render("Report  → ") + styleInfo.Render(p) + "\n")
	}
	if s.LogPath != "" {
		b.WriteString("  " + styleInfo.Render("▸ ") + styleInfo.Render("Run log → ") + styleInfo.Render(s.LogPath) + "\n")
	}
	return b.String()
}

// PrintCompletion writes the rendered completion screen to the active
// ui output writer.
func PrintCompletion(s CompletionSummary) {
	fmt.Fprint(out, RenderCompletion(s))
}
