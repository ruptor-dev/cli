## What this PR does

<!-- One or two sentences on the change. The diff shows the *what*; this box is for the *why*. -->

## Testing

<!-- Unit + integration tests added? `make check` green? Anything you exercised manually? -->

## Pre-flight

- [ ] `make check` is green locally.
- [ ] Functions touched are under 30 lines (`internal/engineering.md` rule).
- [ ] No new `fmt.Println` / `fmt.Printf` outside `internal/ui/`.
- [ ] No `InsecureSkipVerify`, no secrets in source.
- [ ] If this is a fault / evaluator / auth change, unit + integration
      tests ship in this PR.
- [ ] If this is launch-related, `PRE-RELEASE.md` is updated with any new
      blockers I found.
- [ ] Commit messages follow Conventional Commits.
