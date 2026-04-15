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
need ruptor
need curl

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

python agent.py >/tmp/ruptor-02-agent.log 2>&1 &
AGENT_PID=$!
trap 'kill "$MOCK_PID" "$AGENT_PID" 2>/dev/null || true; rm -f ./_mock_tool_server.py' EXIT

for _ in {1..40}; do
  if curl -sf http://localhost:3000/chat -X POST -H 'content-type: application/json' -d '{"message":"ping"}' >/dev/null 2>&1; then
    break
  fi
  sleep 0.5
done

export TOOL_BASE_URL="http://localhost:8080"

echo "▸ ruptor run chaos.yaml"
ruptor run chaos.yaml

echo
echo "▸ ruptor simulate simulate.yaml"
ruptor simulate simulate.yaml
