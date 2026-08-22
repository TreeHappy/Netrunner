# Android: Netrunner — Rules (formalization-oriented restatement)

> Grounded in the official FFG/Null Signal rules as summarized by Wikipedia and my knowledge of
> the *Rules Reference*. This follows the Carcassonne pattern ([../carcassonne/carcassonne_rules.md]):
> restated for Boardy formalization, with **[GDL]** annotations where our current language
> ([docs/03](../docs/03-game-definition-language.md)) meets something it can't yet express.

Android: Netrunner (FFG 2012, Richard Garfield; maintained by Null Signal Games) is an
**asymmetric** card game for 2 players: one plays the **Corporation**, the other the **Runner**
(a hacker). Both race to 7 **agenda points** — but only the Corp's deck contains agendas, so the
Runner must steal them from the Corp's servers while avoiding lethal damage.

---

## 1. Zones

### Corp zones
| Zone | Contents | Visibility |
|---|---|---|
| **Identity** | 1 card, public | public |
| **R&D** | draw deck | hidden; top card secret |
| **HQ** | hand | hidden to Runner |
| **Archives** | discard pile | face down; *count* public, faces hidden |
| **Servers** (remote) | installed Agendas, Assets, Upgrades | cards face DOWN until rezzed |
| **Ice** | protecting each server, outermost first | face down until rezzed |
| **Score area** | scored agendas | public |

### Runner zones
| Zone | Contents | Visibility |
|---|---|---|
| **Identity** | 1 card, public | public |
| **Stack** | draw deck | hidden |
| **Grip** | hand | hidden to Corp |
| **Heap** | discard pile | public |
| **Rig** | installed Programs / Hardware / Resources | **public** |

**[GDL]** This is the first big test of the view-projection model: Corp installs are visible as
*face-down objects* (existence + position + advancement counters public; identity secret).
Our `view(player)` needs per-card visibility flags, not just per-zone.

## 2. Card types

- **Corp**: Agenda (advancement requirement, agenda points, score ability), Asset, Upgrade,
  Operation (one-shot), Ice (subtype: Barrier / Code Gate / Sentry; subroutines; strength).
- **Runner**: Event (one-shot), Program (incl. icebreakers: decoder/fracter/cybernetic...),
  Hardware, Resource.
- All cards have: faction, cost (credits), and abilities text. Ice has rez cost + strength +
  subroutines. Agendas have advancement requirement + points (+ optionally credits on steal).

## 3. Deck building

- Deck minimum 45 cards (both sides), max 3 copies of a card by name.
- Corp deck must contain between X and Y agenda points depending on deck size
  (45–49 cards ⇒ 20–21 points). Identity card sets faction constraints.
- **[GDL]** Decklist validation = pure predicate over a multiset of card ids. Good early target
  for static checking.

## 4. Setup

Corp: shuffle R&D, draw 5-card HQ hand, gain 5 credits, place identity. Runner: shuffle Stack,
draw 5-card Grip, gain 5 credits, place identity. Corp plays first; turns alternate forever
(no "rounds" — the two turn structures differ).

## 5. Corp turn

1. **Draw phase** — mandatory draw of 1 from R&D (no click cost).
2. **Action phase** — up to **3 clicks** (action points), any order:
   - Gain 1 credit [1 click]
   - Draw 1 card from R&D [1 click]
   - Install a card from HQ (face-down into a server, or ice protecting a server) [1 click]
   - Play an Operation from HQ (pay its cost) [1 click]
   - Advance an installed card (pay 1 credit; add 1 advancement counter) [1 click]
   - Rez an installed card (pay its rez cost) [free, not a click]
   - Score an Agenda whose advancement counters ≥ its requirement [1 click] → move to score area,
     gain its agenda points
   - Trash a Runner Resource (requires the Runner to be tagged) [1 click + pay trash cost]
   - Purge virus counters [3 clicks]
   - Use paid abilities on rezzed cards [cost printed]
3. **Discard phase** — discard down to max hand size (**5**).

## 6. Runner turn

1. **Action phase** — up to **4 clicks**:
   - Gain 1 credit [1 click]
   - Draw 1 from Stack [1 click]
   - Install Program/Hardware/Resource from Grip (pay cost) [1 click]
   - Play an Event (pay cost) [1 click]
   - **Run a server** [1 click] (see §7)
   - Remove a tag (pay 2 credits) [1 click]
   - Use paid abilities [printed cost]
2. **Discard phase** — discard down to max hand size (**5**).

**[GDL]** Two different action menus over the same `clicks` resource. Our action-points mechanism
from Carcassonne generalizes cleanly, but actions are now *parameterized by card instances* and
*cost-gated* (credit checks), and some are gated on state (score requires enough counters;
trash-resource requires tag).

