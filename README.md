# Ruptor

Reliability testing for AI agents — Chaos Engineering meets LLM systems.

Ruptor helps you find out how your AI agent behaves when things go wrong:
tool timeouts, invalid JSON, rate limits, empty responses. Before your users do.

---

## Modules

| Module | What it does |
|---|---|
| `ruptor run` | Injects failures into tool calls and observes agent behavior |
| `ruptor simulate` | Simulates real users to evaluate goal completion and conversation quality |

---

## Installation

```bash
go install github.com/ruptor-dev/cli/cmd/ruptor@latest
```

> Requires Go 1.22+

---

## Quickstart — Chaos Testing

**1. Point your agent's tools at Ruptor:**

```bash
export TOOL_BASE_URL=http://localhost:8080
```

**2. Create a `chaos.yaml`:**

```yaml
agent:
  name: my_agent
  entrypoint: python agent.py
  env:
    TOOL_BASE_URL: http://localhost:8080

proxy:
  port: 8080
  passthrough_url: https://my-real-tool-api.com

tests:
  - id: timeout_on_search
    tool: /search
    fault: tool_timeout
    delay_ms: 30000
    probability: 1.0

evaluation:
  max_iterations: 20
  timeout_s: 60
  llm_judge: true

output:
  format: both
  path: ./reports/
```

**3. Run:**

```bash
ruptor run chaos.yaml
ruptor run chaos.yaml --output report.html
ruptor run chaos.yaml --test timeout_on_search
```

---

## Quickstart — Simulate

**1. Create a `simulate.yaml`:**

```yaml
agent:
  name: support_agent
  base_url: http://localhost:3000

simulations:
  - id: frustrated_user
    persona: "Frustrated user who wants to resolve their issue in under 3 messages"
    goal: "Cancel subscription"
    max_turns: 10
    success_criteria: "Agent completed the cancellation"

evaluation:
  goal_completion: true
  tone_quality: true

output:
  format: both
  path: ./reports/
```

**2. Run:**

```bash
ruptor simulate simulate.yaml
ruptor simulate simulate.yaml --sim frustrated_user
```

---

## Available Faults

| Fault | Description |
|---|---|
| `tool_timeout` | No response — does the agent have its own timeout? |
| `slow_response` | Responds after N ms — does the agent wait or cut? |
| `tool_error` | Returns 5xx — does the agent retry or fail gracefully? |
| `invalid_json` | Returns malformed JSON — does the agent handle ParseError? |
| `empty_response` | Returns 200 OK with empty body — common and rarely tested |
| `rate_limit` | Returns 429 — does the agent implement backoff or spam? |

---

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `OPENAI_API_KEY` | Yes (for LLM judge) | OpenAI API key for the evaluator and user simulator |

---

## Development

```bash
make build                  # compile
make test                   # run tests with race detector
make lint                   # run golangci-lint
make check                  # build + test + vet
make run-example            # run chaos example
make run-simulate-example   # run simulate example
make tools                  # install dev tools
make help                   # list all targets
```

---

## Roadmap

- [x] MVP: 6 fault types, chaos testing, simulate module
- [ ] v2: Custom faults, fault combinations, CI/CD integration
- [ ] v3: SaaS hosted proxy, dashboard, team collaboration
