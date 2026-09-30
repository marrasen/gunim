package gunim

import (
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// historyHost fills the window with one child, and keeps the history steps that reach it.
type historyHost struct {
	child Node
	steps []input.HistoryStep
}

func (h *historyHost) Children() []Node { return []Node{h.child} }

func (h *historyHost) Layout(c Constraints, _ Frame, kids Children) geom.Size {
	k := kids.At(0)
	k.Layout(Tight(c.Max))
	k.Place(geom.Point{})
	return c.Max
}

func (h *historyHost) Paint(p *paint.Painter, _ Frame, _ geom.Size, kids Children) {
	kids.At(0).Paint(p)
}

func (h *historyHost) Handle(e input.Event, _ *UI) bool {
	if s, ok := e.(input.HistoryStep); ok {
		h.steps = append(h.steps, s)
		return true
	}
	return false
}

// passer fills its room and takes the keyboard, and takes a pointer press only when grabs is set, counting the
// presses it hears.
type passer struct {
	grabs   bool
	presses int
}

func (p *passer) Layout(c Constraints, _ Frame, _ Children) geom.Size { return c.Max }
func (p *passer) Paint(*paint.Painter, Frame, geom.Size, Children)    {}
func (p *passer) Focusable() bool                                     { return true }
func (p *passer) Handle(e input.Event, _ *UI) bool {
	_, down := e.(input.PointerDown)
	if down {
		p.presses++
	}
	return down && p.grabs
}

func TestASideButtonGoesBackThroughWhatHoldsTheHistory(t *testing.T) {
	w := newTestWindow()
	p := &passer{}
	h := &historyHost{child: p}
	w.ui.Insert(w.ui.Root(), h)
	run(w, 1)
	at := geom.Pt(100, 100)
	for _, b := range []input.Button{input.ButtonBack, input.ButtonForward} {
		w.Input(input.PointerDown{Pos: at, Button: b})
		w.Input(input.PointerUp{Pos: at, Button: b})
	}
	if len(h.steps) != 2 || h.steps[0].Forward || !h.steps[1].Forward {
		t.Fatalf("the history heard %v, want back and then forward", h.steps)
	}
	if w.ui.Focused() != nil {
		t.Fatalf("a side button moved the keyboard to %T", w.ui.Focused())
	}
}

func TestASideButtonPressesNothingUnderThePointer(t *testing.T) {
	w := newTestWindow()
	p := &passer{grabs: true}
	h := &historyHost{child: p}
	w.ui.Insert(w.ui.Root(), h)
	run(w, 1)
	w.Input(input.PointerDown{Pos: geom.Pt(100, 100), Button: input.ButtonBack})
	w.Input(input.PointerUp{Pos: geom.Pt(100, 100), Button: input.ButtonBack})
	if p.presses != 0 {
		t.Fatalf("a node that takes presses heard %d from a side button, want none", p.presses)
	}
	if len(h.steps) != 1 {
		t.Fatalf("the history heard %v, want one step back past the node that takes presses", h.steps)
	}
}

func TestABrowserBackKeyGoesBackFromTheFocusedNode(t *testing.T) {
	w := newTestWindow()
	p := &passer{}
	h := &historyHost{child: p}
	w.ui.Insert(w.ui.Root(), h)
	run(w, 1)
	w.ui.Focus(p)
	// As the driver reports a keyboard's Browser Back key
	w.Input(input.HistoryStep{})
	if len(h.steps) != 1 || h.steps[0].Forward {
		t.Fatalf("the history heard %v, want one step back", h.steps)
	}
}
