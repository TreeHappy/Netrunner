package render

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"strings"

	"boardy/netrunner/internal/carddb"
	"boardy/netrunner/internal/glyphs"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
)

var factionColors = map[string]lipgloss.Color{
	"anarch":             "#ff5a36",
	"criminal":           "#4d7fd1",
	"shaper":             "#3faa4f",
	"neutral-runner":     "#8a8a8a",
	"haas-bioroid":       "#8e44ad",
	"jinteki":            "#c0392b",
	"nbn":                "#e6a817",
	"weyland-consortium": "#2c5f2d",
	"neutral-corp":       "#8a8a8a",
	"adam":               "#b08d57",
	"apex":               "#663366",
	"sunny-lebeau":       "#7f8fa6",
}

// Options controls rendering style.
type Options struct {
	Plain     bool // no ANSI
	Width     int  // 0 = auto-detect terminal width (fallback 60)
	Icons     bool // show emoji type/faction icons
	NerdIcons bool // show nerd-font glyphs instead of emojis (needs a nerd font)

	// ArtBand reserves a centered artwork slot inside the card sheet,
	// between the stat pills and the rules body. ArtNote labels the empty
	// placeholder frame (e.g. "fetching art…").
	ArtBand *ArtBand
	ArtNote string
}

// ArtBand describes the reserved artwork slot in card-sheet cells.
type ArtBand struct {
	W, H int
}

// ArtSentinel marks the first row of the reserved artwork band. TUIs splice
// a graphics-protocol payload over this line (see SpliceArt).
const ArtSentinel = "\x00ART\x00"

// SpliceArt replaces the artwork band's sentinel row in a rendered sheet
// with the raw graphics payload, padding the row to w printable cells so
// layout is unaffected (payloads carry no printable width).
func SpliceArt(sheet, payload string, w int) string {
	lines := strings.Split(sheet, "\n")
	for i, line := range lines {
		if strings.Contains(line, ArtSentinel) {
			pad := w - lipgloss.Width(payload)
			if pad < 0 {
				pad = 0
			}
			lines[i] = payload + strings.Repeat(" ", pad)
			return strings.Join(lines, "\n")
		}
	}
	return sheet
}

// Default returns options suitable for the current terminal.
func Default() Options {
	return Options{Plain: lipgloss.ColorProfile() == 0, Width: 0, Icons: true}
}

func factionColor(faction string) lipgloss.Color {
	if c, ok := factionColors[faction]; ok {
		return c
	}
	return lipgloss.Color("#aaaaaa")
}

func styled(opts Options, fn func() string, fallback string) string {
	if opts.Plain || lipgloss.ColorProfile() == 0 {
		return fallback
	}
	return fn()
}

func colorize(opts Options, faction, text string) string {
	return styled(opts,
		func() string {
			return lipgloss.NewStyle().Foreground(factionColor(faction)).Bold(true).Render(text)
		},
		text)
}

func fmtNull(n sql.NullInt64) (string, bool) {
	if !n.Valid {
		return "", false
	}
	return strconv.FormatInt(n.Int64, 10), true
}

func typeIcon(t string) string {
	switch t {
	case "identity":
		return "🆔"
	case "ice":
		return "🛡️"
	case "agenda":
		return "🎯"
	case "event":
		return "⚡"
	case "operation":
		return "🎭"
	case "hardware":
		return "🔧"
	case "program":
		return "💻"
	case "resource":
		return "💾"
	case "asset":
		return "🏢"
	case "upgrade":
		return "🏗️"
	default:
		return "🃏"
	}
}

func typeGlyph(t string) string {
	switch t {
	case "identity":
		return "[ID]"
	case "ice":
		return "[ICE]"
	case "agenda":
		return "[AG]"
	case "event":
		return "[EV]"
	case "operation":
		return "[OP]"
	case "hardware":
		return "[HW]"
	case "program":
		return "[PG]"
	case "resource":
		return "[RS]"
	case "asset":
		return "[AS]"
	case "upgrade":
		return "[UP]"
	default:
		return "[--]"
	}
}

// nerdIcon returns a nerd-font glyph for a card type (private-use area).
func nerdTypeIcon(t string) string {
	switch t {
	case "identity":
		return "\uf2c2" // id-card
	case "ice":
		return "\uf132" // shield
	case "agenda":
		return "\uf05b" // bullseye
	case "event":
		return "\uf0e7" // bolt
	case "operation":
		return "\uf085" // gears
	case "hardware":
		return "\uf2db" // microchip
	case "program":
		return "\uf121" // code
	case "resource":
		return "\uf0c7" // save
	case "asset":
		return "\uf1ad" // building
	case "upgrade":
		return "\uf062" // arrow-up
	default:
		return "\uf02d" // book
	}
}

func nerdSideIcon(side string) string {
	if side == "corp" {
		return "\uf19c" // building
	}
	return "\uf70c" // running person
}

// icon returns the type icon per the options' style.
func typeIconFor(opts Options, t string) string {
	switch {
	case opts.NerdIcons:
		return nerdTypeIcon(t)
	case opts.Icons:
		return typeIcon(t)
	default:
		return typeGlyph(t)
	}
}

