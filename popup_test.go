package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// menu is popup content: 120 by 80, fading in and out, recording input.
type menu struct {
	fadePanel
	recorder
}

func newMenu() *menu {
	m := &menu{fadePanel: *newFadePanel()}
	m.Add(m.in)
	return m
}

func (m *menu) Layout(c Constraints, _ Frame, _ Children) geom.Size {
	return c.Constrain(geom.Sz(120, 80))
}

func (m *menu) Paint(p *paint.Painter, _ Frame, box geom.Size, _ Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 4, paint.Fill{})
}

// openMenu puts a recorder at (10, 10), 100 by 50, and opens a menu
// below it.
func openMenu(t *testing.T, dismiss func(*UI)) (*Window, *recorder, *menu, *Popup) {
	t.Helper()
	w, _, opener := newStage(t, paint.Identity)
	m := newMenu()
	pop := w.ui.OpenPopup(opener, m, PopupOptions{
		Anchor:  geom.Rect{Max: geom.Pt(100, 50)},
		Dismiss: dismiss,
	})
	run(w, 1)
	if len(w.ui.popups) != 1 || w.ui.popups[0].dw == nil {
		t.Fatal("the popup's window did not open")
	}
	return w, opener, m, pop
}

func popupWindow(t *testing.T, w *Window) *driver.OffscreenWindow {
	t.Helper()
	pw, ok := w.ui.popups[0].dw.(*driver.OffscreenWindow)
	if !ok {
		t.Fatalf("the popup is a %T, want an offscreen window", w.ui.popups[0].dw)
	}
	return pw
}

func TestPopupOpensAtItsContentsSizeBesideItsAnchor(t *testing.T) {
	w, _, _, _ := openMenu(t, nil)
	pw := popupWindow(t, w)
	if got := pw.Size(); got != geom.Sz(120, 80) {
		t.Errorf("popup size = %v, want 120x80", got)
	}
	// The anchor is the opener's box, where the stage drew it.
	if got, want := pw.Anchor(), geom.Rc(10, 10, 100, 50); got != want {
		t.Errorf("popup anchor = %v, want %v", got, want)
	}
	if len(pw.Ops()) == 0 {
		t.Error("the popup presented nothing")
	}
}

func TestPopupFollowsItsAnchor(t *testing.T) {
	w, _, _, pop := openMenu(t, nil)
	pop.Move(geom.Rc(0, 50, 100, 0))
	pw := popupWindow(t, w)
	pw.Tick()
	run(w, 1)
	if got, want := pw.Anchor(), geom.Rc(10, 60, 100, 0); got != want {
		t.Errorf("popup anchor = %v, want %v", got, want)
	}
}

func TestPressInPopupReachesItAndLeavesFocus(t *testing.T) {
	w, opener, m, _ := openMenu(t, func(*UI) { t.Error("a press inside the popup dismissed it") })
	w.ui.Focus(opener)
	s := w.ui.popups[0]
	w.ui.popupEvent(popupEvent{s: s, ev: input.PointerDown{Pos: geom.Pt(30, 20), Time: time.Now()}})
	if !m.got(input.PointerDown{}) {
		t.Fatal("the menu saw no press")
	}
	if got := pressedAt(t, &m.recorder); !near(got, geom.Pt(30, 20)) {
		t.Errorf("press at %v in the menu's space, want (30, 20)", got)
	}
	if w.ui.Focused() != opener {
		t.Error("a press in the popup moved focus")
	}
}

func TestPressOutsideDismisses(t *testing.T) {
	dismissed := 0
	w, _, _, pop := openMenu(t, func(*UI) { dismissed++ })
	press(w, 50, 30) // on the opener
	if dismissed != 0 {
		t.Fatal("a press on the opener dismissed the popup")
	}
	press(w, 500, 500)
	if dismissed != 1 {
		t.Fatalf("a press outside dismissed %d times, want once", dismissed)
	}
	w.ui.handlePlatform(driver.WindowFocus{Focused: false})
	if dismissed != 2 {
		t.Fatal("losing the keyboard did not dismiss the popup")
	}
	if !pop.Open() {
		t.Error("dismissing closed the popup by itself")
	}
}

