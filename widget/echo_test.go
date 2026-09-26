package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// echoOpener is a node that pings when pressed, starts Wait on F2 and
// stops it on F3, and on F4 puts on a theme with a strength of 0.
type echoOpener struct{ e Echo }

func (o *echoOpener) Focusable() bool { return true }

func (o *echoOpener) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		u.Focus(o)
		o.e.Ping(u, EchoProblem)
		return true
	case input.KeyPress:
		switch e.Key {
		case input.KeyF2, input.KeyF3:
			o.e.Wait(u, EchoWait, e.Key == input.KeyF2)
			return true
		case input.KeyF4:
			u.UseTheme(theme.Make("quiet", theme.Set(EchoStrength, 0)))
			return true
		default:
		}
	}
	return false
}

func (o *echoOpener) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}

func (o *echoOpener) Paint(*paint.Painter, gunim.Frame, geom.Size, gunim.Children) {}

// ping presses the opener.
func ping(w *gunim.Window) {
	w.Input(input.PointerDown{Pos: geom.Pt(300, 300), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(300, 300), Time: time.Now()})
}

// frames is how many frames d takes at 60 a second.
func frames(d time.Duration) int { return int(d / (time.Second / 60)) }

// A ping opens a popup over the window, reaching past its edges, and
// closes it once the rings have travelled.
func TestAPingOpensAroundTheWindowAndCloses(t *testing.T) {
	o := &echoOpener{}
	w, run := stage(t, o)
	ping(w)
	run(2)
	if o.e.pop == nil || !o.e.pop.Open() {
		t.Fatal("the ping opened no popup")
	}
	if got, want := o.e.view.size, geom.Sz(800+2*80, 600+2*80); got != want {
		t.Fatalf("the popup is %v, want %v", got, want)
	}
	// A second ping joins the first.
	pop := o.e.pop
	ping(w)
	run(1)
	if o.e.pop != pop {
		t.Fatal("a second ping opened a second popup")
	}
	run(frames(3 * echoLife))
	if pop.Open() {
		t.Fatal("the popup stayed open after its rings had travelled")
	}
}

// Wait sends a ring each beat while it is on, keeping the popup open,
// and the popup closes once it is off and the last ring has gone.
func TestWaitBeatsUntilItIsOff(t *testing.T) {
	o := &echoOpener{}
	w, run := stage(t, o)
	ping(w)
	run(frames(3 * echoLife))
	w.Input(input.KeyPress{Key: input.KeyF2})
	run(2)
	pop := o.e.pop
	if pop == nil || !pop.Open() {
		t.Fatal("Wait opened no popup")
	}
	if len(o.e.view.rings) == 0 {
		t.Fatal("Wait sent no ring")
	}
	// Between beats the last ring has gone and the next is yet to go;
	// the popup stays open for it.
	for beat := range 3 {
		run(frames(echoBeat))
		if !pop.Open() {
			t.Fatalf("beat %d: the popup closed while waiting", beat)
		}
	}
	w.Input(input.KeyPress{Key: input.KeyF3})
	run(frames(3 * echoLife))
	if pop.Open() {
		t.Fatal("the popup stayed open after Wait was turned off")
	}
}

// A theme with a strength of 0 turns the echo off.
func TestAStrengthOfNoneSendsNoEcho(t *testing.T) {
	o := &echoOpener{}
	w, run := stage(t, o)
	ping(w)
	run(frames(3 * echoLife))
	w.Input(input.KeyPress{Key: input.KeyF4})
	run(frames(time.Second))
	o.e.pop = nil
	ping(w)
	run(2)
	if o.e.pop != nil {
		t.Fatal("a ping went out with a strength of 0")
	}
}

// Every ring, glow and all, stays outside the window, which the popup
// lies over.
func TestTheRingsStayOutsideTheWindow(t *testing.T) {
	v := &echoView{size: geom.Sz(360, 260), inner: geom.Rc(80, 80, 200, 100), reach: 64, radius: 8}
	v.rings = []echoRing{
		{tone: EchoDone, strength: 1, flash: true},
		{tone: EchoDone, strength: 1},
		{at: 140 * time.Millisecond, tone: EchoDone, strength: 0.7},
		{tone: EchoWait, strength: 0.35},
	}
	for range 80 {
		var p paint.Painter
		v.Paint(&p, gunim.Frame{}, v.size, gunim.Children{})
		for _, op := range p.Ops() {
			r, ok := op.(*paint.RRectOp)
			if !ok {
				t.Fatalf("the rings painted a %T", op)
			}
			half := r.Stroke.Width / 2
			if r.Rect.Min.X+half > v.inner.Min.X || r.Rect.Min.Y+half > v.inner.Min.Y ||
				r.Rect.Max.X-half < v.inner.Max.X || r.Rect.Max.Y-half < v.inner.Max.Y {
				t.Fatalf("at %v a ring reaches into the window: %v, %v wide", v.age, r.Rect, r.Stroke.Width)
			}
			if r.Rect.Min.X-half < 0 || r.Rect.Max.X+half > v.size.W {
				t.Fatalf("at %v a ring reaches past the popup: %v, %v wide", v.age, r.Rect, r.Stroke.Width)
			}
		}
		v.Step(time.Second / 60)
	}
	if len(v.rings) > 0 {
		t.Fatal("rings still travel after their time")
	}
}
