package main_test

// End-to-end coverage for the cmd/ruptor binary. The package was at
// 0% line coverage because every entrypoint exits the process and
// has no in-package callers — the only honest way to test it is to
// spawn the compiled binary. TestMain builds it once into a temp
// dir; individual tests exec it.
//
// Tests are deliberately conservative on the proxy + simulate paths:
// `ruptor run` requires the user's agent to drive the proxy, and
// `ruptor simulate` requires OPENAI_API_KEY. Both are exercised
// through their fast-fail error paths so the coverage bumps without
// the suite depending on a real LLM endpoint.

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// binPath is set by TestMain so each test reuses the same compiled
// binary instead of paying the build cost per case.
var binPath string

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "ruptor-integration-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmp)

	bin := filepath.Join(tmp, "ruptor")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	build := exec.Command("go", "build", "-o", bin, ".")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		// A build failure here would be reported as "panic" against
		// every test, which is misleading. Surface the error verbatim.
		panic("integration test build failed: " + err.Error())
	}
	binPath = bin

	os.Exit(m.Run())
}

// runRuptor execs the binary with the given args and a tempdir HOME
// so per-test state never leaks into the user's real ~/.ruptor.
// Returns combined stdout+stderr and the exit code.
func runRuptor(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(binPath, args...)
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		// Force NoColor so the assertions match plain text.
		"NO_COLOR=1",
		"TERM=dumb",
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	exit := 0
	if err != nil {
		var ee *exec.ExitError
		if !asExitError(err, &ee) {
			t.Fatalf("ruptor exec: %v\noutput:\n%s", err, buf.String())
		}
		exit = ee.ExitCode()
	}
	return buf.String(), exit
}

func asExitError(err error, target **exec.ExitError) bool {
	if e, ok := err.(*exec.ExitError); ok {
		*target = e
		return true
	}
	return false
}

func TestVersion(t *testing.T) {
	out, code := runRuptor(t, "version")
	assert.Equal(t, 0, code, "ruptor version must exit 0\n%s", out)
	assert.Contains(t, out, "ruptor")
	assert.Contains(t, out, "commit:")
	assert.Contains(t, out, "built:")
}

func TestHelpExitsZero(t *testing.T) {
	out, code := runRuptor(t, "--help")
	assert.Equal(t, 0, code)
	// Spot-check that every shipped subcommand is advertised. If a
	// future PR drops one of these from --help, this test fails loudly.
	for _, sub := range []string{"run", "simulate", "validate", "version", "auth", "doctor", "update", "sync"} {
		assert.Contains(t, out, sub, "help output missing %q", sub)
	}
}

func TestValidateValidConfig(t *testing.T) {
	out, code := runRuptor(t, "validate", repoPath(t, "testdata", "chaos_valid.yaml"))
	assert.Equal(t, 0, code, "valid config must exit 0\n%s", out)
}

func TestValidateInvalidConfig(t *testing.T) {
	// Cobra is configured with SilenceErrors=true; observable
	// contract is the non-zero exit code, not the message text.
	_, code := runRuptor(t, "validate", repoPath(t, "testdata", "chaos_invalid.yaml"))
	assert.NotEqual(t, 0, code, "invalid config must exit non-zero")
}

func TestRunRejectsInvalidConfig(t *testing.T) {
	// `ruptor run` opens the config before binding any port — a bad
	// file must fail fast with a non-zero exit. This is the sole
	// "ruptor run" path the integration suite covers; the proxy +
	// agent round-trip lives in package-level proxy tests.
	_, code := runRuptor(t, "run", repoPath(t, "testdata", "chaos_invalid.yaml"))
	assert.NotEqual(t, 0, code)
}

func TestSimulateRequiresOpenAIKey(t *testing.T) {
	cmd := exec.Command(binPath, "simulate", repoPath(t, "testdata", "simulate_valid.yaml"))
	// Strip OPENAI_API_KEY explicitly — the test runner may have it
	// set from the host shell. Keep the rest of HOME etc. clean.
	cmd.Env = []string{"HOME=" + t.TempDir(), "PATH=" + os.Getenv("PATH")}
	require.Error(t, cmd.Run(), "simulate without OPENAI_API_KEY must fail")
}

func TestSyncShowsWaitlistWhenCloudReportingDisabled(t *testing.T) {
	// CloudReportingEnabled is a build-time const set to false. The
	// gate fires before any I/O, so this exits 0 with the waitlist
	// copy regardless of network state.
	out, code := runRuptor(t, "sync")
	assert.Equal(t, 0, code, "disabled-cloud sync must exit 0\n%s", out)
	assert.Contains(t, out, "Cloud reporting is coming soon")
	assert.Contains(t, out, "https://ruptor.dev")
}

func TestUpdateExitsZeroOnNetworkFailure(t *testing.T) {
	// `ruptor update` is contractually non-blocking: even when the
	// GitHub releases API is unreachable the command must exit 0
	// with a muted warning. We can't guarantee the real endpoint is
	// reachable in CI, but either outcome (StatusUpToDate / Behind /
	// Unknown) keeps exit 0 — that is the assertion that matters.
	_, code := runRuptor(t, "update")
	assert.Equal(t, 0, code)
}

func TestDoctorExitsNonZeroOnLocalFailedCheck(t *testing.T) {
	// Only a LOCAL check failure should fail the doctor. We hold a
	// listener on an ephemeral port and point the doctor at it via
	// RUPTOR_PROXY_PORT — the port probe hits EADDRINUSE and marks
	// the row as ✗, which must drive exit 1. Cloud-side warnings
	// (⚠ coming soon while CloudReportingEnabled=false) must not.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer l.Close()
	port := l.Addr().(*net.TCPAddr).Port

	cmd := exec.Command(binPath, "doctor")
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(),
		"NO_COLOR=1",
		"TERM=dumb",
		fmt.Sprintf("RUPTOR_PROXY_PORT=%d", port),
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err = cmd.Run()
	var ee *exec.ExitError
	require.True(t, asExitError(err, &ee), "doctor should exit with non-zero status\n%s", buf.String())
	assert.Equal(t, 1, ee.ExitCode(), "doctor must exit 1 on local check failure\n%s", buf.String())
	assert.Contains(t, buf.String(), "Proxy port")
	assert.Contains(t, buf.String(), "port")
}

func TestDoctorExitsZeroWhenCloudDisabled(t *testing.T) {
	// On a clean machine with CloudReportingEnabled=false every local
	// check passes and the cloud rows surface as ⚠ "coming soon".
	// Contract: exit 0, warnings do not propagate to the exit code.
	out, code := runRuptor(t, "doctor")
	assert.Equal(t, 0, code, "doctor must exit 0 when local checks pass and cloud is disabled\n%s", out)
	assert.Contains(t, out, "Ruptor doctor")
	assert.Contains(t, out, "Go runtime")
	assert.Contains(t, out, "Cloud reachability")
	assert.Contains(t, out, "Authentication")
	assert.Contains(t, out, "coming soon")
}

// repoPath resolves a path relative to the cli/ repo root regardless
// of the directory the test was invoked from. cmd/ruptor sits two
// levels deep so we walk up to the parent that contains testdata/.
func repoPath(t *testing.T, parts ...string) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	root := wd
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return filepath.Join(append([]string{root}, parts...)...)
		}
		root = filepath.Dir(root)
	}
	t.Fatalf("could not locate repo root from %s", wd)
	return ""
}
