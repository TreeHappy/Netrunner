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
		{"side only", Query{Side: "corp"}, "WHERE side_code = ?", []any{"corp"}},
		{
			"all dims",
			Query{Side: "runner", Faction: "shaper", Type: "program", Pack: "core"},
			"WHERE side_code = ? AND faction_code = ? AND type_code = ? AND pack_code = ?",
			[]any{"runner", "shaper", "program", "core"},
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
	q := Query{Side: "corp", Type: "ice", MaxCost: IntPtr(3)}
	want := "side:corp type:ice cost:<=3"
	if got := q.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	rng := Query{MinCost: IntPtr(1), MaxCost: IntPtr(4)}
	if got := rng.String(); got != "cost:1..4" {
		t.Errorf("String() = %q, want %q", got, "cost:1..4")
	}
}

func TestDebugSQL(t *testing.T) {
	q := Query{Side: "corp", Type: "agenda", MinCost: IntPtr(0)}
	sql := q.DebugSQL()
	for _, want := range []string{
		"SELECT code, title",
		"FROM cards",
		"WHERE side_code = 'corp'",
		"type_code = 'agenda'",
		"(cost IS NOT NULL AND cost >= 0)",
		"ORDER BY title, code",
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
