package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"boardy/netrunner/internal/render"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/x/ansi"
)

// overlayPos extracts the production overlay math from View() so it can be
// unit tested without spawning a daemon or needing a TTY.
func (m model) overlayPos() (x, y int, visible bool) {
	if !m.imageOn || m.artRow < 0 {
		return 0, 0, false
	}
	lw, _ := m.sheetGeometry()
	x, y = lw+5, 2+m.artRow
	if y >= m.height-1 {
		return x, y, false
	}
	return x, y, true
}

func TestRepro_GhosttyPodmanGeometry(t *testing.T) {
	cases := []struct{ w, h int }{
		{160, 40}, // big — reportedly shows no image
		{80, 24},  // half — reportedly overwrites left border
		{60, 15},  // tiny — edge case for View() hard clip
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%dx%d", tc.w, tc.h), func(t *testing.T) {
			t.Setenv("NETRUNNER_IMAGES", filepath.Join(t.TempDir(), "images"))
			t.Setenv("NETRUNNER_ART", filepath.Join(t.TempDir(), "art"))
			m := newModel(nil, render.Default())
			m.width, m.height = tc.w, tc.h
			m.imageOn = true
			lw, _ := m.sheetGeometry()
			m.list.SetSize(lw, tc.h-4)
			items := make([]list.Item, len(sampleCards()))
			for i, c := range sampleCards() {
				items[i] = item{card: c}
			}
			if err := m.list.SetItems(items); err != nil {
				t.Fatalf("SetItems: %v", err)
			}
			for _, c := range sampleCards() {
				m.pending[c.Code] = true
			}
			if cmd := m.updatePreview(); cmd != nil {
				t.Fatalf("updatePreview should not start fetches in this hermetic test, got cmd")
			}
			// Simulate an overlay's blank payload at the band size
			m.preview = render.SpliceArt(m.preview, strings.Repeat("·", m.bandW))
			view := m.View()
			x, y, vis := m.overlayPos()

			t.Logf("\n=== %dx%d === lw=%d sw=%d sheetW=%d band=%dx%d artRow=%d x=%d y=%d visible=%v\n%s",
				tc.w, tc.h, lw, func() int { _, sw := m.sheetGeometry(); return sw }(), m.sheetWidth(), m.bandW, m.bandH, m.artRow, x, y, vis, view)

			lines := strings.Split(view, "\n")
			for i, ln := range lines {
				if w := ansi.StringWidth(ln); w > tc.w {
					t.Errorf("line %d width %d > term %d: %q", i, w, tc.w, ansi.Strip(ln))
				}
			}
			for i, ln := range lines {
				plain := ansi.Strip(ln)
				if strings.Contains(plain, "·") && !(strings.Contains(plain, "│") || strings.Contains(plain, "╭") || strings.Contains(plain, "╰")) {
					// If the art row lost its surrounding box, the border glyphs are gone.
					// Log rather than fail hard — this is the signal for the reported
					// "overwrites left border" bug.
					t.Logf("row %d may have lost border around art band: %q", i, plain)
				}
			}
			if len(lines) > tc.h {
				t.Errorf("view rows %d > term %d", len(lines), tc.h)
			}
		})
	}
}
