# Intentional gaps tracked in backlog

If you see one of these in the code or docs and wonder whether it's a
bug: it is, and we know. Each row points at an open backlog spec that
will close the gap.

| Location | What you see | Tracked as |
|----------|--------------|------------|
| `cmd/ruptor/sync.go:29` | `runSync` does not call `auth.MaybeRefresh` | [auth-silent-refresh-wiring](auth-silent-refresh-wiring.md) |
| `cmd/ruptor/auth.go:127` | `runAuthStatus` does not call `auth.MaybeRefresh` | [auth-silent-refresh-wiring](auth-silent-refresh-wiring.md) |
| *(missing file)* `SECURITY.md` at repo root | GitHub Security tab blank | [security-md](security-md.md) |
| `Makefile:14` | No `test-int` target | [make-test-int-target](make-test-int-target.md) |
| `*_inttest.go` (any) | No build tag; runs with unit tests | [make-test-int-target](make-test-int-target.md) |
| `.goreleaser.yaml:142` | Footer shows `--signature`/`--certificate` (pre-v2) | [goreleaser-footer-bundle-format](goreleaser-footer-bundle-format.md) |
| `cmd/ruptor/auth.go:144-153` | `renderAuthStatus` has no Cloud row | [auth-status-cloud-row](auth-status-cloud-row.md) |
| `cmd/ruptor/main.go:370-372` | `case "both"` appends only HTML | [output-paths-both-format](output-paths-both-format.md) |
| `knowledge/architecture.md:103-104` | `cobra v2.5.1`, `viper v1.31+` (don't exist) | [knowledge-architecture-stack-drift](knowledge-architecture-stack-drift.md) |
| `.github/workflows/ci.yml` | No examples/configs smoke step | [examples-ci-smoke](examples-ci-smoke.md) |
| `internal/evaluator/llmjudge/parsers_test.go` | Unit tests only, no fuzz | [fuzz-parsers](fuzz-parsers.md) |
| `internal/simulate/simulator.go` `parseAgentResponse` | No fuzz | [fuzz-parsers](fuzz-parsers.md) |
| `internal/auth/token.go` `parseRSAPublicKey` | No fuzz | [fuzz-parsers](fuzz-parsers.md) |
| `pkg/types/` | No package-level doc comment | [package-types-godoc](package-types-godoc.md) |
| `.golangci.yml:13-22` | `fmt.Fprint*` exclusion is global | [golangci-errcheck-scope](golangci-errcheck-scope.md) |
| `README.md:25,29` | Image links resolve to missing files | [readme-screenshots](readme-screenshots.md) |
| *(cross-repo)* `.agents/` layout absent | no shared roles / wrappers | [agents-framework](agents-framework.md) |
