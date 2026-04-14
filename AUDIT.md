# Ruptor CLI — AUDIT

Date: 2026-04-14
Scope: full repo snapshot before any new feature work.
Source authorities: `CLAUDE.md`, `../knowledge/architecture.md`, `../knowledge/engineering.md`, `../knowledge/security.md`, `../knowledge/v1-scope.md`, `../knowledge/decisions/001`–`009`.

---

## 1. Identity / naming (MUST change)

The repo is still the previous project, `faultforge`. The target is `ruptor` with module `github.com/ruptor-dev/cli` and binary `ruptor`.

| Location | Current | Target |
|---|---|---|
| `go.mod` module | `github.com/faultforge/faultforge` | `github.com/ruptor-dev/cli` |
| `cmd/` package dir | `cmd/faultforge/` | `cmd/ruptor/` |
| Cobra `Use:` root | `faultforge` (cmd/faultforge/main.go:38) | `ruptor` |
| `version` command | `fmt.Printf("faultforge %s\n", …)` (main.go:388) | `ruptor` + go through ui |
| Makefile `BINARY` | `./bin/faultforge` | `./bin/ruptor` |
| Makefile build target | `./cmd/faultforge` | `./cmd/ruptor` |
| README.md title + install | `FaultForge`, `go install …/faultforge/faultforge/cmd/faultforge@latest` | `Ruptor`, `go install github.com/ruptor-dev/cli/cmd/ruptor@latest` |
| `.claude/launch.json` names + paths | `faultforge-chaos`, `faultforge-simulate`, `./cmd/faultforge` | `ruptor-*`, `./cmd/ruptor` |
| `CLAUDE_CODE_PROMPT.md` | entire prompt is "FaultForge … monorepo `github.com/faultforge/faultforge`" | stale; delete or replace — superseded by `CLAUDE.md` + `../knowledge/` |
| `web/chaos_report.html.tmpl` title + H1 | `FaultForge Reliability Report` | `Ruptor Reliability Report` |
| `web/simulate_report.html.tmpl` title + H1 | `FaultForge Simulation Report` | `Ruptor Simulation Report` |
| `internal/report/html_renderer.go` `fallbackChaosHTML` / `fallbackSimulateHTML` | `FaultForge …` | `Ruptor …` |
| `internal/report/stdout_renderer.go` banner text | `FaultForge Reliability Report` / `FaultForge Simulation Report` (lines 20, 61) | `Ruptor …` |
| `cmd/faultforge/main.go` log lines | `"faultforge chaos proxy starting"`, `"faultforge simulation starting"` | `"ruptor …"` |
| import paths across 40+ files | `github.com/faultforge/faultforge/...` | `github.com/ruptor-dev/cli/...` |
| Stray compiled binary at repo root | `./faultforge` (13.5 MB, checked in untracked) | delete, add to `.gitignore` |

Grep snapshot: 44 files contain `faultforge` / `FaultForge` (case-insensitive).

---

## 2. Fully implemented and working

- `pkg/types/`: `Fault`, `FaultType` consts (6 values), `DetectedBehavior` consts, `TestResult`, `SimulationResult`, `Turn`, `ConversationHistory` (+ `Add`, `ToOpenAIMessages`), `ReliabilityReport`, `ConversationReport`.
- `internal/config/`: `ChaosConfig`, `SimulateConfig`, `LoadChaos`, `LoadSimulate`, validation with `errors.Join`. Tests exist (`loader_test.go` 10.6 KB).
- `internal/proxy/faults/`: registry + 6 faults implemented with table-driven tests (`tool_timeout`, `slow_response`, `tool_error`, `invalid_json`, `empty_response`, `rate_limit`). Factory pattern per `ADR-engineering.md` clean code rules.
- `internal/proxy/`: `Proxy` struct with options pattern (`WithLogger`, `WithTimeout`, `WithRandSource`), `ServeHTTP`, lazy-initialised `httputil.ReverseProxy`, graceful shutdown, path-exact test matching, probability roll with injectable `rand.Source`. Tests exist.
- `internal/evaluator/`: `ChaosEvaluator` (detectors + LLM judge composition), `SimulateEvaluator`; `Detector` interface + 3 detectors (loop/crash/recovery) with tests.
- `internal/evaluator/llmjudge/`: `Judge` interface, `OpenAIJudge`, `NoopJudge`, response parsers.
- `internal/llmclient/`: shared `OpenAIClient` with options, `Complete`, `CompleteMap`. Tests exist (`openai_test.go`).
- `internal/simulate/`: `Simulator.Run`, `PersonaPromptBuilder`, `LLMClient` interface, HTTP-or-LLM agent call, turn loop, goal detection via success_criteria substring match. Tests exist.
- `internal/report/`: `Renderer` interface, `StdoutRenderer`, `JSONRenderer`, `HTMLRenderer` (template from disk w/ embedded fallback), `MultiRenderer`, `NewRendererFromFormat`. Tests exist.
- `cmd/faultforge/main.go`: Cobra root + `run`, `simulate`, `validate`, `version` subcommands wired end-to-end for the existing feature set.
- `configs/chaos.example.yaml`, `configs/simulate.example.yaml`, `testdata/chaos_valid.yaml`, `testdata/chaos_invalid.yaml`, `testdata/simulate_valid.yaml`.
- `web/chaos_report.html.tmpl`, `web/simulate_report.html.tmpl`.
- `Makefile` targets: `build`, `test`, `lint`, `check`, `run-example`, `run-simulate-example`, `clean`, `tools`, `help`.

