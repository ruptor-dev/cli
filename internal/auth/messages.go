package auth

import "github.com/ruptor-dev/cli/internal/ui"

// init wires each sentinel error to its user-facing message so
// callers can do `msg, _ := ui.AuthMessage(err)` without duplicating
// the copy.
func init() {
	ui.RegisterAuthMessage(ErrTokenExpired, "Session expired. Run `ruptor auth login` to reconnect.")
	ui.RegisterAuthMessage(ErrTokenRevoked, "This token was revoked. Run `ruptor auth login` to get a new one.")
	ui.RegisterAuthMessage(ErrPlanInactive, "Your Ruptor plan is inactive. Reactivate at https://ruptor.dev/billing")
	ui.RegisterAuthMessage(ErrPlanDowngrade, "Your plan changed. Run `ruptor auth login` to refresh your entitlements.")
	ui.RegisterAuthMessage(ErrDeviceTimeout, "Login timed out. Run `ruptor auth login` again when you're ready.")
	ui.RegisterAuthMessage(ErrDeviceDenied, "Login denied in browser. Run `ruptor auth login` to try again.")
	ui.RegisterAuthMessage(ErrNotAuthenticated, "Not logged in. Run `ruptor auth login` to connect.")
	ui.RegisterAuthMessage(ErrBadConfigPerms, "~/.ruptor/config.yaml has insecure permissions. Run `chmod 600` and try again.")
}
