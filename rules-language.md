# The Rules Representation Language vs. Netrunner

**Working doc — this is where GDL v0.5 gets designed.**
Method: requirement-driven language design. Each Netrunner mechanic is a *demand* on the language;
for each demand we either show existing GDL ([docs/03](../docs/03-game-definition-language.md))
covering it or propose a minimal extension. Nothing goes into the language that isn't demanded by
a game (Netrunner today; Carcassonne retroactively as regression check).

## 1. Gap analysis

| # | Netrunner demand | GDL today | Verdict |
|---|---|---|---|
| D1 | Asymmetric roles: two disjoint action menus & turn structures | single `turns` block | **gap → R1 roles** |
| D2 | Zones with per-card visibility (face-down installs, hands) | `visibility` per zone only | **gap → R2 card instances + visibility flags** |
| D3 | Cost-gated actions (clicks AND credits AND state conditions) | preconditions exist; multi-resource costs don't | **gap → R3 costs** |
| D4 | Nested protocol: runs with alternating control windows | flat phase machine only | **gap → R4 sub-protocols** |
| D5 | Triggered effects ("when accessed...", subroutine fires) | none | **gap → R5 events & effects (the big one)** |
| D6 | Sudden-death terminals (flatline, deck-out) mid-phase | `end_when` phase-bound | **gap → R6 immediate terminal checks** |
| D7 | Decklist validation (≥45 cards, ≤3 copies, agenda range) | nothing | **easy → R7 deck predicates** |
| D8 | Random access from hidden zones (HQ random card) | seeded RNG exists | covered; view must hide sampled index |
| D9 | Counters (advancement, credits on cards, virus) | not modeled | **gap → R2 counters** |

## 2. Proposed extensions

### R1 — Roles (asymmetry as a first-class concept)

```yaml
game:
  name: netrunner-slice
  players: { fixed_roles: [corp, runner] }   # roles are structural, not interchangeable seats

roles:
  corp:   { hand: HQ, deck: R&D, discard: archives, max_hand: 5 }
  runner: { hand: grip, deck: stack, discard: heap, max_hand: 5 }

turns:
  order: fixed [corp, runner]
  corp_turn:
    - phase draw:    mandatory draw(deck)
    - phase actions: action_points 3, actions: corp_actions
    - phase discard: enforce_max_hand
  runner_turn:
    - phase actions: action_points 4, actions: runner_actions
    - phase discard: enforce_max_hand
```

Each role names its own turn script. A session type falls out mechanically: the game protocol is
`μX. corpTurn ; runnerTurn ; X`, and each role's turn is itself a sequence — exactly what the
Idris layer should type-check (no phase references undefined roles; every declared phase reachable).

### R2 — Card instances, zones, visibility, counters

Cards become typed instances in named zones; visibility is per-facet:

```yaml
card_model:
  facets:            # what can be known about a card
    - existence      # there is a card here          (always public within zone counts)
    - identity       # which card it is              (per-zone default, overridable)
    - counters       # advancement/virus/credit tokens (independently visible!)
    - facedness      # face-up / face-down state
  visibility_rules:
    - default: identity hidden to opponent
    - rig: identity public
    - archives: identity hidden, count public        # netrunner's odd one
    - installed_corp: identity hidden until rezzed   # rez = flip facet
```

Counters are typed tokens attached to instances: `{advancement, virus, credit, power}`.
Advancement counters being public while identity is hidden is THE Netrunner bluff mechanic —
visibility must be per-facet, not per-card.

### R3 — Costs as first-class gate expressions

```yaml
actions:
  - id: advance
    cost: { clicks: 1, credits: 1 }
    precondition: target is installed and advancable
    effect: add counter advancement to target

  - id: score_agenda
    cost: { clicks: 1 }
    precondition: target.type == agenda and counters(target, advancement) >= target.advancement_requirement
    effect: move target to score_area(corp); gain(corp, target.agenda_points points)
```

