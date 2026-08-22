#!/usr/bin/env bash
# Interactive netrunner card browser (TUI).
# Usage: nr-browse.sh [--plain] [--width N] [--no-icons] [code]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

BIN="$ROOT/.cache/nrbrowse"
if [ ! -x "$BIN" ] || [ -n "$(find "$ROOT/cmd/nrbrowse" "$ROOT/internal/image" \
    "$ROOT/internal/render" -name '*.go' -newer "$BIN" 2>/dev/null)" ]; then
  mkdir -p "$(dirname "$BIN")"
  go build -o "$BIN" "$ROOT/cmd/nrbrowse"
fi

exec "$BIN" "$@"
