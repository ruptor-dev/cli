#!/usr/bin/env bash
# smoke-tui.sh — headless regression check for the --verbose TUI log
# panel. Runs the `tuismoke`-tagged tests (pty-driven: they exec the
# real binary, assert AltScreen entry, mouse cell motion, and the
# `Logs · tail` title). A failure here means the panel is either
# unmounted or rendering broken — the same class of bug the operator
# hit when the panel showed up empty on a real TTY.
#
# Usage: bash scripts/smoke-tui.sh  (or `make smoke-tui`).

set -euo pipefail

cd "$(dirname "$0")/.."

echo "▸ building ruptor"
go build -o ./bin/ruptor ./cmd/ruptor

echo "▸ running tuismoke tests (pty)"
go test -tags=tuismoke ./cmd/ruptor/... -count=1 -timeout 60s -run 'TestTUI'

echo "✓ TUI smoke passed"
