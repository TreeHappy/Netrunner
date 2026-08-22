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
	Side    []string // empty = all; values from carddb.Distinct("side_code")
	Faction []string // empty = all
	Type    []string // empty = all
	Pack    []string // empty = all
	Text    string   // substring match on title (case-insensitive)
	// Cost bounds, inclusive; nil = unset (so the zero Query has no filters).
	MinCost *int
	MaxCost *int
	// Exact cost values; empty = unrestricted. Combined with Min/MaxCost.
	Costs []int
	// ORDER BY terms, applied in sequence. Unknown fields are skipped.
	Order []Sort
}

// Sort is one ORDER BY term.
type Sort struct {
	Field string // key into SortColumns
	Desc  bool
}

// SortColumns maps user-facing field names to SQL columns for ordering.
var SortColumns = map[string]string{
	"title":    "title",
	"code":     "code",
	"type":     "type_code",
	"faction":  "faction_code",
	"pack":     "pack_code",
	"cost":     "cost",
	"strength": "strength",
}

// IntPtr returns a pointer to v, for setting Query cost bounds.
func IntPtr(v int) *int { return &v }

const costUnset = -1

func (q Query) Empty() bool {
	return len(q.Side) == 0 && len(q.Faction) == 0 && len(q.Type) == 0 &&
		len(q.Pack) == 0 && q.Text == "" && q.MinCost == nil &&
		q.MaxCost == nil && len(q.Costs) == 0
}

func (q Query) String() string {
	parts := []string{}
	if s := joinLabels("side", q.Side); s != "" {
		parts = append(parts, s)
	}
	if s := joinLabels("faction", q.Faction); s != "" {
		parts = append(parts, s)
	}
	if s := joinLabels("type", q.Type); s != "" {
		parts = append(parts, s)
	}
	if s := joinLabels("pack", q.Pack); s != "" {
		parts = append(parts, s)
	}
	if q.Text != "" {
		parts = append(parts, "text:"+q.Text)
	}
	if c := q.costString(); c != "" {
		parts = append(parts, c)
	}
	if s := q.sortString(); s != "" {
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

// joinLabels renders "name:v1,v2" for a filter dimension (empty if unset).
func joinLabels(name string, vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return name + ":" + strings.Join(vals, ",")
}

// sortString renders the ORDER BY terms as a compact label ("sort:type,cost↓").
func (q Query) sortString() string {
	if len(q.Order) == 0 {
		return ""
	}
	names := make([]string, len(q.Order))
	for i, s := range q.Order {
		names[i] = s.Field + "↑"
		if s.Desc {
			names[i] = s.Field + "↓"
		}
	}
	return "sort:" + strings.Join(names, ",")
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
	if len(q.Costs) > 0 {
		nums := make([]string, len(q.Costs))
		for i, c := range q.Costs {
			nums[i] = strconv.Itoa(c)
		}
		return "cost:" + strings.Join(nums, ",")
	}
	return ""
}

// SQL renders the WHERE clause ("WHERE ..." or "") plus bind arguments.
func (q Query) SQL() (string, []any) {
	var conds []string
	var args []any
	inCond := func(col string, vals []string) {
		if len(vals) == 0 {
			return
		}
		marks := strings.TrimSuffix(strings.Repeat("?,", len(vals)), ",")
		conds = append(conds, col+" IN ("+marks+")")
		for _, v := range vals {
			args = append(args, v)
		}
	}
	inCond("side_code", q.Side)
	inCond("faction_code", q.Faction)
	inCond("type_code", q.Type)
	inCond("pack_code", q.Pack)
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
	if len(q.Costs) > 0 {
		marks := strings.TrimSuffix(strings.Repeat("?,", len(q.Costs)), ",")
		conds = append(conds, "(cost IS NOT NULL AND cost IN ("+marks+"))")
		for _, c := range q.Costs {
			args = append(args, c)
		}
	}
	if len(conds) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

// orderBySQL renders the ORDER BY column list ("" = none; callers fall
// back to the default "title, code").
func (q Query) OrderBySQL() string {
	if len(q.Order) == 0 {
		return ""
	}
	var parts []string
	for _, s := range q.Order {
		col, ok := SortColumns[s.Field]
		if !ok {
			continue
		}
		dir := " ASC"
		if s.Desc {
			dir = " DESC"
		}
		parts = append(parts, col+dir)
	}
	return strings.Join(parts, ", ")
}

// Run executes the query against the cache.
func Run(db DB, q Query) ([]Card, error) {
	where, args := q.SQL()
	order := q.OrderBySQL()
	if order == "" {
		order = "title ASC, code ASC"
	}
	rows, err := db.Query(
		`SELECT `+cardColumns+` FROM cards `+where+` ORDER BY `+order, args...)
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
	order := q.OrderBySQL()
	if order == "" {
		order = "title ASC, code ASC"
	}
	if where != "" {
		where += " "
	}
	return "SELECT code, title, … FROM cards " + where + "ORDER BY " + order
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
