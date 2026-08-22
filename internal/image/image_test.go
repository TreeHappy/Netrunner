package image

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEnsureArtCropsScan(t *testing.T) {
	src := repoScanDir()
	var code string
	for _, f := range []string{"01001.jpg", "01001.png"} {
		if _, err := os.Stat(filepath.Join(src, f)); err == nil {
			code = "01001"
			break
		}
	}
	if code == "" {
		t.Skip("no cached scan available")
	}
	t.Setenv("NETRUNNER_IMAGES", src)
	dir := t.TempDir()
	t.Setenv("NETRUNNER_ART", dir)

	p := EnsureArt(code)
	if p == "" {
		if _, err := execLookPath(); err != nil {
			t.Skip("imagemagick not installed")
		}
		t.Fatal("EnsureArt returned no path despite scan and magick")
	}
	if filepath.Dir(p) != dir {
		t.Fatalf("art written outside art dir: %s", p)
	}
	if fi, err := os.Stat(p); err != nil || fi.Size() == 0 {
		t.Fatalf("cropped art missing/empty: %v", err)
	}
}

func TestCardPrefersArt(t *testing.T) {
	src := repoScanDir()
	if _, err := os.Stat(filepath.Join(src, "01001.jpg")); err != nil {
		t.Skip("no cached scan available")
	}
	t.Setenv("NETRUNNER_IMAGES", src)
	dir := t.TempDir()
	t.Setenv("NETRUNNER_ART", dir)
	if EnsureArt("01001") == "" {
		t.Skip("crop unavailable (magick missing)")
	}
	if !Supported() {
		t.Skip("terminal has no graphics protocol")
	}
	payload, w, h := Card("01001", 40, 30)
	if payload == "" || w <= 0 || h <= 0 {
		t.Fatal("expected renderable artwork from cropped art")
	}
}

func execLookPath() (string, error) {
	return exec.LookPath("magick")
}

// repoScanDir finds the repo's data/card-images dir from the package dir.
func repoScanDir() string {
	for dir, i := ".", 0; i < 4; i++ {
		p := filepath.Join(dir, "data", "card-images")
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
		dir = filepath.Join("..", dir)
	}
	return filepath.Join("..", "..", "data", "card-images")
}
