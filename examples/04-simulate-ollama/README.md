# 04 — Simulate with Ollama

Pure `ruptor simulate` example. Three personas have a real multi-turn
conversation with an Ollama-backed chat agent, and the LLM judge scores
each transcript on goal completion and tone.

## What it demonstrates

- `ruptor simulate` end-to-end against a local LLM (no cloud key required)
- Per-session conversation history so multi-turn dialogs have continuity
- Stricter scoring thresholds (`goal_completion >= 0.8`,
  `tone_quality >= 0.7`) so borderline runs are flagged instead of
  rubber-stamped

## Prerequisites

```bash
brew install ruptor-dev/tap/ruptor
brew install ollama
ollama serve &
ollama pull qwen2

python3 --version
```

## Configuration

| Env | Default | Purpose |
|---|---|---|
| `OLLAMA_URL`   | `http://localhost:11434` | Ollama API root |
| `OLLAMA_MODEL` | `qwen2`                  | Ollama model tag |
| `SYSTEM_PROMPT`| support agent default    | System message seed for the agent |

## Run

```bash
./run.sh
```

## Expected output

Three personas each produce a transcript and a score. The HTML report
highlights which persona failed a threshold. Typical pattern on
`qwen2`: `patient_explorer` passes, `frustrated_user` often fails on
tone, `non_native_speaker` fails on goal completion when the agent
slips into jargon.

## What to look for

- **Tone vs goal decoupling.** A conversation can reach the goal while
  scoring poorly on tone, or vice versa — the two thresholds catch
  both failure modes.
- **Model choice matters.** Swap `OLLAMA_MODEL=llama3.1` or similar and
  rerun. The score delta across models is the headline number.
- **Transcript reading.** Open the generated HTML and read the actual
  conversation — the score alone hides which turn derailed the run.
