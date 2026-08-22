package carddb

import (
	"strings"
	"testing"
)

func TestQuerySQL(t *testing.T) {
	cases := []struct {
		name  string
		q     Query
		where string
		args  []any
	}{
		{"empty", Query{}, "", nil},
		{"side only", Query{Side: []string{"corp"}}, "WHERE side_code IN (?)", []any{"corp"}},
		{
			"all dims",
			Query{Side: []string{"runner"}, Faction: []string{"shaper"}, Type: []string{"program"}, Pack: []string{"core"}},
			"WHERE side_code IN (?) AND faction_code IN (?) AND type_code IN (?) AND pack_code IN (?)",
			[]any{"runner", "shaper", "program", "core"},
		},
		{
			"multi-select sides",
			Query{Side: []string{"corp", "runner"}, Faction: []string{"shaper", "anarch"}},
			"WHERE side_code IN (?,?) AND faction_code IN (?,?)",
			[]any{"corp", "runner", "shaper", "anarch"},
		},
		{
			"cost set",
			Query{Costs: []int{1, 3}},
			"WHERE (cost IS NOT NULL AND cost IN (?,?))",
			[]any{1, 3},
		},
		{"title text", Query{Text: "Ice Wall"}, "WHERE lower(title) LIKE ?", []any{"%ice wall%"}},
		{"cost cap", Query{MaxCost: IntPtr(2)}, "WHERE (cost IS NOT NULL AND cost <= ?)", []any{2}},
		{"cost floor", Query{MinCost: IntPtr(5)}, "WHERE (cost IS NOT NULL AND cost >= ?)", []any{5}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			where, args := tc.q.SQL()
			if where != tc.where {
				t.Errorf("SQL() where = %q, want %q", where, tc.where)
			}
			if len(args) != len(tc.args) {
				t.Fatalf("SQL() args = %v, want %v", args, tc.args)
			}
			for i := range args {
				if args[i] != tc.args[i] {
					t.Errorf("arg[%d] = %v, want %v", i, args[i], tc.args[i])
				}
			}
		})
	}
}

func TestQueryStringAndEmpty(t *testing.T) {
	if !(Query{}).Empty() {
		t.Error("zero Query should be Empty")
	}
	if (Query{MinCost: IntPtr(0)}).Empty() {
		t.Error("MinCost 0 is an active filter (free cards), not empty")
	}
	q := Query{Side: []string{"corp"}, Type: []string{"ice"}, MaxCost: IntPtr(3)}
	want := "side:corp type:ice cost:<=3"
	if got := q.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	set := Query{Pack: []string{"core", "su21"}}
	if got := set.String(); got != "pack:core,su21" {
		t.Errorf("String() = %q, want %q", got, "pack:core,su21")
	}
	if got := (Query{Costs: []int{0, 2}}).String(); got != "cost:0,2" {
		t.Errorf("String() = %q, want %q", got, "cost:0,2")
	}
	rng := Query{MinCost: IntPtr(1), MaxCost: IntPtr(4)}
	if got := rng.String(); got != "cost:1..4" {
		t.Errorf("String() = %q, want %q", got, "cost:1..4")
	}
}

func TestQueryOrder(t *testing.T) {
	q := Query{Order: []Sort{{Field: "type"}, {Field: "cost", Desc: true}}}
	if got := q.OrderBySQL(); got != "type_code ASC, cost DESC" {
		t.Errorf("orderBySQL() = %q, want %q", got, "type_code ASC, cost DESC")
	}
	// Unknown fields are skipped rather than breaking the SQL.
	bad := Query{Order: []Sort{{Field: "nope"}, {Field: "title"}}}
	if got := bad.OrderBySQL(); got != "title ASC" {
		t.Errorf("orderBySQL() = %q, want %q", got, "title ASC")
	}
	if got := (Query{}).OrderBySQL(); got != "" {
		t.Errorf("empty orderBySQL() = %q, want \"\"", got)
	}
	if got := q.String(); got != "sort:type↑,cost↓" {
		t.Errorf("String() = %q, want %q", got, "sort:type,cost↓")
	}
	sql := q.DebugSQL()
	if !strings.Contains(sql, "ORDER BY type_code ASC, cost DESC") {
		t.Errorf("DebugSQL() missing custom ORDER BY: %s", sql)
	}
	def := (Query{}).DebugSQL()
	if !strings.Contains(def, "ORDER BY title ASC, code ASC") {
		t.Errorf("DebugSQL() missing default ORDER BY: %s", def)
	}
}

func TestDebugSQL(t *testing.T) {
	q := Query{Side: []string{"corp"}, Type: []string{"agenda"}, MinCost: IntPtr(0)}
	sql := q.DebugSQL()
	for _, want := range []string{
		"SELECT code, title",
		"FROM cards",
		"side_code IN ('corp')",
		"type_code IN ('agenda')",
		"(cost IS NOT NULL AND cost >= 0)",
		"ORDER BY title ASC, code ASC",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("DebugSQL() missing %q in:\n%s", want, sql)
		}
	}
	// Quotes in values must be escaped so the display stays valid SQL.
	if s := (Query{Text: "it's"}).DebugSQL(); !strings.Contains(s, "'%it''s%'") {
		t.Errorf("DebugSQL() did not escape quote: %s", s)
	}
}
