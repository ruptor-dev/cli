#!/usr/bin/env bash
# 02-ollama runner. Needs Ollama running locally with the configured
# model pulled. Starts the Flask-wrapped agent on :3000 (for simulate)
# plus the mock tool backend on :9090 (for chaos), then runs both
# ruptor flows back to back.

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
  echo "error: Ollama is not reachable at $OLLAMA_URL." >&2
  echo "  start it with: ollama serve" >&2
  echo "  pull the model: ollama pull $OLLAMA_MODEL" >&2
  exit 1
fi

if ! curl -sf "$OLLAMA_URL/api/tags" | grep -q "\"$OLLAMA_MODEL\""; then
  echo "warn: model '$OLLAMA_MODEL' is not pulled. Run: ollama pull $OLLAMA_MODEL" >&2
fi

if [ ! -d .venv ]; then
  python3 -m venv .venv
fi
# shellcheck disable=SC1091
source .venv/bin/activate
pip install --quiet --upgrade pip
pip install --quiet -r requirements.txt

# Reuse the mock server from 01 so we do not duplicate scaffolding.
cp ../01-quickstart/mock_tool_server.py ./_mock_tool_server.py
python _mock_tool_server.py >/tmp/ruptor-02-mock.log 2>&1 &
MOCK_PID=$!
trap 'kill "$MOCK_PID" 2>/dev/null || true; rm -f ./_mock_tool_server.py' EXIT

export TOOL_BASE_URL="http://localhost:8080"

# Chaos: ruptor's own runner launches agent.py (mode: persistent) and
# stops it at the end — no manual &-launch here, the runner owns the
# lifecycle.
echo "▸ $RUPTOR run chaos.yaml $*"
"$RUPTOR" run chaos.yaml "$@"

# Simulate: ruptor does not yet launch the entrypoint for simulate
# mode, so we spin up agent.py manually for this phase and take it
# down on exit.
python agent.py >/tmp/ruptor-02-agent.log 2>&1 &
AGENT_PID=$!
trap 'kill "$MOCK_PID" "$AGENT_PID" 2>/dev/null || true; rm -f ./_mock_tool_server.py' EXIT

for _ in {1..40}; do
  if curl -sf http://localhost:3000/chat -X POST -H 'content-type: application/json' \
       -d '{"message":"ping","session_id":"warmup"}' >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done

echo
echo "▸ $RUPTOR simulate simulate.yaml"
"$RUPTOR" simulate simulate.yaml
