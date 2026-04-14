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

// AuthMessage maps an auth error to a user-facing line that tells the
// user what to do next. Callers match their own errors with errors.Is
// and pass them in; unknown errors are handed back as-is so the
// caller can decide whether to show them.
//
// Kept in ui so the wording is a single source of truth across
// subcommands.
func AuthMessage(err error) (string, bool) {
	msg, known := authMsg(err)
	return msg, known
}

// authMsgMap is populated by internal/auth via RegisterAuthMessage to
// avoid an import cycle between ui and auth.
var authMsgMap = map[error]string{}

// RegisterAuthMessage wires an auth sentinel error to its UI copy.
// internal/auth calls this in an init() block so the strings stay
// colocated with the errors while ui remains the only package that
// renders them.
func RegisterAuthMessage(err error, msg string) {
	authMsgMap[err] = msg
}

func authMsg(err error) (string, bool) {
	for sentinel, msg := range authMsgMap {
		if err == sentinel {
			return msg, true
		}
	}
	return err.Error(), false
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
