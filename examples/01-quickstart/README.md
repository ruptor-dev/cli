# 01 — Quickstart

**No API keys needed. Runs in 60 seconds.**

Runs the four most common fault types against a trivial `/search` tool
so you can see what ruptor's proxy does and what the HTML report looks
like, without any LLM in the loop.

## What it demonstrates

- `ruptor run` pointing at a local passthrough service
- Fault injection: `tool_timeout`, `invalid_json`, `empty_response`,
  `rate_limit`
- How the agent reacts to each injected fault
- HTML + JSON report generation in `./reports/`

## Prerequisites

```bash
# ruptor on PATH
brew install ruptor-dev/tap/ruptor     # or: go install github.com/ruptor-dev/cli/cmd/ruptor@latest

# python 3 is enough — run.sh creates .venv and installs Flask for you
python3 --version
```

## Run

```bash
./run.sh
```

`run.sh` sets up a venv, starts the mock tool server on `:9090`, and
invokes `ruptor run chaos.yaml` with `TOOL_BASE_URL=http://localhost:8080`
so the agent talks to the proxy instead of the real server.

## Expected output

Bubbletea TUI showing four tests cycling through Pending → Passed/Failed,
a Robustness Score, and on completion a pair of report paths:

```
✓ search_timeout
✗ search_invalid_json
✗ search_empty
✓ search_rate_limit

Robustness Score: 50%
Report: ./reports/chaos_report.html
```

## What to look for

- **`search_invalid_json`**: the toy agent prints `non-JSON response`
  because it does not guard `resp.json()`. That is the kind of crash
  ruptor is designed to surface.
- **`search_empty`**: a 200 OK with zero bytes is frequently untested —
  watch whether your own agent treats it as "no results" or explodes.
- **`search_timeout`**: the toy agent has a `timeout=35` hard-coded,
  longer than the injected 5s delay, so it waits through the hold and
  records the behavior. Drop the agent timeout below 5s and the agent
  gives up before the fault releases — the report flags that pattern.
- **`search_rate_limit`**: 429 with `Retry-After`; production agents
  should back off, not retry immediately.
