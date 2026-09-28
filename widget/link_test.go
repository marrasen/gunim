package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

type followed struct{}

func TestAClickOnALinkRunsItAndSendsItsIntent(t *testing.T) {
	l := NewLink("show 12 more")
	l.On = followed{}
	ran := 0
	l.OnActivate(func(*gunim.UI) { ran++ })
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
