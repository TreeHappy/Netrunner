# Boardy — Netrunner

This repository contains the **Android: Netrunner formalization** for
[Boardy](https://github.com/TreeHappy/Boardy), a research project building an *interpreter for board games*:
a runtime that takes a formal game definition and can enforce move legality, detect
win/loss/draw states, expose the game to AI agents over MCP, render state and legal moves
for humans, and search for strong play with Monte Carlo Tree Search.

Netrunner is Boardy's second stress test (after Carcassonne). Where Carcassonne stretched
*spatial* mechanics, Netrunner stresses nearly everything else at once: pervasive hidden
information, asymmetric Corp/Runner rule sets, two different turn loops plus a nested run
sub-protocol, and constant/paid ability effects. The design-theory grounding comes from
Engelstein & Shalev's *Building Blocks of Tabletop Game Design* and Schell's
*The Art of Game Design*.

## Repository layout

- `index.md` — the Netrunner formalization work index; start here.
- `netrunner_rules.md`, `rules-language.md` — rules and the rules language under work.
- `docs/` — the full Boardy research notes (vision, GDL, interpreter architecture, MCP,
  rendering, MCTS, multiplayer, roadmap).
- `cmd/nrbrowse/`, `cmd/nrrender/` — CLI tools for browsing cards and rendering.
- `internal/carddb/` — card database loading (DuckDB + netrunner-cards-json schema).
- `scripts/` — helper scripts (see below).
- `data/netrunner-cards-json` — git submodule pinning
  [Null-Signal-Games/netrunner-cards-json](https://github.com/Null-Signal-Games/netrunner-cards-json)
  with a sparse checkout of just the card data.

## Setup

Requirements: [Go](https://go.dev), the [DuckDB CLI](https://duckdb.org), and git.

```sh
git clone --recurse-submodules https://github.com/TreeHappy/Netrunner.git
cd Netrunner

# fetch/update the card data submodule (sparse: pack files + cycles/factions/types/sides)
scripts/nr-cards.sh            # add --update to pull the latest upstream cards

# build the DuckDB card cache from the submodule data
scripts/nr-build-cache.sh
```

## Usage

```sh
scripts/nr-list.sh             # list cards from the cache
scripts/nr-browse.sh <code>    # inspect a single card
scripts/nr-render.sh           # render cards via cmd/nrrender
```
