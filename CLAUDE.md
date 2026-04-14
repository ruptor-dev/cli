# Ruptor CLI — CLAUDE.md

## Read shared context FIRST

Before any task, read these files (relative paths assume ~/dev/ruptor-dev/):
- ../knowledge/architecture.md
- ../knowledge/engineering.md
- ../knowledge/security.md
- ../knowledge/v1-scope.md
- ../knowledge/decisions/ (skim all ADRs)

## What this repo is

OSS CLI for reliability testing of AI agents.
License: Apache 2.0
Module: github.com/ruptor-dev/cli
Binary: ruptor (NOT faultforge — see AUDIT section below)
Install: go install github.com/ruptor-dev/cli/cmd/ruptor@latest

Two commands:
- `ruptor run chaos.yaml` — HTTP/MCP proxy that injects faults into tool calls
- `ruptor simulate simulate.yaml` — simulates user personas to test goal completion

## ⚠️ MANDATORY FIRST TASK: AUDIT

The codebase was previously named "faultforge" and needs to be corrected.
Before implementing ANY new feature, complete this audit:

### Step 1 — Read everything
Read ALL files in internal/, pkg/, cmd/, configs/, and the root.
Understand what exists before adding anything.

### Step 2 — Create AUDIT.md at repo root
Document findings:
- What is fully implemented and working
- What is partially implemented
- What is in cmd/ but not in internal/ (or vice versa)
- What contradicts decisions in ../knowledge/decisions/
- What code references "faultforge" that needs renaming
- What is missing from v1 scope (see ../knowledge/v1-scope.md)
- What exists but is OUT of v1 scope and should be noted/deferred

### Step 3 — Rename faultforge → ruptor
- cmd/faultforge/ → cmd/ruptor/
- All internal references to "faultforge" in strings, comments, variable names
- go.mod module path: github.com/ruptor-dev/cli
- Binary name in Makefile and goreleaser config
- Any README references

### Step 4 — Fix architecture violations
Based on the audit, fix code that violates:
- fmt.Println in command files (move to internal/ui/)
- Missing interfaces in pkg/types/ (interfaces before implementations)
- Secrets or API keys in code
- InsecureSkipVerify anywhere

### Step 5 — Report
Update AUDIT.md with what was fixed, what was deferred to a later PR, and
what new work is needed for v1 scope.

Only after AUDIT.md is complete and fixes are applied, proceed to new features.

## Directory structure

```
cli/
├── bin/                    # compiled binaries (gitignored)
├── cmd/
│   └── ruptor/             # main package — was cmd/faultforge/, rename this
│       └── main.go
├── configs/                # example config files for users
│   ├── chaos.example.yaml
│   └── simulate.example.yaml
├── docs/
│   └── superpowers/        # skill files for Claude Code agents
│       └── specs/
├── internal/
│   ├── auth/               # OAuth device flow, token management
│   ├── cloud/              # cloud reporting client (disabled in v1)
│   │   └── feature.go      # CloudReportingEnabled = false
│   ├── config/             # Viper config loading, ~/.ruptor/config.yaml
│   ├── evaluator/
│   │   ├── llmjudge/       # LLM-based evaluation
│   │   │   └── prompts/    # prompt files as .txt (NOT hardcoded in Go)
│   │   └── rules/          # deterministic rule-based evaluation
│   ├── llmclient/          # LLM API client (OpenAI, Anthropic)
│   ├── proxy/
│   │   ├── faults/         # fault handlers (Chain of Responsibility)
│   │   └── mcp/            # MCP proxy mode (v1)
│   ├── report/             # HTML + JSON report generation
│   ├── simulate/           # user persona simulation
│   ├── telemetry/          # OTel opt-in telemetry
│   └── ui/                 # ALL terminal output — Bubbletea + Lipgloss
│       └── theme.go        # Ruptor color theme
├── pkg/
│   └── types/              # shared interfaces and types (public API)
├── testdata/               # test fixtures, example configs, persona files
└── web/                    # local report viewer assets (HTML/CSS/JS)
```

## Stack

