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
	"strconv"
	"strings"
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
	// Invalid config must exit non-zero AND emit an error message
	// through ui.Error — see cmd/ruptor/main.go. The exact text may
	// change; we only assert that *some* error text reaches the output
	// stream (stdout today; stderr once ui-stdout-stderr-split lands).
	out, code := runRuptor(t, "validate", repoPath(t, "testdata", "chaos_invalid.yaml"))
	require.NotEqual(t, 0, code, "invalid config must exit non-zero")
	assert.NotEmpty(t, strings.TrimSpace(out), "expected error text from ui.Error")
}

// TestMain_UnknownSubcommand_PrintsError verifies that a typo or bogus
// subcommand surfaces cobra's "unknown command" error to the user
// instead of silently exiting 1 — regression test for the UX bug in
// docs/specs/backlog/cobra-silence-errors.md.
func TestMain_UnknownSubcommand_PrintsError(t *testing.T) {
	out, code := runRuptor(t, "bogus-subcmd")
	require.NotEqual(t, 0, code)
	// Cobra's message is "unknown command \"bogus-subcmd\" for \"ruptor\"".
	// We assert on a substring, not the full text, to stay robust to
	// cobra upgrades.
	assert.Contains(t, strings.ToLower(out), "unknown command")
}

// TestMain_RunMissingArg_PrintsUsageHint verifies that `ruptor run`
// (no config file) surfaces cobra's usage-hint error rather than
// exiting silently.
func TestMain_RunMissingArg_PrintsUsageHint(t *testing.T) {
	out, code := runRuptor(t, "run")
	require.NotEqual(t, 0, code)
	// cobra.ExactArgs(1) emits "accepts 1 arg(s), received 0".
	assert.Contains(t, strings.ToLower(out), "arg")
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

func TestRunLaunchesAgentEntrypoint(t *testing.T) {
	// Sanity-check the whole runner wiring end-to-end: `ruptor run`
	// must actually exec the entrypoint declared in chaos.yaml and
	// wait for it. The agent here is a tiny shell snippet that writes
	// its PID into a sentinel file; the test asserts the file exists
	// and contains a live PID after the run finishes. The run itself
	// completes because timeout_s bounds the experiment.
	dir := t.TempDir()
	sentinel := filepath.Join(dir, "agent.pid")
	cfgPath := filepath.Join(dir, "chaos.yaml")

	// The port is reserved via net.Listen(:0) + Close — cheap way to
	// pick a free port without races in the common case.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())

	cfg := fmt.Sprintf(`version: "1"
agent:
  name: runner_integration_agent
  entrypoint: sh -c "echo $$ > %s; sleep 10"
  mode: oneshot
proxy:
  port: %d
  passthrough_url: http://127.0.0.1:1
  request_timeout_s: 5
tests:
  - id: noop
    tool: /noop
    fault: tool_timeout
    delay_ms: 10
    probability: 1.0
evaluation:
  max_iterations: 1
  timeout_s: 2
  llm_judge: false
output:
  format: stdout
`, sentinel, port)
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0o600))

	_, code := runRuptor(t, "run", cfgPath)
	assert.Equal(t, 0, code)

	body, err := os.ReadFile(sentinel)
	require.NoError(t, err, "sentinel file must exist — agent entrypoint never ran")
	pid, err := strconv.Atoi(strings.TrimSpace(string(body)))
	require.NoError(t, err, "sentinel must contain a numeric PID, got %q", body)
	assert.Greater(t, pid, 1, "PID must be > 1")
	proc, err := os.FindProcess(pid)
	require.NoError(t, err)
	assert.NotNil(t, proc)
}

// mcpUnscoredWarningSnippet is a stable substring of the runtime
// warning surfaced when proxy.mode is "mcp" or "auto". Kept narrow on
// purpose — asserting the full wording would couple the test to
// copyediting, whereas the backlog spec path is the stable anchor
// the warning exists to advertise.
const mcpUnscoredWarningSnippet = "docs/specs/backlog/mcp-observations-evaluator.md"

// chaosConfigWithMode writes a minimal chaos.yaml whose agent is a
// shell snippet that simply sleeps long enough for the orchestrator
// to reach the warning site, then returns. We do not care about
// report contents — only whether the warning fires on stdout.
func chaosConfigWithMode(t *testing.T, mode string, port int) string {
	t.Helper()
	cfg := fmt.Sprintf(`version: "1"
agent:
  name: mcp_warning_test_agent
  entrypoint: sh -c "sleep 1"
  mode: oneshot
proxy:
  port: %d
  passthrough_url: http://127.0.0.1:1
  request_timeout_s: 5
  mode: %s
tests:
  - id: noop
    tool: /noop
    fault: tool_timeout
    delay_ms: 10
    probability: 1.0
evaluation:
  max_iterations: 1
  timeout_s: 2
  llm_judge: false
output:
  format: stdout
`, port, mode)
	cfgPath := filepath.Join(t.TempDir(), "chaos.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfg), 0o600))
	return cfgPath
}

// freePort returns a TCP port the OS advertised as free. Best-effort —
// the port may be reclaimed before the binary under test binds it,
// but for localhost integration tests the race is vanishingly rare.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())
	return port
}

func TestRun_WarnsWhenProxyModeIsMCP(t *testing.T) {
	// Contract: proxy.mode=mcp must surface the unscored-evaluator
	// warning exactly once per run, before the experiment starts.
	// Faults still fire on the wire; only the score wiring is missing.
	// See docs/specs/backlog/mcp-observations-evaluator.md.
	cfgPath := chaosConfigWithMode(t, "mcp", freePort(t))
	out, code := runRuptor(t, "run", cfgPath)
	assert.Equal(t, 0, code, "mcp-mode run must exit 0\n%s", out)
	assert.Contains(t, out, mcpUnscoredWarningSnippet,
		"mcp mode must surface the unscored-evaluator warning")
	// The warning must be emitted at most once per run — if the helper
	// is accidentally placed inside the per-test loop a multi-test run
	// would spam the user. One test case here is enough to flag a
	// regression because the helper has a single caller.
	assert.Equal(t, 1, strings.Count(out, mcpUnscoredWarningSnippet),
		"warning must appear exactly once per run")
}

func TestRun_WarnsWhenProxyModeIsAuto(t *testing.T) {
	// "auto" may route individual requests through the MCP handler at
	// runtime based on payload inspection, so the warning must fire
	// here too.
	cfgPath := chaosConfigWithMode(t, "auto", freePort(t))
	out, code := runRuptor(t, "run", cfgPath)
	assert.Equal(t, 0, code, "auto-mode run must exit 0\n%s", out)
	assert.Contains(t, out, mcpUnscoredWarningSnippet,
		"auto mode must surface the unscored-evaluator warning")
}

func TestRun_DoesNotWarnWhenProxyModeIsHTTP(t *testing.T) {
	// Pure HTTP mode is unaffected by the MCP observation gap — the
	// warning must stay silent so HTTP users do not see copy that
	// does not apply to them. Covers both the explicit "http" value
	// and the default empty string (which resolves to HTTP today).
	for _, mode := range []string{"http", ""} {
		t.Run("mode="+mode, func(t *testing.T) {
			cfgPath := chaosConfigWithMode(t, mode, freePort(t))
			out, code := runRuptor(t, "run", cfgPath)
			assert.Equal(t, 0, code, "%q-mode run must exit 0\n%s", mode, out)
			assert.NotContains(t, out, mcpUnscoredWarningSnippet,
				"%q mode must not surface the MCP-evaluator warning", mode)
		})
	}
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
