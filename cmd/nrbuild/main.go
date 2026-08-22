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
	preview string

	d         *deck.Deck
	deckSel   int
	imageMode bool
	file      string
	status    string

	focus  focus
	width  int
	height int
	opts   render.Options

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
	return model{db: db, list: l, opts: opts, file: file, d: &deck.Deck{}, status: "no identity yet — press i to pick one"}
}

func (m *model) refresh() {
	cards, err := carddb.Run(m.db, m.query)
	if err != nil {
		m.status = "error: " + err.Error()
		return
	}
	items := make([]list.Item, len(cards))
	for i, c := range cards {
		items[i] = item{card: c}
	}
	m.list.SetItems(items)
	m.status = fmt.Sprintf("%d cards · %s", len(items), m.query.String())
	m.updatePreview()
}

func (m *model) updatePreview() {
	sel, ok := m.list.SelectedItem().(item)
	if !ok {
		m.preview = ""
		return
	}
	w := m.width - m.listWidth() - m.deckWidth()
	if w < 30 {
		w = 30
	}
	if m.imageMode {
		h := m.height - 6
		if s := image.Card(sel.card.Code, w, h); s != "" {
			m.preview = s
			return
		}
	}
	o := m.opts
	o.Width = w - 2
	m.preview = render.Card(sel.card, o)
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
		m.list.SetSize(m.listWidth(), msg.Height-5)
		m.refresh()
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
			m.refresh()
			return m, nil
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
			m.refresh()
			return m, nil
		case "2":
			cycle(&m.query.Type, types)
			m.refresh()
			return m, nil
		case "3":
			cycle(&m.query.Faction, factions)
			m.refresh()
			return m, nil
		case "v":
			m.imageMode = !m.imageMode
			if m.imageMode && !image.Supported() {
				m.status = "terminal has no graphics protocol; using text"
			} else {
				m.status = "image preview " + map[bool]string{true: "on", false: "off"}[m.imageMode]
			}
			m.updatePreview()
			return m, nil
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
				}
				return m, nil
			case "k", "up":
				if m.deckSel > 0 {
					m.deckSel--
				}
				return m, nil
			case "g", "home":
				m.deckSel = 0
				return m, nil
			case "G", "end":
				m.deckSel = max(0, len(m.deckEntries())-1)
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
		m.updatePreview()
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
	m.query = carddb.Query{Side: d.Identity.Side}
	m.refresh()
	m.status = "loaded " + path + " (" + strconv.Itoa(d.Size()) + " cards)"
	return nil
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

func (m model) listWidth() int {
	w := m.width / 4
	if w < 30 {
		w = 30
	}
	if w > 46 {
		w = 46
	}
	return w
}

func (m model) deckWidth() int {
	w := m.width / 4
	if w < 34 {
		w = 34
	}
	if w > 52 {
		w = 52
	}
	return w
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

	listStyle := lipgloss.NewStyle().Width(m.listWidth())
	if m.focus == focusBrowser {
		listStyle = listStyle.Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#5577aa"))
	}
	left := listStyle.Render(m.list.View())

	previewWidth := m.width - m.listWidth() - m.deckWidth()
	center := lipgloss.NewStyle().Width(max(20, previewWidth)).Render(m.preview)

	deckStyle := lipgloss.NewStyle().Width(m.deckWidth())
	if m.focus == focusDeck {
		deckStyle = deckStyle.Border(lipgloss.DoubleBorder()).BorderForeground(lipgloss.Color("#88cc88"))
	} else {
		deckStyle = deckStyle.Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#444444"))
	}
	deckStyle = deckStyle.Height(m.height - 5)

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", bar(inactive))
	id := "(no identity)"
	if m.d.Identity.Type == "identity" {
		id = m.d.Identity.Title
	}
	fmt.Fprintf(&b, "%s\n", active.Render("◆ "+id))

	issues := m.d.Validate()
	summary := m.d.Summary()
	if len(issues) == 0 && m.d.Identity.Type == "identity" {
		summary = "✓ " + summary
	}
	fmt.Fprintf(&b, "%s\n", summary)

	for i, e := range m.deckEntries() {
		cursor := " "
		style := lipgloss.NewStyle()
		if i == m.deckSel {
			cursor = ">"
			style = style.Bold(true)
		}
		line := fmt.Sprintf("%s%dx %s", cursor, e.Qty, e.Card.Title)
		if inf := influenceOf(m.d, e); inf > 0 {
			line += fmt.Sprintf(" (%d)", inf)
		}
		fmt.Fprintln(&b, style.Render(truncate(line, m.deckWidth()-4)))
	}

	right := deckStyle.Render(b.String())

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, center, right)

	foot := bar(active)
	if len(issues) > 0 {
		msgs := make([]string, 0, min(3, len(issues)))
		for _, iss := range issues[:min(3, len(issues))] {
			msgs = append(msgs, "✗ "+iss.Msg)
		}
		extra := ""
		if len(issues) > 3 {
			extra = fmt.Sprintf(" … +%d more", len(issues)-3)
		}
		foot = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6666")).Render(strings.Join(msgs, "; ") + extra)
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
