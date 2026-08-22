package carddb

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/marcboeker/go-duckdb/v2"
)

type Card struct {
	Code            string
	Title           string
	Side            string
	Faction         string
	Type            string
	Keywords        string
	Text            string
	Flavor          string
	Cost            sql.NullInt64
	Strength        sql.NullInt64
	MemoryCost      sql.NullInt64
	TrashCost       sql.NullInt64
	AdvancementCost sql.NullInt64
	AgendaPoints    sql.NullInt64
	BaseLink        sql.NullInt64
	InfluenceLimit  sql.NullInt64
	InfluenceCost   sql.NullInt64
	MinimumDeckSize sql.NullInt64
	DeckLimit       sql.NullInt64
	Uniqueness      bool
	PackCode        string
	Illustrator     string
}

// Open opens the duckdb cache. Path resolution order:
// $NETRUNNER_DB, ./data/netrunner.duckdb, repo root relative to this file.
func Open() (*sql.DB, error) {
	path := os.Getenv("NETRUNNER_DB")
	if path == "" {
		path = filepath.Join("data", "netrunner.duckdb")
		if _, err := os.Stat(path); err != nil {
			path = filepath.Join(repoRoot(), "data", "netrunner.duckdb")
		}
	}
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("card cache not found at %s (run scripts/nr-build-cache.sh)", path)
	}
	return sql.Open("duckdb", path)
}

func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

const cardColumns = `code, title, side_code, faction_code, type_code,
	coalesce(keywords,''), coalesce(text,''), coalesce(flavor,''),
	cost, strength, memory_cost, trash_cost, advancement_cost, agenda_points,
	base_link, influence_limit, faction_cost, minimum_deck_size, deck_limit,
	uniqueness, pack_code, coalesce(illustrator,'')`

func scanCard(row interface{ Scan(...any) error }) (Card, error) {
	var c Card
	err := row.Scan(&c.Code, &c.Title, &c.Side, &c.Faction, &c.Type,
		&c.Keywords, &c.Text, &c.Flavor,
		&c.Cost, &c.Strength, &c.MemoryCost, &c.TrashCost, &c.AdvancementCost, &c.AgendaPoints,
		&c.BaseLink, &c.InfluenceLimit, &c.InfluenceCost, &c.MinimumDeckSize, &c.DeckLimit,
		&c.Uniqueness, &c.PackCode, &c.Illustrator)
	return c, err
}

// ByCode fetches a single card by its 5-digit code.
func ByCode(db DB, code string) (Card, error) {
	row := db.QueryRow(
		`SELECT `+cardColumns+` FROM cards WHERE code = ?`, code)
	c, err := scanCard(row)
	if err == sql.ErrNoRows {
		return c, fmt.Errorf("no card with code %q", code)
	}
	return c, err
}
