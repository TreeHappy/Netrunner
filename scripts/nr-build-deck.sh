#!/usr/bin/env bash
# Interactive Netrunner deck builder (TUI).
# Usage: nr-build-deck.sh [--plain] [--width N] [--no-icons] [--images] [deckfile]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

BIN="$ROOT/.cache/nrbuild"
if [ ! -x "$BIN" ] || [ "$ROOT/cmd/nrbuild" -nt "$BIN" ]; then
  mkdir -p "$(dirname "$BIN")"
  go build -o "$BIN" "$ROOT/cmd/nrbuild"
fi

exec "$BIN" "$@"
