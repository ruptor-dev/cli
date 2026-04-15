# 05 — All 8 faults

A single chaos run that exercises every fault type ruptor ships with.
The agent is intentionally defensive — every path handles failures
without crashing — so the expected outcome is a perfect Robustness
Score. Useful as a regression fixture and as a reference for what a
"well-behaved" agent looks like.

## Faults covered

| Fault | What the agent does |
|---|---|
| `tool_timeout`   | `requests.get(timeout=2.5s)`; catches `Timeout`, prints a message |
| `slow_response`  | Completes normally and logs elapsed time |
| `tool_error`     | Detects `>= 500` and returns an empty fallback result |
| `invalid_json`   | Catches `ValueError`, logs the raw body snippet |
| `empty_response` | Detects empty body, treats as "no results" |
| `rate_limit`     | Reads `Retry-After`, sleeps up to 3s, retries once |
| `llm_error`      | Detects `>= 500` on `/llm/complete`, falls back to canned answer |
| `llm_timeout`    | `requests.post(timeout=2.5s)`, catches `Timeout` |

## Prerequisites

```bash
brew install ruptor-dev/tap/ruptor   # or go install …
python3 --version
```

## Run

```bash
./run.sh
```

Set `RUPTOR_BIN=/path/to/ruptor` to point at a locally-built binary
instead of the installed one.

## Expected output

- 8 experiments, all PASS
- Robustness Score: 100%
- Total runtime: well under 90 seconds
- Reports written to `./reports/chaos_report.html` and `.json`
