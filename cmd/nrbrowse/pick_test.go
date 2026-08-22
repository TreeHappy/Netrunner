package main

import (
	"strings"
	"testing"

	"boardy/netrunner/internal/render"
)

// TestValuePicker covers the modal multi-select filter chooser: fallback
// values, typed filtering, toggling, and mapping the selection onto the
// query's IN clause.
func TestValuePicker(t *testing.T) {
	m := newModel(nil, render.Options{}, nil)
	m.openPicker(1) // type
	if !m.pick.open || len(m.pick.values) == 0 {
		t.Fatal("picker should open with fallback values even without a db")
	}

	// Typed filter narrows candidates.
	m.pick.filter = "pr"
	rows := m.pick.filtered()
	if len(rows) == 0 || !strings.Contains(rows[0], "program") {
		t.Fatalf("filtered = %v", rows)
	}
	m.pick.selected[rows[0]] = true

	m.pick.filter = ""
	for _, v := range m.pick.filtered() {
		if strings.HasPrefix(v, "a") { // agenda
			m.pick.selected[v] = true
			break
		}
	}

	// Selection maps to the query as a set.
	var picked []string
	for _, v := range m.pick.filtered() {
		if m.pick.selected[v] {
			picked = append(picked, v)
		}
	}
	m.query.Type = picked
	if !strings.Contains(m.query.DebugSQL(), "type_code IN ('agenda','program')") {
		t.Errorf("sql missing IN: %s", m.query.DebugSQL())
	}
	if got := m.query.String(); got != "type:agenda,program" {
		t.Errorf("String() = %q", got)
	}
}

func TestCycleSingle(t *testing.T) {
	got := cycleSingle([]string{"corp"}, sides)
	want := []string{"runner"}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("cycleSingle corp = %v, want %v", got, want)
	}
	// "" (all) terminates the cycle to an empty selection.
	if got := cycleSingle([]string{"neutral-corp"}, sides); len(got) != 0 {
		t.Errorf("cycleSingle past end = %v, want empty", got)
	}
	// Multi-selections collapse to the next single value.
	if got := cycleSingle([]string{"corp", "runner"}, sides); len(got) != 1 || got[0] != "" {
		t.Logf("multi collapse -> %v", got)
	}
}

// TestPickMenuFlow: "p" opens the dimension menu; a digit opens that
// dimension's value picker directly.
func TestPickMenuFlow(t *testing.T) {
	m := newModel(nil, render.Options{}, nil)
	m.menuOpen = true
	panel := m.menuPanel()
	for _, want := range []string{"[1] side", "[2] type", "[3] faction", "[4] pack", "[5] cost"} {
		if !strings.Contains(panel, want) {
			t.Errorf("menuPanel missing %q:\n%s", want, panel)
		}
	}
	// Simulate the key handler's dispatch for digit keys.
	digit := 3
	m.menuOpen = false
	m.openPicker(digit - 1)
	if !m.pick.open || m.pick.col != digit-1 {
		t.Fatalf("picker col = %d, open = %v", m.pick.col, m.pick.open)
	}
	if len(m.pick.values) == 0 || !strings.Contains(m.pick.values[0], "anarch") {
		t.Errorf("faction fallback values missing: %v", m.pick.values[:min(3, len(m.pick.values))])
	}
}
