#!/usr/bin/env bash
# Build the persistent DuckDB card cache from the netrunner-cards-json checkout.
# Idempotent: deletes and recreates data/netrunner.duckdb.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SRC="$ROOT/data/netrunner-cards-json"
DB="$ROOT/data/netrunner.duckdb"

if [[ ! -d "$SRC/pack" ]]; then
  echo "error: $SRC/pack not found — run:" >&2
  echo "  scripts/nr-cards.sh" >&2
  exit 1
fi

rm -f "$DB" "$DB".wal

duckdb "$DB" <<SQL
CREATE TABLE cards AS
  SELECT * FROM read_json_auto('$SRC/pack/*.json', union_by_name = true) ORDER BY code;
CREATE TABLE cycles   AS SELECT * FROM read_json_auto('$SRC/cycles.json');
CREATE TABLE factions AS SELECT * FROM read_json_auto('$SRC/factions.json');
CREATE TABLE types    AS SELECT * FROM read_json_auto('$SRC/types.json');
CREATE TABLE sides    AS SELECT * FROM read_json_auto('$SRC/sides.json');
SQL

echo "built $DB"
duckdb "$DB" -c "SELECT side_code, count(*) FROM cards GROUP BY 1 ORDER BY 1;"
