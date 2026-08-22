// ueberzug.go implements the ueberzugpp backend: an overlay-image daemon
// (X11/Wayland child windows) driven by JSON commands, used as a fallback
// rendering path alongside the inline graphics protocols (kitty/sixel/
// iTerm2). Unlike inline payloads, ueberzug placements live outside the
// terminal buffer, so the TUI reserves blank cell space and reports the
// band's absolute position via ApplyUeberzug.
package image

import (
	stdimage "image"
	_ "image/jpeg"
	_ "image/png"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const ueberzugID = "netrunner"

// Placement describes a pending overlay image sized in terminal cells.
type Placement struct {
	Code string
	Path string
	W, H int
}

type uzCommand struct {
	Action            string `json:"action"`
	Identifier        string `json:"identifier,omitempty"`
	Path              string `json:"path,omitempty"`
	X                 int    `json:"x,omitempty"`
	Y                 int    `json:"y,omitempty"`
	MaxWidth          int    `json:"max_width,omitempty"`
	MaxHeight         int    `json:"max_height,omitempty"`
	SynchronouslyDraw bool   `json:"synchronously_draw,omitempty"`
}

var (
	uzMu      sync.Mutex
	uzProc    *exec.Cmd
	uzSock    string
	uzPending Placement // set by cardUeberzug, consumed by ApplyUeberzug
	dimCache  sync.Map  // path -> [2]int pixel dims
)

// ueberzugPossible reports whether a ueberzugpp backend could be used:
// either an existing daemon socket is advertised via UEBERZUGPP_SOCKET or
// the binary is installed. Does not spawn anything.
func ueberzugPossible() bool {
	if os.Getenv("UEBERZUGPP_SOCKET") != "" {
		return true
	}
	_, err := exec.LookPath("ueberzugpp")
	return err == nil
}

// imageSize returns the pixel dimensions of a cached image file.
func imageSize(path string) (w, h int, ok bool) {
	if v, hit := dimCache.Load(path); hit {
		d := v.([2]int)
		return d[0], d[1], true
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()
	cfg, _, err := stdimage.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 {
		return 0, 0, false
	}
	dimCache.Store(path, [2]int{cfg.Width, cfg.Height})
	return cfg.Width, cfg.Height, true
}

// fitCells fits pixel dimensions into a maxW×maxH cell box assuming the
// common ~1:2 character-cell aspect ratio.
func fitCells(wpx, hpx, maxW, maxH int) (int, int) {
	if wpx <= 0 || hpx <= 0 || maxW < 4 || maxH < 3 {
		return 0, 0
	}
	aspect := float64(hpx) / float64(wpx)
	wc := maxW
	hc := int(float64(wc)*aspect*0.5 + 0.5)
	if hc > maxH {
		hc = maxH
		wc = int(float64(hc)*2/aspect + 0.5)
	}
	if wc > maxW {
		wc = maxW
	}
	if wc < 4 || hc < 3 {
		return 0, 0
	}
	return wc, hc
}

// cardUeberzug reserves cell space for the overlay image and records a
// pending placement. The payload is printable blanks spliced over the art
// band's sentinel row; the overlay itself is drawn by ApplyUeberzug once
// the view knows the band's absolute position.
func cardUeberzug(code, path string, width, height int) (string, int, int) {
	wpx, hpx, ok := imageSize(path)
	if !ok {
		return "", 0, 0
	}
	wc, hc := fitCells(wpx, hpx, width, height)
	if wc == 0 {
		return "", 0, 0
	}
	uzMu.Lock()
	uzPending = Placement{Code: code, Path: path, W: wc, H: hc}
	uzMu.Unlock()
	return strings.Repeat(" ", wc), wc, hc
}

// stdoutIsTTY guards against spawning the daemon during non-interactive
// runs (tests, --render-test, pipes).
func stdoutIsTTY() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ensureUeberzugDaemon returns the unix socket path of a running ueberzugpp
// daemon, spawning one if needed. An externally managed daemon can be
// advertised via UEBERZUGPP_SOCKET.
func ensureUeberzugDaemon() (string, error) {
	uzMu.Lock()
	defer uzMu.Unlock()
	if uzSock != "" {
		return uzSock, nil
	}
	if s := os.Getenv("UEBERZUGPP_SOCKET"); s != "" {
		uzSock = s
		return s, nil
	}
	if !stdoutIsTTY() {
		return "", errors.New("not a terminal")
	}
	bin, err := exec.LookPath("ueberzugpp")
	if err != nil {
		return "", fmt.Errorf("ueberzugpp not installed: %w", err)
	}
	tmp := os.Getenv("UEBERZUGPP_TMPDIR")
	if tmp == "" {
		tmp = os.TempDir()
	}
	pidFile := filepath.Join(tmp, "netrunner-ueberzugpp.pid")
	cmd := exec.Command(bin, "layer", "-s", "--pid-file", pidFile, "--no-stdin")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("spawn ueberzugpp layer: %w", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if b, err := os.ReadFile(pidFile); err == nil {
			sock := filepath.Join(tmp, fmt.Sprintf("ueberzugpp-%s.socket", strings.TrimSpace(string(b))))
			if _, err := os.Stat(sock); err == nil {
				uzProc, uzSock = cmd, sock
				return sock, nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = cmd.Process.Kill()
	_, _ = cmd.Process.Wait()
	os.Remove(pidFile)
	return "", errors.New("ueberzugpp daemon did not create its socket")
}

// uzSend delivers one command to the daemon over its unix socket.
func uzSend(c uzCommand) error {
	sock, err := ensureUeberzugDaemon()
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("unix", sock, time.Second)
	if err != nil {
		return fmt.Errorf("connect %s: %w", sock, err)
	}
	defer conn.Close()
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	conn.SetWriteDeadline(time.Now().Add(time.Second))
	_, err = conn.Write(append(b, '\n'))
	return err
}

// ApplyUeberzug draws the pending placement at absolute cell position
// (x, y). Safe to call on every view render; repeated adds replace the
// previous placement under the same identifier.
func ApplyUeberzug(x, y int) {
	uzMu.Lock()
	p := uzPending
	uzMu.Unlock()
	if p.Path == "" {
		return
	}
	err := uzSend(uzCommand{
		Action:            "add",
		Identifier:        ueberzugID,
		Path:              p.Path,
		X:                 x,
		Y:                 y,
		MaxWidth:          p.W,
		MaxHeight:         p.H,
		SynchronouslyDraw: true,
	})
	if err == nil {
		return
	}
	// Daemon may have died; drop cached socket/proc so the next call respawns.
	uzMu.Lock()
	uzSock = ""
	if uzProc != nil && uzProc.Process != nil {
		_ = uzProc.Process.Kill()
		uzProc = nil
	}
	uzMu.Unlock()
}

// HideArt removes any drawn overlay image (no-op without a daemon).
func HideArt() {
	uzMu.Lock()
	started := uzSock != "" || uzProc != nil
	uzMu.Unlock()
	if !started {
		return
	}
	_ = uzSend(uzCommand{Action: "remove", Identifier: ueberzugID})
}

// Shutdown hides the overlay and terminates a daemon we spawned. Call via
// defer from main.
func Shutdown() {
	HideArt()
	uzMu.Lock()
	defer uzMu.Unlock()
	if uzProc != nil && uzProc.Process != nil {
		_ = uzProc.Process.Kill()
		_, _ = uzProc.Process.Wait()
		uzProc = nil
	}
	uzSock = ""
}
