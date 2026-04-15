# ruptor examples

Worked examples that exercise `ruptor run` and `ruptor simulate` against
real (or mocked) agents. Each directory stands on its own: no shared
state, no shared virtualenv, no order dependency.

| Example | What it tests | Requirements |
|---|---|---|
| [01-quickstart](./01-quickstart) | Fault injection basics | Python 3, Flask |
| [02-ollama](./02-ollama) | Fault injection with a real LLM | Python 3, Ollama + qwen2 |
| [03-multi-tool](./03-multi-tool) | Multiple tool fault injection | Python 3, Flask |
| [04-simulate-ollama](./04-simulate-ollama) | User simulation with a real LLM | Python 3, Ollama + qwen2 |

## Conventions

- **No hardcoded keys.** Every external endpoint is configurable.
- **`TOOL_BASE_URL`** points the agent at the ruptor proxy; overridable
  per run so the same script works with and without fault injection.
- **`OLLAMA_MODEL`** (Ollama examples) defaults to `qwen2`; override to
  try smaller or larger models without editing code.
- **`run.sh`** is the single entrypoint per example. It installs Python
  dependencies into a local `.venv`, starts supporting processes, and
  invokes `ruptor`. A missing dependency is reported as a clear error
  rather than a stack trace.

## Prerequisites (global)

- `ruptor` on your `$PATH` (`brew install ruptor-dev/tap/ruptor` or
  `go install github.com/ruptor-dev/cli/cmd/ruptor@latest`)
- Python 3.10+
- `bash` (run.sh scripts use bash features)

Ollama-dependent examples additionally need Ollama running locally:
`ollama serve` and `ollama pull qwen2`.
