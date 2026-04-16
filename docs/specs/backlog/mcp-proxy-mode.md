# MCP proxy mode — v1 scope and transport plan

> Status: **Done — 2026-04-16** (decision locked 2026-04-15, code
> landed, audit passed, ADR-011 filed)
> Opened: 2026-04-15
> Priority: **ship-critical**
> Est. effort: **L** (2-3 days) — actual: landed in code before audit
> Decision: Option A with scoped transports (Streamable HTTP +
> `tools/call` only). See `knowledge/decisions/011-mcp-transport-scoping.md`.

## Problem

Two source-of-truth documents disagree on whether MCP proxy support
is in v1. `knowledge/v1-scope.md` says yes; `docs/specs/proxy.md`
defers it to v2. The landing page positions MCP tool-call interception
as a competitive differentiator (✓ Ruptor vs ✗ others). Shipping
without it means launching with a false feature claim.

## Evidence

- `knowledge/v1-scope.md:24` — "MCP proxy mode for `ruptor run`
  (JSON-RPC 2.0 tool call interception)".
- `docs/specs/proxy.md` (around the "Fault catalogue" section near
  line 213) — implies HTTP-only for v1; no `internal/proxy/mcp/`
  implementation anywhere in the tree.
- Landing page competitive grid: "MCP proxy support" is ✓ for Ruptor,
  ✗ for competitors.
- Directory probe: `internal/proxy/` contains only HTTP handler,
  observations, faults, and proxy core — no MCP files.

## Decision (resolved)

**Option A — Ship MCP in v1**, with scoped transports:

### v1 scope (shipped)

| Transport | Why |
|-----------|-----|
| **Streamable HTTP — POST → JSON response** | Current MCP standard. Ruptor is already an HTTP reverse proxy; JSON-RPC 2.0 rides on HTTP POST and the server responds with a JSON body. Lowest effort, highest coverage of modern MCP servers. |

**Not shipping in v1**: SSE streaming responses (POST →
`text/event-stream`) were on the original v1 acceptance list but
were scoped out during code review. The request-response JSON path
covers 95%+ of `tools/call` usage. SSE is for long-running
operations with progress notifications — tracked in
`docs/specs/backlog/mcp-sse-streaming.md`.

Interception scope: **`tools/call` requests only**. The proxy identifies
JSON-RPC 2.0 requests with `method: "tools/call"`, applies the existing
fault chain (latency, error, garbage, etc.), and forwards to the real
MCP server. All other MCP methods (`resources/*`, `prompts/*`,
`completion/*`, `sampling/*`, `initialize`, `ping`) pass through
unmodified.

### v2 scope (deferred)

| Item | Why deferred |
|------|-------------|
| **stdio transport** | Completely different architecture — requires spawning MCP server as child process and sitting as man-in-the-middle on stdin/stdout pipes. Not a network proxy. Significant effort with low payoff until stdio-heavy MCP deployments are common. |
| **SSE streaming responses** | POST → `text/event-stream` path. Request-response JSON covers 95%+ of `tools/call` usage. Tracked in `docs/specs/backlog/mcp-sse-streaming.md`. |
| **Intercept `resources/*`, `prompts/*`, `sampling/*`** | Low value for reliability testing. Faults on tool calls cover the primary failure mode (agent calls a tool, tool fails). Resource/prompt faults are edge cases. |
| **`ruptor simulate` with MCP** | Explicitly OUT in `knowledge/v1-scope.md`. Simulation is conversation-level, not transport-level. See ADR-009, ADR-011. |
| **WebSocket transport** | Not part of the MCP spec as of 2026-04. If added to spec, evaluate then. |

## Proposed solution

1. **Types**: Create `pkg/types/mcp.go` defining JSON-RPC 2.0 message
   shapes: `JSONRPCRequest`, `JSONRPCResponse`, `JSONRPCError`,
   `ToolCallParams`, `ToolCallResult`.

2. **Proxy**: Create `internal/proxy/mcp/` package:
   - `proxy.go` — HTTP handler that detects MCP requests (by
     `Content-Type: application/json` + JSON-RPC method field), parses
     `tools/call`, applies the fault chain, and forwards. Non-tool-call
     requests pass through unmodified.
   - `stream.go` — SSE response handling: for Streamable HTTP
     responses that use SSE, proxy the event stream while injecting
     faults on the `tools/call` result event.
   - `detect.go` — Heuristic to distinguish MCP JSON-RPC traffic from
     regular REST API traffic on the same proxy port, so `mode: auto`
     can work.

3. **Config**: Add `mode` field to chaos.yaml test config:
   ```yaml
   mode: mcp          # new — MCP JSON-RPC proxy
   mode: http         # existing default — REST/HTTP proxy
   mode: auto         # new — detect based on request shape
   ```

