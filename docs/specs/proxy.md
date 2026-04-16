> Status: **Shipped** (v0.0.1-rc1) — includes MCP Streamable HTTP (`tools/call` only)
> Last updated: 2026-04-16
> Gaps tracked in: docs/specs/backlog/_gaps.md

## Implementation status

### Shipped
- Registry + Factory pattern for faults — `internal/proxy/faults/registry.go`, `internal/proxy/faults/fault.go`.
- All 8 fault types registered and unit+integration tested: `tool_timeout`, `slow_response`, `tool_error`, `invalid_json`, `empty_response`, `rate_limit`, `llm_error`, `llm_timeout` — see enum in `pkg/types/fault.go` and registrations in `internal/proxy/faults/registry.go`.
- `tool_timeout` honors `delay_ms` before emitting 504 — `internal/proxy/faults/tool_timeout.go`.
- Per-experiment dispatch pinning via `proxy.SetActiveTest(id)` — prevents collision when multiple tests share the same `tool` path — `internal/proxy/proxy.go`.
- Per-experiment observation reset via `proxy.ResetObservation(id)` — `internal/proxy/proxy.go`.
- Port auto-detection (`Port: 0`) handled by `proxy.Start` — `internal/proxy/proxy.go`.
- Observation tracking (`Hits`, `FaultsInjected`, `HadError`, `LastStatusCode`) exposed via `proxy.Observations()`.
- Recovery signal derived from the observation via `proxy.Observation.Recovered()` and fed into the evaluator — see § Recovery semantics below.

### Shipped — MCP (scoped)
- MCP JSON-RPC 2.0 proxy mode (`proxy.mode: mcp | auto`) —
  `internal/proxy/mcp/handler.go`, wired via
  `internal/proxy/handler.go` + `internal/proxy/proxy.go`.
- Transport: **Streamable HTTP** (POST → JSON response).
- Interception scope: **`tools/call` only**. `initialize`,
  `resources/*`, `prompts/*`, `sampling/*`, `ping`, and all
  notifications pass through unmodified.
- `mode: auto` sniffs POST bodies via `mcp.IsMCPRequest` and routes
  JSON-RPC 2.0 to the MCP handler, everything else to the HTTP
  handler.
- Scoping rationale and deferred items: see ADR-011 and
  `docs/specs/backlog/mcp-proxy-mode.md` (Done — 2026-04-16).

### Deferred (v2 — documented, not shipped)
- MCP **stdio** transport — `docs/specs/backlog/mcp-proxy-mode.md` v2.
- MCP **SSE streaming** responses —
  `docs/specs/backlog/mcp-sse-streaming.md`.
- MCP **non-`tools/call` interception** —
  `docs/specs/backlog/mcp-proxy-mode.md` v2.
- MCP **observations → evaluator wiring** —
  `docs/specs/backlog/mcp-observations-evaluator.md`.
- `ruptor simulate` over MCP — see `knowledge/v1-scope.md` OUT.
- MCP WebSocket transport — not in MCP spec as of 2026-04.

---

## Recovery semantics

`proxy.Observation.Recovered()` (defined in `internal/proxy/observations.go`)
is the sole authority on whether an experiment "recovered" from an
injected fault. The evaluator (`internal/evaluator/rules/classify.go`,
`RecoveryDetector` branch) and the runner wire-up
(`cmd/ruptor/main.go:evaluateTest`) both consult it; there are no other
sources of truth.

### Definition (committed — one semantic, no knobs)

An observation is **recovered** when all three hold:

1. `HadError == true` — at least one matching request got a fault
   response (HTTP >= 400 or a 0 status from a timeout fault).
2. `Hits > FaultsInjected` — the same tool path was hit at least one
   extra time beyond the faulted ones (at least one passthrough).
3. `LastStatusCode ∈ [200, 400)` — the most recent matching request
   was a successful (non-fault) response.

Implementation:

```go
func (o Observation) Recovered() bool {
    return o.HadError && o.Hits > o.FaultsInjected &&
        o.LastStatusCode >= 200 && o.LastStatusCode < 400
}
```

### Scope of the signal