| Dependency | Version | Purpose |
|-----------|---------|---------|
| Go | 1.26.2 | Language |
| github.com/spf13/cobra | v2.5.1 | CLI framework |
| github.com/spf13/viper | v1.31+ | Config management |
| github.com/charmbracelet/bubbletea | latest | TUI framework |
| github.com/charmbracelet/lipgloss/v2 | latest | Terminal styling |
| github.com/charmbracelet/bubbles | latest | TUI components |
| github.com/rs/zerolog | latest | Structured logging |
| go.opentelemetry.io/otel | v1.x latest | Telemetry |
| github.com/cenkalti/backoff/v4 | v4 | Retry with jitter |
| github.com/goreleaser/goreleaser | v2 | Release automation |

## Code rules

### Output
ALL terminal output goes through internal/ui/.
NEVER call fmt.Println, fmt.Printf, or log.Printf in cmd/ or internal/ (except ui/).
Use ui.Success(), ui.Error(), ui.Info(), ui.RunProgress() etc.

### Interfaces
Define interfaces in pkg/types/ BEFORE writing implementations.
Multiple agents work in parallel — interfaces are the contract between them.

### Fault types
Each fault is a struct implementing FaultHandler in pkg/types/.
Register in internal/proxy/faults/registry.go.
NEVER modify existing fault handlers to add new behavior.

### Prompts
LLM judge prompts live in internal/evaluator/llmjudge/prompts/*.txt
NEVER hardcode prompt strings in Go source code.

### No secrets in code
Config loads from env vars or ~/.ruptor/config.yaml.
Test API keys use RUPTOR_TEST_OPENAI_KEY etc.

### TLS
InsecureSkipVerify = false always. Use mkcert for local TLS in tests.

## UI/TUI requirements (non-functional but mandatory)

The terminal output MUST be beautiful. This is a product requirement.
Use Lipgloss v2 for all styling. The Ruptor theme is in internal/ui/theme.go.

During a run, show live progress with Bubbletea:
- Box header with command and config file
- Live experiment list with status icons (✓ ✗ ◌ ○)
- Real-time Robustness Score progress bar
- Keyboard shortcuts (q to abort, r to retry failed)

On completion:
- Large Robustness Score display with color coding
- Summary (X passed, X failed, X error)
- Report file paths with → arrows
- "Open report? [Y/n]" prompt

Colors (from theme.go):
- Primary: #E84545 (Ruptor red — fractura)
- Success: #22C55E
- Failure: #EF4444
- Warning: #F59E0B
- Muted: #6B7280
- Border: #374151

## Config hierarchy (Viper)

Priority (highest first):
1. CLI flag (--cloud, --output, etc.)
2. RUPTOR_* environment variable
3. .ruptor.yaml in current directory (gitignored, project-local)
4. ~/.ruptor/config.yaml (created by ruptor auth login)
5. Hardcoded default

## Feature flag

internal/cloud/feature.go:
```go
// CloudReportingEnabled controls whether --cloud flag sends data to ruptor.dev
// Set to true only when the platform API is live.
const CloudReportingEnabled = false
```

When user passes --cloud and CloudReportingEnabled is false:
Show: "☁ Cloud reporting is coming soon. Join the waitlist at https://ruptor.dev"
Do NOT fail the run. Continue normally with local report.

## Retry with jitter (cenkalti/backoff v4)

Used in: reporting client, LLM judge calls, auth token refresh.

```go
b := backoff.NewExponentialBackOff()
b.InitialInterval     = 500 * time.Millisecond
b.Multiplier          = 2.0
b.RandomizationFactor = 0.5   // ±50% jitter
b.MaxInterval         = 30 * time.Second
b.MaxElapsedTime      = 5 * time.Minute
```

## Testing rules

Unit tests: no I/O, no real LLM, mock everything. Run < 30s.
Integration tests: real HTTP server via httptest.NewServer.
E2E tests: build tag `//go:build e2e`. Not in CI by default.

Every fault type and evaluator change requires unit + integration tests in same PR.

## Makefile targets (must exist and work)

make build          # compile binary to bin/ruptor
make test           # unit tests
make test-int       # integration tests
make lint           # golangci-lint
make check          # build + test + lint
make run-example    # run chaos example from testdata/
make run-simulate-example  # run simulate example from testdata/
make release-dry    # goreleaser --snapshot (no publish)

## PR rules

- One logical change per PR
- Tests required for fault/evaluator changes
- No fmt.Println in non-ui code
- No InsecureSkipVerify
- No hardcoded secrets or tokens
- CLAUDE.md updated if a decision changes
