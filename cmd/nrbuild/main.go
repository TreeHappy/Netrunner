// nrbuild is an interactive Netrunner deck builder TUI. It reuses the
// nrbrowse picker backed by SQL queries against the DuckDB cache.
//
// Keys (nvim-style):
//
//	j/k  down/up          Ctrl-d/Ctrl-u  half page down/up
//	g/G  top/bottom       h/l or ←/→     switch pane (browser ⇄ deck)
//	/    search filter    1/2/3          cycle side/type/faction
//	Enter add card (or select identity when browsing identities)
//	i    browse identities for the current side
//	x/X  remove one/all copies of the selected deck entry
//	+/-  increase/decrease copies of the selected deck entry
//	v    toggle image preview        w  save decklist
//	e    edit/load a decklist file   q  quit
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"boardy/netrunner/internal/carddb"
	"boardy/netrunner/internal/deck"
	"boardy/netrunner/internal/image"
	"boardy/netrunner/internal/render"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type item struct {
	card carddb.Card
}

func (i item) Title() string {
	t := i.card.Title
	if i.card.Uniqueness {
		t = "◆ " + t
	}
	return t
}
func (i item) Description() string {
	return fmt.Sprintf("%s · %s · %s · %s", i.card.Code, i.card.Type, i.card.Faction, i.card.PackCode)
}
func (i item) FilterValue() string {
	return strings.ToLower(i.card.Title + " " + i.card.Code)
}

var (
	sides    = []string{"", "corp", "runner"}
	factions = []string{"", "anarch", "criminal", "shaper", "haas-bioroid", "jinteki", "nbn", "weyland-consortium", "neutral-runner", "neutral-corp"}
	types    = []string{"", "agenda", "asset", "event", "hardware", "ice", "identity", "operation", "program", "resource", "upgrade"}
)

type focus int

const (
	focusBrowser focus = iota
	focusDeck
)

type model struct {
	db      carddb.DB
	query   carddb.Query
	list    list.Model
	spinner spinner.Model
	preview string
	pending map[string]bool
	failed  map[string]bool

	d         *deck.Deck
	deckSel   int
	deckOff   int
	imageMode bool
	file      string
	status    string

	focus  focus
	width  int
	height int
	opts   render.Options
	loaded bool

	quitting bool
}

func newModel(db carddb.DB, opts render.Options, file string) model {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 40, 24)
	l.Title = "Netrunner cards"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.FilterInput.Placeholder = "search title/code…"
	l.KeyMap.CursorUp.SetKeys("up", "k")
	l.KeyMap.CursorDown.SetKeys("down", "j")
	l.Styles.Title = l.Styles.Title.Foreground(lipgloss.Color("#dddddd")).Background(lipgloss.Color("#333355"))
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	return model{db: db, list: l, spinner: sp, opts: opts, file: file,
		d: &deck.Deck{}, pending: map[string]bool{}, failed: map[string]bool{},
		status: "no identity yet — press i to pick one"}
}

func (m *model) refresh() tea.Cmd {
	cards, err := carddb.Run(m.db, m.query)
	if err != nil {
		m.status = "error: " + err.Error()
		return nil
	}
	items := make([]list.Item, len(cards))
	for i, c := range cards {
		items[i] = item{card: c}
	}
	m.list.SetItems(items)
	m.status = fmt.Sprintf("%d cards · %s", len(items), m.query.String())
	return m.updatePreview()
}

func (m *model) updatePreview() tea.Cmd {
	sel, ok := m.list.SelectedItem().(item)
	if !ok {
		m.preview = ""
		return nil
	}
	_, s, _, _ := paneInteriors(m.width)
	o := m.opts
	o.Width = s - 2
	m.preview = render.Card(sel.card, o)
	if !m.imageMode || !image.Supported() {
		return nil
	}
	code := sel.card.Code
	if payload, _, _ := image.Card(code, m.artWidth(), bodyHeight(m.height)-2); payload != "" {
		return nil
	}
	if image.Path(code) == "" && !m.pending[code] && !m.failed[code] {
		m.pending[code] = true
		m.status = "fetching art… " + code
		return tea.Batch(fetchArtCmd(code), m.spinner.Tick)
	}
	return nil
}

