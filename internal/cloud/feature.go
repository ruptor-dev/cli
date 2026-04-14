// Package cloud holds the feature flag and endpoint constants for the
// platform control plane. The actual reporting client (POST /v1/runs
// with idempotency + backoff) lands when CloudReportingEnabled flips
// to true; for v1 it stays false and --cloud shows the waitlist
// message.
//
// Auth code imports this package for the endpoint constants only —
// the feature flag gates report uploads, not the login command.
package cloud

import "time"

// CloudReportingEnabled gates whether --cloud sends run reports to
// ruptor.dev. Set to true in the release that ships the reporting
// client. See ADR-006.
const CloudReportingEnabled = false

// Endpoint constants. A viper-driven override in
// ~/.ruptor/config.yaml (cloud.endpoint) replaces these at runtime
// when set; the constants are the shipped default.
const (
	// DefaultAPIURL is the platform API for auth + run ingestion.
	DefaultAPIURL = "https://api.ruptor.dev"
	// DeviceCodePath is the OAuth 2.0 device-authorization endpoint.
	// RFC 8628 naming; our implementation lives under /v1/auth/.
	DeviceCodePath = "/v1/auth/device/code"
	// DeviceTokenPath is the polling endpoint the CLI hits until the
	// user authorizes in the browser.
	DeviceTokenPath = "/v1/auth/device/token"
	// RefreshPath rotates a still-valid token before expiry.
	RefreshPath = "/v1/auth/refresh"
	// RevokePath invalidates a token at logout.
	RevokePath = "/v1/auth/revoke"

	// DefaultHTTPTimeout caps any single request to the platform
	// so a hung server cannot lock a user's shell. Matches the
	// simulate module's convention.
	DefaultHTTPTimeout = 30 * time.Second
)
