package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// dragsTouch is a recorder a finger drags.
type dragsTouch struct{ recorder }

func (*dragsTouch) DragsTouch() bool { return true }

// touchStage opens a window holding n, and returns a finger on it.
func touchStage(t *testing.T, n Node) (w *Window, finger func(ev any)) {
	t.Helper()
	w = newWindow(driver.Offscreen(geom.Sz(400, 400)), nil)
	w.ui.Insert(w.ui.Root(), n)
	w.Frame(time.Second / 60)
	return w, func(ev any) { w.Input(ev) }
}

// swipe presses a finger at from, moves it to to in steps every 16 ms
// from at, and lifts it, the last move held still for rest.
func swipe(finger func(any), from, to geom.Point, steps int, at time.Time, rest time.Duration) time.Time {
	finger(input.PointerDown{Pos: from, Button: input.ButtonPrimary, Clicks: 1, Touch: true, Time: at})
	for i := 1; i <= steps; i++ {
		at = at.Add(16 * time.Millisecond)
		p := from.Add(to.Sub(from).Mul(float32(i) / float32(steps)))
		finger(input.PointerMove{Pos: p, Touch: true, Time: at})
	}
	at = at.Add(rest)
	finger(input.PointerUp{Pos: to, Button: input.ButtonPrimary, Touch: true, Time: at})
	return at
}

// scrolled sums the scroll events r heard, and counts them.
func scrolled(r *recorder) (sum geom.Point, n int) {
	for _, e := range r.events {
		if s, ok := e.(input.Scroll); ok {
			sum, n = sum.Add(s.Delta), n+1
		}
	}
	return sum, n
}

func TestATapStaysAClick(t *testing.T) {
	r := &recorder{}
	_, finger := touchStage(t, r)
	swipe(finger, geom.Pt(100, 100), geom.Pt(104, 103), 2, time.Now(), 0)
	var up *input.PointerUp
	for _, e := range r.events {
		switch e := e.(type) {
		case input.PointerUp:
			up = &e
		case input.Scroll:
			t.Fatal("a finger that stayed within the slop scrolled")
		}
	}
	if up == nil || up.Pos.X < 0 {
		t.Fatalf("the press was let go at %v, want where the finger lifted", up)
	}
}

func TestAFingerMovedScrollsWhatItCameDownOn(t *testing.T) {
	r := &recorder{}
	_, finger := touchStage(t, r)
	swipe(finger, geom.Pt(100, 300), geom.Pt(100, 100), 10, time.Now(), 200*time.Millisecond)
	moves, ups := 0, 0
	for _, e := range r.events {
		switch e := e.(type) {
		case input.PointerMove:
			moves++
		case input.PointerUp:
			ups++
			if e.Pos != input.Away {
				t.Fatalf("the press was let go at %v, want far away, so nothing under it acts", e.Pos)
			}
		case input.Scroll:
			if e.Pos != geom.Pt(100, 300) {
				t.Fatalf("a scroll went to %v, want where the finger came down", e.Pos)
			}
		}
	}
	sum, n := scrolled(r)
	if n == 0 || sum.Y > -150 || sum.Y < -200 {
		t.Fatalf("the finger scrolled %v in %d steps, want up by the 200 it moved less the slop", sum, n)
	}
	if ups != 1 || moves > 1 {
		t.Fatalf("the node heard %d moves and %d releases, want the press let go once and no drag", moves, ups)
	}
}

func TestAFingerDragsWhatDragsByTouch(t *testing.T) {
	d := &dragsTouch{}
	_, finger := touchStage(t, d)
	swipe(finger, geom.Pt(100, 300), geom.Pt(100, 100), 10, time.Now(), 0)
	moves := 0
	for _, e := range d.events {
		switch e := e.(type) {
		case input.PointerMove:
			moves++
		case input.Scroll:
			t.Fatal("a finger on a touch dragger scrolled")
		case input.PointerUp:
			if e.Pos != geom.Pt(100, 100) {
				t.Fatalf("the drag ended at %v, want where the finger lifted", e.Pos)
			}
		}
	}
	if moves < 5 {
		t.Fatalf("the dragger heard %d moves, want the finger's moves past the slop", moves)
	}
}

