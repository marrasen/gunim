package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// pager takes Back while it has pages, and Escape with escapes set.
type pager struct {
	recorder
	pages   int
	escapes bool
	backs   int
	pressed int
}

func (p *pager) TakesBack() bool { return p.pages > 0 }

func (p *pager) Handle(e input.Event, _ *UI) bool {
	switch e := e.(type) {
	case input.HistoryStep:
		if p.pages == 0 || e.Forward {
			return false
		}
		p.pages--
		p.backs++
		return true
	case input.KeyPress:
		p.pressed++
		return p.escapes
	}
	return false
}

// keptPager is a pager that holds the keyboard, as a dialog does.
type keptPager struct{ pager }

func (h *keptPager) Modal() bool     { return true }
func (h *keptPager) Focusable() bool { return true }

// backWindow returns a window that leaves on Back, with the nodes given, after a frame.
func backWindow(ns ...Node) *Window {
	w := NewOffscreenWith(WindowOptions{LeaveOnBack: true})
	for _, n := range ns {
		w.ui.Insert(w.ui.Root(), n)
	}
	run(w, 1)
	return w
}

func backAsked(w *Window) { w.Input(driver.BackAsked{Time: time.Now()}) }

// Back goes a page back at a time, whichever node has the keyboard, and leaves from the first.
func TestBackGoesAPageBackAndThenLeaves(t *testing.T) {
	p, other := &pager{pages: 2}, &pager{}
	w := backWindow(p, other)
	off := w.Offscreen()
	for want := 1; want >= 0; want-- {
		if off.BackLeaves() {
			t.Fatalf("with %d pages to go back, Back leaves", p.pages)
		}
		backAsked(w)
		if p.pages != want {
			t.Fatalf("%d pages are left, want %d", p.pages, want)
		}
	}
	// The driver hears at once, before another frame.
	if !off.BackLeaves() {
		t.Fatal("on the first page Back stays")
	}
	if off.AppLeft() != 0 {
		t.Fatal("a Back that was taken left the application")
	}
	// One that was on its way as the page went.
	backAsked(w)
	if off.AppLeft() != 1 {
		t.Fatalf("a Back that nothing takes left %d times", off.AppLeft())
	}
	p.pages = 1
	w.ui.Invalidate()
	run(w, 1)
	if off.BackLeaves() {
		t.Fatal("a page that takes Back after a frame is not heard of")
	}
}

// Without LeaveOnBack, Back never leaves, as before.
func TestBackStaysUnlessAskedToLeave(t *testing.T) {
	w := newTestWindow()
	n := &pager{}
	w.ui.Insert(w.ui.Root(), n)
	w.ui.Focus(n)
	run(w, 1)
	backAsked(w)
	if w.Offscreen().BackLeaves() || w.Offscreen().AppLeft() != 0 {
		t.Fatal("Back leaves a window that did not ask for it")
	}
}

// Escape comes first, to the node with the keyboard.
func TestBackIsEscapeFirst(t *testing.T) {
	p, f := &pager{pages: 1}, &focusPager{pager{escapes: true}}
	w := backWindow(p, f)
	w.ui.Focus(f)
	backAsked(w)
	if f.pressed != 1 || p.pages != 1 || w.Offscreen().AppLeft() != 0 {
		t.Fatalf("Escape was heard %d times, and %d pages are left", f.pressed, p.pages)
	}
	f.escapes = false
	backAsked(w)
	if f.pressed != 2 || p.pages != 0 {
		t.Fatalf("Escape was heard %d times, and %d pages are left", f.pressed, p.pages)
	}
}

// focusPager is a pager that takes the keyboard.
type focusPager struct{ pager }

func (f *focusPager) Focusable() bool { return true }

// The last in the tree goes back first, and one out of the tree, or leaving it, none.
func TestTheLastPageInTheTreeGoesBackFirst(t *testing.T) {
	a, b := &pager{pages: 1}, &pager{pages: 1}
	w := backWindow(a, b)
	backAsked(w)
	if a.pages != 1 || b.pages != 0 {
		t.Fatalf("%d and %d pages are left", a.pages, b.pages)
	}
	w.ui.Remove(a)
	run(w, 1)
	if !w.Offscreen().BackLeaves() {
		t.Fatal("a page that went still takes Back")
	}
	if len(w.ui.backNodes) != 1 {
		t.Fatalf("%d nodes are kept, want the one in the tree", len(w.ui.backNodes))
	}
}

// A modal holds Back, and so do the pages inside it alone.
func TestAModalHoldsBack(t *testing.T) {
	p, m := &pager{pages: 1}, &keptPager{pager{pages: 1}}
	w := backWindow(p, m)
	if w.Offscreen().BackLeaves() {
		t.Fatal("with a modal open Back leaves")
	}
	backAsked(w)
	if m.pages != 0 || p.pages != 1 {
		t.Fatalf("%d pages are left in the modal and %d behind it", m.pages, p.pages)
	}
	backAsked(w)
	if p.pages != 1 || w.Offscreen().AppLeft() != 0 || w.Offscreen().BackLeaves() {
		t.Fatal("a Back the modal left went past it")
	}
	w.ui.Remove(m)
	run(w, 1)
	backAsked(w)
	if p.pages != 0 {
		t.Fatal("with the modal gone, Back does not reach the page")
	}
}

// Back closes a popup that a press outside closes, and leaves one that stays, as a tooltip.
func TestBackClosesAPopup(t *testing.T) {
	n := &pager{}
	w := backWindow(n)
	dismissed := 0
	var pop *Popup
	pop = w.ui.OpenPopup(n, newMenu(), PopupOptions{Anchor: geom.Rect{Max: geom.Pt(100, 50)}, Dismiss: func(*UI) {
		dismissed++
		pop.Close()
	}})
	run(w, 1)
	if w.Offscreen().BackLeaves() {
		t.Fatal("with a menu open Back leaves")
	}
	backAsked(w)
	if dismissed != 1 || w.Offscreen().AppLeft() != 0 {
		t.Fatalf("the menu was dismissed %d times", dismissed)
	}
	run(w, 60)
	w.ui.OpenPopup(n, newMenu(), PopupOptions{Anchor: geom.Rect{Max: geom.Pt(100, 50)}})
	run(w, 1)
	if !w.Offscreen().BackLeaves() {
		t.Fatal("with a tooltip open Back stays")
	}
}

// A page that is not drawn, as on a hidden tab, takes no Back.
func TestAPageOutOfSightTakesNoBack(t *testing.T) {
	p := &pager{pages: 1}
	h := &coverer{child: p, hide: true}
	w := backWindow(h)
	if !w.Offscreen().BackLeaves() {
		t.Fatal("a page out of sight takes Back")
	}
	h.hide = false
	w.ui.Invalidate()
	run(w, 1)
	if w.Offscreen().BackLeaves() {
		t.Fatal("a page in sight does not take Back")
	}
}

// coverer draws its child, unless hide is set.
type coverer struct {
	child Node
	hide  bool
}

func (c *coverer) Children() []Node { return []Node{c.child} }

func (c *coverer) Layout(cs Constraints, _ Frame, kids Children) geom.Size {
	if !c.hide {
		kids.At(0).Layout(cs)
	}
	return cs.Max
}

func (c *coverer) Paint(p *paint.Painter, _ Frame, _ geom.Size, kids Children) {
	if !c.hide {
		kids.At(0).Paint(p)
	}
}
