# Wire the recovered signal into the chaos evaluator

> Status: **Done — 2026-04-16 (Option A shipped)**
> Opened: 2026-04-15
> Priority: **ship-critical**
> Est. effort: **M** (1 day)
> Decision required: **no — resolved 2026-04-15, Option A with fallback to B**
>
> Shipped summary: `proxy.Observation.Recovered()` (per-test, single-tool,
> "final hit OK after at least one fault") is wired into
> `cmd/ruptor/main.go:evaluateTest` and consumed by
> `internal/evaluator/rules/classify.go`. Recovery semantics committed in
> `docs/specs/proxy.md § Recovery semantics`. Integration coverage in
> `internal/proxy/observations_test.go::TestObservations_RecoverySignalFlipsOnRetry`
> and `cmd/ruptor/main_test.go::TestEvaluateTest_WiresRecoveredFromObservation`.
> MCP-handler observations are out of scope; deferred to
> `docs/specs/backlog/mcp-observations-evaluator.md`.

## Problem

`evaluateTest` hardcodes `recovered=false` on every call into the
chaos evaluator. The evaluator's classification code has a full
"recovery detected" branch that therefore never fires in any real
run. Either the signal is populated from the proxy observation
(agent retried N times then succeeded) or the recovery branch should
be deleted until a retry-aware proxy lands. Shipping with dead logic
that silently downgrades Robustness Scores is worse than either
option.

## Evidence

- `cmd/ruptor/main.go:486` — literal `false, // Recovered — wired
  when retry-aware proxy lands` inside `eval.Evaluate(...)`.
- `internal/evaluator/rules/classify.go` — `ClassifyExperiment` has a
  `RecoveryDetector` code path referenced from
  `internal/evaluator/evaluator.go`.
- `pkg/types/behavior.go` — recovery-related types referenced from
  the classify tests.
- `internal/evaluator/rules/classify_test.go` — unit-tested against
  synthetic recovery input, so the branch is covered in tests but
  never invoked in production.

## Proposed solution

Pick one of the two options in § Decision, then:

1. If **wire it up**:
   - Extend `proxy.Observation` with a `RetryCount` (or equivalent
     `Recovered bool`) computed from repeated hits on the same tool
     within a window.
   - Replace the hardcoded `false` in
     `cmd/ruptor/main.go:evaluateTest` with the observation-derived
     value.
   - Add an integration test in `internal/proxy/` asserting the
     signal flips when the mock agent retries.
2. If **remove until retry-aware proxy lands**:
   - Delete the recovery branch from
     `internal/evaluator/rules/classify.go`, the `RecoveryDetector`
     helper, and the associated `Recovered` parameter on
     `Evaluate`.
   - Update `internal/evaluator/rules/classify_test.go` and every
     caller.
   - Add a follow-up spec for reintroducing the signal.

## Acceptance criteria

- [ ] No `// wired when retry-aware proxy lands` TODOs remain.
- [ ] Either `recovered` is populated from a real observation in at
      least one shipped fault type, OR the evaluator no longer accepts
      the parameter.
- [ ] Tests cover the chosen path end-to-end (proxy observation ->
      evaluator -> report).
- [ ] Robustness Score behavior documented in
      `docs/specs/proxy.md` and/or a decisions ADR.

## Out of scope

- Defining "recovery" semantics across multi-tool agent flows — stick
  to "retried the same tool and eventually succeeded" scope.
- Simulate-mode recovery (conversation-level).

## Decision (resolved 2026-04-15)

**Option A — Wire it up now, with fallback to B.**

Explore deriving `recovered` from `proxy.Observation` (count repeated
hits to same tool within a window). If the implementation stays
contained to `internal/proxy/` + `internal/evaluator/` + `cmd/ruptor/`,
ship it. If it requires deeper proxy architecture changes (retry-aware
request tracking, session correlation), fall back to Option B (remove
the recovery branch entirely and open a follow-up spec).

Rationale: shipping v1 with a silently-disabled score axis is a
credibility risk for a reliability-testing tool. But shipping a
half-baked implementation is worse than removing it cleanly.

## References

- `cmd/ruptor/main.go:486`
- `internal/evaluator/rules/classify.go`
- `internal/evaluator/evaluator.go`
- `internal/proxy/observations.go`
- `pkg/types/behavior.go`
