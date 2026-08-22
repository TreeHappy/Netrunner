#!/usr/bin/env bash
# Emit one line per card for pickers (tv/fzf):
#   CODE<TAB>title · type · faction
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DB="${NETRUNNER_DB:-$ROOT/data/netrunner.duckdb}"

duckdb "$DB" -noheader -list -c "
  SELECT code || chr(9) || title || ' · ' || type_code || ' · ' || faction_code
  FROM cards ORDER BY title;"
