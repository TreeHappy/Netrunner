#!/usr/bin/env bash
# Download card artwork into the local image cache for terminal image
# rendering (kitty/sixel). Default destination is the XDG cache dir
# (~/.cache/netrunner/card-images); override with NETRUNNER_IMAGES.
# Idempotent: skips already-downloaded images.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEST="${NETRUNNER_IMAGES:-${XDG_CACHE_HOME:-$HOME/.cache}/netrunner/card-images}"
DB="$ROOT/data/netrunner.duckdb"
JOBS="${JOBS:-16}"

mkdir -p "$DEST"

if [[ ! -f "$DB" ]]; then
  echo "error: $DB not found — run scripts/nr-build-cache.sh" >&2
  exit 1
fi

codes=$(duckdb -noheader -list "$DB" -c "SELECT code FROM cards ORDER BY code;")
total=$(echo "$codes" | wc -l)
n=0 skipped=0 fetched=0 failed=0

for code in $codes; do
  n=$((n + 1))
  if [[ -s "$DEST/$code.jpg" ]]; then
    skipped=$((skipped + 1))
    continue
  fi
  url="https://card-images.netrunnerdb.com/v2/large/${code}.jpg"
  if curl -fsSL --retry 2 --max-time 30 -o "$DEST/$code.jpg" "$url"; then
    fetched=$((fetched + 1))
  else
    rm -f "$DEST/$code.jpg"
    failed=$((failed + 1))
  fi
  ((n % 25 == 0)) && echo "… $n/$total checked"
done

echo "done: $fetched fetched, $skipped cached, $failed missing ($total total)"
