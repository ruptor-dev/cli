# 03 — Multi-tool

Targets fault injection at three different tool endpoints independently,
so the report shows per-tool Robustness Scores instead of a single
aggregate.

## What it demonstrates

- Multiple tool endpoints behind one proxy
- Per-tool fault targeting: timeout on `/search`, `invalid_json` on
  `/db/user`, 503 on `/notify`, `rate_limit` on `/search`, `empty_response`
  on `/db/user`
- How the HTML report breaks down by `tool` column

## Prerequisites

```bash
brew install ruptor-dev/tap/ruptor
python3 --version
```

## Run

```bash
./run.sh
```

## Expected output

Five experiments, one per row, grouped implicitly by `tool` in the
report. The mock agent walks `/search` → `/db/user` → `/notify` once per
run, so every experiment should see exactly one hit.

## What to look for

- **Per-tool blast radius.** A failing `/db/user` should not pollute
  the `/search` row in the report.
- **`invalid_json` on structured endpoints.** Agents that do
  `resp.json()` without error handling crash here — a common cause of
  prod incidents.
- **503 vs rate-limit distinction.** `/notify` hitting a 503 should be
  retried with backoff; `rate_limit` with `Retry-After` should honor
  the header. The report tells you which pattern you actually
  implemented.
