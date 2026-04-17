# Backlog specs

Each file in this directory is a scoped ticket for a future session.
Self-contained: a developer should be able to execute any one of them
without the context of the session that opened it.

## Pending (spec file present, ready to pick up)

| Spec | Priority | Effort | Decision? | One-liner |
|------|----------|--------|-----------|-----------|
| [ui-stdout-stderr-split](ui-stdout-stderr-split.md) | medium | S | **yes** | Route `ui.Success`/`Error`/`Info`/`Warning`/`Dim` to stderr; previously landed then reverted, needs redesign + test migration. |
| [mcp-sse-streaming](mcp-sse-streaming.md) | medium | M | no | SSE streaming deferred from MCP v1 — faults not injected on SSE responses. v2. |
| [proxy-mode-decode-validator](proxy-mode-decode-validator.md) | low | XS | no | `ProxyMode.UnmarshalYAML/Text` — reject non-canonical values at decode, not just at Validate. |

## Needs drafting (referenced but spec file missing)

These rows existed in a previous README but their `.md` files were never
checked in. A future session must draft the spec itself (citing
file:line evidence) before any implementation.

| Name | Priority (claimed) | Notes |
|------|---------------------|-------|
| `testing-comprehensive-plan` | high / XL | 60-category testing plan; previous README claimed "all decisions locked" but no spec file ever landed in git. Intended to supersede `testing-coverage-matrix`, and subsume `fuzz-parsers`, `examples-ci-smoke`, `make-test-int-target`. Draft before implementation. |
| `readme-screenshots` | medium / S | Top-level `README.md` embeds two images that do not exist on disk — confirm scope, then draft. |
| `agents-framework` | nice-to-have / M / deferred post-v1 | Cross-repo `.agents/` layout. |

## Shipped (historical, for trail — spec files may be removed)

| Spec | Priority | Effort | Note |
|------|----------|--------|------|
| ~~security-md~~ | ship-critical | XS | `SECURITY.md` created at repo root. |
| ~~goreleaser-footer-bundle-format~~ | ship-critical | XS | Release footer uses `--bundle` (cosign v2). |
| ~~auth-status-cloud-row~~ | high | XS | `ruptor auth status` shows Cloud row. |
| ~~output-paths-both-format~~ | high | XS | `format: both` shows HTML + JSON paths. |
| ~~knowledge-architecture-stack-drift~~ | high | XS | `knowledge/architecture.md` synced to real versions. |
| ~~auth-silent-refresh-wiring~~ | ship-critical | S | `MaybeRefresh` wired into `runSync` + `runAuthStatus`. |
| ~~make-test-int-target~~ | ship-critical | S | `make test-int` + build tag + CI job. |
| ~~package-types-godoc~~ | medium | XS | `pkg/types/doc.go` + API stability in CONTRIBUTING.md. |
| ~~golangci-errcheck-scope~~ | medium | XS | errcheck scoped to `internal/ui/` + `internal/report/`. |
| ~~cloud-client-auth-header~~ | ship-critical | S | Option B: TokenSupplier closure on every request. |
| ~~[evaluator-recovered-signal](evaluator-recovered-signal.md)~~ | ship-critical | M | Option A: `proxy.Observation.Recovered()` wired into `evaluateTest`. |
| ~~[otel-exporter-wiring](otel-exporter-wiring.md)~~ | high | XS | Deferred to v2 (ADR-010). |
| ~~[mcp-proxy-mode](mcp-proxy-mode.md)~~ | ship-critical | L | MCP proxy v1: Streamable HTTP, `tools/call` only (ADR-011). |
| ~~[mcp-observations-evaluator](mcp-observations-evaluator.md)~~ | ship-critical | S | Narrow `types.ObservationSink` in `pkg/types/`; MCP observations land in `*proxy.Proxy.Observations()`. `SetActiveTest`/`ResetObservation` preserved. |
| ~~[cobra-silence-errors](cobra-silence-errors.md)~~ | medium | XS | Option A: errors printed via `ui.Error` in `main`. |
| ~~[tui-log-panel-scrollable](tui-log-panel-scrollable.md)~~ | medium | M | `bubbles/v2` viewport in `-v` log panel: scroll keys, mouse wheel, `s` toggle, title `tail`/`paused (s)`. AltScreen + MouseCellMotion enabled when panel mounts. Pty-based `tuismoke` regression harness + `make smoke-tui`. |

## Workflow for opening a new spec

1. Copy the template from any existing spec file in this directory.
2. Cite `file:line` where the gap is visible (verify the line is
   current before committing).
3. If the spec requires a decision, list options with pros/cons and
   give a DE recommendation.
4. Add a row to the Pending table above (sorted by priority).
5. If the gap corresponds to visible code (a hardcoded `false`, a
   TODO comment, a disabled branch) add a row to `_gaps.md`.