Costs are just sugar for precondition + effect pairs, but declaring them separately lets the
renderer explain rejections economically ("not enough credits" instead of generic failure).

### R4 — Sub-protocols (runs)

A run is a nested protocol invoked by an action, with its own states and **windows** — moments
where a specific role gains initiative inside someone else's structure:

```yaml
sub_protocols:
  - id: run
    params: [target_server]
    states:
      - approach(ice_index from outermost)
      - encounter(ice_index)
      - access
      - done
    windows:
      - at: approach            # before encounter resolves
        offer_to: corp
        choice: [rez_ice, pass_unrezzed]
      - at: between_approaches  # runner may exit early
        offer_to: runner
        choice: [continue, jack_out]
    transition:
      approach(i): if ice(i) rezzed -> encounter(i) else -> next_approach_or_access
      encounter(i): resolve unbroken subroutines; if broken_all -> next_approach_or_access
      access: apply access_rules(target_server) -> done
```

Windows are the session-type payload: `corp` gets a branching offer (`&`) at each approach;
`runner` owns the loop exit (`jack_out`). The Idris layer proves no window offers a choice to a
role that cannot legally take any branch.

### R5 — Events, effects, and the trigger queue (the big one)

Effects need sequencing; triggers need queueing. Proposal: a small **effect algebra**, still
total and bounded:

```
effect := noop
        | command(...)                    # primitive: move_card, add_counter, gain, lose,
                                          #           reveal, sample_random, damage, ...
        | seq(e1, e2)
        | if(expr, e1, e2)                # expr = our existing total predicate language
        | foreach(x in collection, e)
        | optional(role, e)               # window: role may decline — bounded, explicit
```

Triggers register interest in events; events are emitted by commands:

```yaml
triggers:
  - id: ambush_trap
    when: card_accessed(card)
    where: card == self
    effect: if rezzed(self): damage(runner, net, 3); trash(self)
```

Resolution rule (v0.5, deliberately simple): pending triggers resolve in **emission order**;
simultaneous ones in active-player order; no player re-ordering (real Netrunner allows priority
reordering — deferred). Termination argument: effects are finite programs over finite state, but
triggers may emit triggering commands ⇒ require acyclicity of the trigger-dependence graph at load
time (static check), plus runtime fuel as backstop. *This is where QTT earns its keep.*

### R6 — Immediate terminal checks

Terminal predicates get an evaluation policy:

```yaml
end:
  - win_when: score(runner) >= 7            , check: after_event
  - win_when: score(corp) >= 7              , check: after_event
  - lose_when: role(corp) must_draw and empty(R&D)  , check: immediate   # deck-out
  - lose_when: grip_size(runner) < pending_damage   , check: immediate   # flatline
```

`immediate` terminals are evaluated inside the event loop; `after_event` at stable points.

### R7 — Deck predicates

```yaml
deck_rules:
  - count(cards) >= 45
  - max_copies_per_name: 3
  - corp: sum(where type==agenda of agenda_points) in 20..21
```

Pure validation, no engine involvement — first thing we implement.

## 3. Vertical slice to validate v0.5

One Corp identity, ~20 simple cards (1 barrier ice, 1 sentry, 2 agendas, 2 assets, basic ops),
Runner with 2 breakers. Expressible entirely in R1–R7 above; no traces, no bad publicity, no
priority games. Success criterion: full game playable via CLI referee with correct sudden-death
endings and legal run interleavings.

## 4. Questions on the table

1. Is `optional(role, effect)` enough for windows, or do windows need to be able to *offer
   parameterized choices* (which ice to rez)? Leaning yes-parameterized: `choice(role, options_expr)`.
2. Do we allow triggers on triggers (chained), gated only by static acyclicity? Or ban chains in
   v0.5? Real cards chain constantly — banning makes many cards inexpressible.
3. Should `access_rules` be a built-in combinator family (`top_card`, `random_from`, `all_cards`,
   `all_installed`) or user-defined programs? Built-ins first; they're the E&S-style standard
   library in miniature.
