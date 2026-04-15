//go:build !windows

package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ruptor-dev/cli/internal/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sleepCmd is a portable 30s sleep used for wait/stop assertions.
const sleepCmd = "sleep 30"

func TestStart_EmptyEntrypointRejected(t *testing.T) {
	_, err := runner.Start(context.Background(), runner.Config{
		Entrypoint: "",
		LogPath:    filepath.Join(t.TempDir(), "agent.log"),
	})
	assert.ErrorIs(t, err, runner.ErrEmptyEntrypoint)
}

func TestStart_MissingLogPathRejected(t *testing.T) {
	_, err := runner.Start(context.Background(), runner.Config{
		Entrypoint: "echo hi",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LogPath")
}

func TestStart_ChildProcessSpawnsAndWriteLog(t *testing.T) {
	log := filepath.Join(t.TempDir(), "agent.log")
	a, err := runner.Start(context.Background(), runner.Config{
		Entrypoint: `sh -c "echo hello-ruptor"`,
		LogPath:    log,
	})
	require.NoError(t, err)

	pid := a.PID()
	assert.NotZero(t, pid, "runner must expose a PID once the child has started")
	// Finding the process must succeed: on POSIX os.FindProcess always
	// returns a handle, but we also confirm the PID is a running
	// process by sending signal 0.
	proc, err := os.FindProcess(pid)
	require.NoError(t, err)
	assert.NotNil(t, proc)

	require.NoError(t, a.Wait(context.Background(), 5*time.Second))
	assert.True(t, a.Exited())

	body, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.Contains(t, string(body), "hello-ruptor")
}

func TestWait_TimeoutKillsProcessGroup(t *testing.T) {
	log := filepath.Join(t.TempDir(), "agent.log")
	a, err := runner.Start(context.Background(), runner.Config{
		Entrypoint: sleepCmd,
		LogPath:    log,
	})
	require.NoError(t, err)
	pid := a.PID()

	start := time.Now()
	err = a.Wait(context.Background(), 200*time.Millisecond)
	assert.ErrorIs(t, err, runner.ErrWaitTimeout)
	assert.Less(t, time.Since(start), 2*time.Second,
		"Wait with timeout must not block past the timeout")

	// Signal 0 against the dead PID must fail.
	assert.Error(t, syscall.Kill(pid, syscall.Signal(0)))
}

func TestStop_TerminatesRunningChild(t *testing.T) {
	log := filepath.Join(t.TempDir(), "agent.log")
	a, err := runner.Start(context.Background(), runner.Config{
		Entrypoint: sleepCmd,
		LogPath:    log,
	})
	require.NoError(t, err)

	assert.False(t, a.Exited())
	require.NoError(t, a.Stop(500*time.Millisecond))
	assert.True(t, a.Exited())

	// Idempotent: calling Stop again returns nil with no panic.
	assert.NoError(t, a.Stop(100*time.Millisecond))
}

func TestEnv_OverridesInheritedValues(t *testing.T) {
	log := filepath.Join(t.TempDir(), "agent.log")
	a, err := runner.Start(context.Background(), runner.Config{
		Entrypoint: `sh -c "echo VAR=$RUPTOR_RUNNER_TEST"`,
		Env:        map[string]string{"RUPTOR_RUNNER_TEST": "from-env"},
		LogPath:    log,
	})
	require.NoError(t, err)
	require.NoError(t, a.Wait(context.Background(), 5*time.Second))

	body, err := os.ReadFile(log)
	require.NoError(t, err)
	assert.True(t, strings.Contains(string(body), "VAR=from-env"), "got: %q", string(body))
}

func TestStart_BadEntrypointFailsFast(t *testing.T) {
	_, err := runner.Start(context.Background(), runner.Config{
		Entrypoint: "__definitely_not_a_binary__",
		LogPath:    filepath.Join(t.TempDir(), "agent.log"),
	})
	require.Error(t, err)
}
