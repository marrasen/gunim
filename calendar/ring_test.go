package calendar

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// ringShows returns how strongly the last frame of w drew the ring round a whole view from the window's corner, from
// 0 for none to 1 for the accent colour in full.
func ringShows(w *gunim.Window) float32 {
	accent := widget.Accent.Get(nil)
	for _, op := range w.Offscreen().Ops() {
		if r, ok := op.(*paint.RRectOp); ok && r.Stroke.Width == 2 && r.Rect.Min == geom.Pt(1, 1) {
			c := r.Stroke.Color
			if c.R == accent.R && c.G == accent.G && c.B == accent.B {
				return float32(c.A) / float32(accent.A)
			}
		}
	}
	return 0
}

// The ring grows in as Tab brings the keyboard, frame by frame, and fades as a click puts it away.
func TestTheViewsShowARingWhileTabHasThemTheKeyboard(t *testing.T) {
	views := map[string]gunim.Node{"days": newWeek(), "month": NewMonth(monday), "small month": NewMiniMonth(monday)}
	for name, n := range views {
		w, run, _ := stage(t, n)
		if r := ringShows(w); r != 0 {
			t.Fatalf("%s: a ring shows at %v before anything has the keyboard", name, r)
		}
		w.Input(input.KeyPress{Key: input.KeyTab})
		last := float32(0)
		for f := range 30 {
			run(1)
			r := ringShows(w)
			if r < last-0.02 || (f == 1 && (r <= 0 || r >= 1)) {
				t.Fatalf("%s, frame %d after Tab: the ring went from %v to %v", name, f, last, r)
			}
			last = r
		}
		if last < 0.98 {
			t.Fatalf("%s: Tab gave it the keyboard and the ring shows at %v", name, last)
		}
		// A click puts the rings away.
		at := geom.Pt(700, 590)
		w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
		w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary})
		for f := range 40 {
			run(1)
			r := ringShows(w)
			if r > last+0.02 || (f == 1 && r >= 1) {
				t.Fatalf("%s, frame %d after a click: the ring went from %v to %v", name, f, last, r)
			}
			last = r
		}
		if last > 0 {
			t.Fatalf("%s: a ring shows at %v after a click", name, last)
		}
	}
}