type artFetchedMsg struct {
	code string
	err  error
}

func fetchArtCmd(code string) tea.Cmd {
	return func() tea.Msg {
		_, err := image.FetchWithArt(code)
		return artFetchedMsg{code: code, err: err}
	}
}

func (m *model) addSelected() {
	sel, ok := m.list.SelectedItem().(item)
	if !ok {
		return
	}
	if sel.card.Type == "identity" {
		if sel.card.Side != "" && m.query.Side != "" && sel.card.Side != m.query.Side {
			m.status = "identity is for the other side"
			return
		}
		m.d.Identity = sel.card
		m.status = "identity: " + sel.card.Title
		return
	}
	if m.d.Identity.Type != "identity" || sel.card.Side != m.d.Identity.Side {
		m.status = "pick an identity first (press i)"
		return
	}
	if !m.d.Add(sel.card) {
		m.status = fmt.Sprintf("%s: at limit (%d)", sel.card.Title, deck.CardLimit(sel.card))
		return
	}
	m.status = "+1 " + sel.card.Title
}

func (m *model) deckEntries() []deck.Entry { return m.d.Entries }

func (m *model) removeFromDeck(all bool) {
	entries := m.deckEntries()
	if m.deckSel >= len(entries) {
		return
	}
	e := entries[m.deckSel]
	if all || e.Qty <= 1 {
		m.d.SetQty(e.Card, 0)
	} else {
		m.d.SetQty(e.Card, e.Qty-1)
	}
	if m.deckSel >= len(m.deckEntries()) {
		m.deckSel = len(m.deckEntries()) - 1
	}
	if m.deckSel < 0 {
		m.deckSel = 0
	}
	m.status = "-" + e.Card.Title
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		lw, _, _, _ := paneInteriors(m.width)
		m.list.SetSize(lw, bodyHeight(m.height)-2)
		if !m.loaded {
			m.loaded = true
			return m, m.refresh()
		}
		return m, m.updatePreview()

	case artFetchedMsg:
		delete(m.pending, msg.code)
		if msg.err != nil {
			m.failed[msg.code] = true
			m.status = "no art for " + msg.code
		}
		return m, m.updatePreview()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if len(m.pending) > 0 {
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break // let the filter input handle keys
		}
		switch msg.String() {
		case "ctrl+c", "q":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			if m.list.FilterState() != list.FilterApplied {
				m.quitting = true
				return m, tea.Quit
			}
		case "enter":
			m.addSelected()
			return m, nil
		case "i":
			side := m.query.Side
			if side == "" {
				side = "corp"
			}
			m.query = carddb.Query{Side: side, Type: "identity"}
			return m, m.refresh()
		case "h", "left":
			m.focus = focusBrowser
			return m, nil
		case "l", "right":
			if m.focus != focusDeck {
				m.focus = focusDeck
				return m, nil
			}
		case "tab":
			if m.focus == focusBrowser {
				m.focus = focusDeck
			} else {
				m.focus = focusBrowser
			}
			return m, nil
		case "1":
			cycle(&m.query.Side, sides)
			return m, m.refresh()
		case "2":
			cycle(&m.query.Type, types)
			return m, m.refresh()
		case "3":
			cycle(&m.query.Faction, factions)
			return m, m.refresh()
		case "v":
			m.imageMode = !m.imageMode
			if m.imageMode && !image.Supported() {
				m.status = "terminal has no graphics protocol; using text"
			} else {
				m.status = "image preview " + map[bool]string{true: "on", false: "off"}[m.imageMode]
			}
			return m, m.updatePreview()
		case "w":
			return m, m.save()
		case "e":
			return m, m.load()
		}
		if m.focus == focusDeck {
			switch msg.String() {
			case "j", "down":
				if m.deckSel < len(m.deckEntries())-1 {
					m.deckSel++
					m.syncDeckOff()
				}
				return m, nil
			case "k", "up":
				if m.deckSel > 0 {
					m.deckSel--
					m.syncDeckOff()
				}
				return m, nil
			case "g", "home":
				m.deckSel = 0
				m.syncDeckOff()
				return m, nil
			case "G", "end":
				m.deckSel = max(0, len(m.deckEntries())-1)
				m.syncDeckOff()
				return m, nil
			case "x":
				m.removeFromDeck(false)
				return m, nil
			case "X":
				m.removeFromDeck(true)
				return m, nil
			case "+", "=":
				entries := m.deckEntries()
				if m.deckSel < len(entries) {
					e := entries[m.deckSel]
					if !m.d.Add(e.Card) {
						m.status = fmt.Sprintf("%s: at limit (%d)", e.Card.Title, deck.CardLimit(e.Card))
					}
				}
				return m, nil
			case "-":
				m.removeFromDeck(false)
				return m, nil
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	prev := m.list.SelectedItem()
	m.list, cmd = m.list.Update(msg)
	if m.list.SelectedItem() != prev {
		cmd = m.updatePreview()
	}
	return m, cmd
}

func (m model) save() tea.Cmd {
	path := m.file
	if path == "" {
		path = "deck.txt"
	}
	err := os.WriteFile(path, []byte(m.d.Encode()), 0o644)
	if err != nil {
		m.status = "save failed: " + err.Error()
	} else {
		m.status = "saved " + path
	}
	return nil
}

func (m model) load() tea.Cmd {
	path := m.file
	if path == "" {
		m.status = "no deck file given (usage: nrbuild [deckfile])"
		return nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		m.status = "load failed: " + err.Error()
		return nil
	}
	d, err := deck.Decode(m.db, string(b))
	if err != nil {
		m.status = "load failed: " + err.Error()
		return nil
	}
	m.d = d
	m.deckSel = 0
	m.syncDeckOff()
	m.query = carddb.Query{Side: d.Identity.Side}
	cmd := m.refresh()
	m.status = "loaded " + path + " (" + strconv.Itoa(d.Size()) + " cards)"
	_ = cmd
	return nil
}

// syncDeckOff keeps the selected deck entry inside the visible viewport.
func (m *model) syncDeckOff() {
	h := bodyHeight(m.height) - 2
	if m.deckSel < m.deckOff {
		m.deckOff = m.deckSel
	}
	if m.deckSel >= m.deckOff+h {
		m.deckOff = m.deckSel - h + 1
	}
	if m.deckOff < 0 {
		m.deckOff = 0
	}
}

func cycle(v *string, values []string) {
	for i, s := range values {
		if s == *v {
			*v = values[(i+1)%len(values)]
			return
		}
	}
	*v = values[0]
}

func label(name, val string) string {
	if val == "" {
		return name + ":all"
	}
	return name + ":" + val
}

// paneInteriors splits the terminal width into the four pane *interior*
// widths (browser, card sheet, artwork, deck). Borders are always drawn on
// every pane (2 columns each), so the sum plus borders equals total
// whenever space allows; below that we overflow gracefully instead of
// clipping the right pane.
func paneInteriors(total int) (l, s, a, d int) {
	const (
		minL, minS, minA, minD = 22, 18, 10, 26
		borderTotal            = 8
	)
	inner := total - borderTotal
	l = clamp(inner*24/100, minL, 38)
	d = clamp(inner*25/100, minD, 42)
	s = clamp(inner*29/100, minS, 34)
	a = inner - l - d - s
	if a < minA {
		a = minA
	}
	// Overflow guard for narrow terminals: shrink panes down to hard minimums.
	for l+s+a+d > inner && (l > 16 || s > 12 || a > 6 || d > 18) {
		switch {
		case s >= a && s >= d && s >= l && s > 12:
			s--
		case d >= a && d >= l && d > 18:
			d--
		case l >= a && l > 16:
			l--
		default:
			a--
		}
	}
	return l, s, a, d
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func bodyHeight(total int) int { return max(6, total-5) }

// artWidth is the artwork pane's interior width.
func (m model) artWidth() int {
	_, _, a, _ := paneInteriors(m.width)
	return a - 2
}

func (m model) paneStyle(w, h int, focused bool) lipgloss.Style {
	s := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(w).
		Height(h)
	if focused {
		return s.BorderForeground(lipgloss.Color("#88cc88"))
	}
	return s.BorderForeground(lipgloss.Color("#444455"))
}

// paneTitle renders the small header line shown at the top of a pane.
func paneTitle(s string, focused bool) string {
	st := lipgloss.NewStyle()
	if focused {
		return st.Foreground(lipgloss.Color("#88ff88")).Bold(true).Render("● " + s)
	}
	return st.Faint(true).Render("○ " + s)
}

// artBlock renders the artwork pane: cropped image when available, a
// spinner while fetching, faint placeholders otherwise.
func (m model) artBlock(w, h int) string {
	title := paneTitle("artwork", false)
	sel, ok := m.list.SelectedItem().(item)
	if !ok {
		return title
	}
	code := sel.card.Code
	if payload, iw, ih := image.Card(code, w, h); payload != "" && m.imageMode {
		return title + "\n" + lipgloss.NewStyle().Width(iw).Height(ih).Render(payload)
	}
	var line string
	faint := lipgloss.NewStyle().Faint(true)
	switch {
	case m.pending[code] || image.Fetching(code):
		line = m.spinner.View() + " fetching art…"
	case m.failed[code]:
		line = faint.Render("✗ no art")
	case image.Path(code) == "":
		line = faint.Render("(no artwork cached)")
	default:
		line = faint.Render("(press v to show)")
	}
	return title + "\n" + line
}

// deckView renders the deck entries with a scrolling viewport and aligned
// qty/influence columns.
func (m model) deckView(w, h int) string {
	entries := m.deckEntries()
	var b strings.Builder
	if len(entries) == 0 {
		b.WriteString(lipgloss.NewStyle().Faint(true).Render("(empty deck)"))
	}
	for i := m.deckOff; i < len(entries) && i < m.deckOff+h; i++ {
		e := entries[i]
		cursor := " "
		style := lipgloss.NewStyle()
		if i == m.deckSel {
			cursor = ">"
			style = style.Bold(true).Foreground(lipgloss.Color("#88ff88"))
		}
		inf := ""
		if v := influenceOf(m.d, e); v > 0 {
			inf = strconv.Itoa(v)
		}
		titleMax := w - 5 - len(inf)
		title := truncate(e.Card.Title, titleMax)
		pad := strings.Repeat(" ", max(0, titleMax-len([]rune(title))))
		fmt.Fprintf(&b, "%s\n", style.Render(fmt.Sprintf("%s%2d %s%s%s", cursor, e.Qty, title, pad, inf)))
	}
	return b.String()
}

func (m model) View() string {
	if m.quitting {
		return ""
	}
	bar := func(focusStyle lipgloss.Style) string {
		return focusStyle.Render(fmt.Sprintf(
			"[1] %s  [2] %s  [3] %s  · %s",
			label("side", m.query.Side),
			label("type", m.query.Type),
			label("faction", m.query.Faction),
			m.status))
	}
	active := lipgloss.NewStyle().Foreground(lipgloss.Color("#88ff88"))
	inactive := lipgloss.NewStyle().Faint(true)

	lw, sw, aw, dw := paneInteriors(m.width)
	bh := bodyHeight(m.height)

	focusBrowserPane := m.focus == focusBrowser
	left := m.paneStyle(lw, bh, focusBrowserPane).Render(
		paneTitle("browser", focusBrowserPane) + "\n" + m.list.View())
	sheet := m.paneStyle(sw, bh, false).Render(
		paneTitle("card", false) + "\n" + m.preview)
	art := m.paneStyle(aw, bh, false).Render(m.artBlock(aw-2, bh-2))

	id := "(no identity)"
	if m.d.Identity.Type == "identity" {
		id = m.d.Identity.Title
	}
	focusDeckPane := m.focus == focusDeck
	right := m.paneStyle(dw, bh, focusDeckPane).Render(
		paneTitle("deck · ◆ "+id, focusDeckPane) + "\n" + m.deckView(dw-2, bh-3))

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, sheet, art, right)

	issues := m.d.Validate()
	foot := bar(active)
	if len(issues) > 0 {
		msgs := make([]string, 0, min(3, len(issues)))
		for _, iss := range issues[:min(3, len(issues))] {
			msgs = append(msgs, "✗ "+iss.Msg)
		}
		if len(issues) > 3 {
			msgs = append(msgs, fmt.Sprintf("… +%d more", len(issues)-3))
		}
		foot = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6666")).Render(strings.Join(msgs, "; "))
	}
	help := inactive.Render("j/k move · h/l panes · enter add · x/X del · +/- qty · i identity · v img · w save · e load · q quit")
	return body + "\n" + foot + "\n" + help
}

func influenceOf(d *deck.Deck, e deck.Entry) int {
	if !e.Card.InfluenceCost.Valid || e.Card.Faction == d.Identity.Faction {
		return 0
	}
	return int(e.Card.InfluenceCost.Int64) * e.Qty
}

func truncate(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	return string(r[:max(0, w-1)]) + "…"
}

const usage = `usage: nrbuild [--plain] [--width N] [--no-icons] [--nerd] [--images] [deckfile]

Interactive Netrunner deck builder. Browse via SQL-backed queries and add
cards to a deck validated against standard construction rules.

If deckfile exists it is loaded; decks are saved there with 'w'.
Piping a decklist (or bare card codes) into stdin seeds the deck.`

func main() {
	opts := render.Default()
	file := ""
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--plain":
			opts.Plain = true
		case "--no-icons":
			opts.Icons = false
		case "--nerd":
			opts.NerdIcons = true
		case "--width":
			i++
			if i >= len(args) {
				fatal(usage)
			}
			n, err := strconv.Atoi(args[i])
			if err != nil {
				fatal(usage)
			}
			opts.Width = n
		default:
			file = args[i]
		}
	}

	db, err := carddb.Open()
	if err != nil {
		fatal(err)
	}
	defer db.Close()

	m := newModel(db, opts, file)
	if file != "" {
		if b, err := os.ReadFile(file); err == nil {
			if d, err := deck.Decode(db, string(b)); err == nil {
				m.d = d
				m.query = carddb.Query{Side: d.Identity.Side}
			}
		}
	}
	seedFromStdin(db, &m)

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fatal(err)
	}
}

// seedFromStdin seeds the deck from piped input: either a decklist
// ("Identity: code", "Nx code") or bare card codes (e.g. from nr-list.sh).
func seedFromStdin(db carddb.DB, m *model) {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil || len(strings.TrimSpace(string(b))) == 0 {
		return
	}
	text := string(b)
	if strings.Contains(text, "x ") || strings.Contains(strings.ToLower(text), "identity:") {
		if d, err := deck.Decode(db, text); err == nil && d.Identity.Type == "identity" {
			m.d = d
			m.query = carddb.Query{Side: d.Identity.Side}
			return
		}
	}
	// bare codes: add one copy each; first identity becomes deck identity
	for _, line := range strings.Fields(text) {
		c, err := carddb.ByCode(db, line)
		if err != nil {
			continue
		}
		if c.Type == "identity" && m.d.Identity.Type != "identity" {
			m.d.Identity = c
			continue
		}
		m.d.Add(c)
	}
}

func fatal(v any) {
	fmt.Fprintln(os.Stderr, v)
	os.Exit(1)
}
