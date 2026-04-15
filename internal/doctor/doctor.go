// Package doctor runs preflight diagnostics against a user's local
// install and prints an actionable report. The output is the canonical
// thing users paste into a bug report — every check must either say
// "ok" or give a single concrete next step. No prose, no stack traces,
// no secret values.
//
// All external collaborators (HTTP, filesystem, runtime version,
// network port) are injected through the Options struct so the checks
// stay deterministic under test.
package doctor

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ruptor-dev/cli/internal/auth"
	"github.com/ruptor-dev/cli/internal/cloud"
)

// Status is the outcome of a single check.
type Status int

const (
	// StatusOK: check passed. No action required.
	StatusOK Status = iota
	// StatusWarn: check passed but a soft issue is worth flagging
	// (e.g. token approaching expiry). Does not change exit code.
	StatusWarn
	// StatusFail: check failed. Hint must contain the fix.
	StatusFail
)

// Result is one row in the doctor report.
type Result struct {
	Name    string
	Status  Status
	Message string // short factual outcome ("Go 1.26.2", "port 8080 free")
	Hint    string // fix on Fail/Warn; empty on OK
}

// minGoVersion is the lowest Go runtime the doctor accepts. Matches
// CLAUDE.md stack table — the build itself enforces this via go.mod,
// the doctor surfaces a friendly hint when a user has stale toolchain.
const minGoVersion = "go1.26.0"

// Options bundles the injectable dependencies. A zero value is
// usable (uses defaults) but callers normally fill in HTTPClient and
// ConfigDir from internal/config.Settings.
type Options struct {
	// HTTPClient hits the cloud API and the GitHub releases feed.
	// nil means use a TLS-enforced default with a 5s timeout.
	HTTPClient *http.Client
	// CloudURL overrides cloud.DefaultAPIURL. Useful for tests.
	CloudURL string
	// ConfigDir is ~/.ruptor — the doctor checks for config.yaml in
	// here. Empty means resolve via os.UserHomeDir at run time.
	ConfigDir string
	// Port is the proxy port to probe. 0 falls back to 8080.
	Port int
	// GoVersion is the runtime version string to evaluate. Empty
	// means runtime.Version().
	GoVersion string
}

// Run executes every check in a fixed order and returns the results.
// Order matters: users read top-to-bottom and we want environment
// before network before auth.
func Run(ctx context.Context, opts Options) []Result {
	opts = withDefaults(opts)
	return []Result{
		checkGoVersion(opts.GoVersion),
		checkConfigPerms(opts.ConfigDir),
		checkPort(opts.Port),
		checkNetwork(ctx, opts.HTTPClient, opts.CloudURL),
		checkAuth(opts.ConfigDir),
	}
}

// HasFailures returns true when any result is StatusFail. Callers use
// this to set the process exit code; warnings do not.
func HasFailures(results []Result) bool {
	for _, r := range results {
		if r.Status == StatusFail {
			return true
		}
	}
	return false
}

func withDefaults(o Options) Options {
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		}
	}
	if o.CloudURL == "" {
		o.CloudURL = cloud.DefaultAPIURL
	}
	if o.Port == 0 {
		o.Port = 8080
	}
	if o.GoVersion == "" {
		o.GoVersion = runtime.Version()
	}
	if o.ConfigDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			o.ConfigDir = filepath.Join(home, ".ruptor")
		}
	}
	return o
}

// checkGoVersion compares the build-time runtime version against
// minGoVersion. A non-standard version string (devel build, custom
// toolchain) downgrades to a warning rather than a hard failure —
// the user clearly knows what they're doing.
func checkGoVersion(have string) Result {
	r := Result{Name: "Go runtime"}
	cmp, ok := compareGoVersion(have, minGoVersion)
	switch {
	case !ok:
		r.Status = StatusWarn
		r.Message = have
		r.Hint = "could not parse Go version; ensure you build with " + minGoVersion + " or newer"
	case cmp < 0:
		r.Status = StatusFail
		r.Message = have
		r.Hint = "upgrade Go to " + minGoVersion + " or newer (https://go.dev/dl/)"
	default:
		r.Status = StatusOK
		r.Message = have
	}
	return r
}

// compareGoVersion returns -1/0/1 like strings.Compare for the leading
// X.Y.Z component of two "goX.Y.Z" strings. The "ok" return is false
// when either string cannot be parsed (devel builds, gccgo, etc.).
func compareGoVersion(a, b string) (int, bool) {
	pa, ok := parseGoVersion(a)
	if !ok {
		return 0, false
	}
	pb, ok := parseGoVersion(b)
	if !ok {
		return 0, false
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			if pa[i] < pb[i] {
				return -1, true
			}
			return 1, true
		}
	}
	return 0, true
}

