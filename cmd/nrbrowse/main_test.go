package main

import (
	"fmt"
	"path/filepath"
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
			t.Setenv("NETRUNNER_IMAGES", filepath.Join(t.TempDir(), "images"))
			t.Setenv("NETRUNNER_ART", filepath.Join(t.TempDir(), "art"))
			m := newModel(nil, render.Default(), nil)
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
			// Mark cards as already pending so updatePreview issues no
			// fetch/crop commands (tests must stay hermetic).
			for _, it := range sampleCards() {
				m.pending[it.Code] = true
			}
			if cmd := m.updatePreview(); cmd != nil {
				t.Fatal("updatePreview should not start fetches in tests")
			}
			view := m.View()
			lines := strings.Split(view, "\n")
			for i, line := range lines {
				if lw := ansi.StringWidth(line); lw > w {
					t.Errorf("line %d is %d columns (terminal %d): %q", i, lw, w, line)
				}
			}
			if len(lines) > h {
				t.Errorf("view has %d rows (terminal %d)", len(lines), h)
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

// TestViewDegenerateSizes ensures View never panics below the minimum
// geometry (e.g. before the first WindowSizeMsg).
func TestViewDegenerateSizes(t *testing.T) {
	sizes := [][2]int{{0, 0}, {10, 2}, {40, 5}, {19, 6}, {80, 1}}
	for _, sz := range sizes {
		w, h := sz[0], sz[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			m := newModel(nil, render.Default(), nil)
			m.width, m.height = w, h
			var view string
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("View panicked at %dx%d: %v", w, h, r)
					}
				}()
				view = m.View()
			}()
			if n := strings.Count(view, "\n") + 1; n > max(1, h) {
				t.Errorf("view has %d rows (terminal %d)", n, h)
			}
		})
	}
}

// TestSpliceArtPreservesRow verifies the spliced row keeps its printable
// width and border glyphs.
func TestSpliceArtPreservesRow(t *testing.T) {
	c := sampleCards()[1]
	o := render.Default()
	o.Width = 60
	o.ArtBand = &render.ArtBand{W: 30, H: 6}
	sheet := render.Card(c, o)
	payload := "\x1b_Gf=100,s=10,v=5\x1b\\fake" // zero-width control + marker
	spliced := render.SpliceArt(sheet, payload)

	sheetRows := strings.Split(sheet, "\n")
	rows := strings.Split(spliced, "\n")
	if len(rows) != len(sheetRows) {
		t.Fatalf("row count changed: %d -> %d", len(sheetRows), len(rows))
	}
	for i := range rows {
		wSheet, wSpliced := lipgloss.Width(sheetRows[i]), lipgloss.Width(rows[i])
		if wSheet != wSpliced {
			t.Errorf("row %d width %d -> %d", i, wSheet, wSpliced)
		}
		if strings.Contains(sheetRows[i], render.ArtSentinel) {
			if !strings.HasPrefix(rows[i], "│") || !strings.HasSuffix(strings.TrimRight(rows[i], " "), "│") {
				t.Errorf("row %d lost border glyphs: %q", i, ansi.Strip(rows[i]))
			}
		}
	}
}

// TestViewFitsWithPayload injects a simulated zero-width image payload into
// the preview band and checks the frame still fits the terminal.
func TestViewFitsWithPayload(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {140, 50}, {200, 60}} {
		w, h := sz[0], sz[1]
		t.Run(fmt.Sprintf("%dx%d", w, h), func(t *testing.T) {
			t.Setenv("NETRUNNER_IMAGES", filepath.Join(t.TempDir(), "images"))
			t.Setenv("NETRUNNER_ART", filepath.Join(t.TempDir(), "art"))
			m := newModel(nil, render.Default(), nil)
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
			for _, it := range sampleCards() {
				m.pending[it.Code] = true
			}
			if cmd := m.updatePreview(); cmd != nil {
				t.Fatal("updatePreview should not start fetches in tests")
			}
			// Simulate an inline payload spliced over the sentinel row.
			m.preview = render.SpliceArt(m.preview, "\x1b_Ga=d,q=1\x1b\\")
			view := m.View()
			for i, line := range strings.Split(view, "\n") {
				if wd := ansi.StringWidth(line); wd > w {
					t.Errorf("line %d is %d columns (terminal %d)", i, wd, w)
				}
			}
		})
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode string
		wantW    int
		wantProt string
		wantErr  bool
	}{
		{"code", []string{"01001"}, "01001", 0, "", false},
		{"images space", []string{"--images", "kitty", "09004"}, "09004", 0, "kitty", false},
		{"images eq", []string{"--images=kitty", "09004"}, "09004", 0, "kitty", false},
		{"images bare", []string{"--images"}, "", 0, "auto", false},
		{"images ueberzug", []string{"--images=ueberzug", "09004"}, "09004", 0, "ueberzug", false},
		{"images ueberzugpp", []string{"--images", "ueberzugpp", "09004"}, "09004", 0, "ueberzugpp", false},
		{"width eq", []string{"--width=80"}, "", 80, "", false},
		{"width space", []string{"--width", "100", "01041"}, "01041", 100, "", false},
		{"plain", []string{"--plain", "--no-icons"}, "", 0, "", false},
		{"bad protocol", []string{"--images=lp0"}, "", 0, "", true},
		{"unknown flag", []string{"--wat"}, "", 0, "", true},
		{"dash code", []string{"-x"}, "", 0, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := parseArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseArgs(%q) = nil error, want error", tt.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseArgs(%q): %v", tt.args, err)
			}
			if p.code != tt.wantCode || p.opts.Width != tt.wantW || p.protocol != tt.wantProt {
				t.Errorf("parseArgs(%q) = %+v, want code=%q w=%d proto=%q",
					tt.args, p, tt.wantCode, tt.wantW, tt.wantProt)
			}
		})
	}
}

func TestRenderCardWithArtPlainFallback(t *testing.T) {
	opts := render.Default()
	opts.Plain = true
	got := renderCardWithArt(sampleCards()[0], opts, 80, 30)
	if want := render.Card(sampleCards()[0], opts); got != want {
		t.Errorf("plain renderCardWithArt differs from render.Card")
	}
}
