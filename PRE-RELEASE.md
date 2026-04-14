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
      real backend — not just the test harness. The `ruptor sync` PR
      added a real upload client (`internal/cloud/{client,sync}.go`)
      gated on this flag; flipping the const enables uploads to start
      immediately, so the platform must be ready first.

## Testing

- [x] Unit tests for `parseChaosResponse` + `parseConversationResponse` in
      `internal/evaluator/llmjudge`. White-box table-driven tests in
      `internal/evaluator/llmjudge/parsers_test.go` cover canonical
      verdicts, casing variants, fenced-code-block JSON, partial
      objects, and malformed input. Package coverage 0 % → 44.2 %.
- [x] Integration test that spawns the compiled `ruptor` binary against
      an `httptest.Server` backend. `cmd/ruptor/integration_test.go`
      builds the binary in `TestMain` and exercises every shipped
      subcommand (`version`, `--help` advertises all 8 subcommands,
      `validate` valid + invalid, `run` rejects bad config, `simulate`
      requires `OPENAI_API_KEY`, `sync` waitlist gate, `update` exits 0
      on network failure, `doctor` exits non-zero on failed check).
      Note: `go test -cover` cannot see across the exec boundary, so
      `cmd/ruptor` line coverage still reports 0 %. The exit-code +
      output assertions are the contract.

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
- [x] Every `TODO` / `FIXME` in the code reviewed. None may be
      user-facing at runtime (`ruptor --help`, error messages, report
      output). `TODO(v2):` markers are fine; user-visible `TODO` is
      not. Sweep on 2026-04-14 found one hit — the `TODO(v1-launch)`
      in `internal/auth/keys.go` tracking the dev-keypair swap. It
      duplicated the Security checklist line above, so it was removed
      in commit `d89cb14`. The canonical tracker for the key swap is
      this file.
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

- Tests: **266 passing** across 18 packages.
- Coverage: ~59 % overall. Notable gaps — `cmd/ruptor` 0 %
  (subprocess tests not visible to `-cover`; see the integration
  test note above), `internal/evaluator/llmjudge` 44.2 %,
  `internal/ui` 37.1 %.
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
