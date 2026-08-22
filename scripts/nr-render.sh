#!/usr/bin/env bash
# Render netrunner cards in the terminal.
# Usage: nr-render.sh [--plain] [--width N] [--no-icons] <code | card.json | pack.json:code>...
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

BIN="$ROOT/.cache/nrrender"
if [ ! -x "$BIN" ] || [ "$ROOT/cmd/nrrender" -nt "$BIN" ] || \
   { [ "$ROOT/internal/render/render.go" -nt "$BIN" ] || [ "$ROOT/internal/carddb/carddb.go" -nt "$BIN" ] || [ "$ROOT/internal/carddb/json.go" -nt "$BIN" ]; }; then
  mkdir -p "$(dirname "$BIN")"
  go build -o "$BIN" "$ROOT/cmd/nrrender"
fi

exec "$BIN" "$@"