func parseGoVersion(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimPrefix(s, "go")
	// Strip any toolchain suffix ("go1.26.2 X:fieldtrack").
	if idx := strings.IndexAny(s, " +"); idx >= 0 {
		s = s[:idx]
	}
	parts := strings.SplitN(s, ".", 3)
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// checkConfigPerms verifies ~/.ruptor/config.yaml is 0600 if it
// exists. A missing file is OK — the user simply has not run
// `ruptor auth login` yet, and the auth check below will say so.
func checkConfigPerms(configDir string) Result {
	r := Result{Name: "Config permissions"}
	if configDir == "" {
		r.Status = StatusWarn
		r.Message = "no home directory"
		r.Hint = "set $HOME so ruptor can locate ~/.ruptor/config.yaml"
		return r
	}
	path := filepath.Join(configDir, "config.yaml")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		r.Status = StatusOK
		r.Message = "no config file yet (" + path + ")"
		return r
	}
	if err != nil {
		r.Status = StatusFail
		r.Message = err.Error()
		r.Hint = "fix permissions on " + path
		return r
	}
	if info.Mode().Perm()&0o077 != 0 {
		r.Status = StatusFail
		r.Message = fmt.Sprintf("%s is %o", path, info.Mode().Perm())
		r.Hint = "run: chmod 600 " + path
		return r
	}
	r.Status = StatusOK
	r.Message = path + " (0600)"
	return r
}

// checkPort tries to bind 127.0.0.1:<port>. A successful bind is
// closed immediately. EADDRINUSE → fail with the auto-detect hint;
// any other error → warn (could be a sandboxed CI with no loopback).
func checkPort(port int) Result {
	r := Result{Name: "Proxy port"}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	l, err := net.Listen("tcp", addr)
	if err == nil {
		_ = l.Close()
		r.Status = StatusOK
		r.Message = fmt.Sprintf("port %d free", port)
		return r
	}
	if isAddrInUse(err) {
		r.Status = StatusFail
		r.Message = fmt.Sprintf("port %d in use", port)
		r.Hint = "stop the listener, set RUPTOR_PROXY_PORT=<other>, or rely on auto-detect (ADR-008)"
		return r
	}
	r.Status = StatusWarn
	r.Message = err.Error()
	r.Hint = "check that ruptor can bind a loopback TCP port"
	return r
}

func isAddrInUse(err error) bool {
	return strings.Contains(err.Error(), "address already in use")
}

// checkNetwork issues a GET against cloudURL. The platform may not
// expose a /healthz yet, so any 2xx/3xx/4xx response counts as
// "reachable" — only network errors fail. We never report 5xx as a
// hard failure because that says nothing about the user's machine.
func checkNetwork(ctx context.Context, hc *http.Client, cloudURL string) Result {
	r := Result{Name: "Cloud reachability"}
	if !cloud.CloudReportingEnabled {
		r.Status = StatusWarn
		r.Message = "coming soon"
		return r
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cloudURL, nil)
	if err != nil {
		r.Status = StatusFail
		r.Message = err.Error()
		r.Hint = "verify the cloud URL in ~/.ruptor/config.yaml"
		return r
	}
	resp, err := hc.Do(req)
	if err != nil {
		r.Status = StatusFail
		r.Message = err.Error()
		r.Hint = "check connectivity to " + cloudURL + " (proxy/firewall/DNS)"
		return r
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 500 {
		r.Status = StatusWarn
		r.Message = fmt.Sprintf("%s returned %d", cloudURL, resp.StatusCode)
		r.Hint = "platform may be having an incident — try again shortly"
		return r
	}
	r.Status = StatusOK
	r.Message = fmt.Sprintf("%s reachable (%d)", cloudURL, resp.StatusCode)
	return r
}

// checkAuth reads the local store and parses the JWT. A missing
// token is reported as a warning (you can still run local-only
// chaos), an expired token is a failure (with the relogin hint).
// The full token is never printed — only MaskedSuffix.
func checkAuth(configDir string) Result {
	r := Result{Name: "Authentication"}
	if !cloud.CloudReportingEnabled {
		r.Status = StatusWarn
		r.Message = "coming soon"
		return r
	}
	path := filepath.Join(configDir, "config.yaml")
	store, err := auth.LoadFrom(path)
	if errors.Is(err, auth.ErrNotAuthenticated) {
		r.Status = StatusWarn
		r.Message = "not logged in"
		r.Hint = "run: ruptor auth login"
		return r
	}
	if err != nil {
		r.Status = StatusFail
		r.Message = err.Error()
		r.Hint = "remove " + path + " and run: ruptor auth login"
		return r
	}
	tok, err := auth.Parse(store.Token, auth.PublicKeyPEM)
	if errors.Is(err, auth.ErrTokenExpired) {
		r.Status = StatusFail
		r.Message = "token expired"
		r.Hint = "run: ruptor auth login"
		return r
	}
	if err != nil {
		r.Status = StatusFail
		r.Message = err.Error()
		r.Hint = "run: ruptor auth login"
		return r
	}
	if tok.NeedsRefresh() {
		r.Status = StatusWarn
		r.Message = fmt.Sprintf("token %s expires soon", tok.MaskedSuffix())
		r.Hint = "any next ruptor command will silently rotate it"
		return r
	}
	r.Status = StatusOK
	r.Message = fmt.Sprintf("%s (%s)", tok.MaskedSuffix(), tok.Claims.UserEmail)
	return r
}
