#!/usr/bin/env bash
# Init or update the netrunner-cards-json data submodule with a sparse
# checkout containing only the card data used by nr-build-cache.sh.
#
# Usage:
#   scripts/nr-cards.sh           # ensure submodule exists at pinned commit (sparse)
#   scripts/nr-cards.sh --update  # also fetch and fast-forward to upstream default branch
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$ROOT/data/netrunner-cards-json"
SPARSE="/pack/ /cycles.json /factions.json /types.json /sides.json"

if [[ "${1:-}" == "--update" ]]; then
  echo "updating netrunner-cards-json to latest upstream..."
  git -C "$ROOT" submodule update --init --remote "$SRC"
else
  git -C "$ROOT" submodule update --init "$SRC"
fi

git -C "$SRC" sparse-checkout set --no-cone $SPARSE

if ! ls "$SRC"/pack/*.json >/dev/null 2>&1; then
  echo "error: $SRC/pack contains no JSON files" >&2
  exit 1
fi

echo "cards-json ready at $SRC ($(ls "$SRC"/pack/*.json | wc -l) pack files)"
