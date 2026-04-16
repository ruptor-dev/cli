# TUI log panel — Layer B (scrollable viewport)

> Status: **Backlog**
> Opened: 2026-04-16
> Priority: **medium** — upgrade on top of shipped Layer A
> Est. effort: **M** (1 day)
> Decision required: **no** — pattern is pinned in `SKILL-ui.md`

## Problem

The Layer 1 log panel (commit `c8fa6f0`) is tail-only: last N lines,
no scroll, no pause-tail. Operators hitting an error mid-run cannot
scroll back within the panel — the panel is constantly overwritten
by newer events at 100 ms tick rate. The full log is always on disk
(`<runDir>/ruptor.log`) but switching to a second terminal disrupts
the live run.

## Proposed solution

Swap the inline-render log panel for a `charm.land/bubbles/v2`
`viewport.Model`. The `bubbles/v2` package is already authorised by
`SKILL-ui.md` as a future-OK dependency.

### Keybindings (spec'd in `SKILL-ui.md`)

- `↑` / `↓` — scroll by one line
- `PgUp` / `PgDn` — scroll by panel height
- `Home` / `End` — jump to top / bottom of buffer
- `s` — toggle auto-tail (sticky bottom) vs. paused
- mouse wheel — scroll when the terminal has mouse capture

Auto-tail is the default on entry; pressing `s` pauses at the
current scroll position so the operator can read. Pressing `s` again
returns to auto-tail and jumps to the bottom.

### Implementation plan

1. Add `charm.land/bubbles/v2` to `go.mod`.
2. In `internal/ui/runprogress.go`, add a `viewport.Model` field to
   `runModel`; size it to `LogPanelHeight` rows on the first tick.
3. On every tick, read `m.ctx.LogReader.Lines(0)` and write to the
   viewport's `SetContent` — viewport handles the scroll state
   internally.
4. Forward the listed keys from `handleKey` to the viewport's
   `Update` method BEFORE matching the existing `q` / `r` shortcuts.
5. Render `viewport.View()` in place of the current `renderLogPanel`
   body; keep the bordered box wrapper so the layout matches
   Experiments.
6. Add a small indicator in the panel title: `· tail · -v` when
   sticky, `· paused (s) · -v` when frozen.

### Tests

- Unit: scroll commands change the viewport offset (`↑`, `PgUp`,
  `End`, `s` toggle).
- Visual: rendered string differs before/after a scroll (oldest line
  becomes visible; most-recent line leaves the render).
- Auto-tail: after scrolling up then new lines arrive, the panel
  does NOT jump to bottom (auto-tail paused).
- `s` toggle: panel DOES jump to bottom after toggle.

## Acceptance criteria

- [ ] `charm.land/bubbles/v2` added to `go.mod` only; no other deps.
- [ ] Keybindings listed above work; `q` still aborts the run.
- [ ] Scroll state survives subsequent ticks (no jump-to-bottom
      unless `s` toggled).
- [ ] Layer 1 behaviour preserved when `--verbose` is off (no panel).
- [ ] `SKILL-ui.md` gains the keybinding table (already previewed
      under "Layer 2" — expand when this ships).
- [ ] Tests listed above.

## Out of scope

- Log filtering (by level, test ID, fault type) — separate spec.
- Highlighting — separate spec.
- Export / copy-to-clipboard — separate spec.

## References

- Layer 1 implementation: commit `c8fa6f0`, files
  `internal/ui/logbuffer.go`, `internal/ui/runprogress.go`.
- Spec: `docs/superpowers/specs/SKILL-ui.md` § "Layer 2 — scrollable".
- Upstream: https://github.com/charmbracelet/bubbles/tree/v2/viewport
