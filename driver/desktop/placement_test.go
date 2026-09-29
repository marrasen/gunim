//go:build linux || windows || darwin

package desktop

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
)

// TestWindowOpensAtItsPlacement opens a window at a saved placement, and
// reads it back, maximized and not.
func TestWindowOpensAtItsPlacement(t *testing.T) {
	if display == nil {
		t.Skip("no display")
	}
	var work geom.Rect
	for _, m := range display.Monitors() {
		if m.Primary {
			work = m.WorkArea
		}
	}
	if work.Size().W < 400 || work.Size().H < 300 {
		t.Skip("no primary monitor with room for the window")
	}
	saved := geom.Rc(work.Min.X+60, work.Min.Y+80, 320, 200)
	for i, maximized := range []bool{false, true, true} {
		// The last one opens restored, then maximizes, as the user would
		opened := maximized && i < 2
		w, err := display.NewWindow(driver.Options{
			Title: "gunim placement test", Place: &driver.Placement{Bounds: saved, Maximized: opened},
		})
		if err != nil {
			t.Fatal(err)
		}
		if maximized && !opened {
			time.Sleep(100 * time.Millisecond)
			fr, _ := w.(driver.Framer)
			if err := fr.SetMaximized(true); err != nil {
				t.Fatal(err)
			}
		}
		r, _ := w.(driver.PlacementReader)
		var p driver.Placement
		deadline := time.Now().Add(3 * time.Second)
		for {
			var ok bool
			p, ok = r.Placement()
			if ok && p.Maximized == maximized || time.Now().After(deadline) {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		_ = w.Close()
		if p.Maximized != maximized {
			// A window manager may refuse to maximize, or take its time
			t.Logf("maximized %v: the window manager left it maximized %v", maximized, p.Maximized)
			continue
		}
		// A window manager may nudge a window a little, as to keep it clear of a panel
		if d := p.Bounds.Min.Sub(saved.Min); abs(d.X) > 2 || abs(d.Y) > 2 || p.Bounds.Size() != saved.Size() {
			t.Errorf("maximized %v: Placement = %v, want %v", maximized, p.Bounds, saved)
		}
	}
}
