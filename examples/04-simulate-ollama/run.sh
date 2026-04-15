#!/usr/bin/env bash
# 04-simulate-ollama runner. Purely a simulate-mode example — no
# fault injection, no tool passthrough. Starts the conversational
# agent on :3000 and runs three personas against it.

set -euo pipefail

cd "$(dirname "$0")"

OLLAMA_URL="${OLLAMA_URL:-http://localhost:11434}"
OLLAMA_MODEL="${OLLAMA_MODEL:-qwen2}"
export OLLAMA_URL OLLAMA_MODEL

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "error: '$1' is required but not installed." >&2
    exit 1
  }
}

need python3
need curl

if ! command -v "${RUPTOR_BIN:-ruptor}" &>/dev/null; then
  if [ -z "${RUPTOR_BIN:-}" ]; then
    echo "✗ ruptor not found. Install: brew install ruptor-dev/tap/ruptor"
  else
    echo "✗ ruptor binary not found at $RUPTOR_BIN"
  fi
  exit 1
fi

RUPTOR="${RUPTOR_BIN:-ruptor}"

if ! curl -sf "$OLLAMA_URL/api/tags" >/dev/null; then
  echo "error: Ollama not reachable at $OLLAMA_URL. Run: ollama serve" >&2
  exit 1
fi

if ! curl -sf "$OLLAMA_URL/api/tags" | grep -q "\"$OLLAMA_MODEL\""; then
  echo "warn: model '$OLLAMA_MODEL' not pulled. Run: ollama pull $OLLAMA_MODEL" >&2
fi

if [ ! -d .venv ]; then
  python3 -m venv .venv
fi
# shellcheck disable=SC1091
source .venv/bin/activate
pip install --quiet --upgrade pip
pip install --quiet -r requirements.txt

python agent.py >/tmp/ruptor-04-agent.log 2>&1 &
AGENT_PID=$!
trap 'kill "$AGENT_PID" 2>/dev/null || true' EXIT

for _ in {1..40}; do
  if curl -sf http://localhost:3000/chat -X POST -H 'content-type: application/json' \
       -d '{"message":"ping","session_id":"warmup"}' >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done

echo "▸ $RUPTOR simulate simulate.yaml"
"$RUPTOR" simulate simulate.yaml
