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

## MCP proxy mode (v2)

MCP conversational mode is v2 scope — no MCP fault types ship in v1.
When MCP lands, faults will likely stay protocol-agnostic: the proxy
will pick either the HTTP or MCP path based on `proxy.protocol` in
`chaos.yaml`, and the same `Fault.Inject` is called — with a
MCP-aware `ResponseWriter` wrapper that marshals back to JSON-RPC 2.0.
The fault-author contract stays the same.

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
