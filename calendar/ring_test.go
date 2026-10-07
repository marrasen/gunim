package calendar

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// ringDrawn reports whether the last frame of w drew the ring round a whole view from the window's corner.
func ringDrawn(w *gunim.Window) bool {
	for _, op := range w.Offscreen().Ops() {
		if r, ok := op.(*paint.RRectOp); ok && r.Stroke.Width == 2 && r.Rect.Min == geom.Pt(1, 1) &&
			r.Stroke.Color == widget.Accent.Get(nil) {
			return true
		}
	}
	return false
}

func TestTheViewsShowARingWhileTabHasThemTheKeyboard(t *testing.T) {
	views := map[string]gunim.Node{"days": newWeek(), "month": NewMonth(monday), "small month": NewMiniMonth(monday)}
	for name, n := range views {
		w, run, _ := stage(t, n)
		if ringDrawn(w) {
			t.Fatalf("%s: a ring shows before anything has the keyboard", name)
		}
		w.Input(input.KeyPress{Key: input.KeyTab})
		for f := range 3 {
			run(1)
			if !ringDrawn(w) {
				t.Fatalf("%s, frame %d: Tab gave it the keyboard and no ring shows", name, f)
			}
		}
		// A click puts the rings away.
		at := geom.Pt(700, 590)
		w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
		w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary})
		run(1)
		if ringDrawn(w) {
			t.Fatalf("%s: a ring shows after a click", name)
		}
	}
}
