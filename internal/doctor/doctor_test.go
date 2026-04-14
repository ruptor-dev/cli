package doctor_test

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ruptor-dev/cli/internal/doctor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findResult locates the named result so tests do not have to depend
// on the iteration order of doctor.Run.
func findResult(t *testing.T, results []doctor.Result, name string) doctor.Result {
	t.Helper()
	for _, r := range results {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("result %q not found in %+v", name, results)
	return doctor.Result{}
}

func TestRun_AllOK(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Listen on an ephemeral port, then close so the doctor can re-bind.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())

	results := doctor.Run(context.Background(), doctor.Options{
		HTTPClient: srv.Client(),
		CloudURL:   srv.URL,
		ConfigDir:  dir,
		Port:       port,
		GoVersion:  "go1.99.0", // future-proof so the min-version check passes
	})

	assert.False(t, doctor.HasFailures(results), "expected no failures, got %+v", results)
	assert.Equal(t, doctor.StatusOK, findResult(t, results, "Go runtime").Status)
	assert.Equal(t, doctor.StatusOK, findResult(t, results, "Proxy port").Status)
	assert.Equal(t, doctor.StatusOK, findResult(t, results, "Cloud reachability").Status)
	// No config file → config perms is OK and auth is Warn (not logged in).
	assert.Equal(t, doctor.StatusOK, findResult(t, results, "Config permissions").Status)
	assert.Equal(t, doctor.StatusWarn, findResult(t, results, "Authentication").Status)
}

func TestRun_GoVersionTooOld(t *testing.T) {
	results := doctor.Run(context.Background(), doctor.Options{GoVersion: "go1.20.0"})
	r := findResult(t, results, "Go runtime")
	assert.Equal(t, doctor.StatusFail, r.Status)
	assert.Contains(t, r.Hint, "https://go.dev/dl/")
}

func TestRun_GoVersionUnparseable(t *testing.T) {
	results := doctor.Run(context.Background(), doctor.Options{GoVersion: "devel +abcd"})
	r := findResult(t, results, "Go runtime")
	assert.Equal(t, doctor.StatusWarn, r.Status)
}

func TestRun_BadConfigPerms(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix permission bits not enforced on windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("token: x"), 0o644))

	results := doctor.Run(context.Background(), doctor.Options{
		ConfigDir: dir,
		GoVersion: "go1.99.0",
		Port:      0, // skip-style: let withDefaults fall back to 8080 — irrelevant here
	})

	r := findResult(t, results, "Config permissions")
	assert.Equal(t, doctor.StatusFail, r.Status)
	assert.Contains(t, r.Hint, "chmod 600")
}

func TestRun_PortInUse(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	results := doctor.Run(context.Background(), doctor.Options{
		Port:      port,
		GoVersion: "go1.99.0",
	})
	r := findResult(t, results, "Proxy port")
	assert.Equal(t, doctor.StatusFail, r.Status)
	assert.Contains(t, r.Hint, "RUPTOR_PROXY_PORT")
}

func TestRun_NetworkUnreachable(t *testing.T) {
	// Already-cancelled context guarantees the network probe returns
	// immediately rather than waiting for a real TCP timeout against
	// the unreachable URL.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	results := doctor.Run(ctx, doctor.Options{
		CloudURL:  "http://192.0.2.1:1",
		GoVersion: "go1.99.0",
	})
	r := findResult(t, results, "Cloud reachability")
	assert.Equal(t, doctor.StatusFail, r.Status)
	assert.NotEmpty(t, r.Hint)
}

func TestRun_NetworkServer5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	results := doctor.Run(context.Background(), doctor.Options{
		HTTPClient: srv.Client(),
		CloudURL:   srv.URL,
		GoVersion:  "go1.99.0",
	})
	r := findResult(t, results, "Cloud reachability")
	assert.Equal(t, doctor.StatusWarn, r.Status)
}

func TestRun_AuthMalformedToken(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix permission bits not enforced on windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("token: not-a-real-jwt"), 0o600))

	results := doctor.Run(context.Background(), doctor.Options{
		ConfigDir: dir,
		GoVersion: "go1.99.0",
	})
	r := findResult(t, results, "Authentication")
	assert.Equal(t, doctor.StatusFail, r.Status)
	assert.Contains(t, r.Hint, "ruptor auth login")
}
