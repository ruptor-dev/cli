// Package runner launches and supervises the agent-under-test process
// declared by AgentConfig.Entrypoint. A runner owns one child process
// plus an open log file receiving its stdout+stderr; callers drive the
// lifecycle through Start → Wait → Stop.
//
// Design notes:
//   - Entrypoints are parsed with shlex.Split so quoted arguments work
//     without spawning a shell. Running under `sh -c` would break exit
//     codes and process-group kill semantics.
//   - Every child is launched in its own process group (Setpgid=true)
//     so Stop can signal the entire group and kill grandchildren too.
//     Flask in particular forks a reloader on SIGTERM unless we target
//     the group.
//   - Stdout and stderr are redirected into a caller-supplied log file;
//     we never mix them into ruptor's own stdout, which would corrupt
//     the Bubbletea TUI.
//   - Wait returns on process exit OR the supplied timeout, whichever
//     fires first. On timeout we kill the group and return
//     ErrWaitTimeout so the caller can record the reason.
package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/shlex"
)

// ErrEmptyEntrypoint is returned when Start is called with an empty
// entrypoint string; callers should check AgentConfig.Entrypoint first
// and skip the runner entirely in that case.
var ErrEmptyEntrypoint = errors.New("runner: empty entrypoint")

// ErrWaitTimeout is returned by Wait when the supplied timeout expires
// before the child exits. On this path Wait has already killed the
// child's process group.
var ErrWaitTimeout = errors.New("runner: wait timeout")

// DefaultStopGrace is how long Stop waits for a SIGTERM-induced exit
// before escalating to SIGKILL.
const DefaultStopGrace = 3 * time.Second

// Config captures the inputs the runner needs. Extra fields (e.g.
// working directory, resource limits) can be added here without
// changing the Start signature.
type Config struct {
	// Entrypoint is the full command line, split with shlex. The first
	// token is the executable; the remainder are arguments.
	Entrypoint string
	// Env is merged over os.Environ(): keys present here override the
	// inherited value. Empty by default.
	Env map[string]string
	// LogPath is the file that receives the child's stdout + stderr.
	// Created/truncated at Start time. Parent dirs are created if
	// missing.
	LogPath string
}

// Agent is the supervisor handle for a single child process.
type Agent struct {
	cmd     *exec.Cmd
	logFile *os.File

	waitOnce sync.Once
	waitErr  error
	waitCh   chan struct{}
}

// Start launches the process declared by cfg. It returns an Agent the
// caller can Wait/Stop, or an error if the entrypoint could not be
// parsed or the child failed to spawn.
func Start(ctx context.Context, cfg Config) (*Agent, error) {
	if cfg.Entrypoint == "" {
		return nil, ErrEmptyEntrypoint
	}

	argv, err := shlex.Split(cfg.Entrypoint)
	if err != nil {
		return nil, fmt.Errorf("runner: parse entrypoint: %w", err)
	}
	if len(argv) == 0 {
		return nil, ErrEmptyEntrypoint
	}

	if cfg.LogPath == "" {
		return nil, errors.New("runner: LogPath is required")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.LogPath), 0o755); err != nil {
		return nil, fmt.Errorf("runner: create log dir: %w", err)
	}
	logFile, err := os.Create(cfg.LogPath)
	if err != nil {
		return nil, fmt.Errorf("runner: open log file: %w", err)
	}

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = mergeEnv(os.Environ(), cfg.Env)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	setProcessGroup(cmd)

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("runner: start %q: %w", argv[0], err)
	}

	a := &Agent{
		cmd:     cmd,
		logFile: logFile,
		waitCh:  make(chan struct{}),
	}

	go func() {
		a.waitErr = cmd.Wait()
		_ = logFile.Close()
		close(a.waitCh)
	}()

	return a, nil
}

// Wait blocks until the child exits or timeout elapses. On timeout the
// runner kills the process group and returns ErrWaitTimeout; the caller
// may still record the agent as "completed with timeout". A zero
// timeout means wait forever.
func (a *Agent) Wait(ctx context.Context, timeout time.Duration) error {
	var timerC <-chan time.Time
	if timeout > 0 {
		t := time.NewTimer(timeout)
		defer t.Stop()
		timerC = t.C
	}

	select {
	case <-a.waitCh:
		return a.waitErr
	case <-timerC:
		a.kill()
		<-a.waitCh
		return ErrWaitTimeout
	case <-ctx.Done():
		a.kill()
		<-a.waitCh
		return ctx.Err()
	}
}

// Stop gracefully terminates the child: SIGTERM, wait up to grace,
// then SIGKILL. Safe to call multiple times; a process that has already
// exited returns nil.
func (a *Agent) Stop(grace time.Duration) error {
	select {
	case <-a.waitCh:
		return nil
	default:
	}

	if grace <= 0 {
		grace = DefaultStopGrace
	}

	a.terminate()

	t := time.NewTimer(grace)
	defer t.Stop()

	select {
	case <-a.waitCh:
		return nil
	case <-t.C:
		a.kill()
		<-a.waitCh
		return nil
	}
}

// Exited reports whether the child has already terminated. Useful for
// "hits > 0 AND exited cleanly" completion checks without blocking on
// Wait.
func (a *Agent) Exited() bool {
	select {
	case <-a.waitCh:
		return true
	default:
		return false
	}
}

// ExitErr returns the child's wait error once Exited is true. Returns
// nil when the child is still running or exited with code 0.
func (a *Agent) ExitErr() error {
	if !a.Exited() {
		return nil
	}
	return a.waitErr
}

// PID returns the child's PID, or 0 if the child has not started.
func (a *Agent) PID() int {
	if a.cmd == nil || a.cmd.Process == nil {
		return 0
	}
	return a.cmd.Process.Pid
}

// terminate and kill dispatch to platform-specific implementations in
// runner_unix.go / runner_windows.go. Errors are swallowed because the
// common case is "process already gone".
func (a *Agent) terminate() {
	if a.cmd == nil || a.cmd.Process == nil {
		return
	}
	_ = terminateGroup(a.cmd)
}

func (a *Agent) kill() {
	if a.cmd == nil || a.cmd.Process == nil {
		return
	}
	_ = killProcessGroup(a.cmd)
}

// mergeEnv overlays overrides on top of the inherited environment. A
// later "KEY=val" pair in the returned slice wins, matching exec
// package semantics.
func mergeEnv(base []string, overrides map[string]string) []string {
	if len(overrides) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(overrides))
	out = append(out, base...)
	for k, v := range overrides {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	return out
}
