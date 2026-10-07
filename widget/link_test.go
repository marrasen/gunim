package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

type followed struct{}

func TestAClickOnALinkRunsItAndSendsItsIntent(t *testing.T) {
	l := NewLink("show 12 more")
	ran := 0
	l.OnClick = func(*gunim.UI) gunim.Intent {
		ran++
		return followed{}
	}
	w, run := stage(t, &frame{child: Row(l), size: geom.Sz(400, 40)})
	click(w, 5, 5)
	run(1)
	if ran != 1 {
		t.Fatalf("the link ran %d times for one click, want once", ran)
	}
	if got := sent(w); len(got) != 1 || got[0] != (followed{}) {
		t.Fatalf("intents %v, want the link's", got)
	}
	if role := l.Access().Role; role != access.RoleLink {
		t.Fatalf("a link reads as a %v", role)
	}
}

func TestSizedHoldsItsChildToItsWidth(t *testing.T) {
	a, b := newSpot(40, 20), newSpot(30, 20)
	stage(t, &frame{child: Row(NewSized(a, 120, 0), b), size: geom.Sz(400, 100)})
	// b starts after the 120 the first child is held to, and the gap.
	at(t, b, geom.Pt(120+Gap.Default(), 0))
}

// A link given less room than its text takes ends the text in an
// ellipsis within its box, where it drew all of it past its edge.
func TestALinkSqueezedEndsInAnEllipsis(t *testing.T) {
	l := NewLink("Show in system file manager")
	var p paint.Painter
	f := gunim.Frame{Scale: 1}
	full := l.run(f.Theme)
	box := geom.Sz(full.Advance/2, full.Box().H)
	l.Paint(&p, f, box, gunim.Children{})
	var glyphs []paint.Glyph
	for _, op := range p.Ops() {
		if t, ok := op.(*paint.TextOp); ok {
			glyphs = append(glyphs, t.Glyphs...)
		}
	}
	if len(glyphs) == 0 || len(glyphs) >= len(full.Glyphs) {
		t.Fatalf("the squeezed link drew %d glyphs of its %d", len(glyphs), len(full.Glyphs))
	}
	for _, g := range glyphs {
		if g.At.X >= box.W {
			t.Fatalf("a glyph starts at %v, past the link's box of %v", g.At.X, box.W)
		}
	}
}

func TestTabReachesALinkAndEnterFollowsIt(t *testing.T) {
	b, l := NewButton("B"), NewLink("show 12 more")
	l.OnClick = Sends(followed{})
	w, run := stage(t, &frame{child: Column(b, l), size: geom.Sz(400, 200)})
	focused := focusProbe(t, w, run)
	tab(w, run, 0)
	tab(w, run, 0)
	if f := focused(); f != l {
		t.Fatalf("two Tabs put the keyboard on %T, want the link", f)
	}
	run(30)
	if r := l.ring.Value(); r < 0.99 {
		t.Fatalf("the focused link's ring is %v grown, want all the way", r)
	}
	for _, k := range []input.Key{input.KeyEnter, input.KeySpace} {
		w.Input(input.KeyPress{Key: k})
		run(1)
		if got := sent(w); len(got) != 1 || got[0] != (followed{}) {
			t.Fatalf("%v on the link sent %v, want the link's intent", k, got)
		}
	}
}
