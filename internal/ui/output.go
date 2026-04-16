package ui

import "fmt"

// Success prints a green check-marked line to stderr. Use for completed
// actions. Routed to stderr because Success is a status confirmation,
// not pipeable data — audit of call sites (auth login/logout, doctor
// check rows, sync per-file acknowledgements, update up-to-date) shows
// every caller uses it diagnostically.
func Success(msg string) {
	fmt.Fprintln(errOut, styleSuccess.Render("✓ ")+styleInfo.Render(msg))
}

// Error prints a red ✗ line to stderr. Tell the user what to do next.
// Bad:  "authentication error"
// Good: "Session expired. Run `ruptor auth login` to reconnect."
func Error(msg string) {
	fmt.Fprintln(errOut, styleFailure.Render("✗ ")+styleInfo.Render(msg))
}

// Info prints a plain line with a subtle ▸ prefix to stderr. Info is
// status-shaped (auth URLs, waitlist notices, sync counts), never
// pipeable data — routed to stderr so `ruptor ... | jq` stays clean.
func Info(msg string) {
	fmt.Fprintln(errOut, styleSubtle.Render("▸ ")+styleInfo.Render(msg))
}

// Warning prints a yellow warning line to stderr.
func Warning(msg string) {
	fmt.Fprintln(errOut, styleWarning.Render("⚠ ")+styleInfo.Render(msg))
}

// Dim prints a muted informational line to stderr (e.g. "Checking for
// updates...").
func Dim(msg string) {
	fmt.Fprintln(errOut, styleMuted.Render(msg))
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

// VersionBanner prints the new-version footer described in SKILL-ui.md
// to stderr. Never called at startup — only at the end of a command
// when a newer version is available. Non-blocking. Routed to stderr
// because it is a diagnostic notification appended after the command's
// real output, not part of the pipeable data stream.
func VersionBanner(current, available string) {
	if available == "" || available == current {
		return
	}
	line := fmt.Sprintf("⚡ ruptor %s available  →  ruptor update", available)
	sep := styleMuted.Render("─────────────────────────────────────────────")
	fmt.Fprintln(errOut, "")
	fmt.Fprintln(errOut, sep)
	fmt.Fprintln(errOut, "  "+stylePrimary.Render(line))
	fmt.Fprintln(errOut, sep)
}
