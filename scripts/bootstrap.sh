#!/usr/bin/env bash
# Bootstrap the project: install mise + tools (go, duckdb), fetch card data,
# build the DuckDB cache, and smoke-test the scripts. Idempotent — safe to re-run.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# 1. Install mise if missing.
if ! command -v mise >/dev/null 2>&1; then
  echo "==> installing mise…"
  curl -fsSL https://mise.run | sh
  export PATH="$HOME/.local/bin:$PATH"
fi

# 2. Install pinned-in-.mise.toml tools (go, duckdb) at latest.
echo "==> mise install (go, duckdb)…"
mise install --yes
eval "$(mise activate bash)"

echo "go:     $(go version)"
echo "duckdb: $(duckdb --version)"

# 3. Fetch/update the card data submodule.
if [[ ! -d "$ROOT/data/netrunner-cards-json/pack" ]]; then
  echo "==> fetching netrunner-cards-json submodule…"
  scripts/nr-cards.sh
else
  echo "==> card data present ($(ls "$ROOT"/data/netrunner-cards-json/pack | wc -l) packs); use scripts/nr-cards.sh --update to refresh."
fi

# 4. Build the DuckDB cache if missing or stale vs. submodule data.
if [[ ! -f "$ROOT/data/netrunner.duckdb" ]] || \
   [[ -n "$(find "$ROOT/data/netrunner-cards-json" -name '*.json' -newer "$ROOT/data/netrunner.duckdb" -print -quit)" ]]; then
  echo "==> building DuckDB cache…"
  scripts/nr-build-cache.sh
else
  echo "==> DuckDB cache up to date."
fi

# 5. Smoke test.
echo "==> smoke test:"
scripts/nr-list.sh | head -3
echo
echo "Done. Try:"
echo "  scripts/nr-browse.sh          # interactive card browser"
echo "  scripts/nr-build-deck.sh      # deck builder TUI"
