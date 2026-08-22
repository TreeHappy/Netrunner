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
//	p then 1-5   multi-select pickers for side/type/faction/pack/cost
//	             (type to narrow, space selects, enter applies)
//	o    multi-select sort fields (each press: ↑ → ↓ → off)
//	v    toggle image preview        w  save decklist
//	e    edit/load a decklist file   q  quit
package main

import (
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
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
	bandW   int
	bandH   int
	pending map[string]bool
	failed  map[string]bool

	d         *deck.Deck
	deckSel   int
	deckOff   int
	imageMode bool
	file      string
	status    string
	packs     []string // distinct pack codes, for the pack-cycling filter

	focus  focus
	width  int
	height int
	opts   render.Options
	loaded bool
	artRow int // art-band row within the preview (-1 when absent)

	sortOpen bool // sort-field picker active ("o")
	menuOpen bool // dimension menu active ("p")
	pick     valuePicker

	quitting bool
}

// pickerKinds maps the pickers (menu digits / Ctrl+1..5) to dimensions.
var pickerKinds = []string{"side", "type", "faction", "pack", "cost"}

// valuePicker is the modal multi-select filter chooser.
type valuePicker struct {
	open     bool
	col      int // index into pickerKinds
	cursor   int
	filter   string
	values   []string
	selected map[string]bool
}

// filtered returns candidate values matching the typed substring.
func (p *valuePicker) filtered() []string {
	if p.filter == "" {
		return p.values
	}
	f := strings.ToLower(p.filter)
	var out []string
	for _, v := range p.values {
		if strings.Contains(strings.ToLower(v), f) {
			out = append(out, v)
		}
	}
	return out
}

// openPicker prepares the picker for a dimension, pre-selecting whatever
// the current query already filters on.
func (m *model) openPicker(col int) {
	if col < 0 || col >= len(pickerKinds) {
		return
	}
	m.sortOpen = false
	m.menuOpen = false
	p := valuePicker{col: col, selected: map[string]bool{}}
	switch col {
	case 0:
		p.values = distinctOr(m.db, "side_code", sides[1:])
		setFromQuery(p.selected, m.query.Side)
	case 1:
		p.values = distinctOr(m.db, "type_code", types[1:])
		setFromQuery(p.selected, m.query.Type)
	case 2:
		p.values = distinctOr(m.db, "faction_code", factions[1:])
		setFromQuery(p.selected, m.query.Faction)
	case 3:
		p.values = distinctOr(m.db, "pack_code", m.packs)
		setFromQuery(p.selected, m.query.Pack)
	case 4:
		for c := range costCaps[1:] { // skip -1 sentinel
			p.values = append(p.values, strconv.Itoa(c))
		}
		for _, c := range m.query.Costs {
			p.selected[strconv.Itoa(c)] = true
		}
	}
	m.pick = p
	m.pick.open = true
}

// distinctOr queries distinct values, falling back statically so the
// picker still works without a database (tests, degraded cache).
func distinctOr(db carddb.DB, column string, fallback []string) []string {
	if db != nil {
		if vals, err := carddb.Distinct(db, column); err == nil && len(vals) > 0 {
			return vals
		}
	}
	return fallback
}

func setFromQuery(dst map[string]bool, vals []string) {
	for _, v := range vals {
		dst[v] = true
	}
}

// applyPicker writes the selection back into the query and refreshes.
func (m *model) applyPicker() tea.Cmd {
	p := &m.pick
	slice := func() []string {
		var out []string
		for _, v := range p.values {
			if p.selected[v] {
				out = append(out, v)
			}
		}
		return out
	}
	switch p.col {
	case 0:
		m.query.Side = slice()
	case 1:
		m.query.Type = slice()
	case 2:
		m.query.Faction = slice()
	case 3:
		m.query.Pack = slice()
	case 4:
		m.query.Costs = nil
		for _, v := range slice() {
			n, _ := strconv.Atoi(v)
			m.query.Costs = append(m.query.Costs, n)
		}
		sort.Ints(m.query.Costs)
	}
	p.open = false
	return m.refresh()
}

