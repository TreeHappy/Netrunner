# Deck builder — design notes & roadmap

Status after the first implementation pass (2026-08). This note pins down
where `cmd/nrbuild` is today, the design ideas behind it, and where to work
next.

## Design ideas

### 1. The browser is a SQL query frontend

Everything card-selection related compiles down to a single abstraction:
`carddb.Query` (`internal/carddb/query.go`). Filter state in any TUI is just a
`Query` value; refreshing the list means `carddb.Run(db, q)` — one parameterized
SQL statement against the DuckDB cache (`data/netrunner.duckdb`). There is no
in-memory filtering anywhere anymore.

This keeps the "CLI tool philosophy": every consumer of card data (browser,
deck builder, future exporters, AI agents) shares the exact same selection
semantics, and adding a filter dimension (pack, cost range, keywords) only
touches one file.

### 2. Deck rules live outside the TUI

`internal/deck` is a pure library — no I/O, no terminal. A deck is
`Deck{Identity Card, Entries []Entry}` and validation is `Validate() []Issue`.
Rules implemented today:

- min deck size = max(45, identity `minimum_deck_size`)
- ≤ 3 copies per name (per code enforced in `Add`; across printings checked in `Validate`)
- influence: out-of-faction cost × copies vs identity `influence_limit`
- corp agenda range: base 18–20, +2 min for every 5 cards above minimum
- uniqueness ≤ 1 copy
- side consistency with the identity

The TUI only *renders* issues; it cannot disagree with the rules engine.

### 3. One process, embedded picker

`nrbuild` embeds the browse picker rather than spawning `nrbrowse` per pick.
Reason: terminal image placements (kitty graphics) must survive repaints;
subprocess chaining would tear them down constantly. Composition is kept at
the edges instead:

- stdin seeding: pipe a decklist or bare codes from `nrbrowse`/`nr-list.sh`
- text decklist files as the interchange format (`Identity:` header + `Nx code`)

### 4. Rendering tiers

Card display degrades gracefully through three tiers, selected at runtime:

1. **Image** — cached artwork via `internal/image` (go-termimg; kitty/sixel/
   iTerm2 auto-detect, with ueberzugpp overlay fallback), toggled with `v`
2. **Text sheet** — `render.Card` box (emoji icons by default)
3. **ASCII/plain** — `--plain`, `--no-icons`

`--nerd` swaps emoji for nerd-font glyphs (private-use area); fonts are a
user/terminal concern, documented in the README, not installed by bootstrap.

## Current state

- Three-pane layout: browser | preview | deck pane (focused pane bordered)
- Keys: `j/k g/G Ctrl-d/u` move · `h/l Tab` panes · `/` search · `1/2/3`
  cycle side/type/faction · `Enter` add / select identity · `i` identity
  picker · `x/X +/-` edit qty · `v` images · `w/e` save/load · `q` quit
- Live footer shows top validation issues; summary line shows cards/min,
  influence used/limit, agenda points/range
- Known rough edges: deck pane re-renders fully each frame (fine for now);
  no qty column alignment; identity switch mid-deck isn't guarded against
  mixed-side decks; no undo.

## Where to work next

Ordered by what unblocks the rest:

1. **Influence breakdown per entry** — show spent influence inline (partially
   done: `(n)` suffix) and a per-faction split line in the summary.
2. **Identity change handling** — when switching identities, validate existing
   entries against the new identity and surface (not silently drop) conflicts.
3. **Undo stack** — snapshot `Deck` on each mutation; `u`/`Ctrl-r` undo/redo.
   Cheap because `Deck` is plain data.
4. **Cross-printing dedup** — same title exists in multiple packs (e.g. three
   `Abagnale`s). Counting "by name" for the 3-copy rule needs a title-level
   view; `Add` currently caps per code only, `Validate` flags the overflow
   after the fact. Decide UX: pick printing on add? merge rows?
5. **Alliance / reduced-influence cards** — requires parsing card text
   conditions ("costs 0 influence if…"); consider a small annotation table
   instead of NLP over rules text.
6. **Export formats** — NRDB/JNet/OCTGN output next to the plain-text format;
   natural home is `internal/deck` (`EncodeTo(format)`).
7. **Query power-ups** — expose more `carddb.Query` dimensions in the UI
   (pack cycling key `4`, cost range) once the builder core is settled.
8. **Image polish** — cache misses should offer to fetch on demand
   (`scripts/nr-fetch-images.sh <code>`); consider thumbnails in the browser
   list if kitty unicode placeholders stabilize upstream.

## Verification checklist for changes

```sh
go build ./... && go vet ./... && go test ./...
scripts/nr-build-deck.sh            # TUI smoke test in a real terminal
echo '3x 01041' | scripts/nr-build-deck.sh   # stdin seeding path
```
