// Package updater queries the GitHub releases feed for the latest
// published ruptor tag and reports whether the running binary is
// behind it. The check is non-blocking by contract — every error
// path returns a Result that the caller can render without a stack
// trace, so a missing network never breaks a `ruptor update` run.
package updater

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultReleasesURL is the GitHub REST endpoint for the latest
// non-prerelease tag of ruptor-dev/cli. Tests inject a httptest
// server URL via Options.URL.
const DefaultReleasesURL = "https://api.github.com/repos/ruptor-dev/cli/releases/latest"

// Status mirrors the three end states of an update check. Callers
// translate to ui.Success/Warning/Info copy.
type Status int

const (
	// StatusUpToDate: current == latest.
	StatusUpToDate Status = iota
	// StatusBehind: latest is strictly newer; UpgradeHint tells the
	// user how to install it.
	StatusBehind
	// StatusUnknown: the check did not complete (network down, rate
	// limit, malformed payload). Never block on this.
	StatusUnknown
)

// Result is the single object the CLI renders.
type Result struct {
	Status      Status
	Current     string // ruptor binary version (input)
	Latest      string // tag returned by the registry; "" on Unknown
	UpgradeHint string // shell command to upgrade; populated on StatusBehind
	Reason      string // populated on StatusUnknown for the muted warning line
}

// Options configures a check. A zero value uses the default URL and
// a TLS-enforced 5s HTTP client.
type Options struct {
	URL        string
	HTTPClient *http.Client
}

// githubReleaseResponse is the subset of the v3 API payload we read.
// The full payload is large; deliberately ignore everything else so
// schema additions don't surface as decode errors.
type githubReleaseResponse struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// Check fetches the latest tag and compares it to current. The
// "current" string comes from main.version (ldflags-injected at
// release time, "0.0.0-dev" otherwise).
func Check(ctx context.Context, current string, opts Options) Result {
	opts = withDefaults(opts)
	r := Result{Current: current}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, opts.URL, nil)
	if err != nil {
		r.Status = StatusUnknown
		r.Reason = err.Error()
		return r
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := opts.HTTPClient.Do(req)
	if err != nil {
		r.Status = StatusUnknown
		r.Reason = err.Error()
		return r
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		r.Status = StatusUnknown
		r.Reason = fmt.Sprintf("github returned %d", resp.StatusCode)
		return r
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		r.Status = StatusUnknown
		r.Reason = err.Error()
		return r
	}
	var rel githubReleaseResponse
	if err := json.Unmarshal(body, &rel); err != nil {
		r.Status = StatusUnknown
		r.Reason = err.Error()
		return r
	}
	if rel.TagName == "" {
		r.Status = StatusUnknown
		r.Reason = "github payload missing tag_name"
		return r
	}

	r.Latest = rel.TagName
	cmp, ok := compareSemver(current, rel.TagName)
	if !ok {
		// Dev build (0.0.0-dev) or a tag we cannot parse — treat as
		// behind so the user still sees the install command, but
		// label it Unknown so we don't lie about an upgrade path.
		r.Status = StatusUnknown
		r.Reason = fmt.Sprintf("cannot compare %q to %q", current, rel.TagName)
		r.UpgradeHint = upgradeHint(rel.TagName)
		return r
	}
	if cmp >= 0 {
		r.Status = StatusUpToDate
		return r
	}
	r.Status = StatusBehind
	r.UpgradeHint = upgradeHint(rel.TagName)
	return r
}

func upgradeHint(tag string) string {
	return "go install github.com/ruptor-dev/cli/cmd/ruptor@" + tag
}

func withDefaults(o Options) Options {
	if o.URL == "" {
		o.URL = DefaultReleasesURL
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
			},
		}
	}
	return o
}

// compareSemver returns -1/0/1 for a vs b. Both strings may carry a
// leading "v". Pre-release suffixes (-rc1, -dev) cause "ok=false" so
// callers can fall back to a softer message rather than risk a
// misleading comparison.
func compareSemver(a, b string) (int, bool) {
	pa, ok := parseSemver(a)
	if !ok {
		return 0, false
	}
	pb, ok := parseSemver(b)
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

func parseSemver(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimPrefix(s, "v")
	if strings.ContainsAny(s, "-+") {
		return v, false
	}
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, false
		}
		v[i] = n
	}
	return v, true
}
