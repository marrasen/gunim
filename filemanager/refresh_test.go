package filemanager

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim/input"
)

// F5 lists the folder again, and shows a file made there behind the
// file manager's back.
func TestF5ShowsAFileMadeSince(t *testing.T) {
	h := newHarness(t, "old.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	if err := os.WriteFile(filepath.Join(h.dir, "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.w.Input(input.KeyPress{Key: input.KeyF5})
	h.until("F5 shows the new file", func() bool { return slices.Equal(h.shown(), []string{"new.txt", "old.txt"}) })
}

// F5 lists a pane's folder again too, as a program's window hosts it.
func TestF5InAPaneShowsAFileMadeSince(t *testing.T) {
	p := newPaneWorld(t, "old.txt")
	s := newPaneScreen(t, p.pw)
	p.pw.Attach(s.w.Client(), "slot")
	shown := func() int {
		b := s.browser()
		if b == nil || b.listing.cur == nil {
			return 0
		}
		return b.listing.cur.grid.Rows()
	}
	s.until("the pane shows the file", func() bool { return shown() == 1 })
	if err := s.w.Client().Patch("pane1/browser", focusPane{}); err != nil {
		t.Fatal(err)
	}
	s.frames(2)
	if err := os.WriteFile(filepath.Join(p.dir, "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	s.w.Input(input.KeyPress{Key: input.KeyF5, Time: time.Now()})
	s.until("F5 shows the new file", func() bool { return shown() == 2 })
}

// Ctrl+R lists the folder again, as F5 does.
func TestCtrlRShowsAFileMadeSince(t *testing.T) {
	h := newHarness(t, "old.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	if err := os.WriteFile(filepath.Join(h.dir, "new.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.w.Input(input.KeyPress{Key: input.KeyR, Mods: input.ModControl})
	h.until("Ctrl+R shows the new file", func() bool { return slices.Equal(h.shown(), []string{"new.txt", "old.txt"}) })
}
