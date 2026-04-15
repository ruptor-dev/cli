#!/usr/bin/env bash
# 01-quickstart runner. Spins up the mock tool server on :9090, points
# the agent at the ruptor proxy on :8080, and runs chaos.yaml. Cleans
# up the mock server on exit whether the run succeeds or not.

set -euo pipefail

cd "$(dirname "$0")"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "error: '$1' is required but not installed." >&2
    exit 1
  }
}

need python3
need ruptor

if [ ! -d .venv ]; then
  echo "▸ creating .venv"
  python3 -m venv .venv
fi

# shellcheck disable=SC1091
source .venv/bin/activate
pip install --quiet --upgrade pip
pip install --quiet -r requirements.txt

echo "▸ starting mock tool server on :9090"
python mock_tool_server.py >/tmp/ruptor-01-mock.log 2>&1 &
MOCK_PID=$!
trap 'kill "$MOCK_PID" 2>/dev/null || true' EXIT

# Give Flask a moment to bind.
for _ in {1..20}; do
  if curl -sf http://localhost:9090/search?q=ping >/dev/null; then
    break
  fi
  sleep 0.25
done

export TOOL_BASE_URL="http://localhost:8080"

echo "▸ running ruptor run chaos.yaml"
ruptor run chaos.yaml