func TestClosedPopupAnimatesOutThenCloses(t *testing.T) {
	w, _, m, pop := openMenu(t, nil)
	run(w, 60)
	pop.Close()
	if pop.Open() {
		t.Fatal("Open is true after Close")
	}
	run(w, 2)
	if len(w.ui.popups) != 1 || w.ui.Presence(m) != Exiting {
		t.Fatal("the popup closed before its content left")
	}
	run(w, 120)
	if len(w.ui.popups) != 0 {
		t.Fatal("the popup stayed open after its content left")
	}
	if inTree(w.ui, m) {
		t.Error("the content is still in the tree")
	}
}

func TestRemovingTheOpenerClosesThePopup(t *testing.T) {
	w, opener, _, pop := openMenu(t, nil)
	w.ui.Remove(opener)
	if pop.Open() {
		t.Error("the popup stayed open after its opener was removed")
	}
}

// holder records input and holds one child, 50 by 50, at (10, 10).
type holder struct{ recorder }

func (h *holder) Layout(c Constraints, _ Frame, kids Children) geom.Size {
	for k := range kids.All {
		k.Layout(Tight(geom.Sz(50, 50)))
		k.Place(geom.Pt(10, 10))
	}
	return c.Max
}

func (h *holder) Paint(p *paint.Painter, _ Frame, _ geom.Size, kids Children) {
	for k := range kids.All {
		k.Paint(p)
	}
}

func TestHoverEntersEveryNodeAroundThePointer(t *testing.T) {
	w := newTestWindow()
	h := &holder{}
	in := &recorder{}
	w.ui.Insert(w.ui.Root(), h)
	w.ui.Insert(h, in)
	run(w, 1)

	move := func(x, y float32) {
		w.ui.handlePlatform(input.PointerMove{Pos: geom.Pt(x, y), Time: time.Now()})
	}
	move(30, 30) // over the child, inside the holder
	if count[input.PointerEnter](&h.recorder) != 1 || count[input.PointerEnter](in) != 1 {
		t.Fatal("entering the child did not enter both")
	}
	move(100, 100) // over the holder alone
	if count[input.PointerLeave](in) != 1 || count[input.PointerLeave](&h.recorder) != 0 {
		t.Fatal("leaving the child for its holder should leave the child alone")
	}
	move(30, 30)
	if count[input.PointerEnter](&h.recorder) != 1 || count[input.PointerEnter](in) != 2 {
		t.Fatal("coming back to the child entered the holder again")
	}
	w.ui.handlePlatform(input.PointerLeave{Time: time.Now()})
	if count[input.PointerLeave](in) != 2 || count[input.PointerLeave](&h.recorder) != 1 {
		t.Fatal("leaving the window did not leave both")
	}
}

// builder builds one child, 50 by 50 at (10, 10), in its first layout.
type builder struct {
	made *recorder
}

func (b *builder) Layout(c Constraints, _ Frame, kids Children) geom.Size {
	if b.made == nil {
		b.made = &recorder{}
		k := kids.Build(b.made)
		k.Layout(Tight(geom.Sz(50, 50)))
		k.Place(geom.Pt(10, 10))
	}
	for k := range kids.All {
		k.Layout(Tight(geom.Sz(50, 50)))
		k.Place(geom.Pt(10, 10))
	}
	return c.Max
}

func (b *builder) Paint(p *paint.Painter, _ Frame, _ geom.Size, kids Children) {
	for k := range kids.All {
		k.Paint(p)
	}
}

func TestAChildBuiltInLayoutPaintsAndTakesInputThatFrame(t *testing.T) {
	w := newTestWindow()
	b := &builder{}
	w.ui.Insert(w.ui.Root(), b)
	run(w, 1)
	press(w, 30, 30)
	if !b.made.got(input.PointerDown{}) {
		t.Fatal("the built child took no press after its first frame")
	}
}