---

## 3. Partially implemented / needs work

### 3.1 `ruptor run` end-to-end
`runChaos` in cmd/faultforge/main.go:81 starts the proxy and then blocks on ctx, but the evaluator and renderer are `_ = eval; _ = renderer` (main.go:169-170). The comment at main.go:166 admits this is a placeholder. Consequence: `ruptor run` never produces a chaos report. Only the proxy side is live.

### 3.2 Simulate agent contract
`internal/simulate/simulator.go:generateAgentResponse` (line 126) does not match ADR-009:
- No auth (`bearer | basic | header | none`).
- No `X-Ruptor-Session-Id`, `X-Ruptor-Run-Id`, `X-Ruptor-Turn`, `X-Ruptor-Fault-*` headers.
- No `response_format: auto | openai | anthropic | custom` — only reads `response` then `message` top-level fields.
- OpenAI `choices[0].message.content` is NOT the first-priority parse (contradicts ADR-009 priority order).
- No SSE streaming support.
- No custom `response_field` override.
- No `metadata` / `session_id` / `run_id` / `turn` / `persona` / `goal` in request body.
- Agent config is missing `protocol`, `endpoint`, `response_format`, `streaming`, `response_field`, `auth` (simulate_config.go:17).

### 3.3 Config schema
- `version: "1"` field (v1-scope "in", line 39 of v1-scope.md) absent from both `ChaosConfig` and `SimulateConfig`. Not validated.
- `schema_version` absent from report JSON output (contradicts ADR-007).
- `ruptor_version` absent from report JSON output.

### 3.4 Chaos report pipeline
`internal/evaluator/evaluator.go:ChaosEvaluator.Evaluate` takes `(statusCode, iterations, hadError, recovered, prompt, agentBehavior)` — but there is no caller that supplies those. Nothing in cmd/ or proxy/ collects those observations. So the evaluator exists but is never invoked. Same for the rule detectors and `BuildReliabilityReport` (there is no such helper).

### 3.5 HTML renderer template path
`HTMLRenderer.loadTemplate` reads `web/chaos_report.html.tmpl` and `web/simulate_report.html.tmpl` with a relative path (html_renderer.go:50, 59). This breaks when `ruptor` is run from a directory other than the repo root. Should embed with `//go:embed` once renamed.

### 3.6 Goal detection (simulate)
`checkGoalReached` in simulator.go:193 does a lowercased `strings.Contains(agentResponse, successCriteria)`. The example configs use criteria like `"The agent completed the cancellation"` — the real agent output will rarely literally contain that string. This is a functional gap, not just a style issue: most real simulations will wrongly return `GoalReached: false`. The ADR-009 / v1 design expects the LLM judge or an explicit signal to determine goal completion.

### 3.7 `runSimulate` evaluator wiring
cmd/faultforge/main.go:287 only runs the evaluator when `cfg.Evaluation.LLMJudgePrompt != ""`. When the prompt is empty but `goal_completion: true`, nothing evaluates. The rest of the eval toggles (`turn_efficiency`, `tone_quality`) are parsed but unused.

### 3.8 Output routing / formatting
- `cmd/faultforge/main.go` calls `fmt.Printf` directly for the `version` subcommand (main.go:388) — violation of CLAUDE.md "no `fmt.Println`/`fmt.Printf` in `cmd/` or `internal/` except ui".
- `internal/report/stdout_renderer.go` uses `fmt.Fprintf` for terminal output with hand-written unicode box-drawing. Policy says terminal output must go through `internal/ui/` using Bubbletea + Lipgloss v2. There is no `internal/ui/` package at all.

