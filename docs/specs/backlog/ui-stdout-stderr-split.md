# Split `ui` helper output into stdout (data) vs stderr (diagnostics)

> Status: **Backlog — reverted once, needs redesign**
> Opened: 2026-04-16
> Priority: **medium**
> Est. effort: **S** (half day)
> Decision required: **yes — see History**

## Problem

`ui.Success`, `ui.Error`, `ui.Warning`, `ui.Info`, `ui.Dim`,
`ui.VersionBanner` all write through a single package-level `out`
writer that defaults to `os.Stdout`. A user running
`ruptor run ... > report.txt` or piping through `jq` sees diagnostic
lines interleaved with the pipeable data (report paths, etc).

Standard CLI convention: data to stdout (pipeable), diagnostics to
stderr. This repo currently conflates them.

## History

Commit `e30165f` (`refactor(ui): route warnings and errors to stderr,
preserve stdout for data`) split the writers — added a second package
global `errOut = os.Stderr` and moved every diagnostic helper to
`errOut`. It was reverted along with the `mcp-observations-evaluator`
sink in `8187783` because the TUI-sync restoration pulled in the
stash's single-writer version wholesale.

Re-attempting this must coexist with the TUI sync work (the log panel
is now in place — see `SKILL-ui.md`). The split is orthogonal to the
TUI itself but requires care:

- Tests capture via `ui.SetWriter` — after the split, diagnostic-
  asserting tests must switch to `ui.SetErrWriter`. Silent pass risk:
  a test that previously read Success/Warning text from `Writer()`
  will quietly see an empty buffer if the migration is missed. File
  the migration as part of the commit that introduces `SetErrWriter`,
  and add a positive-polarity counter-test (stdout polarity locked).

- `zerolog` already writes to `os.Stderr` via `ui.NewLogger`. The
  split does NOT change that. The two are independent streams that
  happen to share `os.Stderr` in a real terminal.

## Proposed approach

Re-introduce the split exactly as `e30165f` had it, but in a commit
that (a) migrates every caller of `SetWriter` that asserted on a
diagnostic helper, and (b) adds a `TestPlainHelpers_WriteThroughWriter`
test to pin the stdout polarity so future helper additions do not
silently migrate to the wrong stream.

Audit the following call sites before committing:
- `cmd/ruptor/auth.go` — `ui.Success` / `ui.Info` for device-flow URL/code
- `cmd/ruptor/doctor.go` — per-check row via `ui.Success`
- `cmd/ruptor/sync.go` — `ui.Success("Uploaded %s")` is status
- `cmd/ruptor/update.go` — `ui.Success` / `ui.Info` updater messages
- `cmd/ruptor/main.go` — `ui.Info(waitlistMessage)`, cloud-mode notices

All are status-shaped; all move to stderr. Add release note.

## Acceptance criteria

- [ ] `ui.go` carries both `out` and `errOut` writers with `SetWriter`
      / `SetErrWriter` / `Writer` / `ErrWriter` setters.
- [ ] `Success`, `Error`, `Warning`, `Info`, `Dim`, `VersionBanner`
      route to `errOut`.
- [ ] `Println`, `Printf`, `PrintCompletion` stay on `out`.
- [ ] Tests that asserted diagnostic text via `Writer()` migrated to
      `ErrWriter()`.
- [ ] New `TestPlainHelpers_WriteThroughWriter` pins stdout polarity.
- [ ] README + changelog mention Success / Info moved to stderr.

## Out of scope

- Changing zerolog's stderr destination.
- Adding a `--quiet` flag (separate concern).

## References

- `internal/ui/ui.go`, `internal/ui/output.go`
- Previous implementation: commit `e30165f`
- Previous revert: commit `8187783`