func factionEmoji(side string) string {
	if side == "corp" {
		return "🏢"
	}
	return "🏴"
}

type stat struct {
	icon  string
	label string
	value string
}

func pill(opts Options, faction string, s stat) string {
	fallback := fmt.Sprintf("%s %s", s.label, s.value)
	if s.icon == "" && s.label == "" {
		fallback = s.value
	} else if s.label == "" {
		fallback = fmt.Sprintf("%s %s", s.icon, s.value)
	}
	return styled(opts,
		func() string {
			c := factionColor(faction)
			label := ""
			if s.icon != "" {
				label = s.icon + " "
			} else if s.label != "" {
				label = s.label + " "
			}
			body := label + s.value
			return lipgloss.NewStyle().
				Foreground(lipgloss.Color("#0d0d0d")).
				Background(c).
				Bold(true).
				Padding(0, 1).
				Render(body)
		},
		fallback)
}

// nerdStatIcon maps the emoji stat icons to nerd-font equivalents.
func nerdStatIcon(emoji string) string {
	switch emoji {
	case "⚡":
		return "\uf0e7" // bolt
	case "💪":
		return "\uf6c3" // dumbbell (nf-md)
	case "🗑️", "🗑":
		return "\uf1f8" // trash
	case "🧠":
		return "\uf2db" // microchip (memory units)
	case "🃏":
		return "\uf02d" // book (deck)
	case "🔗":
		return "\uf0c1" // link
	default:
		return emoji
	}
}

// effectiveWidth resolves opts.Width to a usable content width.
func effectiveWidth(opts Options) int {
	w := opts.Width
	if w <= 0 {
		if tw, _, err := term.GetSize(os.Stdout.Fd()); err == nil {
			w = tw
		}
		if w <= 0 {
			w = 60
		}
	}
	return w
}

// Title renders the card title line (icon + title, uniqueness diamond).
func Title(c carddb.Card, opts Options) string {
	title := c.Title
	if c.Uniqueness {
		title = "◆ " + title
	}
	icon := typeIconFor(opts, c.Type) + " "
	return colorize(opts, c.Faction, icon+title)
}

// Header renders the title plus dim subtitle line (type · keywords · faction).
func Header(c carddb.Card, opts Options) string {
	var b strings.Builder
	b.WriteString(Title(c, opts))
	b.WriteString("\n")

	sub := []string{c.Type}
	if c.Keywords != "" {
		sub = append(sub, strings.Split(c.Keywords, " - ")...)
	}
	factionName := strings.Title(strings.ReplaceAll(c.Faction, "-", " ")) //nolint:staticcheck
	side := ""
	if opts.NerdIcons {
		side = nerdSideIcon(c.Side) + " "
	} else if opts.Icons {
		side = factionEmoji(c.Side) + " "
	}
	b.WriteString(dim(opts, fmt.Sprintf("%s · %s%s", strings.Join(sub, ": "), side, factionName)))
	return b.String()
}

// Stats renders the colored stat pills line, or "" if the card has none.
func Stats(c carddb.Card, opts Options) string {
	var stats []stat
	pillFaction := c.Faction
	if _, ok := factionColors[c.Faction]; !ok {
		pillFaction = ""
	}
	addStat := func(icon, label, value string) {
		if opts.NerdIcons {
			icon = nerdStatIcon(icon)
		}
		if !opts.Icons {
			icon = ""
		}
		stats = append(stats, stat{icon: icon, label: label, value: value})
	}
	if v, ok := fmtNull(c.Cost); ok && c.Type != "identity" {
		addStat("⚡", "Cost", v)
	}
	switch c.Type {
	case "agenda":
		if v, ok := fmtNull(c.AdvancementCost); ok {
			addStat("⬆", "Adv", v)
		}
		if v, ok := fmtNull(c.AgendaPoints); ok {
			addStat("★", "AP", v)
		}
	case "ice":
		if v, ok := fmtNull(c.Strength); ok {
			addStat("💪", "Str", v)
		}
		if v, ok := fmtNull(c.TrashCost); ok {
			addStat("🗑️", "Trash", v)
		}
	case "identity":
		if v, ok := fmtNull(c.InfluenceLimit); ok {
			addStat("∞", "Inf", v)
		}
		if v, ok := fmtNull(c.MinimumDeckSize); ok {
			addStat("🃏", "Min deck", v)
		}
		if v, ok := fmtNull(c.BaseLink); ok && v != "0" {
			addStat("🔗", "Link", v)
		}
	default:
		if v, ok := fmtNull(c.TrashCost); ok {
			addStat("🗑️", "Trash", v)
		}
		if v, ok := fmtNull(c.MemoryCost); ok {
			addStat("🧠", "", v+"μ")
		}
		if v, ok := fmtNull(c.Strength); ok {
			addStat("💪", "Str", v)
		}
	}
	if len(stats) == 0 {
		return ""
	}
	pills := make([]string, len(stats))
	for i, s := range stats {
		pills[i] = pill(opts, pillFaction, s)
	}
	return strings.Join(pills, styled(opts, func() string { return " " }, "   "))
}

