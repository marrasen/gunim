package widget

import (
	"strconv"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// toastHost holds a stack of toasts at its top left, and shows a new
// one for each F2.
type toastHost struct {
	t     *Toasts
	shown int
}

func (h *toastHost) Children() []gunim.Node { return []gunim.Node{h.t} }

func (h *toastHost) Focusable() bool { return true }

func (h *toastHost) Handle(e input.Event, u *gunim.UI) bool {
	if k, ok := e.(input.KeyPress); ok && k.Key == input.KeyF2 {
		h.shown++
		h.t.Show(Toast{Title: "Notice " + strconv.Itoa(h.shown), Body: "Something happened."}, u)
		return true
	}
	return false
}

func (h *toastHost) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	k := kids.At(0)
	k.Layout(gunim.Constraints{Max: c.Max})
	k.Place(geom.Point{})
	return c.Max
}

func (h *toastHost) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}

func newToastStage(t *testing.T) (*gunim.Window, *toastHost, func(time.Duration)) {
	t.Helper()
	h := &toastHost{t: &Toasts{Life: 2 * time.Second}}
	w, run := stage(t, h)
	w.Input(input.PointerDown{Pos: geom.Pt(700, 500), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(700, 500), Time: time.Now()})
	run(1)
	wait := func(d time.Duration) { run(int(d / (time.Second / 60))) }
	return w, h, wait
}

func TestToastsStackNewestNearestTheCornerAndLeaveInTime(t *testing.T) {
	w, h, wait := newToastStage(t)
	w.Input(input.KeyPress{Key: input.KeyF2})
	wait(300 * time.Millisecond)
	w.Input(input.KeyPress{Key: input.KeyF2})
	wait(time.Second)
	if n := h.t.Len(); n != 2 {
		t.Fatalf("%d toasts showing, want 2", n)
	}
	first, second := h.t.cards[0], h.t.cards[1]
	if first.y.Value() >= second.y.Value() {
		t.Fatalf("the older toast is at %v, the newer at %v; want the newer below", first.y.Value(), second.y.Value())
	}
	// The first has had its two seconds; the second follows.
	wait(900 * time.Millisecond)
	if n := h.t.Len(); n != 1 {
		t.Fatalf("after the first toast's time, %d showing, want 1", n)
	}
	wait(time.Second)
	if n := h.t.Len(); n != 0 {
		t.Fatalf("after both toasts' time, %d showing, want 0", n)
	}
}

func TestThePointerKeepsAToastAndAClickDismissesIt(t *testing.T) {
	w, h, wait := newToastStage(t)
	w.Input(input.KeyPress{Key: input.KeyF2})
	wait(500 * time.Millisecond)
	w.Input(input.PointerMove{Pos: geom.Pt(20, 20), Time: time.Now()})
	wait(3 * time.Second)
	if n := h.t.Len(); n != 1 {
		t.Fatalf("with the pointer on it, %d toasts showing after their time, want 1", n)
	}
	w.Input(input.PointerDown{Pos: geom.Pt(20, 20), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(20, 20), Time: time.Now()})
	wait(100 * time.Millisecond)
	if n := h.t.Len(); n != 0 {
		t.Fatalf("after a click, %d toasts showing, want 0", n)
	}
}
