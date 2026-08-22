package main

import (
	"strings"
	"testing"

	"boardy/netrunner/internal/carddb"
	"boardy/netrunner/internal/render"
	"github.com/charmbracelet/x/ansi"
)

// TestValuePicker covers the modal multi-select filter chooser.
func TestValuePicker(t *testing.T) {
	m := newModel(nil, render.Options{}, "", nil)
	m.width, m.height = 100, 30
	m.openPicker(1) // type
	if !m.pick.open || len(m.pick.values) == 0 {
		t.Fatal("picker should open with fallback values even without a db")
	}
	m.pick.filter = "pr"
	rows := m.pick.filtered()
	if len(rows) == 0 || !strings.Contains(rows[0], "program") {
		t.Fatalf("filtered = %v", rows)
	}
	m.pick.selected[rows[0]] = true
	m.query.Type = []string{rows[0]}
	if !strings.Contains(m.query.DebugSQL(), "type_code IN ('program')") {
		t.Errorf("sql missing IN: %s", m.query.DebugSQL())
	}
}

// TestSortPicker covers the multi-field ORDER BY toggling.
func TestSortPicker(t *testing.T) {
	m := newModel(nil, render.Options{}, "", nil)
	m.toggleSort("type")
	m.toggleSort("cost")
	m.toggleSort("cost") // desc
	if got := m.query.OrderBySQL(); got != "type_code ASC, cost DESC" {
		t.Fatalf("OrderBySQL() = %q", got)
	}
	panel := m.sortPanel()
	if !strings.Contains(panel, "↑1") || !strings.Contains(panel, "↓2") {
		t.Errorf("panel missing active markers:\n%s", panel)
	}
}

// TestViewFitsWithPanels ensures the frame stays inside the terminal even
// with both the menu and a picker panel rendered below the panes.
func TestViewFitsWithPanels(t *testing.T) {
	m := newModel(nil, render.Options{}, "", nil)
	m.width, m.height = 110, 24
	lw, _, _ := paneInteriors(m.width)
	m.list.SetSize(lw, bodyHeight(m.height)-2)
	m.menuOpen = true
	m.openPicker(3) // pack — opens and closes the menu
	view := m.View()
	lines := strings.Split(view, "\n")
	if len(lines) > m.height {
		t.Errorf("view rows %d > term %d", len(lines), m.height)
	}
	for i, ln := range lines {
		if w := ansi.StringWidth(ln); w > m.width {
			t.Errorf("line %d width %d > %d", i, w, m.width)
		}
	}
	if !strings.Contains(view, "pack — type to filter") {
		t.Errorf("picker panel missing from view")
	}
	var _ = carddb.IntPtr
}
