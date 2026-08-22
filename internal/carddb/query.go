package carddb

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
)

// DB is the subset of *sql.DB used by the query helpers.
type DB interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Query describes a card selection. It is compiled into a parameterized
// SQL WHERE clause against the DuckDB cache, so browsers and other tools
// share one filtering implementation.
type Query struct {
	Side    string // "", "corp", "runner"
	Faction string // "" = all
	Type    string // "" = all
	Pack    string // "" = all
	Text    string // substring match on title (case-insensitive)
	// Cost bounds, inclusive; nil = unset (so the zero Query has no filters).
	MinCost *int
	MaxCost *int
}

// IntPtr returns a pointer to v, for setting Query cost bounds.
func IntPtr(v int) *int { return &v }

const costUnset = -1

func (q Query) Empty() bool {
	return q.Side == "" && q.Faction == "" && q.Type == "" && q.Pack == "" &&
		q.Text == "" && q.MinCost == nil && q.MaxCost == nil
}

func (q Query) String() string {
	parts := []string{}
	if q.Side != "" {
		parts = append(parts, "side:"+q.Side)
	}
	if q.Faction != "" {
		parts = append(parts, "faction:"+q.Faction)
	}
	if q.Type != "" {
		parts = append(parts, "type:"+q.Type)
	}
	if q.Pack != "" {
		parts = append(parts, "pack:"+q.Pack)
	}
	if q.Text != "" {
		parts = append(parts, "text:"+q.Text)
	}
	if c := q.costString(); c != "" {
		parts = append(parts, c)
	}
	return strings.Join(parts, " ")
}

// costString renders the cost bounds as a compact filter label.
func (q Query) costString() string {
	minSet, maxSet := q.MinCost != nil, q.MaxCost != nil
	switch {
	case minSet && maxSet:
		if *q.MinCost == *q.MaxCost {
			return fmt.Sprintf("cost:%d", *q.MinCost)
		}
		return fmt.Sprintf("cost:%d..%d", *q.MinCost, *q.MaxCost)
	case minSet:
		return fmt.Sprintf("cost:>=%d", *q.MinCost)
	case maxSet:
		return fmt.Sprintf("cost:<=%d", *q.MaxCost)
	}
	return ""
}

// SQL renders the WHERE clause ("WHERE ..." or "") plus bind arguments.
func (q Query) SQL() (string, []any) {
	var conds []string
	var args []any
	if q.Side != "" {
		conds = append(conds, "side_code = ?")
		args = append(args, q.Side)
	}
	if q.Faction != "" {
		conds = append(conds, "faction_code = ?")
		args = append(args, q.Faction)
	}
	if q.Type != "" {
		conds = append(conds, "type_code = ?")
		args = append(args, q.Type)
	}
	if q.Pack != "" {
		conds = append(conds, "pack_code = ?")
		args = append(args, q.Pack)
	}
	if q.Text != "" {
		conds = append(conds, "lower(title) LIKE ?")
		args = append(args, "%"+strings.ToLower(q.Text)+"%")
	}
	if q.MinCost != nil {
		conds = append(conds, "(cost IS NOT NULL AND cost >= ?)")
		args = append(args, *q.MinCost)
	}
	if q.MaxCost != nil {
		conds = append(conds, "(cost IS NOT NULL AND cost <= ?)")
		args = append(args, *q.MaxCost)
	}
	if len(conds) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

// Run executes the query against the cache.
func Run(db DB, q Query) ([]Card, error) {
	where, args := q.SQL()
	rows, err := db.Query(
		`SELECT `+cardColumns+` FROM cards `+where+` ORDER BY title, code`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cards []Card
	for rows.Next() {
		c, err := scanCard(rows)
		if err != nil {
			return nil, err
		}
		cards = append(cards, c)
	}
	return cards, rows.Err()
}

// List returns all cards ordered by title.
func List(db DB) ([]Card, error) { return Run(db, Query{}) }

// DebugSQL renders the statement Run would execute with the bind arguments
// inlined, for display and debugging (e.g. in a TUI status bar). The
// selected column list is abbreviated to keep it readable.
func (q Query) DebugSQL() string {
	where, args := q.SQL()
	for _, a := range args {
		where = strings.Replace(where, "?", bindInline(a), 1)
	}
	if where != "" {
		where += " "
	}
	return "SELECT code, title, … FROM cards " + where + "ORDER BY title, code"
}

// bindInline formats one SQL argument as a literal: ints bare, strings
// single-quoted and escaped.
func bindInline(a any) string {
	switch v := a.(type) {
	case int:
		return strconv.Itoa(v)
	case string:
		return "'" + strings.ReplaceAll(v, "'", "''") + "'"
	default:
		return fmt.Sprintf("'%v'", v)
	}
}

// Distinct returns sorted distinct values of a column (e.g. "faction_code").
func Distinct(db DB, column string) ([]string, error) {
	switch column {
	case "faction_code", "type_code", "pack_code", "side_code":
	default:
		return nil, fmt.Errorf("unsupported distinct column %q", column)
	}
	rows, err := db.Query(
		`SELECT DISTINCT ` + column + ` FROM cards WHERE ` + column + ` IS NOT NULL ORDER BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
