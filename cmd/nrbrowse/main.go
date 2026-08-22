package main

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"boardy/netrunner/internal/carddb"
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

type model struct {
	db       carddb.DB
	cards    []carddb.Card
	list     list.Model
	spinner  spinner.Model
	preview  string
	bandW    int
	bandH    int
	pending  map[string]bool
	failed   map[string]bool
	imageOn  bool
	query    carddb.Query
	packs    []string // distinct pack codes for the pack-cycling filter
	width    int
	height   int
	opts     render.Options
	status   string
	quitting bool
	loaded   bool
	artRow   int  // art-band row within the preview (-1 when absent)
	sortOpen bool // sort-picker overlay active
	menuOpen bool // "p" dimension menu active
	pick     valuePicker
}

// pickerKinds maps the Ctrl/Alt+1..5 pickers to their query dimension.
var pickerKinds = []string{"side", "type", "faction", "pack", "cost"}

// valuePicker is the modal multi-select filter chooser (Ctrl/Alt+1..5).
type valuePicker struct {
	open     bool
	col      int // index into pickerKinds (0-based)
	cursor   int
	filter   string
	values   []string
	selected map[string]bool
}

// filtered returns the candidate values matching the typed substring.
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
		p.values = distinctOr(m.db, "pack_code", nil)
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

// distinctOr queries distinct values, falling back to the static list so
// the picker still works without a database (tests, degraded cache).
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
	const winH = 8
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

var (
	sides    = []string{"", "corp", "runner"}
	factions = []string{"", "anarch", "criminal", "shaper", "haas-bioroid", "jinteki", "nbn", "weyland-consortium", "neutral-runner", "neutral-corp"}
	types    = []string{"", "agenda", "asset", "event", "hardware", "ice", "identity", "operation", "program", "resource", "upgrade"}
)

func newModel(db carddb.DB, opts render.Options, packs []string) model {
	delegate := list.NewDefaultDelegate()
	l := list.New([]list.Item{}, delegate, 40, 24)
	l.Title = "Netrunner cards"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.FilterInput.Placeholder = "search title/code…"
	l.Styles.Title = l.Styles.Title.Foreground(lipgloss.Color("#dddddd")).Background(lipgloss.Color("#333355"))
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	return model{
		db: db, list: l, spinner: sp, opts: opts, packs: packs,
		width: 80, height: 24,
		pending: map[string]bool{}, failed: map[string]bool{},
		// Images are opt-in: rendering is unreliable across terminals, so
		// start with text previews and let "v" turn graphics on.
		imageOn: false, status: imageStatusDefault(),
	}
}

func imageStatusDefault() string {
	if image.Supported() {
		return "images off · v toggles"
	}
	return "terminal has no graphics protocol; text previews"
}

// sortChoices are the fields offered in the sort picker, in display order.
var sortChoices = []string{"title", "type", "faction", "pack", "cost", "strength", "code"}

// sortChoiceByKey maps a picker digit (1-based) to its field, "" if out of range.
func sortChoiceByKey(n int) string {
	if n < 1 || n > len(sortChoices) {
		return ""
	}
	return sortChoices[n-1]
}

// toggleSort cycles a field through the order terms: absent → ascending →
// descending → absent. Reports whether the field is a known sort column.
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

// sortPanel renders the multi-select picker shown while it is open: one
// line per field with its current position/direction in the ORDER BY.
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
				state = fmt.Sprintf("%s%d", mark, j+1) // direction + order position
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("#88ff88"))
			}
		}
		fmt.Fprintf(&b, "\n%s [%d] %-8s", style.Render(fmt.Sprintf("%s", state)), i+1, f)
	}
	return b.String()
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
	m.status = fmt.Sprintf("%d cards", len(items))
	return tea.Batch(clearInlineCmd, m.updatePreview())
}

