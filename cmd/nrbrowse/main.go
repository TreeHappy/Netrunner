package main

import (
	"database/sql"
	"fmt"
	"os"
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
	width    int
	height   int
	opts     render.Options
	status   string
	quitting bool
	loaded   bool
}

var (
	sides    = []string{"", "corp", "runner"}
	factions = []string{"", "anarch", "criminal", "shaper", "haas-bioroid", "jinteki", "nbn", "weyland-consortium", "neutral-runner", "neutral-corp"}
	types    = []string{"", "agenda", "asset", "event", "hardware", "ice", "identity", "operation", "program", "resource", "upgrade"}
)

func newModel(db carddb.DB, opts render.Options) model {
	delegate := list.NewDefaultDelegate()
	l := list.New([]list.Item{}, delegate, 40, 24)
	l.Title = "Netrunner cards"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.FilterInput.Placeholder = "search title/code…"
	l.Styles.Title = l.Styles.Title.Foreground(lipgloss.Color("#dddddd")).Background(lipgloss.Color("#333355"))
	sp := spinner.New(spinner.WithSpinner(spinner.MiniDot))
	return model{
		db: db, list: l, spinner: sp, opts: opts,
		pending: map[string]bool{}, failed: map[string]bool{},
		imageOn: image.Supported(), status: imageStatusDefault(),
	}
}

func imageStatusDefault() string {
	if image.Supported() {
		return "v toggles images"
	}
	return "terminal has no graphics protocol; text previews"
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
	return m.updatePreview()
}

func (m *model) updatePreview() tea.Cmd {
	sel, ok := m.list.SelectedItem().(item)
	if !ok {
		m.preview = ""
		return nil
	}
	sw := m.sheetWidth()
	bandW := sw - 4 // inside the sheet's border + padding
	var cmd tea.Cmd

	// Try to render artwork; its cell size defines the reserved band.
	payload, iw, ih := image.Card(sel.card.Code, bandW, m.height-6)
	if payload != "" && m.imageOn && ih > 0 {
		m.bandW, m.bandH = iw, ih
	} else {
		// Placeholder band while fetching / when missing.
		h := sw * 2 / 5
		if h < 4 {
			h = 4
		}
		if h > 14 {
			h = 14
		}
		m.bandW, m.bandH = bandW, h
		if m.imageOn && image.Supported() && image.Path(sel.card.Code) == "" &&
			!m.pending[sel.card.Code] && !m.failed[sel.card.Code] {
			code := sel.card.Code
			m.pending[code] = true
			cmd = tea.Batch(fetchArtCmd(code), m.spinner.Tick)
		}
	}

	o := m.opts
	o.Width = sw // Card() output spans exactly this many columns
	if m.imageOn {
		o.ArtBand = &render.ArtBand{W: m.bandW, H: m.bandH}
		o.ArtNote = ""
		if payload != "" {
			m.preview = render.SpliceArt(render.Card(sel.card, o), payload, o.Width)
			return cmd
		}
		if image.Supported() {
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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		h := msg.Height - 4
		m.list.SetSize(m.listWidth(), h)
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
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "q":
			if m.list.FilterState() != list.Filtering {
				m.quitting = true
				return m, tea.Quit
			}
		case "esc":
			if m.list.FilterState() == list.Filtering || m.list.FilterState() == list.FilterApplied {
				break
			}
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if sel, ok := m.list.SelectedItem().(item); ok {
				fmt.Println(render.Card(sel.card, m.opts))
				m.quitting = true
				return m, tea.Quit
			}
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
			m.imageOn = !m.imageOn
			if m.imageOn && !image.Supported() {
				m.status = "terminal has no graphics protocol; using text"
			} else {
				m.status = "image preview " + map[bool]string{true: "on", false: "off"}[m.imageOn]
			}
			return m, m.updatePreview()
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
	filterLine := lipgloss.NewStyle().Faint(true).Render(trunc(
		fmt.Sprintf("[1] %s  [2] %s  [3] %s  · %s · / search, enter print+quit, v images, q quit",
			label("side", m.query.Side),
			label("type", m.query.Type),
			label("faction", m.query.Faction),
			m.status), m.width))
	bh := m.height - 3
	if bh < 6 {
		bh = 6
	}
	lw, sw := m.sheetGeometry()

	left := paneStyle(lw, bh, true).Render(paneTitle("browser") + "\n" + m.list.View())
	mid := paneStyle(sw, bh, false).Render(paneTitle("card") + "\n" + m.preview)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, mid)
	return body + "\n" + filterLine
}

// trunc cuts a plain (ANSI-light) string to w printable cells.
func trunc(s string, w int) string {
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	return string(r[:max(0, w-1)]) + "…"
}

const usage = `usage: nrbrowse [--plain] [--width N] [--no-icons] [--nerd] [code]

Interactive card browser. Select a card to render it.
Keys: type to search, ↑/↓ browse, enter print & quit, q quit,
      v toggle image previews, 1 cycle side, 2 cycle type, 3 cycle faction.

If a code is given, render that card and exit.`

func main() {
	opts := render.Default()
	code := ""
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
		case "--render-test":
			i++
			if i >= len(args) {
				fatal(usage)
			}
			parts := strings.SplitN(args[i], "x", 2)
			if len(parts) != 2 {
				fatal(usage)
			}
			w, err1 := strconv.Atoi(parts[0])
			h, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil || w < 20 || h < 6 {
				fatal(usage)
			}
			fmt.Print(renderTestFrame(w, h))
			return
		default:
			code = args[i]
		}
	}

	db, err := carddb.Open()
	if err != nil {
		fatal(err)
	}
	defer db.Close()

	if code != "" {
		c, err := carddb.ByCode(db, code)
		if err != nil {
			fatal(err)
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

	p := tea.NewProgram(newModel(db, opts), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fatal(err)
	}
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
	m := newModel(nil, render.Default())
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
