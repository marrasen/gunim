package gunim

import (
	"math"
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// pinchTaker zooms with a pinch while on, and records what it hears.
type pinchTaker struct {
	recorder
	on bool
}

func (p *pinchTaker) ZoomsWithPinch() bool { return p.on }

// pinches returns the pinches r heard.
func pinches(r *recorder) []input.Pinch {
	var out []input.Pinch
	for _, e := range r.events {
		if p, ok := e.(input.Pinch); ok {
			out = append(out, p)
		}
	}
	return out
}

// pinchStage opens a zooming window with a recorder on the left and n on
// the right, each 100 by 100, and returns the recorder and a hand that
// pinches.
func pinchStage(t *testing.T, n Node) (w *Window, left *recorder, hand func(input.PinchPhase, geom.Point, geom.Point, float32)) {
	t.Helper()
	w = newTestWindow()
	w.ui.zoomKeys = true
	left = &recorder{}
	s := &swapped{}
	w.ui.Insert(w.ui.Root(), s)
	w.ui.Insert(s, left)
	w.ui.Insert(s, n)
	run(w, 1)
	return w, left, func(ph input.PinchPhase, at, delta geom.Point, scale float32) {
		w.ui.handlePlatform(input.Pinch{Pos: at, Delta: delta, Scale: scale, Phase: ph, Time: time.Now()})
	}
}

func TestAPinchZoomerHearsThePinchInItsOwnSpace(t *testing.T) {
	p := &pinchTaker{on: true}
	w, left, hand := pinchStage(t, p)
	hand(input.PinchStart, geom.Pt(150, 50), geom.Point{}, 1)
	hand(input.PinchMove, geom.Pt(152, 51), geom.Pt(2, 1), 1.5)
	hand(input.PinchMove, geom.Pt(153, 51), geom.Pt(1, 0), 0.9)
	hand(input.PinchEnd, geom.Pt(153, 51), geom.Point{}, 1)
	got := pinches(&p.recorder)
	want := []input.Pinch{
		{Pos: geom.Pt(50, 50), Scale: 1, Phase: input.PinchStart},
		{Pos: geom.Pt(52, 51), Delta: geom.Pt(2, 1), Scale: 1.5, Phase: input.PinchMove},
		{Pos: geom.Pt(53, 51), Delta: geom.Pt(1, 0), Scale: 0.9, Phase: input.PinchMove},
		{Pos: geom.Pt(53, 51), Scale: 1, Phase: input.PinchEnd},
	}
	if len(got) != len(want) {
		t.Fatalf("the zoomer heard %d pinches, want %d", len(got), len(want))
	}
	for i := range want {
		g := got[i]
		g.Time = time.Time{}
		if g != want[i] {
			t.Errorf("pinch %d came as %+v, want %+v", i, g, want[i])
		}
	}
	if len(p.events) != len(want) || len(left.events) != 0 {
		t.Errorf("the zoomer heard %v and the node beside it %v, want only the pinches", p.events, left.events)
	}
	if w.ui.Zoom() != 1 {
		t.Errorf("a pinch on a pinch zoomer zoomed the window to %v", w.ui.Zoom())
	}
}

func TestAPinchStaysWithTheZoomerItStartedOver(t *testing.T) {
	p := &pinchTaker{on: true}
	w, left, hand := pinchStage(t, p)
	hand(input.PinchStart, geom.Pt(150, 50), geom.Point{}, 1)
	// The fingers wander over the node beside it, and on out of both.
	hand(input.PinchMove, geom.Pt(50, 50), geom.Pt(-100, 0), 1.5)
	hand(input.PinchMove, geom.Pt(300, 300), geom.Pt(250, 250), 1.5)
	hand(input.PinchEnd, geom.Pt(300, 300), geom.Point{}, 1)
	if got := pinches(&p.recorder); len(got) != 4 || got[1].Pos != geom.Pt(-50, 50) || got[2].Pos != geom.Pt(200, 300) {
		t.Fatalf("the zoomer heard %+v, want the whole pinch wherever the fingers went", got)
	}
	if len(left.events) != 0 || w.ui.Zoom() != 1 {
		t.Errorf("the node beside heard %v and the window zoomed to %v, want neither touched", left.events, w.ui.Zoom())
	}
	// A pinch that starts elsewhere zooms as before.
	p.on = false
	hand(input.PinchStart, geom.Pt(150, 50), geom.Point{}, 1)
	hand(input.PinchMove, geom.Pt(150, 50), geom.Point{}, 1.25)
	if w.ui.Zoom() != 1.1 || len(pinches(&p.recorder)) != 4 {
		t.Errorf("a pinch with the zoomer off zoomed the window to %v, and the zoomer heard %d pinches", w.ui.Zoom(), len(pinches(&p.recorder)))
	}
}

func TestAPinchElsewhereIsCtrlWithTheWheel(t *testing.T) {
	taker := &wheelTaker{on: true}
	w, _, hand := pinchStage(t, taker)
	hand(input.PinchStart, geom.Pt(150, 50), geom.Point{}, 1)
	hand(input.PinchMove, geom.Pt(150, 50), geom.Point{}, 1.25*1.25)
	hand(input.PinchMove, geom.Pt(150, 50), geom.Point{}, 1)
	hand(input.PinchMove, geom.Pt(150, 50), geom.Point{}, 1/1.25)
	hand(input.PinchEnd, geom.Pt(150, 50), geom.Point{}, 1)
	if taker.scroll != 2 || w.ui.Zoom() != 1 {
		t.Fatalf("over a wheel zoomer a pinch gave %d scrolls and zoomed the window to %v, want 2 and none", taker.scroll, w.ui.Zoom())
	}
	// Over the window, a quarter wider is a notch, and a step of zoom.
	hand(input.PinchStart, geom.Pt(50, 50), geom.Point{}, 1)
	hand(input.PinchMove, geom.Pt(50, 50), geom.Point{}, 1.25*1.25)
	if w.ui.Zoom() != 1.25 {
		t.Fatalf("spreading the fingers half again as far zoomed the window to %v, want two steps to 1.25", w.ui.Zoom())
	}
	hand(input.PinchMove, geom.Pt(50, 50), geom.Point{}, 0.75)
	hand(input.PinchEnd, geom.Pt(50, 50), geom.Point{}, 1)
	if w.ui.Zoom() != 1.1 {
		t.Fatalf("closing them by a quarter zoomed the window to %v, want a step down to 1.1", w.ui.Zoom())
	}
}

func TestAPinchesNotchesAreTheLogOfItsScale(t *testing.T) {
	r := &notches{}
	_, _, hand := pinchStage(t, r)
	hand(input.PinchStart, geom.Pt(150, 50), geom.Point{}, 1)
	hand(input.PinchMove, geom.Pt(160, 40), geom.Pt(10, -10), 1.1)
	if len(r.got) != 1 || math.Abs(float64(r.got[0].Notches.Y)-math.Log(1.1)/math.Log(1.25)) > 1e-5 || r.got[0].Pos != geom.Pt(60, 40) {
		t.Fatalf("a pinch a tenth wider scrolled %+v, want log 1.1 over log 1.25 of a notch with Ctrl at the fingers", r.got)
	}
	if !r.got[0].Mods.Has(input.ModControl) {
		t.Errorf("the scroll had mods %v, want Ctrl", r.got[0].Mods)
	}
}

// notches zooms with Ctrl and the wheel, and records the scrolls it
// hears.
type notches struct {
	recorder
	got []input.Scroll
}

func (n *notches) ZoomsWithWheel() bool { return true }

func (n *notches) Handle(e input.Event, _ *UI) bool {
	if s, ok := e.(input.Scroll); ok {
		n.got = append(n.got, s)
	}
	return true
}

func TestAPinchZoomerRemovedMidPinchTakesTheRestNowhere(t *testing.T) {
	p := &pinchTaker{on: true}
	holder := &Box{}
	w, left, hand := pinchStage(t, holder)
	w.ui.Insert(holder, p)
	run(w, 1)
	hand(input.PinchStart, geom.Pt(150, 50), geom.Point{}, 1)
	w.ui.Remove(p)
	run(w, 1)
	hand(input.PinchMove, geom.Pt(50, 50), geom.Point{}, 1.25)
	hand(input.PinchEnd, geom.Pt(50, 50), geom.Point{}, 1)
	if len(pinches(&p.recorder)) != 1 || len(left.events) != 0 || w.ui.Zoom() != 1 {
		t.Errorf("after the zoomer went the rest of its pinch went to it %d times, to the node beside %v, and zoomed the window to %v", len(pinches(&p.recorder))-1, left.events, w.ui.Zoom())
	}
}

func TestAPinchThatStartsAgainEndsTheOneBefore(t *testing.T) {
	p := &pinchTaker{on: true}
	_, _, hand := pinchStage(t, p)
	hand(input.PinchStart, geom.Pt(150, 50), geom.Point{}, 1)
	hand(input.PinchStart, geom.Pt(150, 50), geom.Point{}, 1)
	got := pinches(&p.recorder)
	if len(got) != 3 || got[1].Phase != input.PinchEnd || got[2].Phase != input.PinchStart {
		t.Fatalf("a second start gave %+v, want the first pinch ended before it", got)
	}
}
