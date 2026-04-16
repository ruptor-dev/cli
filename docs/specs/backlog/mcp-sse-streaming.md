# SSE streaming for MCP proxy

> Status: **Backlog**
> Opened: 2026-04-15
> Priority: **medium** (v2)
> Est. effort: **M** (1 day)
> Decision required: **no**

## Problem

MCP Streamable HTTP allows servers to respond to a JSON-RPC request
with `Content-Type: text/event-stream` instead of a single JSON
response. This is used for long-running operations where the server
sends progress notifications before the final result.

The current MCP proxy (`internal/proxy/mcp/handler.go`) only handles
the request-response path: it reads the full response body, inspects
it as JSON-RPC, and injects faults. When the upstream MCP server
responds with an SSE stream, the proxy either passes it through
unchanged (no fault injection) or buffers the entire stream waiting
for a complete JSON body (breaking streaming semantics).

SSE was in the original MCP v1 acceptance criteria
(`docs/specs/backlog/mcp-proxy-mode.md`) but was deferred to v2 during
code review. The request-response JSON-RPC path covers 95%+ of
`tools/call` usage. SSE is for the remaining long-running operations
with progress updates.

## Evidence

- **`internal/proxy/mcp/handler.go`** — no SSE content-type detection,
  no event stream parsing. `ServeHTTP` reads the request body as JSON
  and forwards via `httputil.ReverseProxy`, which treats the response
  as an opaque byte stream.

- **MCP spec — Streamable HTTP transport**
  (https://modelcontextprotocol.io/specification/2025-03-26/basic/transports):
  "The server MAY respond with `Content-Type: text/event-stream` to
  initiate an SSE stream." The stream contains `event: message` lines
  with JSON-RPC responses/notifications as `data:` payloads.

- **`docs/specs/backlog/mcp-proxy-mode.md:103`** — original acceptance
  criterion "SSE transport (POST -> SSE stream) works" is now deferred.

## Proposed solution

1. **Detect SSE responses**: After forwarding the request to the
   upstream MCP server, check the response `Content-Type`. If it is
   `text/event-stream`, switch to the SSE parsing path instead of
   reading the body as a single JSON blob.

2. **Parse the event stream**: Read SSE events line by line
   (`event:`, `data:`, blank-line delimiters). For each complete
   event:
   - Parse the `data:` field as JSON-RPC.
   - If it is a JSON-RPC **result** for a `tools/call` request ID
     that matched a fault rule, inject the fault (replace the result
     event with a fault event, or inject a delay before flushing it).
   - If it is a **notification** (progress, log, etc.) or a result
     for a non-intercepted method, flush it through immediately.

3. **Preserve streaming**: Use `http.Flusher` to flush each event to
   the client as soon as it is processed. Do not buffer the entire
   stream — the whole point of SSE is incremental delivery.

4. **Fault types on SSE**:
   - **Latency**: delay before flushing the result event.
   - **Error**: replace the result event `data:` with a JSON-RPC
     error.
   - **Invalid JSON**: replace the result event `data:` with garbage.
   - **Empty response**: drop the result event entirely (close stream
     after notifications).
   - **Rate limit**: inject a JSON-RPC error with retry metadata.

5. **Implementation location**: `internal/proxy/mcp/stream.go` (the
   placeholder was already planned in `mcp-proxy-mode.md`).

## Acceptance criteria

- [ ] SSE responses from the upstream MCP server have faults injected
      on the `tools/call` result event.
- [ ] Non-result events (progress notifications, log messages) pass
      through unchanged and are flushed immediately.
- [ ] Streaming is preserved — no full-stream buffering. Events are
      delivered to the client incrementally via `http.Flusher`.
- [ ] Observations are recorded for SSE-mode fault injection (hits,
      faults injected).
- [ ] Integration test: mock MCP server sends SSE stream with progress
      events followed by a result; proxy injects fault on result while
      passing progress events through.
- [ ] Request-response (non-SSE) path is unaffected (no regression).

## Out of scope

- **stdio transport** — completely different architecture, not
  network-based. Tracked separately in `mcp-proxy-mode.md` v2 items.
- **Server-initiated notifications** — SSE streams opened by the
  server without a preceding client request. Not part of the
  `tools/call` interception model.

## References

- `internal/proxy/mcp/handler.go`
- `docs/specs/backlog/mcp-proxy-mode.md`
- https://modelcontextprotocol.io/specification/2025-03-26/basic/transports
- https://html.spec.whatwg.org/multipage/server-sent-events.html (SSE format)
