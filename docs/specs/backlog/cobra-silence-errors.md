# `cmd.SilenceErrors: true` swallows cobra error messages

> Status: **Done — 2026-04-16** — Option A landed; `main()` prints
> cobra errors via `ui.Error` (commit ba4d11a) with integration tests
> covering the unknown-subcommand and missing-arg paths (commit b4bf76d).
> Opened: 2026-04-16
> Priority: **medium**
> Est. effort: **XS** (30 min)
> Decision required: **no**

## Problem

The root command at `cmd/ruptor/main.go:67` sets `SilenceErrors: true`.
Combined with `main()`'s bare `os.Exit(1)` when `rootCmd.Execute()`
returns an error, this means any command error — a flag parse failure,
an unknown subcommand, a file-not-found — exits silently with no
message to the user.

Reproducible today:

```
$ ruptor run      # missing required <config-file> arg
$ echo $?         # 1
$                 # no error text anywhere
```

This session spent minutes debugging `../../bin/ruptor ./run.sh`
(user passed the script as a subcommand) before realising cobra had
rejected it quietly.

## Proposed solution

Two equivalent fixes; pick one.

### Option A — print errors from main

Keep `SilenceErrors: true` (useful for RunE handlers that want full
control over formatting) but print the error via `ui.Error` in main:

```go
func main() {
    rootCmd := newRootCmd()
    if err := rootCmd.Execute(); err != nil {
        ui.Error(err.Error())
        os.Exit(1)
    }
}
```

### Option B — let cobra print

Drop `SilenceErrors: true`. Cobra's default prints to stderr with
"Error: " prefix. Simpler, loses styled output.

**Recommendation: Option A.** The `ui.Error` output is styled (red
✗ prefix), consistent with other CLI errors. Works once
`ui-stdout-stderr-split` lands (`ui.Error` then routes to stderr
automatically).

## Acceptance criteria

- [ ] Running `ruptor bogus-subcmd` prints an error message.
- [ ] Running `ruptor run` (missing arg) prints the usage hint.
- [ ] Exit code still 1 on error.
- [ ] RunE handlers that return wrapped errors see the full chain
      in the output (test with `cmd/ruptor/sync.go`'s "not logged
      in" permanent error).

## Out of scope

- Custom error formatting per subcommand.
- Logging errors to disk (already handled via `ruptor.log`).

## References

- `cmd/ruptor/main.go:44-48` (main), `:66-67` (SilenceErrors).
- This session (2026-04-16): TUI debugging episode where a typo'd
  invocation silently exited 1 with no output.
