package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme holds the Ruptor palette. Values come from CLAUDE.md § UI.
// lipgloss v2 switched from a Color string type to image/color.Color
// values — lipgloss.Color("#E84545") is now a constructor function.
var Theme = struct {
	Primary color.Color // #E84545 — Ruptor red (fractura)
	Success color.Color // #22C55E
	Failure color.Color // #EF4444
	Warning color.Color // #F59E0B
	Muted   color.Color // #6B7280
	Border  color.Color // #374151
	Text    color.Color // #F9FAFB
	Subtle  color.Color // #9CA3AF
}{
	Primary: lipgloss.Color("#E84545"),
	Success: lipgloss.Color("#22C55E"),
	Failure: lipgloss.Color("#EF4444"),
	Warning: lipgloss.Color("#F59E0B"),
	Muted:   lipgloss.Color("#6B7280"),
	Border:  lipgloss.Color("#374151"),
	Text:    lipgloss.Color("#F9FAFB"),
	Subtle:  lipgloss.Color("#9CA3AF"),
}

// Reusable styles. Built at init so the allocation cost is paid once.
var (
	styleHeader = lipgloss.NewStyle().
			Foreground(Theme.Text).
			Bold(true)

	styleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Theme.Border).
			Padding(0, 2)

	styleSuccess = lipgloss.NewStyle().Foreground(Theme.Success).Bold(true)
	styleFailure = lipgloss.NewStyle().Foreground(Theme.Failure).Bold(true)
	styleWarning = lipgloss.NewStyle().Foreground(Theme.Warning).Bold(true)
	styleInfo    = lipgloss.NewStyle().Foreground(Theme.Text)
	styleMuted   = lipgloss.NewStyle().Foreground(Theme.Muted)
	styleSubtle  = lipgloss.NewStyle().Foreground(Theme.Subtle)
	stylePrimary = lipgloss.NewStyle().Foreground(Theme.Primary).Bold(true)
)

// Icon sigils for experiment status in the TUI list.
const (
	IconPassed  = "✓"
	IconFailed  = "✗"
	IconRunning = "◌"
	IconPending = "○"
)

// scoreColor picks a foreground for a robustness score percentage per
// SKILL-ui.md: ≥80 green, 50–79 yellow, <50 red.
func scoreColor(percent int) color.Color {
	switch {
	case percent >= 80:
		return Theme.Success
	case percent >= 50:
		return Theme.Warning
	default:
		return Theme.Failure
	}
}
