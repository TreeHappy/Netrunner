#!/usr/bin/env bash
# Interactive netrunner card browser (TUI).
# Usage: nr-browse.sh [--plain] [--width N] [--no-icons] [code]
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

BIN="$ROOT/.cache/nrbrowse"
if [ ! -x "$BIN" ] || [ "$ROOT/cmd/nrbrowse" -nt "$BIN" ] || \
   [ "$ROOT/internal/image/image.go" -nt "$BIN" ] || \
   [ "$ROOT/internal/render/render.go" -nt "$BIN" ]; then
  mkdir -p "$(dirname "$BIN")"
  go build -o "$BIN" "$ROOT/cmd/nrbrowse"
fi

exec "$BIN" "$@"
