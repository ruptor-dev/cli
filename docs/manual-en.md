# FaultForge User Manual

> Reliability testing for AI agents -- Chaos Engineering meets LLM systems.

**Version:** 0.1.0

---

## Table of Contents

1. [Introduction](#1-introduction)
2. [Installation](#2-installation)
3. [Quick Start](#3-quick-start)
4. [Tool 1: FaultForge Chaos](#4-tool-1-faultforge-chaos)
   - 4.1 [Concept and Architecture](#41-concept-and-architecture)
   - 4.2 [Configuration Reference](#42-configuration-reference)
   - 4.3 [Fault Types](#43-fault-types)
   - 4.4 [Evaluation System](#44-evaluation-system)
   - 4.5 [Running Tests](#45-running-tests)
   - 4.6 [Reading Reports](#46-reading-reports)
   - 4.7 [Tutorial: Testing a Python Agent](#47-tutorial-testing-a-python-agent)
5. [Tool 2: FaultForge Simulate](#5-tool-2-faultforge-simulate)
   - 5.1 [Concept and Architecture](#51-concept-and-architecture)
   - 5.2 [Configuration Reference](#52-configuration-reference)
   - 5.3 [Designing Personas](#53-designing-personas)
   - 5.4 [Defining Goals and Success Criteria](#54-defining-goals-and-success-criteria)
   - 5.5 [Evaluation Metrics](#55-evaluation-metrics)
   - 5.6 [Running Simulations](#56-running-simulations)
   - 5.7 [Reading Reports](#57-reading-reports)
   - 5.8 [Tutorial: Simulating Users for a Support Agent](#58-tutorial-simulating-users-for-a-support-agent)
6. [Configuration Validation](#6-configuration-validation)
7. [Environment Variables](#7-environment-variables)
8. [Output Formats](#8-output-formats)
9. [Troubleshooting](#9-troubleshooting)
10. [FAQ](#10-faq)

---

## 1. Introduction

### What is FaultForge?

FaultForge is a reliability testing tool for AI agents, inspired by the principles of Chaos Engineering. It helps you discover how your AI agent behaves when things go wrong -- tool timeouts, malformed responses, rate limits, server errors -- before your users do.

AI agents rely on external tools (APIs, databases, search engines) to accomplish tasks. In production, those tools fail in unpredictable ways: they time out, return garbage data, throttle requests, or go down entirely. Most agent developers never test these failure modes. FaultForge fills that gap.

### The Problem It Solves

When you build an AI agent, you typically test the happy path: the tools work, the data is clean, the user is cooperative. But in production:

- A search API returns a 504 timeout after 30 seconds of waiting.
- A database lookup returns `{ broken json %%%` instead of valid JSON.
- A third-party API starts returning 429 rate-limit responses.
- A tool returns HTTP 200 with a completely empty body.
- A critical service returns a 500 internal server error.

Without testing these scenarios, you have no idea whether your agent will crash, spin in an infinite retry loop, hallucinate an answer, or gracefully inform the user that something went wrong.

FaultForge provides two complementary tools:

1. **FaultForge Chaos** -- An HTTP proxy that sits between your agent and its tools, injecting controlled faults (timeouts, errors, bad data) and evaluating how the agent responds.
2. **FaultForge Simulate** -- A conversation simulator that creates realistic simulated users (impatient, confused, adversarial) and measures whether your agent can achieve goals under conversational pressure.

### Who Is It For?

- **AI agent developers** who want to ship reliable agents.
- **QA engineers** testing LLM-powered applications.
- **Platform teams** building agent frameworks who need automated reliability benchmarks.
- **Anyone** who has had an agent fail in production and never wants it to happen again.

### Key Features

- Six built-in fault types covering the most common tool failure modes.
- Configurable fault injection probability (test 100% failure or intermittent faults).
- Two-layer evaluation: deterministic rule-based detectors plus LLM judge for nuanced assessment.
- Simulated users with configurable personas, goals, and success criteria.
- Multiple output formats: terminal, JSON, and HTML reports.
- Single binary, zero dependencies at runtime.
- YAML-based configuration -- no code changes to your agent required.

---

## 2. Installation

### Prerequisites

- **Go 1.22 or later.** FaultForge is written in Go. You need a Go toolchain to build from source.
- **An OpenAI API key** (optional, required only for the LLM judge and the Simulate tool). Set it as `OPENAI_API_KEY` in your environment.

To check your Go version:

```bash
go version
# Expected output: go version go1.22.x (or later)
```

If you do not have Go installed, follow the instructions at [https://go.dev/doc/install](https://go.dev/doc/install).

### Install from Go Module

The simplest way to install FaultForge:

```bash
go install github.com/faultforge/faultforge/cmd/faultforge@latest
```

This downloads, compiles, and places the `faultforge` binary in your `$GOPATH/bin` (or `$HOME/go/bin` by default). Make sure that directory is in your `PATH`.

### Build from Source

Clone the repository and build:

```bash
git clone https://github.com/faultforge/faultforge.git
cd faultforge
make build
```

The compiled binary will be in `./bin/faultforge`.

You can also build manually without Make:

```bash
go build -o bin/faultforge ./cmd/faultforge
```

### Verify Installation

Run the version command to confirm FaultForge is installed correctly:

```bash
faultforge version
# Expected output: faultforge 0.1.0
```

### Development Tools

If you plan to contribute to FaultForge or run the test suite, install the development tools:

```bash
make tools    # installs golangci-lint and other dev dependencies
make check    # runs build + test + vet
```

Available Make targets:

| Target                   | Description                               |
|--------------------------|-------------------------------------------|
| `make build`             | Compile the binary to `./bin/faultforge`  |
| `make test`              | Run tests with race detector              |
| `make lint`              | Run golangci-lint                         |
| `make check`             | Build + test + vet (full CI check)        |
| `make run-example`       | Run the example chaos configuration       |
| `make run-simulate-example` | Run the example simulate configuration |
| `make tools`             | Install development tools                 |
| `make help`              | List all available targets                |

---

## 3. Quick Start

This section gets you running with both FaultForge tools in under five minutes.

### Quick Start: Chaos Testing

**Step 1.** Create a file called `chaos.yaml`:

```yaml
agent:
  name: my_agent
  entrypoint: python agent.py
  env:
    TOOL_BASE_URL: http://localhost:8080

proxy:
  port: 8080
  passthrough_url: https://my-real-tool-api.com
  request_timeout_s: 30

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

**Step 2.** Point your agent's tool calls at the FaultForge proxy by setting the `TOOL_BASE_URL` environment variable (this is handled automatically by the `agent.env` block in the config, but you can also set it manually):

```bash
export TOOL_BASE_URL=http://localhost:8080
```

**Step 3.** Run the chaos tests:

```bash
faultforge run chaos.yaml
```

FaultForge starts the proxy on port 8080 and launches your agent. Every request your agent makes to `/search` will receive an HTTP 504 timeout instead of the real response. The evaluation layer observes how your agent handles it.

### Quick Start: Simulate

**Step 1.** Create a file called `simulate.yaml`:

```yaml
agent:
  name: support_agent
  entrypoint: python agent.py
  base_url: http://localhost:3000
  request_timeout_s: 30

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

**Step 2.** Make sure your `OPENAI_API_KEY` is set (required for generating simulated user messages):

```bash
export OPENAI_API_KEY=sk-...
```

**Step 3.** Run the simulations:

```bash
faultforge simulate simulate.yaml
```

FaultForge creates a simulated user with the described persona, has it converse with your agent, and evaluates whether the goal was reached.

---

## 4. Tool 1: FaultForge Chaos

### 4.1 Concept and Architecture

FaultForge Chaos is an HTTP proxy that intercepts your AI agent's tool calls and injects controlled faults. It sits between your agent and the real tool API, selectively replacing real responses with faulty ones.

**Architecture overview:**

```
AI Agent --> FaultForge Proxy :8080 --> Real Tool API
                    |
              Fault Injector
                    |
              Evaluator (rules + LLM judge)
                    |
              Report (stdout / JSON / HTML)
```

Here is how it works, step by step:

1. **Your agent starts.** FaultForge launches your agent process using the configured `entrypoint` command. Environment variables (like `TOOL_BASE_URL`) are injected so the agent sends its HTTP requests to the FaultForge proxy instead of the real tool API.

2. **The proxy intercepts requests.** When your agent calls a tool (e.g., `POST /search`), the request hits the FaultForge proxy on the configured port.

3. **The fault injector decides what to do.** FaultForge checks whether the request matches any configured test (by comparing the URL path). If it matches and the probability check passes, FaultForge injects the configured fault instead of forwarding the request to the real backend. If no test matches, the request is forwarded to the real tool API (the `passthrough_url`) unchanged.

4. **The evaluator observes the agent's behavior.** After the test run completes, the evaluation layer analyzes how the agent responded. Rule-based detectors check for crashes, infinite loops, and recovery attempts. An optional LLM judge provides a nuanced assessment of the agent's behavior.

5. **A report is generated.** Results are written to the terminal, a JSON file, an HTML report, or all of the above.

### 4.2 Configuration Reference

The chaos configuration file is a YAML document with five top-level sections: `agent`, `proxy`, `tests`, `evaluation`, and `output`.

#### `agent` Section

Describes the AI agent under test.

```yaml
agent:
  name: support_agent
  entrypoint: python agent.py
  env:
    TOOL_BASE_URL: http://localhost:8080
    DEBUG: "true"
```

| Field        | Type              | Required | Description                                                                 |
|--------------|-------------------|----------|-----------------------------------------------------------------------------|
| `name`       | string            | Yes      | A human-readable identifier for the agent. Appears in reports.              |
| `entrypoint` | string            | No       | The shell command to start the agent process. FaultForge executes this.     |
| `env`        | map[string]string | No       | Environment variables to set for the agent process. Use this to point the agent's tool calls at the proxy (e.g., `TOOL_BASE_URL: http://localhost:8080`). |

**Notes:**

- The `name` field is required and must not be empty.
- The `entrypoint` can be any shell command: `python agent.py`, `node index.js`, `./my-agent`, etc.
- Environment variables in `env` are merged with the current environment when starting the agent process. They do not replace existing variables unless there is a name collision.

#### `proxy` Section

Configures the fault-injecting HTTP proxy.

```yaml
proxy:
  port: 8080
  passthrough_url: https://real-tool-api.com
  request_timeout_s: 30
```

| Field               | Type   | Required | Default | Description                                                                 |
|---------------------|--------|----------|---------|-----------------------------------------------------------------------------|
| `port`              | int    | Yes      | --      | The TCP port the proxy listens on. Your agent must send tool calls to this port. Must be greater than 0. |
| `passthrough_url`   | string | Yes      | --      | The URL of the real tool API. Requests that do not match any test are forwarded here. Must be a valid URL. |
| `request_timeout_s` | int    | No       | 30      | Timeout in seconds for requests forwarded to the real backend. If the real backend does not respond within this time, the proxy returns an error. |

**Notes:**

- Use a port that does not conflict with other services on your machine. Common choices: 8080, 9090, 3001.
- The `passthrough_url` should include the scheme (`http://` or `https://`) and host, but not a trailing path. Individual tool paths (like `/search`) are appended automatically.

#### `tests` Section

Defines the fault injection test cases. This is an array of test objects.

```yaml
tests:
  - id: timeout_on_search
    tool: /search
    fault: tool_timeout
    delay_ms: 30000
    probability: 1.0

  - id: bad_json_on_lookup
    tool: /lookup
    fault: invalid_json
    payload: "{ broken json %%% "
    probability: 0.5
```

| Field         | Type   | Required | Default | Description                                                                 |
|---------------|--------|----------|---------|-----------------------------------------------------------------------------|
| `id`          | string | Yes      | --      | A unique identifier for this test case. Used in CLI filtering (`--test`) and reports. Must not be empty. |
| `tool`        | string | Yes      | --      | The URL path to intercept. When the agent makes a request to this path, the fault is injected. Must not be empty. Examples: `/search`, `/lookup`, `/api/v1/tickets`. |
| `fault`       | string | Yes      | --      | The type of fault to inject. Must be one of: `tool_timeout`, `slow_response`, `tool_error`, `invalid_json`, `empty_response`, `rate_limit`. |
| `delay_ms`    | int    | No       | 0       | Delay in milliseconds. Used by `tool_timeout` (simulated wait time) and `slow_response` (actual delay before responding). |
| `probability` | float  | Yes      | --      | The probability that the fault is injected on any given request, from 0.0 (never) to 1.0 (always). Values outside this range cause a validation error. |
| `payload`     | string | No       | --      | Custom response payload. Used by `invalid_json` to specify the malformed JSON string. |
| `status_code` | int    | No       | --      | HTTP status code to return. Used by `tool_error` (e.g., 500, 502, 503) and `rate_limit` (429). |
| `body`        | string | No       | --      | Custom response body. Used by `tool_error` to specify the error message returned to the agent. |
| `retry_after_s` | int  | No       | --      | Value for the `Retry-After` header in seconds. Required for `rate_limit` faults. Must be greater than 0. |

**Notes:**

- Each test `id` should be unique within the configuration file. This ID is used to filter tests on the command line and identify tests in reports.
- The `tool` path must match the path your agent uses. If your agent calls `http://localhost:8080/search`, set `tool: /search`.
- Setting `probability: 1.0` means the fault is always injected. Setting `probability: 0.5` means the fault is injected roughly half the time, and the other half the request is forwarded to the real backend. This is useful for testing intermittent failures.

#### `evaluation` Section

Controls how test results are evaluated.

```yaml
evaluation:
  max_iterations: 20
  timeout_s: 60
  llm_judge: true
  llm_judge_prompt: |
    Did the agent handle the tool failure gracefully?
    Consider: Did it inform the user? Did it try an alternative? Did it crash?
```

| Field              | Type   | Required | Default | Description                                                                 |
|--------------------|--------|----------|---------|-----------------------------------------------------------------------------|
| `max_iterations`   | int    | No       | 20      | The maximum number of repeated calls to the same tool before the loop detector flags the agent for infinite looping. |
| `timeout_s`        | int    | No       | 60      | Maximum time in seconds for the entire evaluation to complete. If the agent has not finished within this time, the test is marked as timed out. |
| `llm_judge`        | bool   | No       | false   | Whether to enable LLM-based evaluation. When `true`, an OpenAI-compatible LLM evaluates the agent's behavior using the provided prompt. Requires the `OPENAI_API_KEY` environment variable. |
| `llm_judge_prompt` | string | No       | --      | The prompt sent to the LLM judge. This should describe what constitutes good and bad agent behavior. The prompt receives the agent's behavior as context. |

**Notes:**

- The loop detector works by counting how many times the agent calls the same tool endpoint. If it exceeds `max_iterations`, the behavior `infinite_loop` is flagged.
- If `llm_judge` is `true` but `OPENAI_API_KEY` is not set, FaultForge logs a warning and falls back to a no-op judge (rule-based evaluation only).
- Write specific, detailed judge prompts for best results. See the Evaluation System section below for guidance.

#### `output` Section

Controls report generation.

```yaml
output:
  format: both
  path: ./reports/
```

| Field    | Type   | Required | Default  | Description                                                                 |
|----------|--------|----------|----------|-----------------------------------------------------------------------------|
| `format` | string | No       | `stdout` | The output format. Must be one of: `stdout`, `json`, `html`, `both`. The `both` option writes to both the terminal and an HTML file. |
| `path`   | string | No       | `./`     | The directory where JSON and HTML report files are written. The directory is created if it does not exist. |

### 4.3 Fault Types

FaultForge provides six built-in fault types. Each simulates a specific category of tool failure that AI agents commonly encounter in production.

---

#### `tool_timeout`

**What it does:** Immediately returns an HTTP 504 Gateway Timeout response. The agent receives no useful data.

**What it simulates:** A tool that is completely unresponsive -- the server is down, a network partition occurred, or the request is stuck behind a load balancer that eventually gives up.

**When to use it:** To verify that your agent has its own timeout logic, does not hang indefinitely waiting for a response, and communicates the failure to the user.

**What to look for in your agent:**

- Does the agent have a client-side timeout shorter than the default?
- Does it inform the user that the tool is unavailable?
- Does it attempt a fallback strategy (e.g., trying a different tool)?
- Does it crash or raise an unhandled exception?

**Relevant configuration fields:**

| Field      | Notes                                   |
|------------|-----------------------------------------|
| `delay_ms` | Included for documentation but not used at the protocol level -- the 504 is returned immediately. |

**Example:**

```yaml
tests:
  - id: timeout_on_search
    tool: /search
    fault: tool_timeout
    delay_ms: 30000
    probability: 1.0
```

**HTTP response the agent receives:**

```
HTTP/1.1 504 Gateway Timeout
```

---

#### `slow_response`

**What it does:** Delays the response by the specified number of milliseconds before forwarding the actual response from the real backend. The agent eventually receives the real data, but only after waiting.

**What it simulates:** Degraded performance. The tool is working but slow -- high load, cold starts, network congestion, or a distant server.

**When to use it:** To verify that your agent handles latency gracefully. Does it wait patiently? Does it time out and recover? Does it inform the user about the delay?

**What to look for in your agent:**

- Does the agent's HTTP client have a timeout that fires before the delay completes?
- If the agent times out, does it retry or fail gracefully?
- Does the agent show a "thinking" or "loading" indicator to the user during the delay?
- If the response eventually arrives, does the agent use it or has it already moved on?

**Relevant configuration fields:**

| Field      | Notes                                                 |
|------------|-------------------------------------------------------|
| `delay_ms` | Required. The number of milliseconds to delay the response. Common values: 5000 (5s), 10000 (10s), 30000 (30s). |

**Example:**

```yaml
tests:
  - id: slow_search
    tool: /search
    fault: slow_response
    delay_ms: 8000
    probability: 1.0
```

---

#### `tool_error`

**What it does:** Returns a configurable HTTP 5xx error response with a custom error body. The agent receives an error instead of the expected data.

**What it simulates:** Server-side errors. The tool's backend is throwing exceptions, returning database errors, or experiencing a partial outage.

**When to use it:** To verify that your agent handles HTTP error responses correctly. Does it parse the error message? Does it retry with backoff? Does it crash on a non-200 status code?

**What to look for in your agent:**

- Does the agent check HTTP status codes or blindly parse the response body?
- Does it retry the request (and if so, how many times)?
- Does it inform the user about the error in a helpful way?
- Does it try an alternative tool or approach?

**Relevant configuration fields:**

| Field         | Notes                                                           |
|---------------|-----------------------------------------------------------------|
| `status_code` | Required. The HTTP status code to return. Common values: 500 (Internal Server Error), 502 (Bad Gateway), 503 (Service Unavailable). |
| `body`        | Optional. The response body to return. Use a JSON string for APIs that return structured errors. |

**Example:**

```yaml
tests:
  - id: server_error_on_search
    tool: /search
    fault: tool_error
    status_code: 500
    body: '{"error": "internal server error"}'
    probability: 1.0
```

**HTTP response the agent receives:**

```
HTTP/1.1 500 Internal Server Error
Content-Type: application/json

{"error": "internal server error"}
```

---

#### `invalid_json`

**What it does:** Returns an HTTP 200 OK with a malformed JSON payload. The status code says success, but the body cannot be parsed.

**What it simulates:** Data corruption. The tool returned a response, but something went wrong in serialization -- truncated output, encoding errors, a proxy that mangled the response, or a backend that mixed HTML into a JSON endpoint.

**When to use it:** To verify that your agent handles JSON parse errors gracefully. This is one of the most insidious failure modes because the HTTP status code is 200 (success), so agents that only check status codes will not detect the problem.

**What to look for in your agent:**

- Does the agent try to parse the response body and handle parse errors?
- Does it crash with an unhandled JSON decode exception?
- Does it hallucinate data when it cannot parse the response?
- Does it retry the request?

**Relevant configuration fields:**

| Field     | Notes                                                                  |
|-----------|------------------------------------------------------------------------|
| `payload` | Optional. The malformed JSON string to return. Defaults to an internally generated broken payload if not specified. |

**Example:**

```yaml
tests:
  - id: bad_json_on_lookup
    tool: /lookup
    fault: invalid_json
    payload: "{ broken json %%% "
    probability: 1.0
```

**HTTP response the agent receives:**

```
HTTP/1.1 200 OK
Content-Type: application/json

{ broken json %%%
```

---

#### `empty_response`

**What it does:** Returns an HTTP 200 OK with a completely empty body. No data, no error, just silence.

**What it simulates:** A tool that accepts the request and returns successfully but provides no data. This happens more often than you might expect: caches returning stale empty entries, search APIs returning zero results as an empty body, or microservices that return 200 with an empty response when data is not found.

**When to use it:** To verify that your agent handles the case where a tool "succeeds" but returns nothing. This is rarely tested and often causes confusing downstream behavior.

**What to look for in your agent:**

- Does the agent check for empty or null responses?
- Does it crash trying to access fields on a null/empty object?
- Does it tell the user "no results found" or does it hallucinate data?
- Does it retry the request?

**Relevant configuration fields:**

None beyond the standard fields (`id`, `tool`, `fault`, `probability`).

**Example:**

```yaml
tests:
  - id: empty_response_on_lookup
    tool: /lookup
    fault: empty_response
    probability: 1.0
```

**HTTP response the agent receives:**

```
HTTP/1.1 200 OK
Content-Type: application/json

```

(Empty body)

---

#### `rate_limit`

**What it does:** Returns an HTTP 429 Too Many Requests response with a `Retry-After` header and a JSON error body indicating rate limiting.

**What it simulates:** API rate limiting. The tool is healthy but has rejected the request because the agent is making too many calls. This is extremely common with third-party APIs (search engines, LLM APIs, SaaS platforms).

**When to use it:** To verify that your agent implements proper backoff behavior when rate-limited. Does it respect the `Retry-After` header? Does it spam retries? Does it inform the user about the delay?

**What to look for in your agent:**

- Does the agent read the `Retry-After` header and wait the specified time?
- Does it implement exponential backoff?
- Does it stop retrying after a maximum number of attempts?
- Does it spam the API in a tight loop (which would make the rate limiting worse)?
- Does it inform the user about the delay?

**Relevant configuration fields:**

| Field           | Notes                                                              |
|-----------------|--------------------------------------------------------------------|
| `status_code`   | Optional. Defaults to 429. You can also use 429 explicitly.        |
| `retry_after_s` | Required. The value of the `Retry-After` header in seconds. Must be greater than 0. |

**Example:**

```yaml
tests:
  - id: rate_limit_on_search
    tool: /search
    fault: rate_limit
    status_code: 429
    retry_after_s: 60
    probability: 1.0
```

**HTTP response the agent receives:**

```
HTTP/1.1 429 Too Many Requests
Content-Type: application/json
Retry-After: 60

{"error": "rate limit exceeded", "retry_after": 60}
```

---

### 4.4 Evaluation System

FaultForge evaluates agent behavior using a two-layer system: deterministic rule-based detectors and an optional LLM judge.

#### Rule-Based Detectors

Rule-based detectors run automatically on every test. They produce deterministic, repeatable results.

**Loop Detector**

The loop detector counts how many times the agent calls the same tool endpoint during a test. If the count exceeds the `max_iterations` threshold (default: 20), the agent is flagged with the `infinite_loop` behavior.

This catches agents that get stuck in a retry loop: the tool returns an error, the agent retries, gets the same error, retries again, and never stops. This is one of the most common and dangerous failure modes in production agents.

**Crash Detector**

The crash detector checks whether the agent's response includes a 5xx HTTP status code or an unhandled exception. If so, the agent is flagged with the `crash` behavior.

A crash behavior automatically causes the test to fail, regardless of the LLM judge verdict.

**Recovery Detector**

The recovery detector checks whether the agent recovered from the fault. If the agent encountered an error but eventually produced a useful response (e.g., by trying a different tool, using cached data, or informing the user gracefully), it is flagged with `recovery_success`. If it encountered an error and did not recover, it is flagged with `recovery_failed`.

#### Detected Behaviors

The evaluation system can flag the following behaviors:

| Behavior            | Description                                                       | Causes Test Failure? |
|---------------------|-------------------------------------------------------------------|----------------------|
| `crash`             | The agent crashed or returned a 5xx error.                        | Yes                  |
| `infinite_loop`     | The agent called the same tool more times than `max_iterations`.  | No (warning)         |
| `recovery_success`  | The agent encountered an error and recovered gracefully.          | No                   |
| `recovery_failed`   | The agent encountered an error and did not recover.               | No (warning)         |
| `hallucination`     | The agent fabricated data instead of reporting the failure.        | No (warning)         |
| `fallback_used`     | The agent used a fallback strategy (e.g., a different tool).      | No                   |
| `timeout`           | The agent exceeded the evaluation timeout.                        | No (warning)         |

#### LLM Judge

The LLM judge provides nuanced, context-aware evaluation of agent behavior. It uses the OpenAI API (model `gpt-4o-mini` by default) to assess whether the agent handled the fault gracefully.

**How it works:**

1. The agent's behavior during the test (tool calls, responses, errors) is collected.
2. The behavior is sent to the LLM along with your custom `llm_judge_prompt`.
3. The LLM returns a verdict (`PASS` or `FAIL`) and a reason explaining its judgment.

**Verdicts:**

| Verdict   | Meaning                                                    |
|-----------|------------------------------------------------------------|
| `PASS`    | The agent handled the failure appropriately.               |
| `FAIL`    | The agent did not handle the failure appropriately.        |
| `SKIPPED` | No LLM judge prompt was configured; rule-based only.      |

**Writing effective judge prompts:**

Be specific about what you consider good and bad behavior. Generic prompts produce generic results.

Bad prompt:
```yaml
llm_judge_prompt: "Did the agent do well?"
```

Good prompt:
```yaml
llm_judge_prompt: |
  Did the agent handle the tool failure gracefully?

  PASS criteria:
  - The agent informed the user that the tool was unavailable.
  - The agent did NOT fabricate or hallucinate data.
  - The agent tried an alternative approach or offered helpful suggestions.

  FAIL criteria:
  - The agent crashed or returned an error to the user.
  - The agent pretended the tool worked and made up results.
  - The agent got stuck in a loop.
  - The agent silently dropped the user's request.
```

**Pass/fail logic:**

A test passes if and only if:
1. No `crash` behavior was detected by the rule-based detectors, **AND**
2. The LLM judge verdict is `PASS` or `SKIPPED`.

If either condition fails, the test fails.

### 4.5 Running Tests

#### Basic Usage

```bash
faultforge run chaos.yaml
```

This runs all tests defined in `chaos.yaml`, starts the proxy, launches the agent, injects faults, evaluates behavior, and produces reports.

#### CLI Flags

| Flag       | Type   | Description                                                         |
|------------|--------|---------------------------------------------------------------------|
| `--output` | string | Output file path for the report. The format is auto-detected from the file extension: `.html` produces an HTML report, `.json` produces a JSON report. Overrides the `output.format` and `output.path` settings in the config file. |
| `--test`   | string | Run only the test with this ID. All other tests in the config are skipped. Useful for debugging a single fault scenario. |

#### Examples

Run all tests with default output (from config):
```bash
faultforge run chaos.yaml
```

Run all tests and write an HTML report:
```bash
faultforge run chaos.yaml --output reports/my-report.html
```

Run all tests and write a JSON report:
```bash
faultforge run chaos.yaml --output reports/my-report.json
```

Run only a specific test:
```bash
faultforge run chaos.yaml --test timeout_on_search
```

Combine both flags:
```bash
faultforge run chaos.yaml --test timeout_on_search --output report.html
```

#### What Happens When You Run

1. FaultForge loads and validates the configuration file.
2. The fault registry is initialized with all six built-in fault types.
3. The HTTP proxy starts on the configured port.
4. The agent process is launched with the configured entrypoint and environment variables.
5. The proxy intercepts tool calls matching the configured test paths.
6. For matching requests, the proxy injects faults according to the test configuration and probability.
7. Non-matching requests are forwarded to the real backend (`passthrough_url`).
8. After the agent completes (or the process is stopped with Ctrl+C), the evaluator analyzes the behavior.
9. Reports are generated in the configured format.

#### Stopping a Test Run

Press `Ctrl+C` (sends `SIGINT`) to stop the test run gracefully. FaultForge handles the signal, shuts down the proxy, and generates any pending reports.

You can also send `SIGTERM` for the same behavior.

### 4.6 Reading Reports

FaultForge produces reports in three formats: terminal (stdout), JSON, and HTML.

#### Stdout Report

The terminal report is designed for quick feedback during development. It looks like this:

```
=== FaultForge Reliability Report ===
Agent: support_agent  |  Run: 2026-03-27 14:30:00
Tests: 3  |  Passed: 2  |  Failed: 1  |  Score: 66%

--- Results ---
[PASS] timeout_on_search (tool_timeout on /search) -- 1250ms
       Behaviors: recovery_success
       Judge: PASS -- Agent informed the user and suggested trying again later.

[PASS] slow_search (slow_response on /search) -- 8340ms
       Behaviors: recovery_success
       Judge: PASS -- Agent waited for the response and used the data.

[FAIL] bad_json_on_lookup (invalid_json on /lookup) -- 450ms
       Behaviors: recovery_failed
       Judge: FAIL -- Agent attempted to parse the response and crashed.
       Error: json: cannot unmarshal string into Go value
```

**How to read it:**

- **Header:** Agent name, timestamp, test counts, and overall score (percentage of passed tests).
- **Each test result:** Pass/fail marker, test ID, fault type, tool path, and duration.
- **Behaviors:** Rule-based detector findings.
- **Judge:** LLM judge verdict and reason.
- **Error:** Any error message from the agent or the evaluation.

#### JSON Report

The JSON report is designed for programmatic consumption -- CI/CD pipelines, dashboards, or custom analysis tools.

```json
{
  "agent_name": "support_agent",
  "run_at": "2026-03-27T14:30:00Z",
  "total_tests": 3,
  "passed": 2,
  "failed": 1,
  "score": 66,
  "results": [
    {
      "test_id": "timeout_on_search",
      "fault_type": "tool_timeout",
      "tool": "/search",
      "passed": true,
      "detected_behaviors": ["recovery_success"],
      "llm_judge_verdict": "PASS",
      "llm_judge_reason": "Agent informed the user and suggested trying again later.",
      "duration_ms": 1250,
      "error": ""
    }
  ]
}
```

#### HTML Report

The HTML report is a self-contained HTML file with styled tables, color-coded pass/fail indicators, and expandable details for each test result. Open it in any web browser.

Generate an HTML report with:
```bash
faultforge run chaos.yaml --output report.html
```

Or configure it in the YAML:
```yaml
output:
  format: html
  path: ./reports/
```

### 4.7 Tutorial: Testing a Python Agent

This step-by-step tutorial walks you through testing a Python agent with FaultForge. We will inject a timeout fault into a search tool and verify the agent handles it gracefully.

#### Step 1: Write a Simple Python Agent

Create a file called `agent.py`:

```python
import os
import requests
import json
import sys

TOOL_BASE_URL = os.environ.get("TOOL_BASE_URL", "http://localhost:8080")

def search(query):
    """Call the search tool and return results."""
    try:
        response = requests.post(
            f"{TOOL_BASE_URL}/search",
            json={"query": query},
            timeout=10  # 10-second client timeout
        )
        response.raise_for_status()
        return response.json()
    except requests.exceptions.Timeout:
        return {"error": "Search tool timed out. Please try again later."}
    except requests.exceptions.HTTPError as e:
        return {"error": f"Search tool returned an error: {e.response.status_code}"}
    except json.JSONDecodeError:
        return {"error": "Search tool returned invalid data."}

def main():
    query = "How do I reset my password?"
    print(f"Searching for: {query}")
    result = search(query)
    print(f"Result: {json.dumps(result, indent=2)}")

if __name__ == "__main__":
    main()
```

This agent has basic error handling: it catches timeouts, HTTP errors, and JSON decode errors.

#### Step 2: Create the Chaos Configuration

Create a file called `chaos.yaml`:

```yaml
agent:
  name: password_reset_agent
  entrypoint: python agent.py
  env:
    TOOL_BASE_URL: http://localhost:8080

proxy:
  port: 8080
  passthrough_url: https://your-real-search-api.com
  request_timeout_s: 30

tests:
  - id: timeout_on_search
    tool: /search
    fault: tool_timeout
    probability: 1.0

  - id: bad_json_on_search
    tool: /search
    fault: invalid_json
    payload: "<html>502 Bad Gateway</html>"
    probability: 1.0

  - id: empty_search_results
    tool: /search
    fault: empty_response
    probability: 1.0

evaluation:
  max_iterations: 20
  timeout_s: 60
  llm_judge: true
  llm_judge_prompt: |
    Evaluate whether the agent handled the tool failure gracefully.

    PASS if the agent:
    - Caught the error and did not crash
    - Provided a useful message to the user
    - Did not fabricate search results

    FAIL if the agent:
    - Crashed with an unhandled exception
    - Returned raw error details to the user
    - Made up search results that did not come from the tool

output:
  format: both
  path: ./reports/
```

#### Step 3: Run the Tests

```bash
# Set your OpenAI API key for the LLM judge
export OPENAI_API_KEY=sk-...

# Run all chaos tests
faultforge run chaos.yaml

# Or run a specific test
faultforge run chaos.yaml --test timeout_on_search

# Generate an HTML report
faultforge run chaos.yaml --output reports/agent-reliability.html
```

#### Step 4: Analyze the Results

Open the terminal output or the HTML report. For each test, check:

1. **Did the test pass or fail?** If it failed, the agent does not handle that failure mode correctly.
2. **What behaviors were detected?** Look for `crash`, `infinite_loop`, or `recovery_failed`.
3. **What did the LLM judge say?** Read the reason for additional context.

#### Step 5: Fix and Re-Test

If a test fails, update your agent's error handling and run the test again:

```bash
faultforge run chaos.yaml --test bad_json_on_search
```

Iterate until all tests pass. Your agent is now more reliable.

---

## 5. Tool 2: FaultForge Simulate

### 5.1 Concept and Architecture

FaultForge Simulate is a conversation simulator that tests AI agents by having them interact with simulated users. Instead of injecting tool faults, it creates realistic user personas and evaluates whether the agent can achieve goals through conversation.

**Architecture overview:**

```
UserSimulator (LLM) --> AI Agent --> Evaluator --> ConversationReport
```

Here is how it works:

1. **A simulated user is created.** Based on the configured persona (e.g., "frustrated user who wants to cancel their subscription"), an LLM generates realistic user messages.

2. **The conversation begins.** The simulated user sends a message to your agent. Your agent responds. The simulated user responds again. This continues for up to `max_turns` turns.

3. **The evaluator assesses the conversation.** After the conversation ends, the evaluation layer checks whether the goal was reached, how efficiently the agent handled the conversation, and the overall quality of the interaction.

4. **A report is generated.** Results include goal completion, turn count, quality scores, and identified issues.

### 5.2 Configuration Reference

The simulate configuration file is a YAML document with four top-level sections: `agent`, `simulations`, `evaluation`, and `output`.

#### `agent` Section

```yaml
agent:
  name: support_agent
  entrypoint: python agent.py
  base_url: http://localhost:3000
  request_timeout_s: 30
```

| Field               | Type   | Required | Default | Description                                                                 |
|---------------------|--------|----------|---------|-----------------------------------------------------------------------------|
| `name`              | string | Yes      | --      | A human-readable identifier for the agent. Appears in reports.              |
| `entrypoint`        | string | No       | --      | Shell command to start the agent. Used when the agent is not already running. |
| `base_url`          | string | Yes      | --      | The URL where the agent's conversation API is accessible. FaultForge sends simulated user messages to this URL. Must not be empty. |
| `request_timeout_s` | int    | No       | 30      | Timeout in seconds for requests to the agent's API.                         |

#### `simulations` Section

Defines the simulated user scenarios. This is an array of simulation objects.

```yaml
simulations:
  - id: impatient_user
    persona: "Frustrated user who wants to solve their problem in less than 3 messages"
    goal: "Cancel their subscription"
    max_turns: 10
    success_criteria: "The agent completed the cancellation"

  - id: confused_user
    persona: "User who doesn't understand the product well and changes their mind"
    goal: "Subscribe to a premium plan"
    max_turns: 15
    success_criteria: "The agent guided the user to complete the subscription"
```

| Field              | Type   | Required | Default | Description                                                                 |
|--------------------|--------|----------|---------|-----------------------------------------------------------------------------|
| `id`               | string | Yes      | --      | A unique identifier for this simulation. Used in CLI filtering (`--sim`) and reports. |
| `persona`          | string | Yes      | --      | A natural language description of the simulated user's personality, behavior, and constraints. The LLM uses this to generate realistic user messages. |
| `goal`             | string | Yes      | --      | The objective the simulated user is trying to achieve through the conversation. |
| `max_turns`        | int    | Yes      | --      | The maximum number of conversation turns (user message + agent response pairs). The simulation ends when this limit is reached, even if the goal has not been achieved. Must be greater than 0. |
| `success_criteria` | string | No       | --      | A natural language description of what constitutes success. Used by the evaluator to determine if the goal was reached. |

#### `evaluation` Section

```yaml
evaluation:
  goal_completion: true
  turn_efficiency: true
  tone_quality: true
  llm_judge_prompt: |
    Rate the conversation quality from 1-10.
    Consider: Was the goal achieved? Was the agent polite?
    Was the conversation efficient?
```

| Field              | Type   | Required | Default | Description                                                                 |
|--------------------|--------|----------|---------|-----------------------------------------------------------------------------|
| `goal_completion`  | bool   | No       | false   | Whether to evaluate if the simulation's goal was reached.                   |
| `turn_efficiency`  | bool   | No       | false   | Whether to evaluate how efficiently the agent achieved the goal (fewer turns is better). |
| `tone_quality`     | bool   | No       | false   | Whether to evaluate the tone and professionalism of the agent's responses.  |
| `llm_judge_prompt` | string | No       | --      | Custom prompt for the LLM judge to evaluate the conversation. The judge receives the full conversation history and this prompt. |

#### `output` Section

Same as the chaos output section:

```yaml
output:
  format: both
  path: ./reports/
```

| Field    | Type   | Required | Default  | Description                                                                 |
|----------|--------|----------|----------|-----------------------------------------------------------------------------|
| `format` | string | No       | `stdout` | `stdout`, `json`, `html`, or `both`.                                        |
| `path`   | string | No       | `./`     | Directory for report files.                                                 |

### 5.3 Designing Personas

The `persona` field is the most important part of a simulation. It tells the LLM how to behave as a simulated user. Good personas produce realistic, challenging conversations. Bad personas produce generic, easy interactions that do not test your agent meaningfully.

#### Principles for Effective Personas

**Be specific about behavior, not just personality.**

Bad:
```yaml
persona: "An angry user"
```

Good:
```yaml
persona: "A frustrated user who has been transferred between departments three times. They are short-tempered, interrupt frequently, and want to solve their problem in under 3 messages. They will threaten to cancel if not helped immediately."
```

**Include constraints and quirks.**

```yaml
persona: "A user who does not understand technical jargon. When the agent uses terms like 'API', 'endpoint', or 'credentials', the user gets confused and asks for simpler explanations. They sometimes misunderstand instructions and do the wrong thing."
```

**Model real user archetypes.** Think about the actual users of your agent and the difficult interactions your support team handles:

```yaml
# The multi-tasker
persona: "A busy professional who is doing multiple things at once. Their messages are short, sometimes incomplete, and they often change topics mid-conversation."

# The over-explainer
persona: "A user who provides extremely detailed context about their problem, including irrelevant information. They write long messages and expect the agent to read everything carefully."

# The adversarial user
persona: "A user who is trying to get the agent to do something outside its capabilities. They will rephrase their request multiple times and get increasingly creative with their wording."

# The non-native speaker
persona: "A user whose first language is not English. They sometimes use incorrect grammar, misspell words, and may use literal translations of phrases from their language."
```

#### Common Persona Templates

Here are ready-to-use persona templates for common testing scenarios:

```yaml
# Impatient user
persona: "Frustrated user who expects immediate resolution. Will express dissatisfaction if the conversation takes more than 3 exchanges. Uses short, direct messages."

# Confused user
persona: "User who is not tech-savvy. Gets confused by technical instructions, often asks 'what do you mean?' and needs step-by-step guidance with simple language."

# Hostile user
persona: "Angry user who had a bad experience. Uses strong language (but not abusive), demands to speak to a manager, and threatens negative reviews. Tests agent de-escalation."

# Vague user
persona: "User who describes their problem in vague terms. Says things like 'it doesn't work' without specifying what 'it' is. The agent needs to ask clarifying questions."

# Multi-issue user
persona: "User who has multiple unrelated problems and brings them up in a single conversation. The agent needs to handle topic switching and prioritization."
```

### 5.4 Defining Goals and Success Criteria

The `goal` and `success_criteria` fields work together to define what a successful simulation looks like.

#### Goals

The `goal` is what the simulated user is trying to accomplish. It should be concrete and measurable.

Good goals:
```yaml
goal: "Cancel their subscription"
goal: "Reset their password"
goal: "Upgrade from the free plan to the premium plan"
goal: "Get a refund for their last purchase"
goal: "Find out the status of their order #12345"
```

Bad goals:
```yaml
goal: "Have a good conversation"  # Too vague
goal: "Test the agent"            # Not a user goal
```

#### Success Criteria

The `success_criteria` is a natural language description of what the evaluator should look for to determine if the goal was reached.

```yaml
success_criteria: "The agent completed the cancellation and provided a confirmation number"
success_criteria: "The agent sent a password reset email and confirmed the user received it"
success_criteria: "The agent processed the upgrade and the user confirmed the new plan details"
```

If no `success_criteria` is provided, the evaluator uses the `goal` alone to assess completion.

### 5.5 Evaluation Metrics

FaultForge Simulate evaluates conversations across three dimensions:

#### Goal Completion

**What it measures:** Did the simulated user's goal get achieved?

**How it works:** The evaluator examines the full conversation history and the defined goal/success criteria. The LLM judge determines whether the conversation ended with the goal being met.

**Reported as:** `GoalReached: true/false` in the simulation result.

#### Turn Efficiency

**What it measures:** How many conversation turns did it take to reach the goal?

**How it works:** The evaluator counts the total turns and compares against the `max_turns` limit. Fewer turns for the same outcome indicates a more efficient agent.

**Reported as:** `TurnCount: N` and `MaxTurns: M` in the simulation result.

#### Tone Quality

**What it measures:** Was the agent's tone appropriate, professional, and helpful?

**How it works:** The LLM judge evaluates the agent's responses for empathy, professionalism, clarity, and appropriateness given the user's emotional state.

**Reported as:** `QualityScore: 1-10` in the simulation result.

#### Quality Score

The overall quality score is a 1-10 rating produced by the LLM judge based on the `llm_judge_prompt`. It considers all enabled metrics (goal completion, turn efficiency, tone quality) and any custom criteria in the prompt.

#### Issues

The evaluator may also report specific issues found during the conversation:

- Agent used overly technical language with a confused user.
- Agent did not acknowledge the user's frustration.
- Agent asked for information the user already provided.
- Agent went off-topic.
- Goal was not achieved within the turn limit.

### 5.6 Running Simulations

#### Basic Usage

```bash
faultforge simulate simulate.yaml
```

This runs all simulations defined in the config file.

#### CLI Flags

| Flag       | Type   | Description                                                            |
|------------|--------|------------------------------------------------------------------------|
| `--output` | string | Output file path for the report. Format auto-detected from extension (`.html` or `.json`). |
| `--sim`    | string | Run only the simulation with this ID. All other simulations are skipped. |

#### Examples

Run all simulations:
```bash
faultforge simulate simulate.yaml
```

Run a specific simulation:
```bash
faultforge simulate simulate.yaml --sim impatient_user
```

Generate an HTML report:
```bash
faultforge simulate simulate.yaml --output reports/simulation-report.html
```

Combine both flags:
```bash
faultforge simulate simulate.yaml --sim confused_user --output report.html
```

#### Prerequisites

The Simulate tool requires:

1. **`OPENAI_API_KEY` environment variable.** This is mandatory. The LLM is used to generate simulated user messages and to evaluate the conversation. If this variable is not set, FaultForge will exit with an error.

2. **Your agent must be accessible.** Either FaultForge starts it using the `entrypoint` command, or it must already be running and reachable at `base_url`.

### 5.7 Reading Reports

#### Stdout Report

```
=== FaultForge Simulation Report ===
Agent: support_agent  |  Run: 2026-03-27 14:30:00
Simulations: 2  |  Goal Reached: 1  |  Avg Score: 7.5

--- Results ---
[REACHED] impatient_user (persona: Frustrated user who wants...) -- 4500ms
       Goal: Cancel their subscription
       Turns: 4/10  |  Score: 8

[MISSED] confused_user (persona: User who doesn't understand...) -- 12000ms
       Goal: Subscribe to a premium plan
       Turns: 15/15  |  Score: 5
       Issues: Goal not achieved within turn limit, Agent used jargon
```

**How to read it:**

- **[REACHED] / [MISSED]:** Whether the simulation's goal was achieved.
- **Turns: 4/10:** The agent used 4 turns out of a maximum of 10 to reach the goal. Fewer is better.
- **Score:** Quality score from 1 (poor) to 10 (excellent).
- **Issues:** Specific problems identified by the evaluator.

#### JSON Report

```json
{
  "agent_name": "support_agent",
  "run_at": "2026-03-27T14:30:00Z",
  "total_sims": 2,
  "goal_reached": 1,
  "avg_score": 7.5,
  "results": [
    {
      "simulation_id": "impatient_user",
      "persona": "Frustrated user who wants to solve their problem in less than 3 messages",
      "goal": "Cancel their subscription",
      "goal_reached": true,
      "turn_count": 4,
      "max_turns": 10,
      "quality_score": 8,
      "issues": [],
      "duration_ms": 4500
    },
    {
      "simulation_id": "confused_user",
      "persona": "User who doesn't understand the product well",
      "goal": "Subscribe to a premium plan",
      "goal_reached": false,
      "turn_count": 15,
      "max_turns": 15,
      "quality_score": 5,
      "issues": ["Goal not achieved within turn limit", "Agent used jargon"],
      "duration_ms": 12000
    }
  ]
}
```

#### HTML Report

The HTML report shows the same data as above in a styled, interactive format with color-coded results. Generate it with:

```bash
faultforge simulate simulate.yaml --output report.html
```

### 5.8 Tutorial: Simulating Users for a Support Agent

This tutorial walks through testing a customer support agent with simulated users.

#### Step 1: Prepare Your Agent

Make sure your agent is running and accessible over HTTP. For example, if your agent exposes a conversation API at `http://localhost:3000`, verify it works:

```bash
curl -X POST http://localhost:3000 \
  -H "Content-Type: application/json" \
  -d '{"message": "Hello, I need help"}'
```

#### Step 2: Create the Simulation Configuration

Create `simulate.yaml`:

```yaml
agent:
  name: acme_support_agent
  base_url: http://localhost:3000
  request_timeout_s: 30

simulations:
  - id: cancel_subscription
    persona: "Frustrated customer who has been waiting 20 minutes on hold before reaching this chat. They want to cancel their subscription immediately and are not interested in retention offers. They become increasingly agitated if the process takes more than 3 messages."
    goal: "Cancel their premium subscription"
    max_turns: 10
    success_criteria: "The agent processed the cancellation and provided a confirmation"

  - id: billing_dispute
    persona: "A calm but firm customer who noticed a double charge on their credit card statement. They have the transaction IDs ready and expect a quick resolution. They will not accept being told to call a different department."
    goal: "Get the duplicate charge refunded"
    max_turns: 12
    success_criteria: "The agent initiated the refund process and provided a timeline"

  - id: confused_upgrade
    persona: "An elderly user who is not comfortable with technology. They received an email about upgrading their plan but do not understand the difference between plans. They need patient, step-by-step guidance using simple language. They may ask the same question multiple times."
    goal: "Upgrade from the Basic plan to the Premium plan"
    max_turns: 20
    success_criteria: "The agent guided the user through the upgrade and confirmed the new plan"

  - id: multi_issue
    persona: "A power user who has three separate issues: a bug report, a feature request, and a billing question. They present all three in their first message and expect the agent to address each one."
    goal: "Get all three issues acknowledged and routed appropriately"
    max_turns: 15
    success_criteria: "Each issue was acknowledged and the user knows the next step for each"

evaluation:
  goal_completion: true
  turn_efficiency: true
  tone_quality: true
  llm_judge_prompt: |
    Evaluate this customer support conversation on a scale of 1-10.

    Consider the following criteria:
    - Goal completion: Did the agent resolve the customer's issue?
    - Efficiency: Was the conversation resolved in a reasonable number of turns?
    - Tone: Was the agent empathetic, professional, and patient?
    - Clarity: Were the agent's instructions clear and easy to follow?
    - Personalization: Did the agent adapt to the user's communication style?

    Deduct points for:
    - Using technical jargon with non-technical users
    - Asking for information the user already provided
    - Ignoring the user's emotional state
    - Failing to resolve the issue within the turn limit

output:
  format: both
  path: ./reports/
```

#### Step 3: Set the OpenAI API Key

```bash
export OPENAI_API_KEY=sk-...
```

#### Step 4: Run the Simulations

Run all simulations:
```bash
faultforge simulate simulate.yaml
```

Or test one scenario at a time:
```bash
faultforge simulate simulate.yaml --sim cancel_subscription
```

Generate an HTML report for sharing with your team:
```bash
faultforge simulate simulate.yaml --output reports/support-agent-eval.html
```

#### Step 5: Analyze and Iterate

Review the results:

1. **Which goals were reached?** If `confused_upgrade` missed, your agent may need better handling of non-technical users.
2. **How efficient was the agent?** If `cancel_subscription` took 9 out of 10 turns, the cancellation flow is too cumbersome.
3. **What issues were flagged?** Common issues like "agent used jargon" or "agent did not acknowledge frustration" point to specific areas for improvement.
4. **What was the quality score?** Scores below 6 indicate significant problems. Scores of 8+ indicate a well-performing agent.

Update your agent's prompts, tools, or logic based on the findings and re-run the simulations.

---

## 6. Configuration Validation

FaultForge provides a `validate` command to check your configuration files for errors before running tests or simulations.

### Usage

```bash
faultforge validate <config-file>
```

### Examples

Validate a chaos configuration:
```bash
faultforge validate chaos.yaml
# Output: Config valid (chaos)
```

Validate a simulate configuration:
```bash
faultforge validate simulate.yaml
# Output: Config valid (simulate)
```

### How It Works

The `validate` command attempts to parse the file as both a chaos config and a simulate config. If either succeeds validation, the command reports which type it is and exits with code 0.

If both fail validation, the command prints the errors for both formats:

```
config validation failed:
  as chaos: config: validating chaos.yaml: agent.name is required
  as simulate: config: validating simulate.yaml: agent.base_url is required
```

### Validation Rules

**Chaos configuration validation:**

- `agent.name` must not be empty.
- `proxy.port` must be greater than 0.
- `proxy.passthrough_url` must not be empty.
- Each test must have a non-empty `id`, `tool`, and `fault`.
- Each test's `probability` must be between 0.0 and 1.0.
- `output.format` (if set) must be one of: `stdout`, `json`, `html`, `both`.

**Simulate configuration validation:**

- `agent.name` must not be empty.
- `agent.base_url` must not be empty.
- Each simulation must have a non-empty `id`, `persona`, and `goal`.
- Each simulation's `max_turns` must be greater than 0.
- `output.format` (if set) must be one of: `stdout`, `json`, `html`, `both`.

### Best Practice

Always validate your config before running tests, especially in CI/CD pipelines:

```bash
faultforge validate chaos.yaml && faultforge run chaos.yaml
```

---

## 7. Environment Variables

FaultForge uses the following environment variables:

| Variable         | Required                          | Description                                                                 |
|------------------|-----------------------------------|-----------------------------------------------------------------------------|
| `OPENAI_API_KEY` | Yes, for LLM judge and Simulate   | Your OpenAI API key. Used by the LLM judge (in chaos mode when `llm_judge: true`) and by the Simulate tool (always required). The key is used to call the OpenAI Chat Completions API with model `gpt-4o-mini`. |

### Setting Environment Variables

**Temporary (current shell session):**
```bash
export OPENAI_API_KEY=sk-your-key-here
```

**Permanent (add to your shell profile):**

For bash (`~/.bashrc` or `~/.bash_profile`):
```bash
echo 'export OPENAI_API_KEY=sk-your-key-here' >> ~/.bashrc
source ~/.bashrc
```

For zsh (`~/.zshrc`):
```bash
echo 'export OPENAI_API_KEY=sk-your-key-here' >> ~/.zshrc
source ~/.zshrc
```

### Behavior Without `OPENAI_API_KEY`

- **Chaos mode with `llm_judge: true`:** FaultForge logs a warning and falls back to a no-op judge. Only rule-based evaluation runs. Tests can still pass or fail based on rule-based detectors.
- **Chaos mode with `llm_judge: false`:** The variable is not needed.
- **Simulate mode:** FaultForge exits immediately with an error: `OPENAI_API_KEY environment variable is required for simulations`.

### Agent-Specific Environment Variables

You can pass any environment variable to your agent through the `agent.env` block in the configuration:

```yaml
agent:
  name: my_agent
  entrypoint: python agent.py
  env:
    TOOL_BASE_URL: http://localhost:8080
    DEBUG: "true"
    LOG_LEVEL: "verbose"
    DATABASE_URL: "postgresql://localhost/mydb"
```

These variables are set in the agent's process environment when FaultForge launches it.

---

## 8. Output Formats

FaultForge supports four output format options.

### `stdout` (default)

Prints the report to the terminal in a human-readable format. Best for local development and quick feedback.

```yaml
output:
  format: stdout
```

### `json`

Writes a JSON file to the configured path. Best for CI/CD pipelines, custom dashboards, and programmatic analysis.

```yaml
output:
  format: json
  path: ./reports/
```

### `html`

Writes a self-contained HTML file with styled results. Best for sharing with stakeholders, attaching to pull requests, or archiving.

```yaml
output:
  format: html
  path: ./reports/
```

### `both`

Prints to the terminal AND writes an HTML file. Best for development: you get immediate feedback in the terminal and a persistent report file.

```yaml
output:
  format: both
  path: ./reports/
```

### Overriding with `--output`

The `--output` CLI flag overrides the config file's output settings. The format is auto-detected from the file extension:

```bash
# Produces an HTML report regardless of config settings
faultforge run chaos.yaml --output my-report.html

# Produces a JSON report regardless of config settings
faultforge run chaos.yaml --output my-report.json
```

---

## 9. Troubleshooting

### Common Errors and Solutions

---

**Error:** `OPENAI_API_KEY environment variable is required for simulations`

**Cause:** You are running `faultforge simulate` but the `OPENAI_API_KEY` variable is not set.

**Solution:** Set the variable before running:
```bash
export OPENAI_API_KEY=sk-your-key-here
faultforge simulate simulate.yaml
```

---

**Error:** `config: validating chaos.yaml: agent.name is required`

**Cause:** Your chaos configuration is missing the `agent.name` field.

**Solution:** Add a `name` field to the `agent` section:
```yaml
agent:
  name: my_agent
```

---

**Error:** `config: validating chaos.yaml: proxy.port must be greater than 0`

**Cause:** The `proxy.port` field is missing or set to 0.

**Solution:** Set a valid port number:
```yaml
proxy:
  port: 8080
```

---

**Error:** `config: validating chaos.yaml: tests[0].probability must be between 0 and 1`

**Cause:** A test's `probability` value is negative or greater than 1.

**Solution:** Set it to a value between 0.0 and 1.0:
```yaml
probability: 1.0
```

---

**Error:** `no test found with id "my_test"`

**Cause:** You used `--test my_test` but no test in the config file has `id: my_test`.

**Solution:** Check the test IDs in your config file. IDs are case-sensitive.

---

**Error:** `no simulation found with id "my_sim"`

**Cause:** You used `--sim my_sim` but no simulation in the config file has `id: my_sim`.

**Solution:** Check the simulation IDs in your config file.

---

**Error:** `faults: building rate_limit: RetryAfterS must be > 0, got 0`

**Cause:** A `rate_limit` fault test does not have a `retry_after_s` value, or it is set to 0.

**Solution:** Add a positive `retry_after_s` value:
```yaml
tests:
  - id: rate_limit_test
    tool: /search
    fault: rate_limit
    retry_after_s: 60
    probability: 1.0
```

---

**Error:** `output.format must be one of: stdout, json, html, both (got "xml")`

**Cause:** The `output.format` field has an unsupported value.

**Solution:** Use one of: `stdout`, `json`, `html`, `both`.

---

**Error:** `proxy error: listen tcp :8080: bind: address already in use`

**Cause:** Another process is already using port 8080.

**Solution:** Either stop the other process or use a different port:
```yaml
proxy:
  port: 9090
```

---

**Error:** `OpenAI API error: Incorrect API key provided`

**Cause:** The `OPENAI_API_KEY` value is invalid or expired.

**Solution:** Verify your API key at [https://platform.openai.com/api-keys](https://platform.openai.com/api-keys) and update the environment variable.

---

**Error:** Config file not found or is not valid YAML.

**Cause:** The file path is wrong or the YAML syntax is broken.

**Solution:** Check the file path and validate your YAML with an online validator or `faultforge validate`.

---

### General Tips

- **Always validate before running:** `faultforge validate chaos.yaml`
- **Start with one test at a time:** Use `--test` or `--sim` to isolate issues.
- **Check your agent is reachable:** Before running simulations, verify your agent responds to HTTP requests.
- **Use `probability: 1.0` for debugging:** When troubleshooting, set probability to 1.0 so faults are always injected. Lower it for realistic intermittent failure testing later.
- **Read the LLM judge reason:** The reason field often contains the most actionable feedback.

---

## 10. FAQ

**Q: Do I need to modify my agent's code to use FaultForge?**

A: No. FaultForge works as an HTTP proxy. The only change is pointing your agent's tool calls at the proxy URL (e.g., `http://localhost:8080`) instead of the real API. This is typically done with an environment variable. Your agent code stays the same.

**Q: Can I test agents written in any language?**

A: Yes. FaultForge is language-agnostic. It works with any agent that makes HTTP requests to external tools -- Python, Node.js, Go, Java, Rust, or any other language. The `entrypoint` field in the config accepts any shell command.

**Q: What OpenAI models does FaultForge use?**

A: FaultForge uses `gpt-4o-mini` by default for both the LLM judge and the user simulator. This model provides a good balance of quality and cost.

**Q: Can I use a different LLM provider instead of OpenAI?**

A: The current version only supports the OpenAI API. Future versions may add support for other providers.

**Q: How much does the LLM usage cost?**

A: Cost depends on the number of tests and simulations you run. Each chaos test with an LLM judge makes one API call. Each simulation turn makes one API call for the simulated user and one for evaluation. With `gpt-4o-mini`, costs are typically a few cents per test run.

**Q: Can I run FaultForge in CI/CD?**

A: Yes. Use the JSON output format for machine-readable results and check the exit code. FaultForge exits with code 0 on success and code 1 on errors. Example GitHub Actions step:

```yaml
- name: Run reliability tests
  env:
    OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
  run: |
    faultforge validate chaos.yaml
    faultforge run chaos.yaml --output reports/reliability.json
```

**Q: What happens to requests that do not match any test?**

A: They are forwarded to the real backend (`passthrough_url`) unchanged. FaultForge only intercepts requests to tool paths that match a configured test.

**Q: Can I inject faults probabilistically?**

A: Yes. Set the `probability` field to a value between 0.0 and 1.0. For example, `probability: 0.3` means the fault is injected 30% of the time, and the remaining 70% of requests go through to the real backend. This is useful for testing intermittent failures.

**Q: Can I run multiple fault types on the same tool?**

A: Yes. Define multiple tests with different IDs but the same `tool` path. However, only one fault is injected per request. Use `--test` to run them individually, or define separate test runs for each fault.

**Q: What is the difference between `tool_timeout` and `slow_response`?**

A: `tool_timeout` returns an HTTP 504 immediately -- the agent gets an error response right away. `slow_response` delays the real response by the configured time -- the agent eventually gets the actual data, but after waiting. Use `tool_timeout` to test complete failure handling. Use `slow_response` to test latency tolerance.

**Q: How do I test my agent against multiple faults at once?**

A: Define multiple tests in your configuration, each targeting different tool paths or different fault types. Run them all at once with `faultforge run chaos.yaml` (without the `--test` flag). Each test is evaluated independently.

**Q: What is the `both` output format?**

A: It produces both a terminal report (printed to stdout) and an HTML file (written to the configured path). You get immediate feedback in the terminal and a persistent, shareable report file.

**Q: Can I use FaultForge with agents that use WebSockets?**

A: No. FaultForge currently only supports HTTP request-response patterns. WebSocket support may be added in a future version.

**Q: Where can I find example configuration files?**

A: The repository includes example configurations in the `configs/` directory:
- `configs/chaos.example.yaml` -- Example chaos testing configuration.
- `configs/simulate.example.yaml` -- Example simulation configuration.
