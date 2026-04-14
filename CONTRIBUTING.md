# Contributing to Ruptor

Thanks for considering a contribution. This file tells you how to build, test,
and submit changes. If anything here is out of date, that is itself a bug —
please open an issue or a PR.

## Before you start

1. Read `CLAUDE.md` and `../knowledge/` (if you have access) for the
   engineering + security rules.
2. Skim the ADRs under `../knowledge/decisions/` for non-trivial design
   decisions — don't reopen settled debates in a PR description.
3. Check `AUDIT.md` and `PRE-RELEASE.md` — the first lists what's missing
   today; the second lists what must be true before the public v1 ships.

## Development setup

Requires Go 1.26.2.

```bash
git clone https://github.com/ruptor-dev/cli.git
cd cli
make tools       # installs golangci-lint v2 and goreleaser
make build       # compiles bin/ruptor
make check       # build + test + lint + vet — must be green
```

`make run-example` runs the chaos example; `make run-simulate-example` runs
the simulate example. `make release-dry` builds all release artifacts
locally without publishing.

## Code rules (summary; full text in CLAUDE.md)

- Terminal output goes through `internal/ui/`. Never use `fmt.Println`,
  `fmt.Printf`, or `log.Printf` in `cmd/` or `internal/`.
- Functions stay under 30 lines. Split when an edit pushes a function past.
- Errors are wrapped with context: `fmt.Errorf("component: operation: %w", err)`.
- Interfaces live in `pkg/types/` before implementations.
- No `panic()` in library code. Only in `main()` for unrecoverable startup.
- No `InsecureSkipVerify`. Anywhere. Ever.
- No secrets in source. Env vars or `~/.ruptor/config.yaml` only.

## Commits + PRs

- One logical change per PR. If you find yourself writing "and also…" in
  the description, split the PR.
- Conventional Commits: `feat:`, `fix:`, `refactor:`, `test:`, `docs:`,
  `chore:`. Subject under 72 characters.
- Describe the *why* more than the *what* — the diff shows the what.
- Every fault / evaluator / auth change needs unit + integration tests in
  the same PR.
- `make check` must be green before you open the PR.

## Reporting bugs

File an issue with:
1. `ruptor --version` output.
2. OS + Go version.
3. The chaos.yaml or simulate.yaml that reproduces.
4. Actual vs expected behaviour.
5. Any zerolog output at `--log-level=debug`.

Security issues: do **not** file a public issue. Email security@ruptor.dev.
