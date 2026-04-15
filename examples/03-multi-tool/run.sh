#!/usr/bin/env bash
# 03-multi-tool runner. Same shape as 01 but with a three-endpoint
# mock service so faults can be targeted per tool independently.

set -euo pipefail

cd "$(dirname "$0")"

need() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "error: '$1' is required but not installed." >&2
    exit 1
  }
}

need python3

if ! command -v "${RUPTOR_BIN:-ruptor}" &>/dev/null; then
  if [ -z "${RUPTOR_BIN:-}" ]; then
    echo "✗ ruptor not found. Install: brew install ruptor-dev/tap/ruptor"
  else
    echo "✗ ruptor binary not found at $RUPTOR_BIN"
  fi
  exit 1
fi

RUPTOR="${RUPTOR_BIN:-ruptor}"

if [ ! -d .venv ]; then
  python3 -m venv .venv
fi
# shellcheck disable=SC1091
source .venv/bin/activate
pip install --quiet --upgrade pip
pip install --quiet -r requirements.txt

python mock_tool_server.py >/tmp/ruptor-03-mock.log 2>&1 &
MOCK_PID=$!
trap 'kill "$MOCK_PID" 2>/dev/null || true' EXIT

for _ in {1..20}; do
  if curl -sf http://localhost:9090/search?q=ping >/dev/null; then
    break
  fi
  sleep 0.25
done

export TOOL_BASE_URL="http://localhost:8080"

echo "▸ $RUPTOR run chaos.yaml"
"$RUPTOR" run chaos.yaml
