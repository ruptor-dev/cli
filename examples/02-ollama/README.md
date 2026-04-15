# 02 — Ollama

Fault injection and user simulation against a real local LLM. The agent
uses Ollama to drive a tool-use loop; ruptor's proxy sits between the
agent and the tool so you can watch a real model react to injected
failures.

## What it demonstrates

- Tool-use loop with a real LLM (Ollama `qwen2`)
- Chaos proxy injecting `tool_timeout`, `slow_response`, `tool_error`,
  `rate_limit` on the `/search` tool
- `ruptor simulate` running three personas end-to-end against the
  same agent

## Prerequisites

```bash
# ruptor
brew install ruptor-dev/tap/ruptor

# ollama
brew install ollama
ollama serve &
ollama pull qwen2           # or any model that supports tool calls

python3 --version
```

## Configuration

| Env | Default | Purpose |
|---|---|---|
| `OLLAMA_URL`   | `http://localhost:11434` | Ollama API root |
| `OLLAMA_MODEL` | `qwen2`                  | Ollama model tag |
| `TOOL_BASE_URL`| `http://localhost:9090`  | Tool endpoint (overridden to proxy in run.sh) |

## Run

```bash
./run.sh
```

`run.sh` starts the mock tool backend on `:9090`, the Flask-wrapped
agent on `:3000`, runs `ruptor run chaos.yaml`, then `ruptor simulate
simulate.yaml`.

## Expected output

- Chaos run: a four-row TUI with `search_timeout`, `search_slow`,
  `search_5xx`, `search_rate_limit`. The model should degrade
  gracefully on the 5xx and rate-limit rows. If it silently retries
  without backoff, `search_rate_limit` will flag it.
- Simulate run: three personas, a per-simulation quality score, and a
  combined report. Expect `frustrated_user` to be the hardest.

## What to look for

- **Tool errors that leak into the user response.** A well-behaved
  agent should say "I couldn't reach the search service" rather than
  echo a JSON error blob.
- **Persona-specific failures.** `non_native_speaker` often exposes
  agents that fall back to jargon under pressure.
- **Score trend.** Run the chaos pass first, then the simulate pass;
  the Robustness Score is the canonical comparison number across
  model swaps.
