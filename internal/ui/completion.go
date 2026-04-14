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
}

// RenderCompletion builds the boxed completion screen from SKILL-ui.md.
// Returned as a string so the caller decides where to write it (test or
// real stdout).
func RenderCompletion(s CompletionSummary) string {
	bar := ProgressBar(s.ScorePercent, 28)
	pct := lipgloss.NewStyle().
		Foreground(scoreColor(s.ScorePercent)).
		Bold(true).
		Render(fmt.Sprintf(" %d%%", s.ScorePercent))

	summary := fmt.Sprintf("%d passed  •  %d failed  •  %d error", s.Passed, s.Failed, s.Errored)

	var inner strings.Builder
	inner.WriteString("\n")
	inner.WriteString("  " + stylePrimary.Render("Robustness Score"))
	inner.WriteString("\n\n")
	inner.WriteString("  " + bar + pct)
	inner.WriteString("\n\n")
	inner.WriteString("  " + styleInfo.Render(summary))
	inner.WriteString("\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(Theme.Border).
		Width(48).
		Render(inner.String())

	var b strings.Builder
	b.WriteString(box)
	b.WriteString("\n\n")
	for _, p := range s.ReportPaths {
		b.WriteString("  " + styleSuccess.Render("✓ Report saved  →  ") + styleInfo.Render(p) + "\n")
	}
	return b.String()
}

// PrintCompletion writes the rendered completion screen to the active
// ui output writer.
func PrintCompletion(s CompletionSummary) {
	fmt.Fprint(out, RenderCompletion(s))
}
