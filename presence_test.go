package gunim

import (
	"reflect"
	"testing"
	"time"

	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// fadePanel is the smallest possible Transitioner: it fades in and out,
// and holds its place in the tree until the fade is done.
type fadePanel struct {
	anim.Group
	in *anim.Float
}

func newFadePanel() *fadePanel {
	p := &fadePanel{in: anim.NewFloat(0)}
	p.Add(p.in)
	return p
}

func (p *fadePanel) Transition(s Presence) bool {
	switch s {
	case Entering:
		p.in.Animate(1, anim.Snappy)
	case Exiting:
		p.in.Animate(0, anim.Gentle)
	case Present:
		// Settled, so there is nothing left to drive.
	}
	return !p.in.Active()
}

func (p *fadePanel) Layout(c Constraints, _ Frame, _ Children) geom.Size { return c.Max }
func (p *fadePanel) Paint(pt *paint.Painter, _ Frame, box geom.Size, _ Children) {
	pt.RRect(geom.Rect{Max: box.Point()}, 8, paint.Fill{})
}

// run advances the window by n frames at 60 Hz.
func run(w *Window, n int) {
	for range n {
		w.Frame(time.Second / 60)
	}
}

func newTestWindow() *Window { return NewOffscreen(geom.Sz(800, 600), nil) }

func inTree(u *UI, n Node) bool {
	_, ok := u.index[n]
	return ok
}

func TestRemovedNodeStaysUntilItsExitFinishes(t *testing.T) {
	w := newTestWindow()
	panel := newFadePanel()

	w.ui.Insert(w.ui.Root(), panel)
	run(w, 60) // a second: long enough to arrive
	if got := w.ui.Presence(panel); got != Present {
		t.Fatalf("after entering, Presence = %v, want present", got)
	}
	if got := panel.in.Value(); got != 1 {
		t.Fatalf("entrance left in = %v, want 1", got)
	}

	// This is the whole point of the design: the application says go
	// away, and the node takes its time.
	w.ui.Remove(panel)
	run(w, 1)
	if !inTree(w.ui, panel) {
		t.Fatal("node was unlinked on the frame it was removed; it never got to animate out")
	}
	if w.ui.Presence(panel) != Exiting {
		t.Fatalf("Presence = %v, want exiting", w.ui.Presence(panel))
	}

	// It keeps painting while it leaves.
	run(w, 4)
	if !inTree(w.ui, panel) {
		t.Fatal("node vanished five frames in; the exit is meant to take longer")
	}
	if v := panel.in.Value(); v <= 0 || v >= 1 {
		t.Fatalf("mid-exit value = %v, want something between 0 and 1", v)
	}
	if len(w.offscreen(t).Ops()) == 0 {
		t.Fatal("an exiting node painted nothing")
	}

	// And then it goes.
	run(w, 120)
	if inTree(w.ui, panel) {
		t.Fatalf("node still in the tree after its exit settled (in = %v)", panel.in.Value())
	}
}

func TestReinsertingReversesTheExit(t *testing.T) {
	w := newTestWindow()
	panel := newFadePanel()
	w.ui.Insert(w.ui.Root(), panel)
	run(w, 60)

	w.ui.Remove(panel)
	run(w, 6)
	half := panel.in.Value()
	if half >= 1 || half <= 0 {
		t.Fatalf("expected to be part way out, at %v", half)
	}

	// Reopening mid-dismiss picks up from where the fade has got to.
	w.ui.Insert(w.ui.Root(), panel)
	run(w, 1)
	if w.ui.Presence(panel) != Entering {
		t.Fatalf("Presence = %v, want entering", w.ui.Presence(panel))
	}
	run(w, 120)
	if !inTree(w.ui, panel) {
		t.Fatal("reinserted node was reaped anyway")
	}
	if panel.in.Value() != 1 {
		t.Fatalf("in = %v, want 1", panel.in.Value())
	}
}

func TestParentWaitsForItsChildren(t *testing.T) {
	// A plain container with no animation of its own still has to wait
	// for a child that is mid-exit.
	w := newTestWindow()
	group := &Box{}
	child := newFadePanel()
	w.ui.Insert(w.ui.Root(), group)
	w.ui.Insert(group, child)
	run(w, 60)

	w.ui.Remove(group)
	run(w, 3)
	if !inTree(w.ui, group) {
		t.Fatal("container was unlinked while its child was still fading")
	}
	run(w, 120)
	if inTree(w.ui, group) || inTree(w.ui, child) {
		t.Fatal("subtree was not reaped once the child settled")
	}
}

func TestIdleWindowStopsDrawing(t *testing.T) {
	// Animation-first still has to come to rest.
	w := newTestWindow()
	panel := newFadePanel()
	w.ui.Insert(w.ui.Root(), panel)

	run(w, 1)
	if !w.ui.needsFrame() {
		t.Fatal("window went idle while a node was animating in")
	}
	run(w, 120)
	if w.ui.needsFrame() {
		t.Fatal("window still asking for frames with nothing moving; that is a dead battery")
	}
}

func TestReinsertingBringsBackChildrenThatLeftWithTheParent(t *testing.T) {
	w := newTestWindow()
	group := &Box{}
	child := newFadePanel()
	w.ui.Insert(w.ui.Root(), group)
	w.ui.Insert(group, child)
	run(w, 60)

	// The child fades because its parent is leaving.
	w.ui.Remove(group)
	run(w, 6)
	if v := child.in.Value(); v <= 0 || v >= 1 {
		t.Fatalf("child in = %v, want part way out", v)
	}

	w.ui.Insert(w.ui.Root(), group)
	run(w, 120)
	if !inTree(w.ui, child) {
		t.Fatal("child was reaped after its parent came back")
	}
	if v := child.in.Value(); v != 1 {
		t.Fatalf("child in = %v, want 1: it stayed faded after its parent came back", v)
	}
}

func TestReinsertingKeepsAChildRemovedInItsOwnRight(t *testing.T) {
	w := newTestWindow()
	group := &Box{}
	kept := newFadePanel()
	dropped := newFadePanel()
	w.ui.Insert(w.ui.Root(), group)
	w.ui.Insert(group, kept)
	w.ui.Insert(group, dropped)
	run(w, 60)

	w.ui.Remove(dropped)
	w.ui.Remove(group)
	run(w, 3)
	w.ui.Insert(w.ui.Root(), group)
	run(w, 120)

	if !inTree(w.ui, kept) {
		t.Fatal("the child that left with its parent should have come back")
	}
	if inTree(w.ui, dropped) {
		t.Fatal("a child removed on its own came back with its parent")
	}
}

func TestInsertAtMovesANodeAlreadyInTheTree(t *testing.T) {
	w := newTestWindow()
	a, b, c := newFadePanel(), newFadePanel(), newFadePanel()
	for _, n := range []Node{a, b, c} {
		w.ui.Insert(w.ui.Root(), n)
	}
	other := &Box{}
	w.ui.Insert(w.ui.Root(), other)
	run(w, 60)

	order := func() []Node {
		out := make([]Node, 0, len(w.ui.root.kids))
		for _, k := range w.ui.root.kids {
			out = append(out, k.node)
		}
		return out
	}

	// Insert with no index leaves a node where its parent already has it.
	w.ui.Insert(w.ui.Root(), a)
	if got := order(); got[0] != a {
		t.Fatal("Insert moved a node its parent already held")
	}

	w.ui.InsertAt(w.ui.Root(), 0, c)
	if got := order(); got[0] != c || got[1] != a || got[2] != b {
		t.Fatal("InsertAt did not move c to the front")
	}

	w.ui.Remove(b)
	run(w, 3)
	w.ui.InsertAt(other, 0, b)
	if w.ui.index[b].parent != w.ui.index[other] {
		t.Fatal("reviving b under a new parent left it where it was")
	}
	if w.ui.Presence(b) != Entering {
		t.Fatalf("Presence = %v, want entering", w.ui.Presence(b))
	}
	if len(w.ui.root.kids) != 3 {
		t.Fatalf("root has %d children, want 3 once b moved away", len(w.ui.root.kids))
	}
}

func TestInsertIntoOwnSubtreePanics(t *testing.T) {
	w := newTestWindow()
	group := &Box{}
	child := &Box{}
	w.ui.Insert(w.ui.Root(), group)
	w.ui.Insert(group, child)

	defer func() {
		if recover() == nil {
			t.Fatal("inserting a node under its own child did not panic")
		}
	}()
	w.ui.Insert(child, group)
}

// recorder is a node that fills the window and logs what it is sent.
type recorder struct {
	events []input.Event
}

func (r *recorder) Handle(e input.Event, _ *UI) bool {
	r.events = append(r.events, e)
	return true
}

func (r *recorder) Layout(c Constraints, _ Frame, _ Children) geom.Size { return c.Max }
func (r *recorder) Paint(*paint.Painter, Frame, geom.Size, Children)    {}

func (r *recorder) got(want input.Event) bool {
	for _, e := range r.events {
		if reflect.TypeOf(e) == reflect.TypeOf(want) {
			return true
		}
	}
	return false
}

func TestRemovedNodeGivesUpFocusAndHover(t *testing.T) {
	w := newTestWindow()
	group := &Box{}
	r := &recorder{}
	w.ui.Insert(w.ui.Root(), group)
	w.ui.Insert(group, r)
	run(w, 1)

	w.ui.handlePlatform(input.PointerMove{Pos: geom.Pt(10, 10), Time: time.Now()})
	w.ui.Focus(r)
	if w.ui.focus == nil || w.ui.hover == nil {
		t.Fatal("setup: recorder should hold focus and hover")
	}

	// Removing an ancestor is enough.
	w.ui.Remove(group)
	if w.ui.focus != nil {
		t.Fatal("a leaving node kept keyboard focus")
	}
	if w.ui.hover != nil {
		t.Fatal("a leaving node kept hover")
	}
	if !r.got(input.FocusLost{}) || !r.got(input.PointerLeave{}) {
		t.Fatalf("recorder saw %v, want input.FocusLost and input.PointerLeave", r.events)
	}

	w.ui.Focus(r)
	if w.ui.focus != nil {
		t.Fatal("Focus gave focus to a leaving node")
	}
}
