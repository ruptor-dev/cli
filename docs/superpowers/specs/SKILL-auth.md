# SKILL: Ruptor Auth — OAuth Device Flow + Token Management

Use this skill when working on auth-related code.

## Flow overview

```
ruptor auth login
  → CLI generates device_code + prints URL
  → User opens URL in browser, logs in with Clerk
  → CLI polls /v1/auth/device/token every 5s (timeout: 5 min)
  → Platform returns JWT on successful auth
  → CLI saves JWT to ~/.ruptor/config.yaml (perms: 0600)
  → CLI shows: "✓ Authenticated as user@example.com (Starter plan)"
```

## Token structure (JWT claims)

```json
{
  "tenant_id": "ten_xxxx",
  "user_email": "user@example.com",
  "plan": "starter",
  "token_id": "tok_xxxx",
  "token_type": "personal",
  "expires_at": 1234567890,
  "iat": 1234567890
}
```

token_type: "personal" (90-day TTL, autorotate) | "ci" (long-lived, no autorotate)

## Config file format

```yaml
# ~/.ruptor/config.yaml
# Created by: ruptor auth login
# Permissions: 0600 — do not share this file

token: eyJhbGc...
cloud:
  endpoint: https://api.ruptor.dev
  timeout: 30s
telemetry:
  enabled: true
  endpoint: https://telemetry.ruptor.dev
defaults:
  report: local        # local | cloud (cloud disabled until CloudReportingEnabled=true)
  output: both         # json | html | both
```

## Token validation (local, no round trip)

The CLI validates the JWT signature locally on every operation.
Public key is embedded in the binary (hardcoded, rotated via binary update).
No network call needed to check if token is valid — fast startup.

## Silent refresh

Before any operation that touches the cloud:
1. Check token.ExpiresAt
2. If time.Until(ExpiresAt) < 7 days → refresh in background goroutine
3. Continue with current operation (don't wait for refresh)
4. On next operation, the new token is used

If refresh fails: log at debug level, continue with current token.
If current token is expired: return clear error, prompt to re-login.

## Error messages

```go
// All in internal/auth/errors.go

var (
    ErrTokenExpired  = errors.New("session expired")
    ErrTokenRevoked  = errors.New("token revoked")
    ErrPlanInactive  = errors.New("plan inactive")
    ErrPlanDowngrade = errors.New("plan changed")
)

// Display (in internal/ui/)
func ShowAuthError(err error) {
    switch {
    case errors.Is(err, ErrTokenExpired):
        ui.Error("Session expired. Run `ruptor auth login` to reconnect.")
    case errors.Is(err, ErrTokenRevoked):
        ui.Error("This token was revoked. Run `ruptor auth login` to get a new one.")
    case errors.Is(err, ErrPlanInactive):
        ui.Error("Your Ruptor plan is inactive.\n  Reactivate at https://ruptor.dev/billing")
    }
}
```

## Security checklist for auth code

- Token stored with os.WriteFile(..., 0600) — verify this explicitly
- Token never logged — only last 4 chars in debug output: token[len(token)-4:]
- Token never in URL params — only in Authorization header
- HTTP client enforces TLS: tls.Config{InsecureSkipVerify: false}

## ruptor auth status output

```
  Authentication
  ──────────────────────────────────
  User     user@example.com
  Plan     Starter
  Expires  in 45 days  (Jun 28, 2026)
  Token    ...xxxx  (personal)
  Cloud    https://api.ruptor.dev
```

## Testing auth code

Use a test JWT signed with a test private key (in testdata/).
Never use real tokens in tests.
Mock the HTTP polling endpoint with httptest.NewServer.
Test: successful flow, timeout flow, revoked token, expired token.
