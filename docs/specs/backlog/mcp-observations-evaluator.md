# MCP observations not wired to evaluator

> Status: **Backlog**
> Opened: 2026-04-15
> Priority: **ship-critical** (MCP is a v1 feature; must have working scores)
> Est. effort: **S** (half day)
> Decision required: **no**

## Problem

The MCP handler (`internal/proxy/mcp/handler.go`) maintains its own
`Observation` struct and per-test observation map, but this data never
reaches the evaluator. In MCP mode the orchestrator calls
`p.Observations()` on the `Proxy`, which only returns the HTTP
handler's observations. The MCP handler's observations are silently
discarded. As a result, every test in an MCP experiment scores as
"not exercised" — zero hits, zero faults — and the Robustness Score
is meaningless.

## Evidence

- **`internal/proxy/mcp/handler.go:34-40`** — defines its own
  `Observation` struct, identical to `proxy.Observation` but in a
  separate package to avoid an import cycle. Accumulates hits and
  faults in `h.obs`.

- **`internal/proxy/mcp/handler.go:94-103`** — `Handler.Observations()`
  returns `map[string]Observation`, but nothing calls it during
  evaluation.

- **`internal/proxy/observations.go:97-99`** — `Proxy.MCPHandler()`
  returns `interface{}`. The accessor exists but the return type is
  untyped, so callers cannot access MCP observations without a type
  assertion to a package they may not import.

- **`cmd/ruptor/main.go:223`** — `buildChaosReport` receives
  `p.Observations()`, which only contains HTTP-path observations.
  No code path merges MCP observations into this map.

- **`cmd/ruptor/main.go:304`** — `snapshotFn` similarly reads only
  `p.Observations()`, so the live progress UI also shows zero hits
  for MCP tests.

## Proposed solution

Unify the observation path so both HTTP and MCP handlers write to a
single observation store the evaluator already consumes.

**Option A — Shared observation store (recommended)**

Extract `Observation` and the `map[string]*Observation` + mutex into
a small shared type (e.g., `internal/proxy/obsstore` or
`pkg/types/observation.go`). Both the HTTP handler and the MCP handler
hold a pointer to the same store. `Proxy.Observations()` reads from
that store, so no changes are needed in `cmd/ruptor/main.go` or the
evaluator.

**Option B — Callback interface**

Define an `ObservationSink` interface in `internal/proxy`:

```go
type ObservationSink interface {
    RecordFault(testID string, statusCode int)
    RecordPassthrough(testID string, statusCode int)
}
```

Pass the `Proxy` (which already implements these methods) to the MCP
handler at construction time. The MCP handler calls the sink instead
of maintaining its own map. Eliminates the duplicate `Observation`
struct entirely.

Either option works. Option B is slightly simpler (no new package) but
couples the MCP handler to the proxy interface. Option A is cleaner
for testing in isolation.

## Acceptance criteria

- [ ] MCP mode produces observations the evaluator can consume — the
      same `map[string]Observation` returned by `p.Observations()`
      includes MCP hits and faults.
- [ ] Robustness Score reflects MCP fault injection results (non-zero
      scores when the agent exercises MCP tool calls).
- [ ] Live progress UI (`snapshotFn`) shows real-time MCP hits.
- [ ] HTTP-only mode is unaffected (no regression).
- [ ] The duplicate `mcp.Observation` struct is removed or aliased to
      the shared type.
- [ ] Integration test: run an MCP experiment, verify observations are
      non-empty and evaluator scores are populated.

## References

- `internal/proxy/mcp/handler.go`
- `internal/proxy/observations.go`
- `cmd/ruptor/main.go` — `buildChaosReport`, `snapshotFn`
- `internal/evaluator/evaluator.go`
