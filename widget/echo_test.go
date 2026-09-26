package widget

import (
	"image/color"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// echoOpener is a node that pings, red, when pressed.
type echoOpener struct{ e Echo }

func (o *echoOpener) Handle(e input.Event, u *gunim.UI) bool {
	if _, ok := e.(input.PointerDown); ok {
		o.e.Ping(u, color.NRGBA{R: 0xff, A: 0xff})
		return true
	}
	return false
}

// ping presses the opener.
func ping(w *gunim.Window) {
	w.Input(input.PointerDown{Pos: geom.Pt(300, 300), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(300, 300), Time: time.Now()})
}

func (o *echoOpener) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}

func (o *echoOpener) Paint(*paint.Painter, gunim.Frame, geom.Size, gunim.Children) {}

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
	run(int(3 * echoLife / (time.Second / 60)))
	if pop.Open() {
		t.Fatal("the popup stayed open after its rings had travelled")
	}
}

// Every ring, glow and all, stays outside the window, which the popup
// lies over.
func TestTheRingsStayOutsideTheWindow(t *testing.T) {
	v := &echoView{size: geom.Sz(360, 260), inner: geom.Rc(80, 80, 200, 100), reach: 64, radius: 8}
	c := color.NRGBA{G: 0xff, A: 0xff}
	v.rings = []echoRing{{c: c, strength: 1, flash: true}, {c: c, strength: 1}, {at: 140 * time.Millisecond, c: c, strength: 0.7}}
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
