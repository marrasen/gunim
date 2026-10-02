package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// A drag from a press on a window lying behind another leaves the
// window where it is: the drop lands, and the window is not brought to
// the front.
func TestADragFromBehindLeavesTheWindowBehind(t *testing.T) {
	a, b, c, bk := twoWindows(t)
	a.Input(input.PointerDown{Pos: geom.Pt(30, 30), Behind: true, Time: time.Now()})
	a.Input(input.PointerMove{Pos: geom.Pt(40, 30), Time: time.Now()})
	a.Input(input.PointerMove{Pos: geom.Pt(1000, 50), Time: time.Now()})
	run(b, 1)
	a.Input(input.PointerUp{Pos: geom.Pt(1000, 50), Time: time.Now()})
	run(b, 1)
	run(a, 1)
	if !bk.got(input.Drop{}) {
		t.Fatal("the basket heard no drop")
	}
	if len(c.ended) != 1 || !c.ended[0].Taken {
		t.Fatalf("the carrier heard %v, want one DragEnd, taken", c.ended)
	}
	if got := a.mustOffscreen(t).Raised(); got != 0 {
		t.Fatalf("the window dragged from was brought to the front %d times, want 0", got)
	}
}

// A click on a window lying behind another, a press from behind let go
// with no drag, brings the window to the front as the button comes up,
// and not before.
func TestAClickFromBehindBringsTheWindowToTheFront(t *testing.T) {
	a, _, _, _ := twoWindows(t)
	a.Input(input.PointerDown{Pos: geom.Pt(30, 30), Behind: true, Time: time.Now()})
	if got := a.mustOffscreen(t).Raised(); got != 0 {
		t.Fatalf("the window came to the front %d times on the press, want 0", got)
	}
	a.Input(input.PointerUp{Pos: geom.Pt(30, 30), Time: time.Now()})
	if got := a.mustOffscreen(t).Raised(); got != 1 {
		t.Fatalf("the window came to the front %d times on the click, want 1", got)
	}
	// The next click, on a window the system raised on the press, does
	// nothing more.
	a.Input(input.PointerDown{Pos: geom.Pt(30, 30), Time: time.Now()})
	a.Input(input.PointerUp{Pos: geom.Pt(30, 30), Time: time.Now()})
	if got := a.mustOffscreen(t).Raised(); got != 1 {
		t.Fatalf("the window came to the front %d times after an ordinary click, want 1", got)
	}
}

// The node pressed hears that the press came from behind.
func TestAPressFromBehindSaysSo(t *testing.T) {
	w := newTestWindow()
	r := &recorder{}
	w.ui.Insert(w.ui.Root(), r)
	run(w, 1)
	w.Input(input.PointerDown{Pos: geom.Pt(10, 10), Behind: true, Time: time.Now()})
	var down input.PointerDown
	for _, e := range r.events {
		if d, ok := e.(input.PointerDown); ok {
			down = d
		}
	}
	if !down.Behind {
		t.Fatalf("the node heard %+v, want Behind set", down)
	}
}

// Escape, which the system says is held while a window pressed from
// behind has no keyboard, gives the drag up, and the window stays
// behind.
func TestEscapeGivesADragFromBehindUp(t *testing.T) {
	a, _, c, _ := twoWindows(t)
	a.Input(input.PointerDown{Pos: geom.Pt(30, 30), Behind: true, Time: time.Now()})
	a.Input(input.PointerMove{Pos: geom.Pt(40, 30), Time: time.Now()})
	a.Input(input.KeyPress{Key: input.KeyEscape, Time: time.Now()})
	if a.ui.drag != nil {
		t.Fatal("Escape left the drag going")
	}
	if len(c.ended) != 1 || c.ended[0].Taken {
		t.Fatalf("the carrier heard %v, want one DragEnd, untaken", c.ended)
	}
	a.Input(input.PointerUp{Pos: geom.Pt(40, 30), Time: time.Now()})
	if got := a.mustOffscreen(t).Raised(); got != 0 {
		t.Fatalf("the window came to the front %d times after a drag given up, want 0", got)
	}
}

// Escape held during a press from behind with no drag reaches no node:
// the window has no keyboard.
func TestEscapeDuringAPressFromBehindReachesNoNode(t *testing.T) {
	w := newTestWindow()
	r := &recorder{}
	w.ui.Insert(w.ui.Root(), r)
	run(w, 1)
	if !w.ui.Focus(r) {
		t.Fatal("the recorder took no focus")
	}
	w.Input(input.PointerDown{Pos: geom.Pt(10, 10), Behind: true, Time: time.Now()})
	w.Input(input.KeyPress{Key: input.KeyEscape, Time: time.Now()})
	if r.got(input.KeyPress{}) {
		t.Fatal("a key reached a node of a window pressed from behind")
	}
	w.Input(input.PointerUp{Pos: geom.Pt(10, 10), Time: time.Now()})
	if got := w.mustOffscreen(t).Raised(); got != 1 {
		t.Fatalf("the window came to the front %d times on the click, want 1", got)
	}
}

// A window that takes the keyboard while pressed from behind has
// nothing to bring to the front as the button comes up.
func TestAPressFromBehindThatGainsTheKeyboardDoesNotRaise(t *testing.T) {
	w := newTestWindow()
	run(w, 1)
	w.Input(input.PointerDown{Pos: geom.Pt(10, 10), Behind: true, Time: time.Now()})
	w.Input(driver.WindowFocus{Focused: true})
	w.Input(input.PointerUp{Pos: geom.Pt(10, 10), Time: time.Now()})
	if got := w.mustOffscreen(t).Raised(); got != 0 {
		t.Fatalf("the window came to the front %d times, want 0", got)
	}
}
