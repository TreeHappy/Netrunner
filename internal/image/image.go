// Package image renders card images in the terminal using graphics
// protocols (kitty, sixel, iTerm2) with automatic fallback, backed by
// a local on-disk cache of card artwork.
//
// Artwork is fetched from NetrunnerDB's public card-image CDN and cached
// under the user's cache directory. It is third-party copyrighted material
// (FFG / Wizards of the Coast / Null Signal Games); this package only
// caches it locally for personal use and never distributes it.
package image

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	termimg "github.com/blacktop/go-termimg"
)

// Dir returns the original card-scan cache directory. Defaults to the XDG
// user cache dir (~/.cache on Linux), overridable with NETRUNNER_IMAGES.
func Dir() string {
	if d := os.Getenv("NETRUNNER_IMAGES"); d != "" {
		return d
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "netrunner", "card-images")
}

// ArtDir returns the cropped-artwork cache directory (image-only crops of
// the full scans kept in Dir).
func ArtDir() string {
	if d := os.Getenv("NETRUNNER_ART"); d != "" {
		return d
	}
	return filepath.Join(filepath.Dir(Dir()), "card-art")
}

// artBox is the artwork band as percentages of the card scan
// (x, y, w, h): title band above, text box below.
var artBox = [4]float64{4, 11, 92, 52}

// Path is the cached full-scan path for a card code ("" if absent).
func Path(code string) string {
	for _, ext := range []string{".jpg", ".png"} {
		p := filepath.Join(Dir(), code+ext)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ArtPath is the cropped artwork path for a card code ("" if absent).
func ArtPath(code string) string {
	p := filepath.Join(ArtDir(), code+".jpg")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

// EnsureArt crops the card's cached scan down to the artwork band using
// ImageMagick, writing into ArtDir. Originals are never modified. Returns
// the artwork path ("" when the source scan or magick is missing).
func EnsureArt(code string) string {
	if p := ArtPath(code); p != "" {
		return p
	}
	src := Path(code)
	if src == "" {
		return ""
	}
	magick, err := exec.LookPath("magick")
	if err != nil {
		if magick, err = exec.LookPath("convert"); err != nil {
			return ""
		}
	}
	if err := os.MkdirAll(ArtDir(), 0o755); err != nil {
		return ""
	}
	dst := filepath.Join(ArtDir(), code+".jpg")
	box := artBox
	geo := fmt.Sprintf("%g%%x%g%%+%g%%+%g%%", box[2], box[3], box[0], box[1])
	cmd := exec.Command(magick, src, "-crop", geo, "+repage", "-quality", "90", dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		os.Remove(dst)
		_ = out
		return ""
	}
	if ArtPath(code) == "" {
		return ""
	}
	return dst
}

// Supported reports whether the terminal can show real images.
func Supported() bool {
	return termimg.DetectProtocol() != termimg.Halfblocks
}

type rendered struct {
	payload string
	w, h    int
}

var (
	renderCache sync.Map // "code|maxW|maxH" -> rendered
	fetching    sync.Map // code -> bool
)

// Card renders the card's artwork (cropped art when available, else the
// full scan) scaled to fit within width×height cells while preserving
// aspect ratio. It returns the raw protocol payload (emit unstyled) and
// the number of cells it actually occupies. Returns empty payload when
// unsupported or not cached; renders are cached per (card, size).
func Card(code string, width, height int) (string, int, int) {
	if !Supported() || width < 4 || height < 4 {
		return "", 0, 0
	}
	p := ArtPath(code)
	if p == "" {
		p = Path(code)
	}
	if p == "" {
		return "", 0, 0
	}
	key := fmt.Sprintf("%s|%d|%d", p, width, height)
	if v, ok := renderCache.Load(key); ok {
		r := v.(rendered)
		return r.payload, r.w, r.h
	}
	widget, err := termimg.NewImageWidgetFromFile(p)
	if err != nil {
		return "", 0, 0
	}
	widget.SetSizeWithCorrection(width, height).SetProtocol(termimg.Auto)
	s, err := widget.Render()
	if err != nil {
		return "", 0, 0
	}
	w, h := widget.GetSize()
	r := rendered{s, w, h}
	renderCache.Store(key, r)
	return r.payload, r.w, r.h
}

// Fetch downloads artwork for a card code into the cache if not present.
// Safe to call concurrently for the same code.
func Fetch(code string) error {
	if Path(code) != "" {
		return nil
	}
	if _, busy := fetching.LoadOrStore(code, true); busy {
		return nil
	}
	defer fetching.Delete(code)

	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return err
	}
	url := fmt.Sprintf("https://card-images.netrunnerdb.com/v2/large/%s.jpg", code)
	dst := filepath.Join(Dir(), code+".jpg")
	tmp := dst + ".part"

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: HTTP %d", code, resp.StatusCode)
	}
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, resp.Body)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		os.Remove(tmp)
		if err == nil {
			err = closeErr
		}
		return err
	}
	return os.Rename(tmp, dst)
}

// FetchWithArt downloads the scan and then crops artwork. Returns the
// cropped art path ("" if cropping unavailable) and any fetch error.
func FetchWithArt(code string) (string, error) {
	err := Fetch(code)
	if err != nil {
		return "", err
	}
	return EnsureArt(code), nil
}

// Fetching reports whether a fetch for code is in flight.
func Fetching(code string) bool {
	_, ok := fetching.Load(code)
	return ok
}