// Body renders rules text, flavor text and meta line (no rule lines, no border).
func Body(c carddb.Card, opts Options, innerWidth int) string {
	var b strings.Builder

	if body := strings.TrimSpace(glyphs.CleanText(c.Text)); body != "" {
		if !opts.Plain && lipgloss.ColorProfile() != 0 {
			body = lipgloss.NewStyle().Foreground(lipgloss.Color("#dddddd")).Width(innerWidth).Render(body)
		} else {
			body = lipgloss.NewStyle().Width(innerWidth).Render(body)
		}
		b.WriteString(strings.TrimRight(body, " \t"))
		b.WriteString("\n")
	}

	if c.Flavor != "" {
		flavor := glyphs.ReplaceSymbols(c.Flavor)
		b.WriteString("\n")
		flavored := italic(opts, "❝ "+flavor+" ❞")
		flavored = lipgloss.NewStyle().Width(innerWidth).Align(lipgloss.Center).Render(flavored)
		b.WriteString(flavored)
		b.WriteString("\n")
	}

	meta := fmt.Sprintf("%s · %s", c.Code, c.PackCode)
	if c.Illustrator != "" {
		meta += " · ill. " + c.Illustrator
	}
	b.WriteString(dim(opts, meta))

	return b.String()
}

// Card renders a full bordered card sheet. When opts.Width is set the
// output never exceeds that many columns: over-long lines are truncated,
// not allowed to expand the box.
func Card(c carddb.Card, opts Options) string {
	innerWidth := effectiveWidth(opts) - 4 // account for border + padding
	if innerWidth < 10 {
		innerWidth = 10
	}

	sections := []string{Header(c, opts)}
	if stats := Stats(c, opts); stats != "" {
		sections = append(sections, stats)
	}
	if opts.ArtBand != nil {
		sections = append(sections, artBand(*opts.ArtBand, innerWidth, opts.ArtNote))
	}
	sections = append(sections, Body(c, opts, innerWidth))
	content := strings.Join(sections, "\n\x00RULE\x00\n") + "\n"

	content = strings.ReplaceAll(content, "\x00RULE\x00", ruleLine(opts, innerWidth))

	// Enforce the width budget strictly; sentinel rows already have exact
	// printable width and must not pass through MaxWidth (control runes).
	var enforced strings.Builder
	style := lipgloss.NewStyle().MaxWidth(innerWidth)
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, ArtSentinel) {
			enforced.WriteString(line)
		} else {
			enforced.WriteString(style.Render(line))
		}
		enforced.WriteString("\n")
	}
	content = strings.TrimSuffix(enforced.String(), "\n")

	if opts.Plain || lipgloss.ColorProfile() == 0 {
		return content
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(factionColor(c.Faction)).
		Padding(0, 1).
		Render(content)

	return box + "\n"
}

// artBand renders the reserved artwork slot: a sentinel row for payload
// splicing plus a faint dashed frame with an optional note.
func artBand(band ArtBand, innerWidth int, note string) string {
	w := band.W
	if w > innerWidth {
		w = innerWidth
	}
	if w < 4 {
		w = 4
	}
	h := band.H
	if h < 3 {
		h = 3
	}
	faint := lipgloss.NewStyle().Faint(true)

	var b strings.Builder
	b.WriteString(ArtSentinel + strings.Repeat(" ", max(0, w-1)))
	b.WriteString("\n")
	noteRow := h / 2
	for i := 1; i < h-1; i++ {
		mid := ""
		if i == noteRow && note != "" {
			r := []rune(note)
			if len(r) > w-4 {
				r = r[:w-4]
			}
			mid = string(r)
			padL := (w - 2 - len([]rune(mid))) / 2
			padR := w - 2 - padL - len([]rune(mid))
			mid = strings.Repeat(" ", padL) + mid + strings.Repeat(" ", max(0, padR))
		} else {
			mid = strings.Repeat(" ", w-2)
		}
		b.WriteString(faint.Render("┆" + mid + "┆"))
		b.WriteString("\n")
	}
	b.WriteString(faint.Render(strings.Repeat("┈", w)))
	return b.String()
}

func ruleLine(opts Options, width int) string {
	line := strings.Repeat("┈", width)
	return styled(opts,
		func() string { return lipgloss.NewStyle().Faint(true).Render(line) },
		strings.Repeat("─", width))
}

func dim(opts Options, s string) string {
	return styled(opts,
		func() string { return lipgloss.NewStyle().Foreground(lipgloss.Color("#888888")).Render(s) },
		s)
}

func italic(opts Options, s string) string {
	return styled(opts,
		func() string { return lipgloss.NewStyle().Italic(true).Faint(true).Render(s) },
		"("+strings.TrimSuffix(strings.TrimPrefix(s, "❝ "), " ❞")+")")
}

// ListRow renders the compact entry used by pickers: CODE<TAB>title · type · faction
func ListRow(c carddb.Card) string {
	return fmt.Sprintf("%s\t%s · %s · %s", c.Code, c.Title, c.Type, c.Faction)
}
