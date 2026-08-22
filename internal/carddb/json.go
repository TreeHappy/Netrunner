package carddb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// rawCard mirrors the netrunner-cards-json schema.
type rawCard struct {
	Code            string  `json:"code"`
	Title           string  `json:"title"`
	SideCode        string  `json:"side_code"`
	FactionCode     string  `json:"faction_code"`
	TypeCode        string  `json:"type_code"`
	Keywords        *string `json:"keywords"`
	Text            *string `json:"text"`
	Flavor          *string `json:"flavor"`
	Cost            *int    `json:"cost"`
	Strength        *int    `json:"strength"`
	MemoryCost      *int    `json:"memory_cost"`
	TrashCost       *int    `json:"trash_cost"`
	AdvancementCost *int    `json:"advancement_cost"`
	AgendaPoints    *int    `json:"agenda_points"`
	BaseLink        *int    `json:"base_link"`
	InfluenceLimit  *int    `json:"influence_limit"`
	FactionCost     *int    `json:"faction_cost"`
	MinimumDeckSize *int    `json:"minimum_deck_size"`
	DeckLimit       *int    `json:"deck_limit"`
	Uniqueness      bool    `json:"uniqueness"`
	PackCode        string  `json:"pack_code"`
	Illustrator     *string `json:"illustrator"`
}

func nullInt(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

func deref(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (r rawCard) toCard() Card {
	return Card{
		Code: r.Code, Title: r.Title, Side: r.SideCode,
		Faction: r.FactionCode, Type: r.TypeCode,
		Keywords: deref(r.Keywords), Text: deref(r.Text), Flavor: deref(r.Flavor),
		Cost: nullInt(r.Cost), Strength: nullInt(r.Strength),
		MemoryCost: nullInt(r.MemoryCost), TrashCost: nullInt(r.TrashCost),
		AdvancementCost: nullInt(r.AdvancementCost), AgendaPoints: nullInt(r.AgendaPoints),
		BaseLink: nullInt(r.BaseLink), InfluenceLimit: nullInt(r.InfluenceLimit),
		InfluenceCost: nullInt(r.FactionCost), MinimumDeckSize: nullInt(r.MinimumDeckSize),
		DeckLimit:  nullInt(r.DeckLimit),
		Uniqueness: r.Uniqueness, PackCode: r.PackCode, Illustrator: deref(r.Illustrator),
	}
}

// FromJSON loads a single card from a netrunner-cards-json file.
func FromJSON(path string) (Card, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Card{}, err
	}
	return FromJSONBytes(b)
}

// FromJSONBytes parses a card from raw JSON bytes. Accepts either a single
// card object or an array (in which case exactly one element is required).
func FromJSONBytes(b []byte) (Card, error) {
	trimmed := strings.TrimSpace(string(b))
	if strings.HasPrefix(trimmed, "[") {
		var rs []rawCard
		if err := json.Unmarshal(b, &rs); err != nil {
			return Card{}, err
		}
		if len(rs) != 1 {
			return Card{}, fmt.Errorf("json array has %d cards; use \"path/to/pack.json:code\" to pick one", len(rs))
		}
		return rs[0].toCard(), nil
	}
	var r rawCard
	if err := json.Unmarshal(b, &r); err != nil {
		return Card{}, err
	}
	if r.Code == "" || r.Title == "" {
		return Card{}, fmt.Errorf("not a card JSON document")
	}
	return r.toCard(), nil
}

// FromFile loads a card from "path" or "path:code", where path may be a
// single-card JSON document or a pack file containing an array.
func FromFile(spec string) (Card, error) {
	path := spec
	code := ""
	if i := strings.LastIndex(spec, ":"); i >= 0 && isAllDigits(spec[i+1:]) {
		path, code = spec[:i], spec[i+1:]
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Card{}, err
	}
	if code != "" {
		var rs []rawCard
		if err := json.Unmarshal(b, &rs); err != nil {
			return Card{}, fmt.Errorf("%s: %w", path, err)
		}
		for _, r := range rs {
			if r.Code == code {
				return r.toCard(), nil
			}
		}
		return Card{}, fmt.Errorf("%s: no card with code %q", path, code)
	}
	c, err := FromJSONBytes(b)
	if err != nil {
		return Card{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