// sortChoices are the fields offered in the sort picker.
var sortChoices = []string{"title", "type", "faction", "pack", "cost", "strength", "code"}

// sortChoiceByKey maps a picker digit (1-based) to its field.
func sortChoiceByKey(n int) string {
	if n < 1 || n > len(sortChoices) {
		return ""
	}
	return sortChoices[n-1]
}

// toggleSort cycles a field through absent → ascending → descending.
func (m *model) toggleSort(field string) bool {
	if _, ok := carddb.SortColumns[field]; !ok {
		return false
	}
	for i, s := range m.query.Order {
		if s.Field != field {
			continue
		}
		if !s.Desc {
			m.query.Order[i].Desc = true
		} else {
			m.query.Order = append(m.query.Order[:i], m.query.Order[i+1:]...)
		}
		return true
	}
	m.query.Order = append(m.query.Order, carddb.Sort{Field: field})
	return true
}

func newModel(db carddb.DB, opts render.Options, file string, packs []string) model {
	l := list.New([]list.Item{}, list.NewDefaultDelegate(), 40, 24)
	l.Title = "Netrunner cards"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.FilterInput.Placeholder = "search title/code…"
	l.KeyMap.CursorUp.SetKeys("up", "k")
	l.KeyMap.CursorDown.SetKeys("down", "j")
	l.Styles.Title = l.Styles.Title.Foreground(lipgloss.Color("#dddddd")).Background(lipgloss.Color("#333355"))
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	return model{db: db, list: l, spinner: sp, opts: opts, file: file, packs: packs,
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
	_, s, _ := paneInteriors(m.width)
	bandW := s - 6 // sheet border+padding + pane border margin
	var cmd tea.Cmd

	payload, iw, ih := image.Card(sel.card.Code, bandW, bodyHeight(m.height)-2)
	if payload != "" && m.imageMode && ih > 0 {
		m.bandW, m.bandH = iw, ih
	} else {
		h := bandW * 2 / 5
		if h < 4 {
			h = 4
		}
		if h > 14 {
			h = 14
		}
		m.bandW, m.bandH = bandW, h
		if m.imageMode && image.Supported() && image.Path(sel.card.Code) == "" &&
			!m.pending[sel.card.Code] && !m.failed[sel.card.Code] {
			code := sel.card.Code
			m.pending[code] = true
			m.status = "fetching art… " + code
			cmd = tea.Batch(fetchArtCmd(code), m.spinner.Tick)
		}
	}

	o := m.opts
	o.Width = s - 2 // Card() subtracts border + padding itself
	m.artRow = -1
	if m.imageMode {
		o.ArtBand = &render.ArtBand{W: m.bandW, H: m.bandH}
		if payload == "" && image.Supported() {
			switch {
			case m.pending[sel.card.Code] || image.Fetching(sel.card.Code):
				o.ArtNote = "fetching art…"
			case m.failed[sel.card.Code]:
				o.ArtNote = "✗ no art"
			default:
				o.ArtNote = "no artwork cached"
			}
		}
	}
	m.preview = render.Card(sel.card, o)
	for i, ln := range strings.Split(m.preview, "\n") {
		if strings.Contains(ln, render.ArtSentinel) {
			m.artRow = i
			break
		}
	}
	if payload != "" && m.imageMode {
		m.preview = render.SpliceArt(m.preview, payload)
	} else if image.UseUeberzug() {
		image.HideArt()
	}
	return cmd
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
		if len(m.query.Side) > 0 && !slices.Contains(m.query.Side, sel.card.Side) {
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

func (m *model) quit() (tea.Model, tea.Cmd) {
	m.quitting = true
	return m, tea.Sequence(func() tea.Msg { image.HideArt(); return nil }, tea.Quit)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if image.UseUeberzug() {
			image.HideArt()
		}
		m.width, m.height = msg.Width, msg.Height
		lw, _, _ := paneInteriors(m.width)
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
		if m.pick.open {
			rows := m.pick.filtered()
			switch msg.String() {
			case "ctrl+c":
				return m.quit()
			case "esc":
				m.pick.open = false
				return m, nil
			case "enter":
				return m, m.applyPicker()
			case "down", "j", "ctrl+n":
				if m.pick.cursor < len(rows)-1 {
					m.pick.cursor++
				}
				return m, nil
			case "up", "k", "ctrl+p":
				if m.pick.cursor > 0 {
					m.pick.cursor--
				}
				return m, nil
			case " ", "space":
				if m.pick.cursor < len(rows) {
					v := rows[m.pick.cursor]
					m.pick.selected[v] = !m.pick.selected[v]
				}
				return m, nil
			case "backspace", "ctrl+h":
				r := []rune(m.pick.filter)
				if n := len(r); n > 0 {
					m.pick.filter = string(r[:n-1])
					m.pick.cursor = 0
				}
				return m, nil
			default:
				if len(msg.Runes) == 1 && msg.Runes[0] >= ' ' {
					m.pick.filter += string(msg.Runes)
					m.pick.cursor = 0
				}
				return m, nil
			}
		}
		if m.menuOpen {
			switch msg.String() {
			case "ctrl+c":
				return m.quit()
			case "esc", "enter", "p":
				m.menuOpen = false
				return m, nil
			default:
				n, err := strconv.Atoi(msg.String())
				m.menuOpen = false
				if err == nil && n >= 1 && n <= len(pickerKinds) {
					m.openPicker(n - 1)
				}
				return m, nil
			}
		}
		if m.sortOpen {
			switch msg.String() {
			case "ctrl+c":
				return m.quit()
			case "esc", "enter", "o", "ctrl+o", "s":
				m.sortOpen = false
				return m, nil
			default:
				n, err := strconv.Atoi(msg.String())
				if err == nil && m.toggleSort(sortChoiceByKey(n)) {
					return m, m.refresh()
				}
				return m, nil
			}
		}
		if m.list.FilterState() == list.Filtering {
			break // let the filter input handle keys
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m.quit()
		case "esc":
			if m.list.FilterState() != list.FilterApplied {
				return m.quit()
			}
		case "enter":
			m.addSelected()
			return m, nil
		case "i":
			side := "corp"
			if len(m.query.Side) > 0 {
				side = m.query.Side[0]
			}
			m.query = carddb.Query{Side: []string{side}, Type: []string{"identity"}}
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
		case "ctrl+1", "alt+1":
			m.openPicker(0)
			return m, nil
		case "ctrl+2", "alt+2":
			m.openPicker(1)
			return m, nil
		case "ctrl+3", "alt+3":
			m.openPicker(2)
			return m, nil
		case "ctrl+4", "alt+4":
			m.openPicker(3)
			return m, nil
		case "ctrl+5", "alt+5":
			m.openPicker(4)
			return m, nil
		case "p":
			m.menuOpen = true
			m.sortOpen = false
			m.pick.open = false
			return m, nil
		case "o":
			m.sortOpen = true
			m.pick.open = false
			m.menuOpen = false
			return m, nil
		case "1":
			m.query.Side = cycleSingle(m.query.Side, sides)
			return m, m.refresh()
		case "2":
			m.query.Type = cycleSingle(m.query.Type, types)
			return m, m.refresh()
		case "3":
			m.query.Faction = cycleSingle(m.query.Faction, factions)
			return m, m.refresh()
		case "4":
			m.cyclePack()
			return m, m.refresh()
		case "5":
			cur := costUnset
			if m.query.MaxCost != nil {
				cur = *m.query.MaxCost
			}
			m.query.MaxCost = carddb.IntPtr(nextCostCap(cur))
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
	m.query = carddb.Query{Side: []string{d.Identity.Side}}
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

// cyclePack advances the pack filter through the distinct pack codes in
// the cache ("" = all first). No-op when the cache has no packs.
func (m *model) cyclePack() {
	if len(m.packs) == 0 {
		return
	}
	idx := 0
	pack := ""
	if len(m.query.Pack) == 1 {
		pack = m.query.Pack[0]
	}
	for i, p := range m.packs {
		if p == pack {
			idx = i
			break
		}
	}
	m.query.Pack = []string{m.packs[(idx+1)%len(m.packs)]}
}

// costCaps is the max-cost ladder the cost filter cycles through.
var costCaps = []int{-1, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9} // -1 = no cap

const costUnset = -1

// nextCostCap returns the cap following c (wrapping back to "no cap").
func nextCostCap(c int) int {
	for i, v := range costCaps {
		if v == c {
			return costCaps[(i+1)%len(costCaps)]
		}
	}
	return costUnset
}

// cycleSingle advances a single-value selection through choices ("" in the
// list means "all" → empty slice). Multi-selections collapse to the next
// single value.
func cycleSingle(cur []string, choices []string) []string {
	curVal := ""
	if len(cur) == 1 {
		curVal = cur[0]
	}
	for i, c := range choices {
		if c == curVal {
			next := choices[(i+1)%len(choices)]
			if next == "" {
				return nil
			}
			return []string{next}
		}
	}
	return nil
}

// containsStr reports whether vals contains v.
func containsStr(vals []string, v string) bool { return slices.Contains(vals, v) }

// label renders "name:all" for an unset dimension or the joined values.
func label(name string, vals []string) string {
	if len(vals) == 0 {
		return name + ":all"
	}
	return name + ":" + strings.Join(vals, ",")
}

// paneInteriors splits the terminal width into the three pane *interior*
// widths (browser, card sheet, deck). Borders are always drawn on every
// pane (2 columns each), so the sum plus borders equals total whenever
// space allows; below that we overflow gracefully instead of clipping.
func paneInteriors(total int) (l, s, d int) {
	const (
		minL, minS, minD = 22, 18, 26
		borderTotal      = 6
	)
	inner := total - borderTotal
	l = clamp(inner*24/100, minL, 38)
	d = clamp(inner*27/100, minD, 42)
	s = inner - l - d
	if s < minS {
		s = minS
	}
	// Overflow guard for narrow terminals: shrink panes down to hard minimums.
	for l+s+d > inner && (l > 16 || s > 12 || d > 18) {
		switch {
		case s >= d && s >= l && s > 12:
			s--
		case d >= l && d > 18:
			d--
		default:
			l--
		}
	}
	return l, s, d
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

// bodyHeight is the pane height: total minus pane borders/title and the
// three lines below (status, SQL, help).
func bodyHeight(total int) int { return max(6, total-6) }

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

// menuPanel is the one-line dimension chooser shown while "p" is active.
func (m model) menuPanel() string {
	head := lipgloss.NewStyle().Bold(true).Render("pick filters — ")
	faint := lipgloss.NewStyle().Faint(true)
	return head + faint.Render("[1] side · [2] type · [3] faction · [4] pack · [5] cost · esc cancel")
}

// pickPanel renders the modal multi-select chooser below the panes.
func (m model) pickPanel() string {
	p := &m.pick
	title := pickerKinds[p.col]
	head := lipgloss.NewStyle().Bold(true).Render(
		title + " — type to filter · ↑/↓ move · space select · enter apply · esc cancel")
	faint := lipgloss.NewStyle().Faint(true)
	rows := p.filtered()
	var b strings.Builder
	b.WriteString(head + "\n" + faint.Render("/"+p.filter+"▏"))
	if len(rows) == 0 {
		b.WriteString("\n" + faint.Render("(no matches)"))
		return b.String()
	}
	const winH = 6
	start := p.cursor - winH/2
	if start > len(rows)-winH {
		start = len(rows) - winH
	}
	if start < 0 {
		start = 0
	}
	end := min(len(rows), start+winH)
	for i := start; i < end; i++ {
		box := "[ ]"
		style := faint
		if p.selected[rows[i]] {
			box = "[x]"
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("#88ff88"))
		}
		cur := "  "
		if i == p.cursor {
			cur = "> "
		}
		b.WriteString("\n" + style.Render(cur+box+" "+rows[i]))
	}
	return b.String()
}

// sortPanel renders the multi-select ORDER BY picker while open.
func (m model) sortPanel() string {
	head := lipgloss.NewStyle().Bold(true).Render("sort — [1-7] toggle ↑/↓/off · esc close")
	faint := lipgloss.NewStyle().Faint(true)
	var b strings.Builder
	b.WriteString(head)
	for i, f := range sortChoices {
		state := "·"
		style := faint
		for j, s := range m.query.Order {
			if s.Field == f {
				mark := "↑"
				if s.Desc {
					mark = "↓"
				}
				state = fmt.Sprintf("%s%d", mark, j+1)
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("#88ff88"))
			}
		}
		fmt.Fprintf(&b, "\n%s [%d] %-8s", style.Render(state), i+1, f)
	}
	return b.String()
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

	var panels []string
	extra := 0
	if m.menuOpen {
		panels = append(panels, m.menuPanel())
	}
	if m.pick.open {
		panels = append(panels, m.pickPanel())
	}
	if m.sortOpen {
		panels = append(panels, m.sortPanel())
	}
	for _, pn := range panels {
		extra += strings.Count(pn, "\n") + 1
	}

	lw, sw, dw := paneInteriors(m.width)
	bh := max(6, bodyHeight(m.height)-extra) // keep the frame inside the terminal

	focusBrowserPane := m.focus == focusBrowser
	left := m.paneStyle(lw, bh, focusBrowserPane).Render(
		paneTitle("browser", focusBrowserPane) + "\n" + m.list.View())
	sheet := m.paneStyle(sw, bh, false).Render(
		paneTitle("card", false) + "\n" + m.preview)

	id := "(no identity)"
	if m.d.Identity.Type == "identity" {
		id = m.d.Identity.Title
	}
	focusDeckPane := m.focus == focusDeck
	right := m.paneStyle(dw, bh, focusDeckPane).Render(
		paneTitle("deck · ◆ "+id, focusDeckPane) + "\n" + m.deckView(dw-2, bh-3))

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, sheet, right)

	// Hard geometry guard: pane content can outgrow its configured height
	// (list status bars, wrapped text); clip so the frame always fits.
	maxRows := m.height - extra - 3 // foot, sql, help lines
	if maxRows < 4 {
		maxRows = 4
	}
	if rows := strings.Split(body, "\n"); len(rows) > maxRows {
		bottom := rows[len(rows)-1] // keep the pane bottom border
		rows = append(rows[:max(1, maxRows-1)], bottom)
		body = strings.Join(rows, "\n")
	}

	// ueberzugpp overlay: report the art band's absolute cell position
	// (sheet pane interior starts at column lw+3; sheet border+padding +2).
	if image.UseUeberzug() {
		if m.imageMode && m.artRow >= 0 {
			image.ApplyUeberzug(lw+5, 2+m.artRow)
		} else {
			image.HideArt()
		}
	}

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
	// The exact statement the current filter selections compile to; always
	// visible so the GUI stays honest about what hits DuckDB.
	sqlLine := inactive.Render(truncate("sql: "+m.query.DebugSQL(), m.width))
	help := inactive.Render(truncate("j/k move · h/l panes · enter add · x/X del · +/- qty · i identity · p pick · o sort · v img · w save · e load · q quit", m.width))
	out := body + "\n" + foot + "\n" + sqlLine + "\n" + help
	if panelText := strings.Join(panels, "\n"); panelText != "" {
		out = body + "\n" + panelText + "\n" + foot + "\n" + sqlLine + "\n" + help
	}
	return out
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
	defer image.Shutdown()
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

	// Pack codes for the pack-cycling filter; empty on error (key no-op).
	packs, _ := carddb.Distinct(db, "pack_code")

	m := newModel(db, opts, file, packs)
	if file != "" {
		if b, err := os.ReadFile(file); err == nil {
			if d, err := deck.Decode(db, string(b)); err == nil {
				m.d = d
				m.query = carddb.Query{Side: []string{d.Identity.Side}}
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
			m.query = carddb.Query{Side: []string{d.Identity.Side}}
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
