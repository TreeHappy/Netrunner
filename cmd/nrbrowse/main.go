package main

import (
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
	o := m.opts
	if o.Width > m.sheetWidth() {
		o.Width = m.sheetWidth()
	}
	m.preview = render.Card(sel.card, o)
	if !m.imageOn || !image.Supported() {
		return nil
	}
	code := sel.card.Code
	if payload, _, _ := image.Card(code, m.artWidth(), m.height-6); payload != "" {
		return nil
	}
	if image.Path(code) == "" && !m.pending[code] && !m.failed[code] {
		m.pending[code] = true
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

func (m model) sheetArtWidths() (sheet, art int) {
	total := m.width - m.listWidth() - 6
	sheet = total * 58 / 100
	art = total - sheet
	if sheet < 22 {
		sheet = 22
	}
	if art < 12 {
		art = 12
	}
	return sheet, art
}

func (m model) sheetWidth() int {
	s, _ := m.sheetArtWidths()
	return s - 2
}

func (m model) artWidth() int {
	_, a := m.sheetArtWidths()
	return a - 2
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
	filterLine := lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(
		"[1] %s  [2] %s  [3] %s  · %s · / search, enter print+quit, v images, q quit",
		label("side", m.query.Side),
		label("type", m.query.Type),
		label("faction", m.query.Faction),
		m.status,
	))
	bh := m.height - 3
	if bh < 6 {
		bh = 6
	}
	lw := m.listWidth()
	sw, aw := m.sheetArtWidths()

	left := paneStyle(lw, bh, true).Render(paneTitle("browser") + "\n" + m.list.View())
	mid := paneStyle(sw, bh, false).Render(paneTitle("card") + "\n" + m.preview)
	right := paneStyle(aw, bh, false).Render(m.artBlock(aw-2, bh-2))
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, mid, right)
	return body + "\n" + filterLine
}

// artBlock renders the artwork pane: cropped image when available, a
// spinner while fetching, faint placeholders otherwise.
func (m model) artBlock(w, h int) string {
	title := paneTitle("artwork")
	sel, ok := m.list.SelectedItem().(item)
	if !ok {
		return title
	}
	code := sel.card.Code
	if payload, iw, ih := image.Card(code, w, h); payload != "" && m.imageOn {
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
