package main

import (
	"fmt"
	"strings"
	"testing"

	"boardy/netrunner/internal/carddb"
	"boardy/netrunner/internal/render"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// TestViewFitsTerminal renders the full UI at several sizes and asserts no
// output line exceeds the terminal width.
func TestViewFitsTerminal(t *testing.T) {
	sizes := [][2]int{{80, 24}, {100, 30}, {140, 50}, {60, 20}, {200, 60}}
	for _, sz := range sizes {
		w, h := sz[0], sz[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			m := newModel(nil, render.Default())
			m.width, m.height = w, h
			lw, _ := m.sheetGeometry()
			m.list.SetSize(lw, h-4)
			items := make([]list.Item, len(sampleCards()))
			for i, c := range sampleCards() {
				items[i] = item{card: c}
			}
			if err := m.list.SetItems(items); err != nil {
				t.Fatal(err)
			}
			if cmd := m.updatePreview(); cmd != nil {
				t.Fatal("updatePreview should not start fetches in tests")
			}
			view := m.View()
			for i, line := range strings.Split(view, "\n") {
				if lw := ansi.StringWidth(line); lw > w {
					t.Errorf("line %d is %d columns (terminal %d): %q", i, lw, w, line)
				}
			}
		})
	}
}

// TestCardSheetNeverExceedsWidth checks render.Card honors opts.Width even
// with long titles and art bands.
func TestCardSheetNeverExceedsWidth(t *testing.T) {
	c := carddb.Card{
		Code: "01001", Title: "An Extremely Long Card Title That Should Never Expand The Box",
		Faction: "shaper", Type: "identity",
		Text:     strings.Repeat("word wrap me please ", 12),
		PackCode: "core",
	}
	for _, width := range []int{30, 44, 60, 90} {
		o := render.Default()
		o.Width = width
		o.ArtBand = &render.ArtBand{W: width / 2, H: 6}
		o.ArtNote = "fetching art…"
		sheet := render.Card(c, o)
		for i, line := range strings.Split(sheet, "\n") {
			if lw := lipgloss.Width(line); lw > width {
				t.Errorf("width %d: line %d is %d cols: %q", width, i, lw, line)
			}
		}
		if !strings.Contains(sheet, render.ArtSentinel) && o.ArtBand != nil {
			// sentinel must survive in the unstyled path too
			oPlain := o
			oPlain.Plain = true
			if !strings.Contains(render.Card(c, oPlain), render.ArtSentinel) {
				t.Errorf("width %d: art band missing", width)
			}
		}
	}
}