## 7. Runs (the nested protocol)

A run attacks exactly one server (central: R&D / HQ / Archives; or remote). Timing:

```
INITIATE run (declared target)
  APPROACH outermost unrezzed/rezzed ice
    └─ CORP WINDOW: rez approached ice? (or let runner pass it)
    ENCOUNTER rezzed ice
      └─ resolve its subroutines (unbroken ones fire)
      └─ RUNNER may use icebreakers to break subroutines / boost strength
    PASS → approach next inner ice ... repeat until no ice remains
  SUCCESSFUL RUN → ACCESS cards:
      remote      : all cards in the server (corp may rez before access)
      R&D         : top card of R&D (may spend clicks to access further, if allowed)
      HQ          : random card from HQ hand
      Archives    : ALL cards in Archives (face-up during this access)
  ACCESS resolution: Agenda → runner STEALS (gain its points); Asset/Upgrade → corp may rez,
      otherwise trash at printed trash cost optional; others just viewed.
END RUN (runner may jack out earlier, between approach steps unless locked)
```

Key facts: un-rezzed ice is passed freely; encountering rezzed ice means dealing with subroutines
(e.g., end the run, lose credits, take damage); a run ends unsuccessful (no access) if ended by
effect; jacking out is a runner choice between approaches.

**[GDL]** This is a **nested state machine with alternating control windows** — precisely the
session-type showcase ([NR-002]). Note also that access of a *stolen agenda* flips a win condition
mid-run, and ambushes (e.g., traps with "when accessed" effects) mean accessing is effect-triggering.

## 8. Damage, tags, traces & death

- **Damage types**: net, meat, brain. Damage N → runner discards N cards from Grip.
- **Flatline**: if damage exceeds cards in grip (i.e., runner must discard more than they have) →
  **Corp wins instantly**.
- **Brain damage** also permanently reduces runner max hand size.
- **Tags**: runner tagged (via traces/effects); enables corp trash-resource action and meat-damage
  bonuses.
- **Traces**: corp sets trace strength X (pays credits), runner boosts with link+credits; if
  trace ≥ runner, effect fires.
- **Bad publicity**: corp takes BP; runner gains 1 credit at run start per BP.

## 9. Winning & losing

Three mutually exclusive endings:

1. **Agenda threshold**: first side with **7+ agenda points** wins (runner steals; corp scores).
2. **Deck-out (corp loses)**: corp required to draw from empty R&D → **Runner wins instantly**.
3. **Flatline**: runner cannot pay full damage → **Corp wins instantly**.

**[GDL]** Three terminal predicates, two of them sudden-death mid-phase checks. Our current
`end_when` model assumes end-of-turn evaluation; we need immediate trigger evaluation ([NR-006]).

## 10. Effects & priority (the deep water)

Cards generate: constant abilities, **paid abilities** (cost:clicks/credits, usable when their
conditions hold), **triggered events** ("when X happens, do Y"), and **subroutines**. When several
would act, official timing rules define windows and *priority*: active player acts first within
windows. Simultaneous effects resolve in active-player order.

**[GDL]** Our expression language has no notion of triggered effects, ability windows, or a
priority queue. Options sketched in [index NR-003]: extend GDL with an event queue + window
declarations, or restrict v1 to cards without triggers. This is the single biggest language gap
Netrunner exposes — bigger than anything Carcassonne found.

## 11. Why this game stress-tests Boardy differently than Carcassonne

| Dimension | Formalization difficulty | Notes |
|---|---|---|
| Asymmetric protocols | high | two disjoint rule sets; session types must handle role-indexed behavior |
| Hidden info density | high | face-down installs with public metadata; random access from HQ |
| Nested protocol (runs) | high | approach/encounter loop with mid-loop windows and jack-out exits |
| Trigger/effect system | very high | GDL gap; candidate research contribution |
| Sudden-death endings | medium | terminal checks must be event-immediate, not phase-bound |
| Deck construction | low | pure validation predicate — quick win |
| Click economy | low | reuse action_points combinator |

## 12. Open questions

1. **Card text is code.** Real Netrunner is unbounded card effects. v1 must pick a closed card
   pool whose effects fit the extended GDL — effectively building a mini-DSL inside the DSL.
2. **Random access from HQ** consumes RNG deterministically — fine, but views must not leak which
   index was sampled.
3. **Rez-as-response** (corp deciding mid-run whether to rez) is a *choice offered to the
   non-active player inside the runner's protocol* — the clearest case yet for two-party session
   types with branching offers (`&`/`⊕`).
4. Do we model **mulligans/tournament rules**? No — out of scope for v1.