---

## 4. Missing from v1 scope (to be built after audit, new work)

These are in `../knowledge/v1-scope.md` CLI-IN and absent from the codebase.

| Area | Status |
|---|---|
| Fault `llm_error` | not implemented; not in `FaultType` enum (pkg/types/fault.go:7) |
| Fault `llm_timeout` | not implemented; not in `FaultType` enum |
| MCP proxy mode (`proxy.protocol: mcp`) | no `internal/proxy/mcp/` package, no JSON-RPC 2.0 interception |
| `ruptor auth login` (OAuth device flow) | no `internal/auth/`, no command |
| `ruptor auth status` | missing |
| `ruptor auth logout` | missing |
| `ruptor doctor` | missing |
| `ruptor update` | missing |
| `ruptor sync` | missing |
| `~/.ruptor/config.yaml` + Viper hierarchy | uses `gopkg.in/yaml.v3` directly; no Viper, no env, no `.ruptor.yaml` precedence |
| `--cloud` flag | not present; no `internal/cloud/feature.go`; `CloudReportingEnabled` constant missing |
| Reporting client + retry with jitter | no `cenkalti/backoff/v4`, no `~/.ruptor/pending/` spooler |
| OTel opt-in telemetry | no `internal/telemetry/`, no `go.opentelemetry.io/otel` |
| Robustness Score (0.0–1.0) prominently displayed | `ReliabilityReport.Score` is `int` (percent), not 0.0–1.0 float; no "Robustness Score" label |
| Bubbletea TUI with live progress | no `internal/ui/`, no `charmbracelet/*` deps |
| Ruptor theme (lipgloss) — #E84545, #22C55E, etc. | no `internal/ui/theme.go` |
| goreleaser multi-arch + cosign | no `.goreleaser.yaml`, no release workflow |
| `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, issue templates | missing |
| `version: "1"` required in chaos.yaml / simulate.yaml | not in schema, not validated |
| LLM judge prompts as `.txt` files in `internal/evaluator/llmjudge/prompts/` | prompts are hardcoded string literals in `openai_judge.go:41` and `openai_judge.go:67` — violates architecture.md "Never hardcode prompts" |
| Port auto-detection (ADR-008) | `proxy.Start` fails with `address already in use` if port 8080 busy; no increment, no `RUPTOR_PROXY_PORT` env var handling |
| Structured logger choice | uses stdlib `slog`; stack mandates `zerolog` |

---

## 5. In repo but OUT of v1 scope (defer, do not expand)

None of the OUT items (Behavioral Contracts, Replay Regression, adversarial inputs, CI/CD integration, MCP simulate protocol, custom fault types, fault combinations, platform code) are present. The codebase is under-scoped, not over-scoped.

Legacy artifacts that should be removed or reframed as they pre-date the Ruptor direction:
- `CLAUDE_CODE_PROMPT.md` — 16 KB Spanish "FaultForge" distinguished-engineer prompt. Superseded by `CLAUDE.md` + `../knowledge/`. Delete or archive.
- `./faultforge` binary (13.5 MB) at repo root — build artifact, not gitignored.
- Deleted-in-git files per `git status`: `docs/manual-en.md`, `docs/manual-es.md`, `docs/use-cases.md`, `docs/superpowers/specs/2026-04-06-notion-tidy-design.md`. Removals look correct; ensure the commit that deletes them is clean.
- `docs/superpowers/specs/SKILL-*.md` (auth, proxy, testing, ui) — untracked; keep if they are the agent skill files the future work will use, otherwise move them under a clearer `docs/specs/`.

---

## 6. Security / policy violations

| Rule | Violation |
|---|---|
| "No `fmt.Println`/`fmt.Printf` in `cmd/` or `internal/` except `ui/`" | `cmd/faultforge/main.go:388` (`fmt.Printf` for version), `internal/report/stdout_renderer.go` uses `fmt.Fprintf` (the whole file renders to stdout without going through `internal/ui/`). |
| "Never log tool response bodies / user data" | Not violated today — proxy logs `path`, `fault_type`, `tool`, `test_id` only. Simulate logs user/agent message content (simulator.go:70, 87) — REVIEW: simulate conversation content is generated by the simulator itself (not user PII from a tool), but the policy still says "never log content". Consider demoting these `Info` lines to `Debug` or behind a verbose flag. |
| "InsecureSkipVerify = false always" | Not violated — no TLS config anywhere yet. |
| "No secrets in code" | Not violated — `OPENAI_API_KEY` via env. But `RUPTOR_TEST_*` test-key convention not yet adopted. |
| "Interfaces before implementation in `pkg/types/`" | `Fault` interface lives in `pkg/types/fault.go` ✓. `Judge`, `Detector`, `Renderer`, `LLMClient`, `FaultHandler` chain-handler contracts — live in `internal/` only. Per CLAUDE.md rules these "public API" contracts belong in `pkg/types/` for the platform repo to import (ADR-001 + architecture.md "Shared types live in `cli/pkg/types/`"). |

---

## 7. Contradictions with `../knowledge/` decisions

| Decision | Code state |
|---|---|
| ADR-001 two-repo / shared types | `pkg/types/` exists but is thin; interfaces (`Judge`, `Detector`, `Renderer`, `FaultHandler`) are internal, so the platform repo cannot depend on them. |
| ADR-002 proxy architecture — Chain of Responsibility | Implementation is a registry + factory per-request (`FaultRegistry.Build`), not a chain. Functionally close, but the ADR wording says "chain of handlers". Low-cost rename / restructure; behavior is sound. |
| ADR-007 report schema versioning | `schema_version` + `ruptor_version` missing from JSON output. |
| ADR-008 port auto-detection | Not implemented. |
| ADR-009 simulate contract | See §3.2 above — major gap. |
| `architecture.md` "prompts live in `.txt`" | Violated — hardcoded Go strings (§4). |
| `engineering.md` "never `panic()` in library code" | Not violated today. |
| `engineering.md` Go 1.26.2 | `go.mod` pins `go 1.22`. |
| `engineering.md` cobra v2.5.1 | `go.sum` shows `cobra v1.10.2`. |
| CLAUDE.md stack table (zerolog, viper, bubbletea, otel, backoff, goreleaser) | None of these deps are in `go.mod`. |

---

## 8. Punch list (dependency-ordered)

1. **Rename** (mechanical, no behavior change). go.mod module path, cmd dir, binary, strings, imports, launch.json, README.md, web templates, stdout banners, Cobra `Use:` + log lines, `CLAUDE_CODE_PROMPT.md` disposition, `./faultforge` binary + `.gitignore`.
2. **Delete `fmt.Printf` in `cmd/ruptor/main.go:version`** — route through a minimal temporary helper until `internal/ui/` is stood up.
3. **Wire chaos report pipeline**: agent-orchestration scaffold so `runChaos` actually collects observations, calls `ChaosEvaluator`, builds `ReliabilityReport`, calls renderer. Until this works, `ruptor run` produces nothing.
4. **Fix simulate agent contract** to ADR-009: response priority order, auth config, `X-Ruptor-*` headers, response_format, streaming opt-in, metadata body. Add config fields to `SimAgentConfig`.
5. **Fix goal detection**: stop using `strings.Contains(agentResponse, successCriteria)`. Drive goal-reached via the LLM judge or an explicit persona-emitted signal token.
6. **Schema versioning**: add `Version string `yaml:"version"`` to both configs (required = `"1"`), add `SchemaVersion`/`RuptorVersion` to report JSON.
7. **Extract prompts** to `internal/evaluator/llmjudge/prompts/*.txt` + `//go:embed`.
8. **Port auto-detect** per ADR-008; add `RUPTOR_PROXY_PORT` env var.
9. **Switch stack to match spec**: zerolog (replace slog), viper (config hierarchy), bubbletea + lipgloss v2 + bubbles (new `internal/ui/`), backoff v4, otel v1. Go 1.26.2, cobra v2.
10. **Build new packages**: `internal/ui/`, `internal/auth/`, `internal/cloud/` (with `CloudReportingEnabled = false`), `internal/telemetry/`, `internal/proxy/mcp/`.
11. **Add v1 faults**: `llm_error`, `llm_timeout` (enum + handlers + tests).
12. **Add v1 subcommands**: `auth login|status|logout`, `doctor`, `update`, `sync`.
13. **Release**: `.goreleaser.yaml` + cosign + GitHub Actions release workflow.
14. **OSS hygiene**: `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, issue templates, LICENSE check (Apache 2.0).
15. **Robustness Score** typed as `float64` 0.0–1.0 in `ReliabilityReport`; update renderers.

Items 1–2 are mechanical and unblock everything else. Items 3–8 close gaps against current spec. Items 9–15 are the remainder of v1 scope and should each be separate PRs.

---

## 9. Applied in this audit pass

Punch-list items 1–8 landed as one commit each on `main`:

| Item | Commit | Summary |
|---|---|---|
| 0 (prep) | `85598b8` | `.gitignore` for build artifacts + OS files |
| 0 (prep) | `78e6f25` | Squash pre-audit WIP (detector interface unification, rng injection, ConversationHistory on result, llmclient extraction, doc cleanup) |
| 0 (prep) | `8d531ff` | Add `CLAUDE.md`, `AUDIT.md`, `docs/superpowers/specs/SKILL-*.md`; drop stale `CLAUDE_CODE_PROMPT.md` |
| 1 | `d29737a` | Rename `faultforge` → `ruptor` (module, cmd dir, binary, strings, imports, templates) |
| 2 | `21986a7` | Route `version` cmd through new `internal/ui/` boundary |
| 8 | `9926349` | Port auto-detect on startup (ADR-008) |
| 7 | `2a9b30d` | Extract judge prompts to embedded `.txt` files |
| 6 | `151fd6e` | `version: "1"` required in configs; `schema_version` + `ruptor_version` in reports (ADR-007) |
| 4 | `4fd41ec` | Full ADR-009 simulate contract (response priority, `X-Ruptor-*` headers, auth, metadata, env-var token) |
| 5 | `6b3e93f` | `GoalChecker` interface + LLM-backed default (replaces substring match) |
| 3 | `b174c56` | Proxy observations + end-to-end chaos report pipeline |

Test suite: **143 passing** across 13 packages. `go build ./...` and `go vet ./...` both green.

---

## 10. Deferred to later PRs

The stack-swap PR (see §11) closed §8 item 9. Remaining items deserve isolated PRs per CLAUDE.md "one logical change per PR":

- **New packages**: `internal/auth/` (OAuth device flow), `internal/cloud/` (`CloudReportingEnabled = false` feature flag + pending-report spooler), `internal/proxy/mcp/` (JSON-RPC 2.0 tool call interception). `internal/telemetry/` shipped as a stub in the stack-swap PR; the OTLP exporter wiring is part of the `ruptor auth login` PR because it needs the token.
- **New v1 faults**: `llm_error`, `llm_timeout` (enum + handlers + tests).
- **New v1 subcommands**: `auth login|status|logout`, `doctor`, `update`, `sync`.
- **Release + OSS hygiene**: `.goreleaser.yaml`, cosign signing, GitHub Actions release workflow, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, issue templates.
- **Robustness Score**: retype `ReliabilityReport.Score` as `float64` in `[0.0, 1.0]`; update renderers.
- **Chaos pipeline follow-ups**: `Recovered` signal (requires retry-aware proxy), agent-behavior transcript for LLM judge input, `Open report? [Y/n]` completion prompt.
- **TUI follow-up**: replace the 100ms tick-poller (TUI reads `proxy.Observations()` periodically) with a push channel from proxy → UI for lower-latency state updates.
- **Test coverage gaps surfaced by the stack-swap PR**:
  - Unit tests for `parseChaosResponse` and `parseConversationResponse` in `internal/evaluator/llmjudge`. The `OpenAIJudge` struct is at 0% coverage because the parsers run only inside API calls; extract them or test them directly.
  - Integration test that spawns the `ruptor` binary against an `httptest.Server` backend. Covers `cmd/ruptor` (currently 0%) end-to-end.

### Known non-blocking issues

- **golangci-lint spurious log line.** Both `v1.64.8` and `v2.11.4` built against Go 1.26.2 emit a `level=error msg="[linters_context] typechecking error: stat …/cli/<first-arg>: directory not found"` line on every run. Exit code is still 0; `0 issues.` is reported. Not a source problem — looks like a golangci-lint↔Go 1.26 incompatibility. Ignore until a fixed linter release ships.

## 11. Stack-swap PR (landed)

| Commit | Summary |
|---|---|
| `1f82406` | Correct CLAUDE.md stack table to real published versions |
| `ef5f9f6` | Replace stdlib `log/slog` with `github.com/rs/zerolog` |
| `f86cbf4` | Viper-backed `Settings` + `RUPTOR_*` env overrides |
| `69be2c4` | OTel tracing stub (`internal/telemetry`) |
| `40e14f9` | Retry-with-jitter tests for llmclient (`backoff/v4`) |
| `9df5c82` | Bubbletea TUI for `ruptor run` + `.golangci.yml` |

Net result: `make check` green, 167 tests passing, 14 packages. Go 1.26.2, zerolog, viper v1.21.0, bubbletea v2.0.5, lipgloss v2.0.3, otel v1.43.0, backoff/v4 v4.3.0. Cobra stays on v1.10.2 and viper stays on v1.21.0 because the spec's v2.5.1 / v1.31+ versions never shipped publicly.