// paneInterior returns the card pane's interior rows: total height minus
// the outer chrome (filter line, pane border top/bottom, pane title).
func (m model) paneInterior() int {
	h := m.height - 3 - 3
	if h < 6 {
		h = 6
	}
	return h
}

func (m *model) updatePreview() tea.Cmd {
	sel, ok := m.list.SelectedItem().(item)
	if !ok {
		m.preview = ""
		return nil
	}
	sw := m.sheetWidth()
	bandW := sw - 4 // inside the sheet's border + padding
	interior := m.paneInterior()
	var cmd tea.Cmd

	// Reserve room for header/stats/rules/body around the art band.
	reserved := 8
	maxBandH := interior - reserved
	if maxBandH < 4 {
		maxBandH = 4
	}

	// Try to render artwork; its cell size defines the reserved band.
	payload, iw, ih := image.Card(sel.card.Code, bandW, maxBandH)
	if payload != "" && m.imageOn && ih > 0 {
		m.bandW, m.bandH = iw, ih
	} else {
		// Placeholder band while fetching / when missing.
		h := sw * 2 / 5
		if h < 4 {
			h = 4
		}
		if h > maxBandH {
			h = maxBandH
		}
		m.bandW, m.bandH = bandW, h
		code := sel.card.Code
		if m.imageOn && image.Supported() && !m.pending[code] && !m.failed[code] {
			switch {
			case image.Path(code) == "":
				m.pending[code] = true
				cmd = tea.Batch(fetchArtCmd(code), m.spinner.Tick)
			case image.ArtPath(code) == "":
				// Scan cached but artwork not cropped yet; crop it instead
				// of silently rendering the full scan.
				m.pending[code] = true
				cmd = tea.Batch(ensureArtCmd(code), m.spinner.Tick)
			}
		}
	}

	o := m.opts
	o.Width = sw // Card() output spans exactly this many columns
	m.artRow = -1
	if m.imageOn {
		o.ArtBand = &render.ArtBand{W: m.bandW, H: m.bandH}
		o.ArtNote = ""
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
		// Shrink the band until the sheet fits the pane interior so the
		// view never exceeds the terminal (inline images break on scroll).
		for {
			sheet := render.Card(sel.card, o)
			if lipgloss.Height(sheet) <= interior || m.bandH <= 4 {
				if payload != "" && m.bandH >= ih {
					for i, ln := range strings.Split(sheet, "\n") {
						if strings.Contains(ln, render.ArtSentinel) {
							m.artRow = i
							break
						}
					}
					m.preview = render.SpliceArt(sheet, payload)
				} else {
					payload = ""
					for i, ln := range strings.Split(sheet, "\n") {
						if strings.Contains(ln, render.ArtSentinel) {
							m.artRow = i
							break
						}
					}
					m.preview = sheet
					if image.UseUeberzug() {
						image.HideArt()
					}
				}
				break
			}
			m.bandH--
			o.ArtBand.H = m.bandH
		}
		// Final guard: never let the preview overflow the pane interior.
		if rows := strings.Split(m.preview, "\n"); len(rows) > interior {
			m.preview = strings.Join(rows[:interior], "\n")
		}
		return cmd
	}
	m.preview = render.Card(sel.card, o)
	if image.UseUeberzug() {
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

func ensureArtCmd(code string) tea.Cmd {
	return func() tea.Msg {
		_, err := image.EnsureArtErr(code)
		return artFetchedMsg{code: code, err: err}
	}
}

func clearInlineCmd() tea.Msg {
	image.ClearInline()
	return nil
}

// sheetGeometry returns the browser-list and card-sheet interior widths so
// that both panes including borders sum exactly to the terminal width.
func (m model) sheetGeometry() (lw, sw int) {
	lw = m.listWidth()
	sw = m.width - lw - 4 // two pane borders
	for sw < 20 && lw > 24 {
		lw--
		sw++
	}
	if sw < 16 {
		sw = 16 // overflow gracefully below 44 columns
	}
	return lw, sw
}

func (m model) sheetWidth() int {
	_, sw := m.sheetGeometry()
	return sw - 2
}

func (m model) listWidth() int {
	w := m.width / 3
	if w < 32 {
		w = 32
	}
	if w > 50 {
		w = 50
	}
	return w
}

func (m model) Init() tea.Cmd { return nil }

func hideArtCmd() tea.Msg {
	image.HideArt()
	return nil
}

func (m *model) quit() (tea.Model, tea.Cmd) {
	m.quitting = true
	return m, tea.Sequence(hideArtCmd, clearInlineCmd, tea.Quit)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if image.UseUeberzug() {
			image.HideArt()
		}
		m.width, m.height = msg.Width, msg.Height
		h := msg.Height - 4
		m.list.SetSize(m.listWidth(), h)
		if !m.loaded {
			m.loaded = true
			return m, m.refresh()
		}
		return m, tea.Batch(clearInlineCmd, m.updatePreview())

	case artFetchedMsg:
		delete(m.pending, msg.code)
		if msg.err != nil {
			m.failed[msg.code] = true
			m.status = "no art for " + msg.code
		} else {
			m.status = fmt.Sprintf("%d cards", len(m.cards))
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
		if m.sortOpen {
			// Modal sort picker: capture everything except quit/close.
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
		switch msg.String() {
		case "ctrl+c":
			return m.quit()
		case "o", "ctrl+o":
			if m.list.FilterState() != list.Filtering {
				m.sortOpen = true
				m.pick.open = false
				m.menuOpen = false
				return m, nil
			}
		case "p":
			if m.list.FilterState() != list.Filtering {
				m.menuOpen = true
				m.sortOpen = false
				m.pick.open = false
				return m, nil
			}
		case "q":
			if m.list.FilterState() != list.Filtering {
				return m.quit()
			}
		case "esc":
			if m.list.FilterState() == list.Filtering || m.list.FilterState() == list.FilterApplied {
				break
			}
			return m.quit()
		case "enter":
			if sel, ok := m.list.SelectedItem().(item); ok {
				image.HideArt()
				image.ClearInline()
				fmt.Println(render.Card(sel.card, m.opts))
				m.quitting = true
				return m, tea.Quit
			}
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
			m.imageOn = !m.imageOn
			if m.imageOn && !image.Supported() {
				m.status = "terminal has no graphics protocol; using text"
			} else {
				m.status = "image preview " + map[bool]string{true: "on", false: "off"}[m.imageOn]
			}
			return m, tea.Batch(clearInlineCmd, m.updatePreview())
		}
	}

	var cmd tea.Cmd
	prev := m.list.SelectedItem()
	m.list, cmd = m.list.Update(msg)
	if m.list.SelectedItem() != prev {
		cmd = tea.Batch(clearInlineCmd, m.updatePreview())
	}
	return m, cmd
}

// cycleSingle advances a single-value selection through choices ("" in the
// list means "all" → empty slice). Multi-selections collapse to one value.
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

// cyclePack advances the pack filter through the distinct pack codes in
// the cache ("" = all first). No-op when the cache has no packs.
func (m *model) cyclePack() {
	if len(m.packs) == 0 {
		return
	}
	pack := ""
	if len(m.query.Pack) == 1 {
		pack = m.query.Pack[0]
	}
	idx := 0
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

// label renders "name:all" for an unset dimension or the joined values.
func label(name string, vals []string) string {
	if len(vals) == 0 {
		return name + ":all"
	}
	return name + ":" + strings.Join(vals, ",")
}

func paneStyle(w, h int, focused bool) lipgloss.Style {
	s := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Width(w).Height(h)
	if focused {
		return s.BorderForeground(lipgloss.Color("#88cc88"))
	}
	return s.BorderForeground(lipgloss.Color("#444455"))
}

func paneTitle(s string) string {
	return lipgloss.NewStyle().Faint(true).Render("○ " + s)
}

func (m model) View() string {
	if m.quitting {
		return ""
	}
	if m.width < 20 || m.height < 6 {
		// Degenerate terminal (or View before the first WindowSizeMsg):
		// the pane geometry math needs sane dimensions to work with.
		return trunc("terminal too small", max(1, m.width))
	}
	filterLine := lipgloss.NewStyle().Faint(true).Render(trunc(
		fmt.Sprintf("[1] %s  [2] %s  [3] %s  [4] %s  [5] %s · p pick · o sort · / search · v img · q quit · %s",
			label("side", m.query.Side),
			label("type", m.query.Type),
			label("faction", m.query.Faction),
			label("pack", m.query.Pack),
			costLabel(m.query),
			m.status), m.width))
	bh := m.height - 3
	if bh < 6 {
		bh = 6
	}
	lw, sw := m.sheetGeometry()

	left := paneStyle(lw, bh, true).Render(paneTitle("browser") + "\n" + m.list.View())
	mid := paneStyle(sw, bh, false).Render(paneTitle("card") + "\n" + m.preview)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, mid)
	// The exact statement the current filters compile to; always visible so
	// the GUI stays honest about what hits DuckDB.
	sqlLine := lipgloss.NewStyle().Faint(true).Render(trunc("sql: "+m.query.DebugSQL(), m.width))
	// Hard geometry guard: pane content can rewrap inside lipgloss and
	// grow past its configured height; clip so the view always fits the
	// terminal exactly (inline images break when the frame scrolls).
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
	for _, p := range panels {
		extra += strings.Count(p, "\n") + 1
	}
	maxRows := m.height - 2 - extra // leave room for sql + filter lines
	if maxRows < 4 {
		maxRows = 4
	}
	if rows := strings.Split(body, "\n"); len(rows) > maxRows {
		bottom := rows[len(rows)-1] // preserve the pane bottom border
		rows = append(rows[:max(1, maxRows-1)], bottom)
		body = strings.Join(rows, "\n")
	}
	panelText := strings.Join(panels, "\n")
	out := body + "\n" + sqlLine + "\n" + filterLine
	if panelText != "" {
		out = body + "\n" + panelText + "\n" + sqlLine + "\n" + filterLine
	}
	// ueberzugpp overlays are drawn outside the terminal buffer; report the
	// art band's absolute cell position each frame (mid interior starts at
	// column lw+3, sheet border+padding add 2 more; rows 0-1 are border+title).
	if image.UseUeberzug() {
		if m.imageOn && m.artRow >= 0 {
			image.ApplyUeberzug(lw+5, 2+m.artRow)
		} else {
			image.HideArt()
		}
	}
	return out
}

// costLabel renders the cost filter's value for the filter line.
func costLabel(q carddb.Query) string {
	if q.MaxCost == nil {
		return "cost:all"
	}
	return fmt.Sprintf("cost:<=%d", *q.MaxCost)
}

// trunc cuts a plain (ANSI-light) string to w printable cells.
func trunc(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	return string(r[:max(0, w-1)]) + "…"
}

// parsedArgs holds the result of command-line parsing.
type parsedArgs struct {
	opts       render.Options
	code       string // card code positional arg
	protocol   string // --images override ("" = none)
	renderTest string // --render-test WxH frame request
}

// parseArgs parses flags in both `--flag value` and `--flag=value` forms.
// Unknown dash-prefixed args are rejected; the single positional arg is the
// card code.
func parseArgs(args []string) (parsedArgs, error) {
	var p parsedArgs
	next := func(i *int) (string, bool) {
		if *i+1 >= len(args) {
			return "", false
		}
		*i++
		return args[*i], true
	}
	for i := 0; i < len(args); i++ {
		name, val, hasVal := strings.Cut(args[i], "=")
		switch name {
		case "--plain":
			p.opts.Plain = true
		case "--no-icons":
			p.opts.Icons = false
		case "--nerd":
			p.opts.NerdIcons = true
		case "--width":
			if !hasVal {
				var ok bool
				if val, ok = next(&i); !ok {
					return p, errUsage("missing value for --width")
				}
			}
			n, err := strconv.Atoi(val)
			if err != nil {
				return p, errUsage("invalid --width %q", val)
			}
			p.opts.Width = n
		case "--images":
			if !hasVal {
				var ok bool
				if val, ok = next(&i); !ok || strings.HasPrefix(val, "-") {
					val = os.Getenv("NETRUNNER_IMAGE_PROTOCOL")
				}
				if val == "" {
					val = "auto"
				}
			}
			if !isProtocolName(val) {
				return p, fmt.Errorf("unknown protocol %q (want kitty, sixel, iterm2, ueberzug, auto)", val)
			}
			p.protocol = val
		case "--render-test":
			if !hasVal {
				var ok bool
				if val, ok = next(&i); !ok {
					return p, errUsage("missing WxH for --render-test")
				}
			}
			p.renderTest = val
		default:
			if strings.HasPrefix(args[i], "-") {
				return p, errUsage("unknown flag %s", args[i])
			}
			p.code = args[i]
		}
	}
	return p, nil
}

func errUsage(format string, a ...any) error {
	return fmt.Errorf(format+":\n%s", append(a, usage)...)
}

// wantArt reports whether one-shot card output should attempt inline
// artwork. Rendering is opt-in: only an explicit --images request enables
// it (bare --images inherits NETRUNNER_IMAGE_PROTOCOL or "auto");
// off/none/halfblocks keep plain text.
func wantArt(protocol string) bool {
	switch strings.ToLower(protocol) {
	case "", "none", "off", "halfblocks":
		return false
	}
	return true
}

// renderCardWithArt renders a card sheet with inline artwork when the
// terminal supports a graphics protocol, downloading and cropping the
// image on demand. Falls back to plain rendering on any failure.
func renderCardWithArt(c carddb.Card, opts render.Options, width, height int) string {
	if opts.Plain || !image.Supported() || image.UseUeberzug() {
		return render.Card(c, opts)
	}
	payload, iw, ih := image.Card(c.Code, width-4, height)
	if payload == "" && image.Path(c.Code) == "" && !image.Fetching(c.Code) {
		if _, err := image.FetchWithArt(c.Code); err != nil {
			return render.Card(c, opts)
		}
		payload, iw, ih = image.Card(c.Code, width-4, height)
	}
	if payload == "" || ih == 0 {
		return render.Card(c, opts)
	}
	o := opts
	o.Width = width // Card() output spans exactly this many columns
	o.ArtBand = &render.ArtBand{W: iw, H: ih}
	o.ArtNote = ""
	return render.SpliceArt(render.Card(c, o), payload)
}

const usage = `usage: nrbrowse [--plain] [--width N] [--no-icons] [--nerd]
                [--images [kitty|sixel|iterm2|ueberzug|auto]] [code]

Interactive card browser. Select a card to render it.
Keys: type to search, ↑/↓ browse, enter print & quit, q quit,
      1 side · 2 type · 3 faction · 4 pack · 5 cost cap,
      p then 1-5 multi-select pickers for side/type/faction/pack/cost
        (type to narrow, space selects, enter applies);
        Ctrl/Alt+1..5 also open them where the terminal allows,
      o multi-select sort fields (each press: ↑ → ↓ → off),
      v toggle image previews.

Note: many terminals send indistinguishable control codes for Ctrl+digits
(Ctrl+3 = Esc); use Alt+1..5 there.

The filters always compile to one SQL query over the DuckDB cache; the
exact statement is shown under the panes.

Images are OFF by default (text previews everywhere); press v in the
browser to try them. The --images flag forces a graphics protocol (also
via NETRUNNER_IMAGE_PROTOCOL), bypassing detection — useful inside
tmux/ssh — and enables art for one-shot "nrbrowse <code>" output.
ueberzugpp uses an overlay daemon; in auto mode it is only tried when no
inline protocol is detected.
If a code is given, render that card and exit.`

func main() {
	defer image.Shutdown()
	parsed, err := parseArgs(os.Args[1:])
	if err != nil {
		fatal(err)
	}
	opts := parsed.opts

	if parsed.protocol != "" {
		if err := image.SetProtocolOverride(parsed.protocol); err != nil {
			fatal(err)
		}
	}

	db, err := carddb.Open()
	if err != nil {
		fatal(err)
	}
	defer db.Close()

	// Pack codes for the pack-cycling filter; empty on error (key no-op).
	packs, _ := carddb.Distinct(db, "pack_code")

	if parsed.renderTest != "" {
		parts := strings.SplitN(parsed.renderTest, "x", 2)
		w, err1 := strconv.Atoi(parts[0])
		h, err2 := strconv.Atoi(parts[1])
		if len(parts) != 2 || err1 != nil || err2 != nil || w < 20 || h < 6 {
			fatal(fmt.Sprintf("invalid --render-test frame %q", parsed.renderTest))
		}
		fmt.Print(renderTestFrame(w, h))
		return
	}

	if parsed.code != "" {
		c, err := carddb.ByCode(db, parsed.code)
		if err != nil {
			fatal(err)
		}
		if isTTY() {
			w := opts.Width
			if w == 0 {
				w = 80
			}
			if wantArt(parsed.protocol) {
				fmt.Println(renderCardWithArt(c, opts, w, 30))
			} else {
				fmt.Println(render.Card(c, opts))
			}
			return
		}
		fmt.Println(render.Card(c, opts))
		return
	}

	cards, err := carddb.Run(db, carddb.Query{})
	if err != nil {
		fatal(err)
	}
	if !isTTY() {
		for _, c := range cards {
			fmt.Println(render.ListRow(c))
		}
		return
	}

	p := tea.NewProgram(newModel(db, opts, packs), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fatal(err)
	}
}

func isProtocolName(s string) bool {
	switch strings.ToLower(s) {
	case "kitty", "sixel", "iterm2", "iterm", "auto", "halfblocks", "none", "off", "ueberzug", "ueberzugpp":
		return true
	}
	return false
}

func isTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func fatal(v any) {
	fmt.Fprintln(os.Stderr, v)
	os.Exit(1)
}

// sampleCards provides stub data for --render-test and unit tests.
func sampleCards() []carddb.Card {
	return []carddb.Card{
		{Code: "01001", Title: "The Professor: Refactoring the Human", Side: "runner",
			Faction: "shaper", Type: "identity",
			Text:           "Begin each game with one copy of up to 3 different programs from outside the game (in your deck).",
			InfluenceLimit: sqlNull(45), MinimumDeckSize: sqlNull(45), PackCode: "core"},
		{Code: "01041", Title: "Bank Job", Side: "runner", Faction: "neutral-runner",
			Type: "event", Cost: sqlNull(2),
			Text:     "Run any server. The first time you access a Corp card each run, instead of accessing it you may place it on this event.",
			PackCode: "core"},
	}
}

func sqlNull(v int64) sql.NullInt64 {
	return sql.NullInt64{Valid: true, Int64: v}
}

// renderTestFrame builds a static UI frame at WxH with sample data.
func renderTestFrame(w, h int) string {
	m := newModel(nil, render.Default(), nil)
	m.width, m.height = w, h
	lw, _ := m.sheetGeometry()
	m.list.SetSize(lw, h-4)
	items := make([]list.Item, len(sampleCards()))
	for i, c := range sampleCards() {
		items[i] = item{card: c}
	}
	_ = m.list.SetItems(items)
	m.updatePreview()
	return m.View()
}
