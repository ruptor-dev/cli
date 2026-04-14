# SKILL: Ruptor Proxy — Adding Fault Types

Use this skill when adding a new fault type to ruptor run.

## Pattern

Every fault type implements FaultHandler in pkg/types/proxy.go:

```go
type FaultHandler interface {
    // Name returns the fault type identifier used in chaos.yaml
    Name() string
    // CanHandle returns true if this handler should process this request
    CanHandle(faultName string) bool
    // Inject applies the fault to the request/response cycle.
    // Call next() to pass through to the real tool.
    Inject(ctx context.Context, req *ProxyRequest, next HandlerFunc) (*ProxyResponse, error)
    // ShouldApply returns true based on the configured probability
    ShouldApply(probability float64) bool
}
```

## Steps to add a new fault type

1. Create internal/proxy/faults/{fault_name}_fault.go
2. Implement FaultHandler interface
3. Register in internal/proxy/faults/registry.go
4. Add to pkg/types/faults.go constants
5. Write unit test: internal/proxy/faults/{fault_name}_fault_test.go
6. Write integration test with httptest.NewServer
7. Add to testdata/chaos.example.yaml
8. Update docs/superpowers/specs/ if behavior is non-obvious

## Template for a new fault

```go
package faults

import (
    "context"
    "math/rand"
    "github.com/ruptor-dev/cli/pkg/types"
)

type MyNewFault struct{}

func (f *MyNewFault) Name() string { return "my_new_fault" }

func (f *MyNewFault) CanHandle(faultName string) bool {
    return faultName == f.Name()
}

func (f *MyNewFault) ShouldApply(probability float64) bool {
    return rand.Float64() < probability
}

func (f *MyNewFault) Inject(
    ctx context.Context,
    req *types.ProxyRequest,
    next types.HandlerFunc,
) (*types.ProxyResponse, error) {
    if !f.ShouldApply(req.Config.Probability) {
        return next(ctx, req)
    }
    // Apply the fault here
    // ...
    return &types.ProxyResponse{...}, nil
}
```

## Existing fault types for reference

- tool_timeout: no response (context.DeadlineExceeded after delay_ms)
- slow_response: responds after delay_ms
- tool_error: returns 5xx status code
- invalid_json: returns malformed JSON body
- empty_response: returns 200 with empty body
- rate_limit: returns 429 with Retry-After header
- llm_error: returns 5xx from the LLM backend path
- llm_timeout: hangs the LLM backend path

## MCP proxy mode

For faults on MCP tool calls, the fault handler receives a MCPProxyRequest
instead of a regular ProxyRequest. Both implement ProxyRequestInterface.
The fault logic is usually identical — only the transport differs.
