package ui

import "fmt"

// Success prints a green check-marked line. Use for completed actions.
func Success(msg string) {
	fmt.Fprintln(out, styleSuccess.Render("✓ ")+styleInfo.Render(msg))
}

// Error prints a red ✗ line. Tell the user what to do next.
// Bad:  "authentication error"
// Good: "Session expired. Run `ruptor auth login` to reconnect."
func Error(msg string) {
	fmt.Fprintln(out, styleFailure.Render("✗ ")+styleInfo.Render(msg))
}

// Info prints a plain line with a subtle ▸ prefix.
func Info(msg string) {
	fmt.Fprintln(out, styleSubtle.Render("▸ ")+styleInfo.Render(msg))
}

// Warning prints a yellow warning line.
func Warning(msg string) {
	fmt.Fprintln(out, styleWarning.Render("⚠ ")+styleInfo.Render(msg))
}

// Dim prints a muted informational line (e.g. "Checking for updates...").
func Dim(msg string) {
	fmt.Fprintln(out, styleMuted.Render(msg))
}

// VersionBanner prints the new-version footer described in SKILL-ui.md.
// Never called at startup — only at the end of a command when a newer
// version is available. Non-blocking.
func VersionBanner(current, available string) {
	if available == "" || available == current {
		return
	}
	line := fmt.Sprintf("⚡ ruptor %s available  →  ruptor update", available)
	sep := styleMuted.Render("─────────────────────────────────────────────")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, sep)
	fmt.Fprintln(out, "  "+stylePrimary.Render(line))
	fmt.Fprintln(out, sep)
}
