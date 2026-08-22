// Package image renders card images in the terminal using graphics
// protocols (kitty, sixel, iTerm2) with automatic fallback, backed by
// a local on-disk cache of card artwork.
package image

import (
	"fmt"
	"os"
	"path/filepath"

	termimg "github.com/blacktop/go-termimg"
)

// Dir returns the card image cache directory.
func Dir() string {
	if d := os.Getenv("NETRUNNER_IMAGES"); d != "" {
		return d
	}
	return filepath.Join("data", "card-images")
}

// Path is the cached image path for a card code ("" if absent).
func Path(code string) string {
	for _, ext := range []string{".jpg", ".png"} {
		p := filepath.Join(Dir(), code+ext)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// Supported reports whether the terminal can show real images.
func Supported() bool {
	return termimg.DetectProtocol() != termimg.Halfblocks
}

// Card renders the card's cached image scaled into width×height cells.
// Returns "" when unsupported or not cached.
func Card(code string, width, height int) string {
	p := Path(code)
	if p == "" || !Supported() || width < 4 || height < 4 {
		return ""
	}
	w, err := termimg.NewImageWidgetFromFile(p)
	if err != nil {
		return ""
	}
	w.SetSize(width, height).SetProtocol(termimg.Auto)
	s, err := w.Render()
	if err != nil {
		return fmt.Sprintf("(image error: %v)", err)
	}
	return s
}
