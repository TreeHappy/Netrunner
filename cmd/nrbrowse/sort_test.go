package main

import (
	"strings"
	"testing"

	"boardy/netrunner/internal/render"
)

// TestSortPicker covers the modal multi-select ordering logic: each toggle
// cycles absent → asc → desc → absent, and the panel reflects the state.
func TestSortPicker(t *testing.T) {
	m := newModel(nil, render.Options{}, nil)
	if m.sortOpen {
		t.Fatal("sort picker should start closed")
	}

	m.toggleSort("type") // asc
	m.toggleSort("cost") // asc
	if got := m.query.OrderBySQL(); got != "type_code ASC, cost ASC" {
		t.Fatalf("orderBySQL() = %q", got)
	}
	m.toggleSort("cost") // desc
	if got := m.query.OrderBySQL(); got != "type_code ASC, cost DESC" {
		t.Fatalf("orderBySQL() = %q", got)
	}
	m.toggleSort("cost") // removed
	if got := m.query.OrderBySQL(); got != "type_code ASC" {
		t.Fatalf("orderBySQL() = %q", got)
	}

	panel := m.sortPanel()
	if !strings.Contains(panel, "[1] title") || !strings.Contains(panel, "[3] faction") {
		t.Errorf("panel missing field rows:\n%s", panel)
	}
	if !strings.Contains(panel, "↑1") {
		t.Errorf("panel missing active sort marker:\n%s", panel)
	}
}

func TestSortPanelKeysMapToChoices(t *testing.T) {
	for i, want := range sortChoices {
		got := sortChoiceByKey(i + 1)
		if got != want {
			t.Errorf("key %d = %q, want %q", i+1, got, want)
		}
	}
	if sortChoiceByKey(0) != "" || sortChoiceByKey(len(sortChoices)+1) != "" {
		t.Error("out-of-range keys should map to no field")
	}
}
