# Wire the OTLP exporter (or defer telemetry to v2)

> Status: **Done — 2026-04-16**
> Opened: 2026-04-15
> Priority: **high**
> Est. effort: **XS** (2h for scope doc update + cleanup)
> Decision required: **no — resolved 2026-04-15, Option B (defer to v2)**

## Problem

`internal/telemetry/telemetry.go` builds an SDK `TracerProvider` but
never registers an OTLP exporter. Every span produced by the app is
therefore discarded. `knowledge/v1-scope.md` lists "OTel telemetry
opt-in" as in-scope for v1. Either wire the exporter now or update
the scope doc to move telemetry to v2; shipping with a
sink-less tracer lets us claim a feature that does nothing.

## Evidence

- `internal/telemetry/telemetry.go:74` — `sdktrace.NewTracerProvider(sdktrace.WithResource(res))`,
  no `WithBatcher` / `WithSyncer` / exporter.
- `internal/telemetry/telemetry.go:11` — comment: "the OTLP exporter
  lands with the `ruptor auth login` PR (auth token required to hit
  telemetry.ruptor.dev)".
- `knowledge/v1-scope.md:29` — "OTel telemetry opt-in (disabled
  until auth login)".

## Proposed solution

If **Option A (wire now)**:

1. Add `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`
   to `go.mod`.
2. In `Init`, when `cfg.Enabled && cfg.Endpoint != ""`, build an
   `otlptracehttp.New(...)` client with:
   - `WithEndpoint(cfg.Endpoint)`
   - `WithHeaders(map[string]string{"Authorization": "Bearer "+token})`
     (threaded from the same token supplier introduced in
     `cloud-client-auth-header.md`)
   - `WithCompression(otlptracehttp.GzipCompression)`
3. Wrap with `sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second))`.
4. Extend `Provider.Shutdown` to flush the exporter.
5. Add an integration test that spins up a `httptest.NewServer`
   receiving `/v1/traces` and asserts the span attribute allowlist in
   `RecordRun` is preserved end-to-end.

If **Option B (defer to v2)**:

1. Edit `knowledge/v1-scope.md:29` to move "OTel telemetry" to the
   OUT list.
2. Update the docstring at `internal/telemetry/telemetry.go:1-12`.
3. Remove any `--telemetry`/`RUPTOR_TELEMETRY_ENABLED` wiring from
   the CLI flag/config layer (keep the package for v2, but make it
   unreachable from user input).

## Acceptance criteria

- [ ] Decision recorded in `knowledge/decisions/` as a new ADR.
- [ ] Either spans reach a running collector in a test, OR v1-scope
      no longer claims telemetry is shipped.
- [ ] `TelemetryEnabled` config field kept for forward-compat with v2 comment.
- [ ] No exporter-related dead code paths left behind.

## Out of scope

- Metrics and logs exporters (traces only).
- Server-side collector — a platform concern.

## Decision (resolved 2026-04-15)

**Option B — Defer to v2.** The v1 launch story is "local reports and
a Robustness Score you can trust" — telemetry is not part of that
pitch. `telemetry.ruptor.dev` does not exist yet.

Implementation plan for Option B:
1. Move "OTel telemetry opt-in" from IN to OUT in `knowledge/v1-scope.md:29`.
2. Update docstring at `internal/telemetry/telemetry.go:1-12`.
3. Keep `TelemetryEnabled` in Settings with a "deferred to v2" comment.
   The env var `RUPTOR_TELEMETRY_ENABLED` remains functional but has no
   effect until an exporter is wired. Removing it would break forward-compat
   for users who set it in anticipation of v2.
4. Keep the `internal/telemetry/` package for v2 but make it unreachable.
5. File ADR in `knowledge/decisions/`.

> **IMPORTANT**: This is deferred, not cancelled. Telemetry must ship
> in v2 alongside the platform API going live. When `cloud-client-auth-header`
> lands (providing TokenSupplier), the exporter can reuse the same
> supplier for `Authorization` headers to `telemetry.ruptor.dev`.
> Convert this spec to a v2 tracker when v2 planning begins.

## References

- `internal/telemetry/telemetry.go`
- `knowledge/v1-scope.md`
- Sibling: `cloud-client-auth-header.md`