4. **Integration**: Wire into `ruptor run` via the existing proxy
   setup in `cmd/ruptor/main.go`. When `mode: mcp` or auto-detected,
   use the MCP handler instead of the HTTP handler.

5. **Tests**: Integration tests against a mock MCP server using
   `httptest.NewServer` that speaks JSON-RPC 2.0:
   - `tools/call` with latency fault → delayed response
   - `tools/call` with error fault → JSON-RPC error response
   - Non-tool-call method → passthrough unmodified
   - Auto-detection: send both REST and MCP traffic to same proxy
   - (SSE streaming response fault injection — deferred to v2,
     tracked in `docs/specs/backlog/mcp-sse-streaming.md`.)

6. **Docs**: Update `docs/specs/proxy.md`, `README.md`, and
   `knowledge/v1-scope.md` to reflect the scoped transport support.

## Acceptance criteria — audit 2026-04-16

All criteria PASS. Evidence per row.

- [x] `mode: mcp` in chaos.yaml works against a mock MCP server in
      integration test. — `internal/config/chaos_config.go:47-50,102-105`;
      `internal/proxy/mcp/handler_test.go:72-155`;
      `internal/proxy/proxy_test.go:320-376` (`TestMCPModeRouting`).
- [x] `mode: auto` correctly detects JSON-RPC 2.0 `tools/call` vs
      REST traffic. — `internal/proxy/handler.go:34-48`;
      `internal/proxy/proxy_test.go:260-318`;
      `internal/proxy/mcp/handler_test.go:279-318` (`TestMCPIsMCPRequest`).
- [x] Streamable HTTP (POST → HTTP response) works. —
      `internal/proxy/mcp/handler.go:115-174`; covered by all MCP
      fault-injection tests in `handler_test.go`.
- [x] SSE streaming deferred to v2 — see `mcp-sse-streaming.md`. —
      `docs/specs/backlog/mcp-sse-streaming.md` exists and is
      cross-referenced here and in `docs/specs/proxy.md`.
- [x] Faults apply only to `tools/call` requests; other MCP methods
      pass through. — `internal/proxy/mcp/handler.go:141-146`;
      `TestMCPNonToolCall_Passthrough`
      (`handler_test.go:157-194`).
- [x] All existing HTTP proxy tests still pass (no regression). —
      `go test ./...` 626 passed, 20 packages.
- [x] `knowledge/v1-scope.md` and `docs/specs/proxy.md` agree on
      scope. — reconciled in ADR-011 close-out (2026-04-16).
- [x] ADR filed documenting transport scoping decision. —
      `knowledge/decisions/011-mcp-transport-scoping.md`.
- [x] v2 items documented in this spec and in `knowledge/v1-scope.md`
      OUT section. — stdio, SSE, non-tool-call, WebSocket, and
      simulate-over-MCP are all listed in both places.

## Out of scope (v2 — documented for tracking)

These items are explicitly deferred to v2. Each has a rationale —
not a blanket "maybe later". Also listed in `knowledge/v1-scope.md`
OUT section and in ADR-011.

1. **stdio transport** — `internal/proxy/mcp/stdio.go` (not written).
   Requires: child process spawning, pipe-based interception, process
   lifecycle management. Est. effort: M (1-2 days). Blocked on:
   architecture decision for how Ruptor manages MCP server child
   processes. File a dedicated spec when v2 begins.

2. **SSE streaming responses** — Streamable HTTP POST →
   `text/event-stream`. Tracked as its own backlog spec:
   `docs/specs/backlog/mcp-sse-streaming.md`. Blocked on: demand
   signal (most production MCP servers respond with JSON).

3. **Non-tool-call interception** — faults on `resources/read`,
   `prompts/get`, `sampling/createMessage`. Est. effort: S per method.
   Blocked on: user demand signal.

4. **`ruptor simulate` with MCP** — conversation simulation over MCP
   transport. Est. effort: L. Blocked on: simulate architecture
   redesign for multi-transport. See ADR-009 (simulate HTTP contract)
   and ADR-011.

5. **WebSocket transport** — not in MCP spec as of 2026-04. Monitor
   MCP spec evolution and re-evaluate if added.

## References

- `knowledge/v1-scope.md`
- `knowledge/decisions/011-mcp-transport-scoping.md` (ADR-011)
- `docs/specs/proxy.md`
- `docs/specs/backlog/mcp-sse-streaming.md` (v2 SSE spec)
- `docs/specs/backlog/mcp-observations-evaluator.md` (observations wiring)
- `internal/proxy/mcp/handler.go`, `internal/proxy/mcp/handler_test.go`
- `internal/proxy/handler.go`, `internal/proxy/proxy.go`
- `pkg/types/mcp.go` — JSON-RPC 2.0 types
- https://modelcontextprotocol.io/specification/2025-03-26/basic/transports
- https://modelcontextprotocol.io/specification/2025-03-26/basic/lifecycle
- Landing page competitive grid (MCP as differentiator)
