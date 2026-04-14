# SKILL: Ruptor UI — Terminal Output with Bubbletea + Lipgloss

Use this skill for ANY terminal output work.

## Rule #1

ALL output goes through internal/ui/. No fmt.Println anywhere else.
If you are in cmd/ or internal/ (non-ui), import and use the ui package.

## Theme

```go
// internal/ui/theme.go
var Theme = struct {
    Primary lipgloss.Color  // #E84545 — Ruptor red
    Success lipgloss.Color  // #22C55E
    Failure lipgloss.Color  // #EF4444
    Warning lipgloss.Color  // #F59E0B
    Muted   lipgloss.Color  // #6B7280
    Border  lipgloss.Color  // #374151
    Text    lipgloss.Color  // #F9FAFB
    Subtle  lipgloss.Color  // #9CA3AF
}{
    Primary: "#E84545",
    Success: "#22C55E",
    Failure: "#EF4444",
    Warning: "#F59E0B",
    Muted:   "#6B7280",
    Border:  "#374151",
    Text:    "#F9FAFB",
    Subtle:  "#9CA3AF",
}
```

## Run progress TUI (Bubbletea model)

During `ruptor run`, show a live Bubbletea program:

```
  ╭─────────────────────────────────────────────╮
  │  ruptor  run  •  chaos.yaml                 │
  ╰─────────────────────────────────────────────╯

  ▸ Proxy started on :8081
  ▸ Agent: python agent.py

  ┌─ Experiments ─────────────────────── 3/8 ──┐
  │  ✓  timeout_on_search     847ms   PASSED    │
  │  ✓  rate_limit_on_llm     1.2s    PASSED    │
  │  ✗  invalid_json          203ms   FAILED    │
  │  ◌  slow_response         running...        │
  │  ○  tool_error            pending           │
  │  ○  empty_response        pending           │
  │  ○  llm_error             pending           │
  │  ○  llm_timeout           pending           │
  └─────────────────────────────────────────────┘

  Robustness  ██████████████░░░░░░  68%

  Press q to abort  •  r to retry failed
```

Status icons:
- ✓ green = passed
- ✗ red = failed
- ◌ yellow = running
- ○ muted = pending

## Completion screen

```
  ╭──────────────────────────────────────────╮
  │                                          │
  │   Robustness Score                       │
  │                                          │
  │   ████████████████████░░░░░░   82%      │
  │                                          │
  │   6 passed  •  2 failed  •  0 error     │
  │                                          │
  ╰──────────────────────────────────────────╯

  ✓ Report saved  →  ./reports/run-2026-04-14-143022.html
  ✓ Report saved  →  ./reports/run-2026-04-14-143022.json

  Open report? [Y/n]
```

Score color: ≥80% green, 50-79% yellow, <50% red.

## Simple output functions (for non-TUI commands)

```go
ui.Success("Auth token saved")
ui.Error("Token expired — run `ruptor auth login`")
ui.Info("Proxy started on :8081")
ui.Warning("Port 8080 in use, using :8081 instead")
ui.Dim("Checking for updates...")
```

## Version check banner (non-blocking)

When a new version is available, show at the END of any command output:
```
─────────────────────────────────────────────
  ⚡ ruptor v1.2.0 available  →  ruptor update
─────────────────────────────────────────────
```
Never show at the start. Never block the command.

## Error messages style

Error messages tell the user exactly what to do next:

Bad:  "authentication error"
Good: "✗ Session expired. Run `ruptor auth login` to reconnect."

Bad:  "plan error"
Good: "✗ Your plan is inactive. Reactivate at https://ruptor.dev/billing"

Always use backticks for commands. Always include the next action.
