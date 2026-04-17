# Backlog specs

Each file in this directory is a scoped ticket for a future session.
Self-contained: a developer should be able to execute any one of them
without the context of the session that opened it. Sorted by priority.

| Spec | Priority | Effort | Decision? | Status | One-liner |
|------|----------|--------|-----------|--------|-----------|
| ~~[security-md](security-md.md)~~ | ship-critical | XS | no | ✅ done | `SECURITY.md` created at repo root. |
| ~~[goreleaser-footer-bundle-format](goreleaser-footer-bundle-format.md)~~ | ship-critical | XS | no | ✅ done | Release footer uses `--bundle` (cosign v2). |
| ~~[auth-status-cloud-row](auth-status-cloud-row.md)~~ | high | XS | no | ✅ done | `ruptor auth status` shows Cloud row. |
| ~~[output-paths-both-format](output-paths-both-format.md)~~ | high | XS | no | ✅ done | `format: both` shows HTML + JSON paths. |
| ~~[knowledge-architecture-stack-drift](knowledge-architecture-stack-drift.md)~~ | high | XS | no | ✅ done | `knowledge/architecture.md` synced to real versions. |
| ~~[auth-silent-refresh-wiring](auth-silent-refresh-wiring.md)~~ | ship-critical | S | no | ✅ done | `MaybeRefresh` wired into `runSync` + `runAuthStatus`. |
| ~~[make-test-int-target](make-test-int-target.md)~~ | ship-critical | S | no | ✅ done | `make test-int` + build tag + CI job. Subsumed by testing-comprehensive-plan. |
| ~~[package-types-godoc](package-types-godoc.md)~~ | medium | XS | no | ✅ done | `pkg/types/doc.go` + API stability in CONTRIBUTING.md. |
| ~~[golangci-errcheck-scope](golangci-errcheck-scope.md)~~ | medium | XS | no | ✅ done | errcheck scoped to `internal/ui/` + `internal/report/`. |
| ~~[cloud-client-auth-header](cloud-client-auth-header.md)~~ | ship-critical | S | **resolved** | ✅ done | Option B: TokenSupplier closure on every request. |
| ~~[evaluator-recovered-signal](evaluator-recovered-signal.md)~~ | ship-critical | M | **resolved** | ✅ done | Option A shipped: `proxy.Observation.Recovered()` wired into `evaluateTest`; recovery semantics documented. |
| ~~[otel-exporter-wiring](otel-exporter-wiring.md)~~ | high | XS | **resolved** | ✅ done | Option B: deferred to v2, v1-scope updated, ADR-010 filed. Must ship in v2. |
| ~~[mcp-proxy-mode](mcp-proxy-mode.md)~~ | ship-critical | L | **resolved** | ✅ done | MCP proxy v1 shipped: Streamable HTTP, `tools/call` only. stdio/SSE/non-tool-call/WebSocket → v2. See ADR-011. |
| ~~[mcp-observations-evaluator](mcp-observations-evaluator.md)~~ | ship-critical | S | **resolved** | ✅ done | Narrow `types.ObservationSink` in `pkg/types/` routes MCP observations into `*proxy.Proxy`; `SetActiveTest` / `ResetObservation` preserved. |
| [mcp-sse-streaming](mcp-sse-streaming.md) | medium | M | no | pending | SSE streaming deferred from MCP v1 — faults not injected on SSE responses. |
| [ui-stdout-stderr-split](ui-stdout-stderr-split.md) | medium | S | **yes** | pending | Route `ui.Success`/`Error`/`Info`/`Warning`/`Dim` to stderr; previously landed then reverted, needs redesign + test migration. |
| ~~[tui-log-panel-scrollable](tui-log-panel-scrollable.md)~~ | medium | M | no | ✅ done | `bubbles/v2` viewport wired into the `-v` log panel: scroll keys, mouse wheel, `s` toggles auto-tail, title shows `tail`/`paused (s)`. |
| [proxy-mode-decode-validator](proxy-mode-decode-validator.md) | low | XS | no | pending | `ProxyMode.UnmarshalYAML/Text` — reject non-canonical values at decode, not just at Validate. |
| ~~[cobra-silence-errors](cobra-silence-errors.md)~~ | medium | XS | no | ✅ done | Option A: errors printed via `ui.Error` in `main`; integration tests cover unknown-subcmd and missing-arg paths. |
| [testing-comprehensive-plan](testing-comprehensive-plan.md) | high | XL | **resolved** | pending | 60-category testing plan, all decisions locked. Supersedes `testing-coverage-matrix.md`. Subsumes `fuzz-parsers`, `examples-ci-smoke`, `make-test-int-target`. |
| [examples-ci-smoke](examples-ci-smoke.md) | medium | M | no | subsumed | Subsumed by testing-comprehensive-plan (#3 system, #31 smoke). |
| [fuzz-parsers](fuzz-parsers.md) | medium | S | no | subsumed | Subsumed by testing-comprehensive-plan (#21 fuzz). |
| ~~[testing-coverage-matrix](testing-coverage-matrix.md)~~ | — | — | — | superseded | Replaced by `testing-comprehensive-plan.md`. |
| [readme-screenshots](readme-screenshots.md) | medium | S | no | pending | README embeds two images that do not exist on disk. |
| [agents-framework](agents-framework.md) | nice-to-have | M | no | deferred | Cross-repo `.agents/` layout — post-v1. |

## Workflow for opening a new spec

1. Copy the template from any existing file in this directory.
2. Cite file:line where the gap is visible (verify the line is
   current before committing).
3. If the spec requires a decision, list options with pros/cons and
   give a DE recommendation.
4. Add an entry to the table above. Keep it sorted by priority.
5. If the gap corresponds to visible code (a hardcoded `false`, a
   TODO comment, a disabled branch) add a row to `_gaps.md`.
