package main

import (
	"database/sql"
	"flag"
	"fmt"
	"os"
	"strings"

	"boardy/netrunner/internal/carddb"
	"boardy/netrunner/internal/render"

	"github.com/charmbracelet/lipgloss"
)

const usage = `usage: nrrender [--plain] [--width N] [--no-icons] <card>...

Each <card> is one of:
  06081                        a card code, looked up in the local cache
  /path/to/card.json           a single-card JSON document
  /path/to/pack.json:06081     pick a card from a pack JSON file

Multiple cards are rendered in sequence.`

func main() {
	plain := flag.Bool("plain", false, "disable ANSI colors")
	width := flag.Int("width", 0, "render width (0 = terminal width)")
	noIcons := flag.Bool("no-icons", false, "replace emoji icons with ASCII glyphs")
	flag.Usage = func() { fmt.Fprintln(os.Stderr, usage) }
	flag.Parse()

	opts := render.Default()
	if *plain || lipgloss.ColorProfile() == 0 {
		opts.Plain = true
	}
	opts.Width = *width
	opts.Icons = !*noIcons

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}

	var db *sql.DB
	defer func() {
		if db != nil {
			db.Close()
		}
	}()

	for i, arg := range args {
		c, err := loadCard(arg, &db)
		if err != nil {
			fatal(err)
		}
		if i > 0 {
			fmt.Println()
		}
		fmt.Print(render.Card(c, opts))
	}
}

func loadCard(arg string, dbp **sql.DB) (carddb.Card, error) {
	if isFileArg(arg) {
		return carddb.FromFile(arg)
	}
	if *dbp == nil {
		d, err := carddb.Open()
		if err != nil {
			return carddb.Card{}, err
		}
		*dbp = d
	}
	return carddb.ByCode(*dbp, arg)
}

func isFileArg(arg string) bool {
	base := arg
	if i := strings.LastIndex(arg, ":"); i >= 0 {
		base = arg[:i]
	}
	if strings.ContainsRune(base, os.PathSeparator) || strings.HasSuffix(base, ".json") {
		return true
	}
	info, err := os.Stat(base)
	return err == nil && !info.IsDir() && !isAllDigits(arg)
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "nrrender:", err)
	os.Exit(1)
}
