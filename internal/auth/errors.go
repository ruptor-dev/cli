package auth

import "errors"

// Sentinel errors exposed to the rest of the codebase. Callers should
// match with errors.Is so that deeper wrappers (fmt.Errorf("%w")) still
// route to the right UI message.
var (
	// ErrTokenExpired: the stored token's expires_at is in the past.
	// UI: "Session expired. Run `ruptor auth login` to reconnect."
	ErrTokenExpired = errors.New("auth: session expired")

	// ErrTokenRevoked: the platform reported the token as revoked
	// during refresh or run upload.
	// UI: "This token was revoked. Run `ruptor auth login`..."
	ErrTokenRevoked = errors.New("auth: token revoked")

	// ErrPlanInactive: the tenant's plan is paused / not active.
	// UI: "Your Ruptor plan is inactive. Reactivate at …/billing"
	ErrPlanInactive = errors.New("auth: plan inactive")

	// ErrPlanDowngrade: the plan changed between login and the current
	// operation; the user should re-authorize to pick up new
	// entitlements.
	ErrPlanDowngrade = errors.New("auth: plan changed")

	// ErrDeviceTimeout: the user did not complete login in the allotted
	// window (5 minutes per SKILL-auth.md).
	ErrDeviceTimeout = errors.New("auth: login timed out; run `ruptor auth login` again")

	// ErrDeviceDenied: the user pressed "Deny" in the browser flow.
	ErrDeviceDenied = errors.New("auth: login denied in browser")

	// ErrNotAuthenticated: no token on disk yet.
	ErrNotAuthenticated = errors.New("auth: not logged in; run `ruptor auth login`")

	// ErrBadConfigPerms: the config file exists but is readable by
	// group or others. Refuse to read until the user tightens the
	// permissions.
	ErrBadConfigPerms = errors.New("auth: config file permissions too open (must be 0600)")
)
