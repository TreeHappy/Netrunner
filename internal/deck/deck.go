// Package deck models a Netrunner decklist and validates it against the
// standard construction rules (min deck size, copies per card, influence,
// agenda point range, uniqueness).
package deck

import (
	"fmt"
	"strings"

	"boardy/netrunner/internal/carddb"
)

type Entry struct {
	Card carddb.Card
	Qty  int
}

type Deck struct {
	Identity carddb.Card
	Entries  []Entry // excludes the identity
}

func (d *Deck) Size() int {
	n := 0
	for _, e := range d.Entries {
		n += e.Qty
	}
	return n
}

// MinSize is the smallest legal deck size for this identity.
func (d *Deck) MinSize() int {
	if d.Identity.MinimumDeckSize.Valid {
		return int(d.Identity.MinimumDeckSize.Int64)
	}
	return 45
}

// InfluenceLimit returns the identity's influence limit.
func (d *Deck) InfluenceLimit() int {
	if d.Identity.InfluenceLimit.Valid {
		return int(d.Identity.InfluenceLimit.Int64)
	}
	return 0
}

// Influence sums influence spent on out-of-faction cards (per copy).
// Identities themselves never cost influence.
func (d *Deck) Influence() int {
	total := 0
	for _, e := range d.Entries {
		cost := 0
		if e.Card.InfluenceCost.Valid {
			cost = int(e.Card.InfluenceCost.Int64)
		}
		if cost > 0 && e.Card.Faction != d.Identity.Faction {
			total += cost * e.Qty
		}
	}
	return total
}

// AgendaPoints sums agenda points in a corp deck.
func (d *Deck) AgendaPoints() int {
	total := 0
	for _, e := range d.Entries {
		if e.Card.Type == "agenda" && e.Card.AgendaPoints.Valid {
			total += int(e.Card.AgendaPoints.Int64) * e.Qty
		}
	}
	return total
}

// AgendaRange returns [min,max] agenda points allowed at the current deck
// size (corp only). Range grows by 2 for every 5 cards above 45.
func (d *Deck) AgendaRange() (int, int) {
	min := 18
	if extra := d.Size() - d.MinSize(); extra > 0 {
		min += 2 * (extra / 5)
	}
	return min, min + 2
}

// CopiesOf returns the number of copies currently in the deck.
func (d *Deck) CopiesOf(code string) int {
	for _, e := range d.Entries {
		if e.Card.Code == code {
			return e.Qty
		}
	}
	return 0
}

// Add adds one copy of c (up to its deck limit).
func (d *Deck) Add(c carddb.Card) bool {
	max := CardLimit(c)
	for i := range d.Entries {
		if d.Entries[i].Card.Code == c.Code {
			if d.Entries[i].Qty >= max {
				return false
			}
			d.Entries[i].Qty++
			return true
		}
	}
	if max <= 0 {
		return false
	}
	d.Entries = append(d.Entries, Entry{Card: c, Qty: 1})
	return true
}

// SetQty sets the number of copies of c (0 removes the entry).
// Unlike Add it bypasses the per-card limit (used for loading decks).
func (d *Deck) SetQty(c carddb.Card, qty int) {
	for i := range d.Entries {
		if d.Entries[i].Card.Code == c.Code {
			if qty <= 0 {
				d.Entries = append(d.Entries[:i], d.Entries[i+1:]...)
			} else {
				d.Entries[i].Qty = qty
			}
			return
		}
	}
	if qty > 0 {
		d.Entries = append(d.Entries, Entry{Card: c, Qty: qty})
	}
}

// CardLimit is the max copies of a card allowed by name (default 3).
func CardLimit(c carddb.Card) int {
	if c.DeckLimit.Valid {
		return int(c.DeckLimit.Int64)
	}
	return 3
}

type Issue struct{ Msg string }

func (i Issue) Error() string { return i.Msg }

// Validate checks all construction rules; empty result means legal.
func (d *Deck) Validate() []Issue {
	var issues []Issue
	add := func(format string, args ...any) {
		issues = append(issues, Issue{fmt.Sprintf(format, args...)})
	}

	size := d.Size()
	min := d.MinSize()
	if size < min {
		add("deck has %d cards, needs %d", size, min)
	}
	if d.Identity.Type != "identity" {
		add("no identity selected")
	}

	byName := map[string]int{}
	for _, e := range d.Entries {
		byName[e.Card.Title] += e.Qty
		if e.Qty > CardLimit(e.Card) {
			add("%s: %d copies, limit %d", e.Card.Title, e.Qty, CardLimit(e.Card))
		}
		if e.Card.Uniqueness && e.Qty > 1 {
			add("%s is unique, only 1 copy allowed", e.Card.Title)
		}
		if e.Card.Side != "" && e.Card.Side != d.Identity.Side && e.Card.Type != "identity" {
			add("%s belongs to the %s side", e.Card.Title, e.Card.Side)
		}
	}
	for name, n := range byName {
		if n > CardLimit(carddb.Card{}) {
			add("%s: %d total copies across printings, limit %d", name, n, 3)
		}
	}

	inf := d.Influence()
	limit := d.InfluenceLimit()
	if inf > limit {
		add("influence %d exceeds limit %d", inf, limit)
	}

	if d.Identity.Side == "corp" {
		pts := d.AgendaPoints()
		lo, hi := d.AgendaRange()
		switch {
		case pts < lo:
			add("agenda points %d below minimum %d", pts, lo)
		case pts > hi:
			add("agenda points %d above maximum %d", pts, hi)
		}
	}
	return issues
}

// Summary renders the running totals line shown under the deck pane.
func (d *Deck) Summary() string {
	size, min := d.Size(), d.MinSize()
	s := fmt.Sprintf("cards %d/%d", size, min)
	inf := d.Influence()
	s += fmt.Sprintf(" · influence %d/%d", inf, d.InfluenceLimit())
	if d.Identity.Side == "corp" {
		lo, hi := d.AgendaRange()
		s += fmt.Sprintf(" · agenda %d [%d-%d]", d.AgendaPoints(), lo, hi)
	}
	return s
}

// Encode writes the decklist as plain text:
//
//	Identity: <code>
//	Nx <code>            (one line per distinct card)
func (d *Deck) Encode() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Identity: %s\n", d.Identity.Code)
	for _, e := range d.Entries {
		fmt.Fprintf(&b, "%dx %s\n", e.Qty, e.Card.Code)
	}
	return b.String()
}

// Decode parses a decklist produced by Encode (comments/blank lines ignored;
// lines may carry trailing titles which are stripped).
func Decode(db carddb.DB, text string) (*Deck, error) {
	d := &Deck{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lower := strings.ToLower(line)
		var qty int
		var code string
		switch {
		case strings.HasPrefix(lower, "identity:"):
			code = strings.TrimSpace(line[len("identity:"):])
			qty = 1
		default:
			fields := strings.Fields(line)
			if len(fields) < 2 {
				return nil, fmt.Errorf("bad decklist line %q", line)
			}
			n, err := fmt.Sscanf(fields[0], "%dx", &qty)
			if err != nil || n != 1 {
				return nil, fmt.Errorf("bad quantity in line %q", line)
			}
			code = fields[1]
		}
		card, err := carddb.ByCode(db, code)
		if err != nil {
			return nil, err
		}
		if qty >= 1 && strings.HasPrefix(lower, "identity:") {
			d.Identity = card
		} else {
			for i := 0; i < qty; i++ {
				d.Add(card)
			}
		}
	}
	return d, nil
}
