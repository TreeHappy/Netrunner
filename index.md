# Android: Netrunner — Formalization Work Index

Second stress test for Boardy ([GDL spec](../docs/03-game-definition-language.md),
[two-tier architecture](../docs/11-two-tier-architecture.md)). Carcassonne is **tabled** at
TT-001 decided / RENDER-001 prototype (Carcassonne lives outside this repository).

> Rules are freely available: the official *Rules Reference* and core-set rulebook circulate free
> from Fantasy Flight Games and successor Null Signal Games (who maintain the game today).
> Grounded against the Wikipedia summary of the official rules.

## Why Netrunner after Carcassonne

Carcassonne stretched *spatial* mechanics (dynamic graph, incremental features). Netrunner
stretches almost everything else at once:

| Challenge | Carcassonne | Netrunner |
|---|---|---|
| Hidden information | one draw pile | nearly everything (face-down installs, hands, decks) |
| Asymmetry | none | two disjoint rule sets & vocabularies |
| Turn protocol | fixed 4-step loop | two different loops + a nested run sub-machine |
| Effects/triggers | none | constant abilities, paid abilities, events, subroutines |
| Choice under uncertainty | low | the core skill (bluffing, reading face-down cards) |
| Win conditions | points at end | three different endings incl. sudden death |

If session types ([doc 10](../docs/10-language-choice.md)) earn their keep anywhere, it's here:
the Corp/Runner interleaving is a textbook two-party session with branching, and the run is a
nested protocol.

## Open aspects

| ID | Aspect | Status | Notes |
|---|---|---|---|
| NR-001 | Scope cut for v1 formalization | 🔶 open | which mechanics make the first playable subset |
| NR-002 | Run timing as nested session type | ⬜ not started | the deepest protocol question |
| NR-003 | Effect/trigger system vs. GDL expression language | ⬜ not started | current GDL has no event/trigger story |
| NR-004 | Hidden-info views: corp's face-down cards | ⬜ not started | view projections must blur identity but show count/state |
| NR-005 | Card data model & decklists | ⬜ not started | card DB format, min 45 / max 3 copies / agenda point ranges |
| NR-006 | Win condition triple (agenda steal / score-out / flatline) | ⬜ not started | mutually exclusive endings, checked when? |

## Suggested v1 scope cut (to be argued)

A minimal playable slice: basic actions only (draw/gain credit/install/play-agenda-score/run/
trash-resource/take tag...), no traces, no bad publicity, ~20 hand-picked simple cards, one ice
per server type. Everything else logged as future work.
