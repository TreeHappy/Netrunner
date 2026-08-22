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
- `cmd/nrbrowse/`, `cmd/nrbuild/`, `cmd/nrrender/` — CLI tools for browsing cards,
  deck building, and rendering.
- `internal/carddb/` — card database loading + SQL query builder (DuckDB + netrunner-cards-json schema).
- `internal/render/`, `internal/glyphs/` — card sheet rendering (emoji / nerd-font / ASCII).
- `internal/image/` — terminal card artwork via kitty/sixel/iTerm2 graphics protocols plus a ueberzugpp overlay fallback.
- `internal/deck/` — decklist model and construction-rule validation.
- `scripts/` — helper scripts (see below).
- `data/netrunner-cards-json` — git submodule pinning
  [Null-Signal-Games/netrunner-cards-json](https://github.com/Null-Signal-Games/netrunner-cards-json)
  with a sparse checkout of just the card data.

## Setup

Requirements: [Go](https://go.dev), the [DuckDB CLI](https://duckdb.org), and git —
or nothing but git if you use [mise](https://mise.jdx.dev).

### Quickstart (bootstrap)

```sh
git clone --recurse-submodules https://github.com/TreeHappy/Netrunner.git
cd Netrunner
./scripts/bootstrap.sh
```

The bootstrap script is idempotent and will:

1. install [mise](https://mise.jdx.dev) if missing,
2. `mise install` the tools declared in `.mise.toml` (latest Go + DuckDB CLI),
3. fetch/update the card data submodule (`scripts/nr-cards.sh`),
4. build the DuckDB card cache (`scripts/nr-build-cache.sh`),
5. smoke-test the tooling.

### Manual setup

```sh
# fetch/update the card data submodule (sparse: pack files + cycles/factions/types/sides)
scripts/nr-cards.sh            # add --update to pull the latest upstream cards

# build the DuckDB card cache from the submodule data
scripts/nr-build-cache.sh
```

### Optional: card artwork for terminal image rendering

```sh
scripts/nr-fetch-images.sh     # downloads card scans (also fetched on demand by the TUIs)
```

Requires [mise](https://mise.jdx.dev)-installed ImageMagick (`mise use -g imagemagick`)
for cropping; the TUIs crop artwork automatically after download.

With images cached, open the browser or deck builder in a graphics-capable
terminal (kitty, WezTerm, ghostty, foot, …); on terminals without an inline protocol, ueberzugpp (if installed) draws artwork as an overlay via X11/Wayland and press `v` to toggle image
preview of the selected card. Terminals without a graphics protocol fall back
to text rendering automatically.

## Usage

```sh
scripts/nr-list.sh             # list cards from the cache
scripts/nr-browse.sh <code>    # inspect a single card
scripts/nr-render.sh           # render cards via cmd/nrrender
scripts/nr-build-deck.sh       # interactive deck builder TUI
scripts/nr-build-deck.sh my-deck.txt   # load/save a decklist
```

## Deck builder

`nr-build-deck.sh` embeds the card browser as its picker: filter selections are
compiled into SQL queries against the DuckDB cache. The right pane shows your
deck with live validation against standard construction rules (min deck size,
≤ 3 copies per name, influence limit, agenda point range, uniqueness).

Keybindings (nvim-style):

| Key | Action |
| --- | --- |
| `j` / `k`, `Ctrl-d` / `Ctrl-u`, `g` / `G` | move, half page, top/bottom |
| `h` / `l`, `←/→`, `Tab` | switch focus between browser and deck pane |
| `/` | search title/code in the browser |
| `1` / `2` / `3` | cycle side / type / faction filters (SQL-backed) |
| `Enter` | add card to deck · select identity when browsing identities |
| `i` | browse identities for the current side |
| `x` / `X` | remove one / all copies of selected deck entry |
| `+` / `-` | adjust copies of selected deck entry |
| `v` | toggle terminal image preview (kitty/sixel/ueberzugpp) |
| `w` / `e` | save / reload the decklist file |
| `q` | quit |

Both TUIs accept `--nerd` to render nerd-font glyphs instead of emojis
(requires a [nerd font](https://www.nerdfonts.com) in your terminal),
and `--plain` / `--no-icons` for plain output.

Decklists are plain text:

```
Identity: 30077
3x 01041
2x 02086
```

Card artwork is fetched on demand from NetrunnerDB's public card-image CDN
and cached under the XDG user cache directory: full scans in
`~/.cache/netrunner/card-images`, artwork-only crops (made with ImageMagick)
in `~/.cache/netrunner/card-art` (override with `NETRUNNER_IMAGES` /
`NETRUNNER_ART`). It is never stored in the repository.

## License

The code in this repository is licensed under the [MIT License](LICENSE).

Android: Netrunner and all card names, rules text, and artwork are the
property of Fantasy Flight Games, Wizards of the Coast, and Null Signal
Games. Card data is sourced from [NetrunnerDB](https://netrunnerdb.com) and
card images from its public CDN; this tool caches them locally for personal
use only and ships none of them. Nothing in this repository grants any
right to redistribute that content.

