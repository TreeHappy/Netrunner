package glyphs

import "testing"

func TestReplaceSymbols(t *testing.T) {
	cases := map[string]string{
		"Gain 3[credit].":  "Gain 3⬤.",
		"Lose [click]":     "Lose ◆",
		"[subroutine] End": "⟐ End",
		"[mu] left":        "μ left",
		"[unknown]":        "[unknown]",
	}
	for in, want := range cases {
		if got := ReplaceSymbols(in); got != want {
			t.Errorf("ReplaceSymbols(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanText(t *testing.T) {
	in := "<strong>Barrier</strong> - [subroutine] Do 1 net damage."
	want := "BARRIER - ⟐ Do 1 net damage."
	if got := CleanText(in); got != want {
		t.Errorf("CleanText(%q) = %q, want %q", in, got, want)
	}
}
