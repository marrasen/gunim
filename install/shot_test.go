package install

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/marrasen/gunim"
)

// TestShots opens the installer's window on the display and writes
// pictures of its pages, as they arrive and as they settle, to the
// folder GUNIM_INSTALL_SHOTS names. It is for looking at, and skipped
// without that folder.
func TestShots(t *testing.T) {
	dir := os.Getenv("GUNIM_INSTALL_SHOTS")
	if dir == "" {
		t.Skip("set GUNIM_INSTALL_SHOTS to a folder to write pictures of the installer to")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	program := filepath.Join(t.TempDir(), "studio")
	if err := os.WriteFile(program, make([]byte, 18<<20), 0o755); err != nil {
		t.Fatal(err)
	}
	a := App{
		Name: "Marras Mastering Studio", Version: "v0.2.0", Publisher: "Marcus Johansson",
		Description: "Masters an album, an EP or a single, through chains of VST3 plugins.",
		Icon:        testIcon(),
		FileTypes:   []FileType{{Name: "Mastering album", Exts: []string{".mastering"}, Default: true}},
		Updates:     GitHub{Repo: "marrasen/mastering-studio"},
		Data:        []string{filepath.Join(home, ".config", "studio")},
	}
	if os.Getenv("GUNIM_INSTALL_NOICON") != "" {
		a.Icon = nil
	}
	s, err := newSession(a, program, false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	err = gunim.Main(ctx, func(app *gunim.App) error {
		r, err := openWindow(app, s)
		if err != nil {
			return err
		}
		go func() { _ = r.serve(ctx) }()
		shot := func(name string, after time.Duration) {
			time.Sleep(after)
			img, err := r.c.Shot(ctx)
			if err != nil {
				t.Error(err)
				return
			}
			f, err := os.Create(filepath.Join(dir, name+".png"))
			if err != nil {
				t.Error(err)
				return
			}
			_ = png.Encode(f, img)
			_ = f.Close()
		}
		do := func(in gunim.Intent) { r.events <- func() { r.handle(ctx, in) } }
		shot("1-arriving", 150*time.Millisecond)
		shot("2-welcome", 1200*time.Millisecond)
		do(started{Picks: map[string]bool{PickDesktop: true}})
		shot("3-working", 500*time.Millisecond)
		shot("4-working-late", 800*time.Millisecond)
		shot("5-done-ring", 500*time.Millisecond)
		shot("6-done-burst", 450*time.Millisecond)
		shot("7-done", 1600*time.Millisecond)
		r.events <- func() {
			r.failed(os.ErrPermission)
		}
		shot("8-failed", 200*time.Millisecond)
		r.events <- func() {
			r.s.Mode = Remove
			r.sc.Page, r.sc.Mode = pageRemove, Remove
			r.show()
		}
		shot("9-remove", 900*time.Millisecond)
		do(removed{Data: true})
		shot("10-removing", 500*time.Millisecond)
		shot("11-removed", 2200*time.Millisecond)
		r.events <- func() {
			r.s.Mode, r.sc.Mode, r.sc.Removing, r.sc.Quit = Upgrade, Upgrade, false, true
			r.s.App.Quit = func(context.Context) error { return nil }
			r.running = func() []int { return []int{1} }
			r.handle(ctx, started{})
		}
		shot("12-running", 900*time.Millisecond)
		do(quitThem{})
		shot("13-closing", 900*time.Millisecond)
		r.c.Close()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// testIcon is an icon to look at: a rounded tile in a magenta to violet
// gradient with a white disc in it.
func testIcon() image.Image {
	const n = 256
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		for x := range n {
			// Rounded corners.
			cx, cy := float64(x)-n/2+0.5, float64(y)-n/2+0.5
			ex, ey := max(0, abs(cx)-(n/2-48)), max(0, abs(cy)-(n/2-48))
			if ex*ex+ey*ey > 48*48 {
				continue
			}
			t := float64(y) / n
			c := color.NRGBA{R: uint8(230 - 120*t), G: uint8(60 + 20*t), B: uint8(170 + 70*t), A: 0xff}
			if cx*cx+cy*cy < 60*60 && cx*cx+cy*cy > 36*36 {
				c = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
