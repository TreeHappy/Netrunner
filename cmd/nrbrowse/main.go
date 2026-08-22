package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"boardy/netrunner/internal/carddb"
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

type filters struct {
	side    string // "", "corp", "runner"
	faction string // "" = all
	typ     string // "" = all
}

type model struct {
	all      []carddb.Card
	list     list.Model
	preview  string
	filters  filters
	width    int
	height   int
	opts     render.Options
	status   string
	quitting bool
}

var (
	sides    = []string{"", "corp", "runner"}
	types    = []string{"", "agenda", "asset", "event", "hardware", "ice", "identity", "operation", "program", "resource", "upgrade"}
	factions = []string{"", "anarch", "criminal", "shaper", "haas-bioroid", "jinteki", "nbn", "weyland-consortium", "neutral-runner", "neutral-corp"}
)

func newModel(cards []carddb.Card, opts render.Options) model {
	delegate := list.NewDefaultDelegate()
	l := list.New([]list.Item{}, delegate, 40, 24)
	l.Title = "Netrunner cards"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.FilterInput.Placeholder = "search title/code…"
	l.Styles.Title = l.Styles.Title.Foreground(lipgloss.Color("#dddddd")).Background(lipgloss.Color("#333355"))
	return model{all: cards, list: l, opts: opts}
}

func (m model) filtered() []carddb.Card {
	var out []carddb.Card
	for _, c := range m.all {
		if m.filters.side != "" && c.Side != m.filters.side {
			continue
		}
		if m.filters.faction != "" && c.Faction != m.filters.faction {
			continue
		}
		if m.filters.typ != "" && c.Type != m.filters.typ {
			continue
		}
		out = append(out, c)
	}
	return out
}

func (m *model) refresh() {
	cards := m.filtered()
	items := make([]list.Item, len(cards))
	for i, c := range cards {
		items[i] = item{card: c}
	}
	m.list.SetItems(items)
	m.status = fmt.Sprintf("%d cards", len(items))
	m.updatePreview()
}

func (m *model) updatePreview() {
	sel, ok := m.list.SelectedItem().(item)
	if !ok {
		m.preview = ""
		return
	}
	o := m.opts
	if o.Width > m.previewWidth() {
		o.Width = m.previewWidth()
	}
	m.preview = render.Card(sel.card, o)
}

func (m model) previewWidth() int {
	w := m.width - m.listWidth()
	if w < 30 {
		w = 30
	}
	return w - 2
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
		m.refresh()
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
			cycle(&m.filters.side, sides)
			m.refresh()
			return m, nil
		case "2":
			cycle(&m.filters.typ, types)
			m.refresh()
			return m, nil
		case "3":
			cycle(&m.filters.faction, factions)
			m.refresh()
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

func (m model) View() string {
	if m.quitting {
		return ""
	}
	filterLine := lipgloss.NewStyle().Faint(true).Render(fmt.Sprintf(
		"[1] %s  [2] %s  [3] %s  · %s · / search, enter print+quit, q quit",
		label("side", m.filters.side),
		label("type", m.filters.typ),
		label("faction", m.filters.faction),
		m.status,
	))
	left := lipgloss.NewStyle().Width(m.listWidth()).Render(m.list.View())
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, m.preview)
	return filterLine + "\n" + body + "\n" + filterLine
}

const usage = `usage: nrbrowse [--plain] [--width N] [--no-icons] [code]

Interactive card browser. Select a card to render it.
Keys: type to search, ↑/↓ browse, enter print & quit, q quit,
      1 cycle side, 2 cycle type, 3 cycle faction.

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

	cards, err := carddb.List(db)
	if err != nil {
		fatal(err)
	}
	if !isTTY() {
		for _, c := range cards {
			fmt.Println(render.ListRow(c))
		}
		return
	}

	p := tea.NewProgram(newModel(cards, opts), tea.WithAltScreen())
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