- **One test window, one tool path.** The observation is per-test-ID
  and the proxy pins the active test during each experiment via
  `SetActiveTest`. Cross-tool reasoning ("agent failed on /search, fell
  back to /list") is NOT in scope — it is a v2 concern.
- **No session correlation.** We do not identify which agent iteration
  produced which hit. "Retry" means "the same path was hit again"; the
  signal is Markov on the observation, not on agent intent.
- **Final-hit semantics, not majority.** Five faulted hits followed by
  one successful hit counts as recovery. The evaluator's loop detector
  still fires on excessive iteration, so "recovered after looping 50
  times" surfaces as `recovery_success + infinite_loop`. The classifier
  then waives the loop failure (`isFailure` in classify.go) because
  the agent eventually made progress.
- **HTTP mode only.** MCP observations live in `internal/proxy/mcp/handler.go`
  and are not yet consumed by the evaluator — tracked in
  `docs/specs/backlog/mcp-observations-evaluator.md`. When that lands
  it will extend the same semantics to MCP `tools/call` hits.

### Truth table

Observation                                            | `Recovered()` | Rationale
-------------------------------------------------------|:-------------:|----------
`Hits=0`                                               | false         | Path never exercised.
`Hits=1, FaultsInjected=0, LastStatusCode=200`         | false         | No fault ever hit — nothing to recover from.
`Hits=1, FaultsInjected=1, LastStatusCode=500`         | false         | Fault hit, no retry.
`Hits=3, FaultsInjected=1, LastStatusCode=200`         | **true**      | Faulted once, subsequent hit passed.
`Hits=3, FaultsInjected=1, LastStatusCode=500`         | false         | Retried but final still faulted.
`Hits=2, FaultsInjected=2, LastStatusCode=200`         | false         | Every hit was faulted; the 200 cannot belong to this window.
`Hits=5, FaultsInjected=2, LastStatusCode=301`         | **true**      | Redirects count as success (2xx/3xx).

Unit coverage: `internal/proxy/observations_test.go::TestObservation_Recovered`.

### Wire path

```
HTTP request → proxy.ServeHTTP
             → matchTest → (inject fault | passthrough)
             → recordFault / recordPassthrough → Observation accumulates
…experiment end…
             → proxy.Observations()[testID].Recovered()
             → evaluator.Evaluate(..., recovered: bool, …)
             → rules.ClassifyExperiment → BehaviorRecoverySuccess / BehaviorRecoveryFailed
             → types.TestResult.DetectedBehaviors → report JSON / HTML
```

Integration coverage:
- `internal/proxy/observations_test.go::TestObservations_RecoverySignalFlipsOnRetry`
  drives the handler end-to-end with a deterministic RNG and asserts
  `Recovered()` flips from false to true across two hits.
- `cmd/ruptor/main_test.go::TestEvaluateTest_WiresRecoveredFromObservation`
  asserts the evaluator emits `recovery_success` when given a
  recovery-shaped observation.

---

# SKILL: Ruptor Proxy — Adding Fault Types

Use this skill when adding a new fault type to `ruptor run`.

## Pattern: Registry + Factory

Adding a new fault = writing one struct that satisfies the `types.Fault`
interface, writing one factory function, and registering the factory in
`NewFaultRegistry`. Nothing else in the proxy changes — the `ServeHTTP`
handler looks up the registered factory by name at request time.

> Historical note: earlier drafts of this doc described an aspirational
> `FaultHandler { Name(), CanHandle(), Inject(ctx, req, next) }` chain of
> responsibility with `ProxyRequest` / `HandlerFunc` types. That contract
> was never implemented. If a reviewer asks you to follow it, point them
> at this paragraph.

## The actual interface

`pkg/types/fault.go`:

```go
type FaultType string

const (
    FaultToolTimeout   FaultType = "tool_timeout"
    FaultSlowResponse  FaultType = "slow_response"
    FaultToolError     FaultType = "tool_error"
    FaultInvalidJSON   FaultType = "invalid_json"
    FaultEmptyResponse FaultType = "empty_response"
    FaultRateLimit     FaultType = "rate_limit"
    FaultLLMError      FaultType = "llm_error"
    FaultLLMTimeout    FaultType = "llm_timeout"
)

type Fault interface {
    Type() FaultType
    Inject(w http.ResponseWriter, r *http.Request) error
}
```

`internal/proxy/faults/fault.go`:

```go
// FaultConfig carries every tunable any of the fault types read. New
// faults add fields here rather than growing a bag of per-fault configs.
type FaultConfig struct {
    Type        types.FaultType
    DelayMs     int
    StatusCode  int
    Body        string
    Payload     string
    Probability float64
    RetryAfterS int
}

// BaseFault supplies the common Type() implementation so each fault
// struct only has to write Inject.
type BaseFault struct{ Cfg FaultConfig }
func (b *BaseFault) Type() types.FaultType { return b.Cfg.Type }
```

`internal/proxy/faults/registry.go`:

```go
type FaultFactory func(cfg FaultConfig) (types.Fault, error)

type FaultRegistry struct { faults map[types.FaultType]FaultFactory }

func NewFaultRegistry() *FaultRegistry { ... registers every built-in ... }
func (r *FaultRegistry) Register(ft types.FaultType, factory FaultFactory)
func (r *FaultRegistry) Build(ft types.FaultType, cfg FaultConfig) (types.Fault, error)
```

The proxy handler (`internal/proxy/handler.go`) calls
`registry.Build(testCfg.Fault, cfg)` per matched request, then the
returned `Fault.Inject(w, r)` writes the response.

## Steps to add a new fault type

1. **Extend the enum.** Add a `FaultXxx FaultType = "xxx"` constant to
   `pkg/types/fault.go`.
2. **Write the fault.** Create `internal/proxy/faults/{name}.go`.
3. **Write the factory** in the same file: `New{Name}Fault(cfg FaultConfig) (types.Fault, error)`.
4. **Register it.** Add `r.Register(types.FaultXxx, NewXxxFault)` to
   `NewFaultRegistry` in `internal/proxy/faults/registry.go`.
5. **Unit test.** `internal/proxy/faults/{name}_test.go` with table-driven
   cases using `httptest.ResponseRecorder`. Assert `Type()`, default
   behaviours, status code, headers, and body.
6. **Integration test.** In the same test file (package `faults_test`),
   build a real `proxy.NewProxy` pointed at a `httptest.NewServer`
   backend. Fire a request that matches the fault's `Tool` path and
   assert (a) the fault response, (b) the backend was never reached,
   and (c) `proxy.Observations()` records `Hits=1`, `FaultsInjected=1`,
   `HadError`, and `LastStatusCode` correctly.
7. **Add a scenario to `configs/chaos.example.yaml`** so users have a
   copy-pasteable starting point.
8. **Run `make check`.** Build + test + lint + vet must be green.
9. **If the fault introduces non-obvious semantics** (fault type overlap
   with another, new reporting field, platform coupling), append a note
   to this skill under "Fault catalogue".

## Template: struct + factory

```go
package faults

import (
    "net/http"

    "github.com/ruptor-dev/cli/pkg/types"
)

// MyFault describes <one-line what it simulates>. Doc-comment should
// say *why* this exists distinctly from the closest existing fault —
// that is the part reviewers cannot derive from the code.
type MyFault struct {
    BaseFault
}

// NewMyFault constructs a MyFault. Apply defaults here; reject
// nonsense inputs here (e.g. required fields, invalid ranges). Don't
// defer validation to Inject — fail at registry.Build time so
// config errors surface at startup instead of mid-run.
func NewMyFault(cfg FaultConfig) (types.Fault, error) {
    // Defaults go here.
    cfg.Type = types.FaultMy
    return &MyFault{BaseFault: BaseFault{Cfg: cfg}}, nil
}

// Inject writes the fault response. Respect r.Context() if the fault
// blocks — never sleep on a bare time.Sleep.
func (f *MyFault) Inject(w http.ResponseWriter, r *http.Request) error {
    // ...
    return nil
}
```

## Template: tests

```go
package faults_test

import (
    // ... standard testing imports ...
    "github.com/ruptor-dev/cli/internal/config"
    "github.com/ruptor-dev/cli/internal/proxy"
    "github.com/ruptor-dev/cli/internal/proxy/faults"
    "github.com/ruptor-dev/cli/internal/ui"
    "github.com/ruptor-dev/cli/pkg/types"
)

func TestMyFault_Inject(t *testing.T) {
    f, err := faults.NewMyFault(faults.FaultConfig{...})
    require.NoError(t, err)

    rec := httptest.NewRecorder()
    req := httptest.NewRequest(http.MethodGet, "/path", nil)
    require.NoError(t, f.Inject(rec, req))

    assert.Equal(t, <expected status>, rec.Code)
    // ... body / header assertions ...
}

func TestMyFault_EndToEndThroughProxy(t *testing.T) {
    backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Fatal("fault must short-circuit; backend should not be called")
    }))
    defer backend.Close()

    registry := faults.NewFaultRegistry()
    cfg := &config.ProxyConfig{Port: 0, PassthroughURL: backend.URL}
    tests := []config.TestConfig{{
        ID: "my_fault_on_foo", Tool: "/foo",
        Fault: types.FaultMy, Probability: 1.0,
    }}

    p := proxy.NewProxy(cfg, tests, registry,
        proxy.WithLogger(ui.SilentLogger()),
        proxy.WithRandSource(rand.NewSource(1)),
    )

    rec := httptest.NewRecorder()
    p.ServeHTTP(rec, httptest.NewRequest("GET", "/foo", nil))

    obs := p.Observations()["my_fault_on_foo"]
    assert.Equal(t, 1, obs.Hits)
    assert.True(t, obs.HadError)
    // etc.
}
```

## Fault catalogue (shipped)

| FaultType | Semantics |
|---|---|
| `tool_timeout` | Immediate 504; tool did not respond. |
| `slow_response` | Sleep `delay_ms`, then 200. Context-cancellation safe. |
| `tool_error` | Configurable 5xx + body. Default 500. |
| `invalid_json` | 200 with malformed JSON payload. |
| `empty_response` | 200 with zero-byte body. |
| `rate_limit` | 429 + `Retry-After` header. `retry_after_s` required. |
| `llm_error` | 5xx from LLM backend path. Default 503 + OpenAI-shaped error envelope. Reports label it distinct from `tool_error` so agent remediation metrics can separate tool-API vs model-API failures. |
| `llm_timeout` | Hang `delay_ms` (default 30 s) then 504 from LLM backend path. Context-cancellation safe. |

## MCP proxy mode

Shipped in v1 with scoped transports. The MCP handler is **not**
plugged into the HTTP `Fault.Inject` contract — instead it maps each
`types.FaultType` to a JSON-RPC 2.0 error or garbage body in
`internal/proxy/mcp/handler.go:injectFault`. This was a deliberate
split: the HTTP path writes status codes; the MCP path writes
JSON-RPC error envelopes over HTTP 200. The user-facing `FaultType`
enum is shared, so a single fault definition in `chaos.yaml` behaves
consistently under both `mode: http` and `mode: mcp`.

- Transport in v1: **Streamable HTTP** only (POST → JSON response).
- Interception in v1: **`tools/call` only**.
- Deferred to v2: stdio, SSE streaming responses, non-tool-call
  interception, WebSocket, `ruptor simulate` over MCP. Each item has
  its own backlog spec — see ADR-011.

Fault authors: there is nothing to do in the MCP path. When you add
a new `FaultType`, extend the `switch` in
`internal/proxy/mcp/handler.go:injectFault` if the default
JSON-RPC-error mapping is not appropriate for your fault's semantics.
Otherwise the default branch returns a generic `-32603` with your
fault type name in the message — good enough for most cases.

## Do NOT

- Do **not** introduce `FaultHandler`, `HandlerFunc`, or `ProxyRequest`
  types to satisfy a legacy draft of this doc. The actual interface is
  `types.Fault` + factory + registry.
- Do **not** modify existing fault source files to add new behaviour.
  Write a new fault.
- Do **not** skip the integration test. The unit test proves the fault
  writes the right bytes; the integration test proves the registry +
  handler + observation tracking all agree.
- Do **not** log response bodies or any non-metadata per the security
  rules in `../knowledge/security.md`.
