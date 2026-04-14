# Pre-Release Checklist

Living document. Every agent reading this at the start of a launch-related
PR updates it if they find a new blocker. Unchecked items block `ruptor v1`
from being published publicly. Checked items have a short note explaining
what "done" means for that line so later reviewers don't have to guess.

## Security

- [ ] `internal/auth/keys/public_key.pem` — replace the dev keypair with
      the real platform key before launch. Delete
      `internal/auth/testdata/jwt_private_key.pem` or regenerate it as a
      test-only key that doesn't match the shipped public key.
- [ ] Verify `CloudReportingEnabled = false` in `internal/cloud/feature.go`
      stays `false` until the platform API (`https://api.ruptor.dev`) is
      live and the `--cloud` path has been exercised end-to-end against a
      real backend — not just the test harness.

## Testing

- [ ] Unit tests for `parseChaosResponse` + `parseConversationResponse` in
      `internal/evaluator/llmjudge`. The package sits at 0 % coverage
      today because the parsers only run inside `OpenAIJudge.Evaluate*`
      calls; extract them or cover them directly.
- [ ] Integration test that spawns the compiled `ruptor` binary against
      an `httptest.Server` backend. Covers `cmd/ruptor` (currently 0 %)
      end-to-end: `ruptor run` writes a report, `ruptor simulate` writes a
      report, both print the completion screen, both return non-zero on
      config errors.

## Known non-blockers

Document but do not block launch on these.

- [ ] **golangci-lint spurious `level=error` log** on Go 1.26.2 — exit
      code is 0, `0 issues.` is reported, the log line is cosmetic.
      Track the upstream golangci-lint issue; remove this line when a
      fixed linter release ships.
- [ ] **`ruptor auth status` does not check server-side revocation.**
      Deliberate — local JWT validation only, per SKILL-auth.md
      ("No network call needed to check if token is valid — fast
      startup"). Consider a periodic server-side check in v2.
- [ ] **TUI uses a 100 ms tick-based poller** that reads
      `proxy.Observations()` on every tick. A push channel from proxy
      → UI is the v2 follow-up (AUDIT.md §10).
- [ ] **`charm.land/bubbles/v2` not pinned in `go.mod`.** Correct — no
      component from `bubbles` is imported yet. Add when the first
      component lands.

## Infrastructure

- [ ] `knowledge` repo has a git remote configured and is pushed to the
      `ruptor-dev/knowledge` GitHub repo. Currently only a local
      initial commit exists.
- [ ] `ruptor-dev` GitHub organisation created with the four repos at
      the right visibility:
      - `ruptor-dev/cli` — public (Apache 2.0)
      - `ruptor-dev/platform` — private
      - `ruptor-dev/landing` — public
      - `ruptor-dev/knowledge` — private
- [ ] `ruptor-dev/homebrew-tap` repo created so goreleaser can publish
      the Homebrew formula.
- [ ] `ruptor.dev` domain DNS points to the landing page (Cloudflare
      Pages).
- [ ] Cloudflare Pages configured for the `landing` repo, auto-deploy
      on push to `main`.

## Before first public release

- [x] `.goreleaser.yaml` exists and `make release-dry` produces the
      expected artifact set locally: 5 archives (linux/darwin/windows ×
      amd64/arm64 minus windows/arm64), sha-256 checksums, homebrew
      cask. `goreleaser check` is green.
- [ ] goreleaser + cosign pipeline exercised with a real tag (not
      `goreleaser --snapshot`). Blocked on the GitHub org + homebrew
      tap creation; when unblocked, cut `v0.0.1-rc1`, watch the
      `release` workflow, verify signatures with
      `cosign verify-blob --certificate-identity-regexp …`, confirm
      the tap repo receives a cask commit.
- [x] `CONTRIBUTING.md` present at repo root. Review on a clean
      checkout before v1 and confirm every `make …` line still works.
- [x] `LICENSE` present at repo root (Apache 2.0 canonical text).
- [x] `CODE_OF_CONDUCT.md` present at repo root (Contributor
      Covenant v2.1 pointer + conduct@ruptor.dev contact).
- [x] `.github/workflows/ci.yml` runs build + test + lint + vet on
      push/PR to main.
- [x] `.github/workflows/release.yml` runs on `v*` tags: cosign
      installer → goreleaser release --clean with `id-token: write`
      so keyless OIDC signing works.
- [x] `.github/ISSUE_TEMPLATE/` (bug + feature) and
      `.github/PULL_REQUEST_TEMPLATE.md` present.
- [ ] Every `TODO` / `FIXME` in the code reviewed. None may be
      user-facing at runtime (`ruptor --help`, error messages, report
      output). `TODO(v2):` markers are fine; user-visible `TODO` is
      not.
- [x] `ruptor doctor` output eyeballed on a clean machine — no `nil`
      dereferences, no paths that assume `~/.ruptor/config.yaml`
      already exists, no surfacing of secret values. Subcommand
      lives at `internal/doctor` + `cmd/ruptor/doctor.go`. Verified
      on a clean checkout with no token: missing config file is
      reported as OK (with the path), token check warns "not logged
      in", `MaskedSuffix` is the only token form ever rendered.
      Smoke run produces colored ✓/⚠/✗ rows + a one-line fix hint
      below every non-OK row.
- [x] `ruptor version` prints the ldflags-injected version, commit,
      and build date. Verified against a `make release-dry` artifact:
      the packaged darwin/arm64 binary reports
      `ruptor 0.0.1-next / commit: <sha> / built: <date>`.

## Pipeline status (maintained by CI + each PR)

Update the lines below whenever the underlying state changes. Keep
counts honest — stale numbers here are worse than no numbers.

- Tests: **207 passing** across 16 packages.
- Coverage: ~57 % overall. Notable gaps — `cmd/ruptor` 0 %,
  `internal/evaluator/llmjudge` 0 %, `internal/ui` 38.8 %.
- `make check`: green.
- `make release-dry`: green — 5 archives, checksums, homebrew cask.
- CI workflow: `.github/workflows/ci.yml`.
- Release workflow: `.github/workflows/release.yml` (fires on `v*`).

## How to use this file

1. Before writing a line of launch-related code, `grep -n "- \[ \]" PRE-RELEASE.md`
   to see what still blocks release.
2. If you close a line, tick it in the same PR that closes it. Never
   tick speculatively.
3. If you find a new blocker while working on something unrelated, add
   it to the appropriate section in the same PR — don't defer the
   discovery to a later cleanup pass.
4. Keep each item specific enough that a reviewer can verify it
   without asking questions. "Secrets reviewed" is useless; "All
   TODO comments in code reviewed" is auditable.