func TestAFlickCoastsAndStops(t *testing.T) {
	r := &recorder{}
	w, finger := touchStage(t, r)
	swipe(finger, geom.Pt(100, 300), geom.Pt(100, 200), 5, time.Now(), 0)
	_, during := scrolled(r)
	for range 10 {
		w.Frame(time.Second / 60)
	}
	_, after := scrolled(r)
	if after <= during {
		t.Fatal("the scroll stopped as the finger lifted, want it to coast on")
	}
	for range 300 {
		w.Frame(time.Second / 60)
	}
	_, settled := scrolled(r)
	for range 30 {
		w.Frame(time.Second / 60)
	}
	if _, later := scrolled(r); later != settled {
		t.Fatal("the fling never stopped")
	}
	// A finger pressing again stops a fling at once.
	swipe(finger, geom.Pt(100, 300), geom.Pt(100, 200), 5, time.Now(), 0)
	finger(input.PointerDown{Pos: geom.Pt(50, 50), Button: input.ButtonPrimary, Clicks: 1, Touch: true, Time: time.Now()})
	_, pressed := scrolled(r)
	w.Frame(time.Second / 60)
	if _, next := scrolled(r); next != pressed {
		t.Fatal("the fling went on under a finger pressing again")
	}
}

func TestAFingerHeldStillIsALongPress(t *testing.T) {
	r := &recorder{}
	w, finger := touchStage(t, r)
	at := time.Now()
	finger(input.PointerDown{Pos: geom.Pt(100, 100), Button: input.ButtonPrimary, Clicks: 1, Touch: true, Time: at})
	finger(input.PointerMove{Pos: geom.Pt(103, 102), Touch: true, Time: at.Add(100 * time.Millisecond)})
	for range 40 {
		w.Frame(time.Second / 60)
	}
	var downs []input.PointerDown
	for _, e := range r.events {
		if d, ok := e.(input.PointerDown); ok {
			downs = append(downs, d)
		}
	}
	if len(downs) != 2 || downs[1].Button != input.ButtonSecondary || downs[1].Pos != geom.Pt(100, 100) {
		t.Fatalf("a finger held still pressed %+v, want the secondary button after the primary", downs)
	}
	// Moves go on with the button held, and the lift lets it go.
	finger(input.PointerMove{Pos: geom.Pt(160, 100), Touch: true, Time: time.Now()})
	finger(input.PointerUp{Pos: geom.Pt(160, 100), Button: input.ButtonPrimary, Touch: true, Time: time.Now()})
	var moved, up bool
	for _, e := range r.events {
		switch e := e.(type) {
		case input.PointerMove:
			moved = moved || e.Pos == geom.Pt(160, 100)
		case input.PointerUp:
			up = up || e.Button == input.ButtonSecondary && e.Pos == geom.Pt(160, 100)
		case input.Scroll:
			t.Fatal("a long press scrolled")
		}
	}
	if !moved || !up {
		t.Fatalf("after the long press the node heard the move %v and the release %v, want both", moved, up)
	}
}

func TestAFingerHoldingADragStillStaysADrag(t *testing.T) {
	// A handle or a thumb held still is a drag paused, and stays one.
	d := &dragsTouch{}
	w, finger := touchStage(t, d)
	finger(input.PointerDown{Pos: geom.Pt(100, 100), Button: input.ButtonPrimary, Clicks: 1, Touch: true, Time: time.Now()})
	for range 40 {
		w.Frame(time.Second / 60)
	}
	for _, e := range d.events {
		if p, ok := e.(input.PointerDown); ok && p.Button == input.ButtonSecondary {
			t.Fatal("a finger holding a drag still became a long press")
		}
	}
}
