package gunim

import (
	"fmt"
	"strings"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// part is a node that names itself, as a widget does when it removes itself or says where an intent came from.
type part struct{ w float32 }

func (p *part) Layout(c Constraints, _ Frame, _ Children) geom.Size {
	return c.Constrain(geom.Sz(p.w, 10))
}
func (p *part) Paint(*paint.Painter, Frame, geom.Size, Children) {}

func (p *part) leave(u *UI)                    { u.Remove(p) }
func (p *part) bounds(u *UI) (geom.Rect, bool) { return u.Bounds(p) }

// wrapper embeds a part, as a view embeds the widget it extends.
type wrapper struct {
	*part
}

// deeper embeds a wrapper by value.
type deeper struct {
	wrapper
}

func TestANodeEmbeddedInAnotherStandsForIt(t *testing.T) {
	w := newTestWindow()
	outer := &wrapper{part: &part{w: 40}}
	w.ui.Insert(w.ui.Root(), outer)
	run(w, 2)
	r, ok := outer.bounds(w.ui)
	if !ok || r.Size().W != 800 {
		t.Fatalf("the embedded part's bounds are %v, %v; want the wrapper's", r, ok)
	}
	outer.leave(w.ui)
	if w.ui.Presence(outer) != Exiting {
		t.Fatalf("removing the embedded part left the wrapper %v", w.ui.Presence(outer))
	}
	run(w, 60)
	if inTree(w.ui, outer) || inTree(w.ui, outer.part) {
		t.Fatal("the wrapper or its part is still in the index after it left")
	}
}

func TestANodeEmbeddedTwoDeepStandsForItsOuterNode(t *testing.T) {
	w := newTestWindow()
	outer := &deeper{wrapper{part: &part{w: 40}}}
	w.ui.Insert(w.ui.Root(), outer)
	run(w, 1)
	outer.leave(w.ui)
	if w.ui.Presence(outer) != Exiting {
		t.Fatal("removing a part embedded two deep did not remove its outer node")
	}
}

func TestInsertingAnEmbeddedNodeElsewherePanics(t *testing.T) {
	w := newTestWindow()
	outer := &wrapper{part: &part{w: 40}}
	w.ui.Insert(w.ui.Root(), outer)
	defer func() {
		if e := recover(); e == nil || !strings.Contains(fmt.Sprint(e), "embedded") {
			t.Fatalf("inserting the embedded part panicked with %v", e)
		}
	}()
	w.ui.Insert(w.ui.Root(), outer.part)
}

func TestNamingANodeThatIsNotInTheTreePanicsUnderTest(t *testing.T) {
	w := newTestWindow()
	stray := &part{}
	for name, call := range map[string]func(){
		"Remove": func() { w.ui.Remove(stray) },
		"Bounds": func() { w.ui.Bounds(stray) },
		"Send":   func() { w.ui.Send(stray, nil) },
		"Focus":  func() { w.ui.Focus(stray) },
	} {
		func() {
			defer func() {
				if e := recover(); e == nil || !strings.Contains(fmt.Sprint(e), name) {
					t.Errorf("%s of a node not in the tree panicked with %v", name, e)
				}
			}()
			call()
		}()
	}
}
