package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// claimWindow is an offscreen window that keeps what the engine says of the claim on
// the keys, as a browser's driver does.
type claimWindow struct {
	*driver.OffscreenWindow
	said []bool
}

func (w *claimWindow) SetKeysClaimed(on bool) { w.said = append(w.said, on) }

// take returns what the engine has said since the last take.
func (w *claimWindow) take() []bool {
	s := w.said
	w.said = nil
	return s
}

// claims is a node that claims the keys while on says so. Space turns that over, and
// so does a press.
type claims struct {
	recorder
	on bool
}

func (c *claims) ClaimsKeys() bool { return c.on }

func (c *claims) Handle(e input.Event, u *UI) bool {
	switch e := e.(type) {
	case input.KeyPress:
		if e.Key == input.KeySpace {
			c.on = !c.on
		}
	case input.PointerDown:
		c.on = !c.on
	}
	return c.recorder.Handle(e, u)
}

func TestTheDriverHearsOfAClaimAtOnce(t *testing.T) {
	cw := &claimWindow{OffscreenWindow: driver.Offscreen(geom.Sz(800, 600))}
	w := newWindow(cw, nil)
	term, plain, calm := &claims{on: true}, &recorder{}, &claims{}
	for _, n := range []Node{term, plain, calm} {
		w.ui.Insert(w.ui.Root(), n)
	}
	frame := func() { w.Frame(time.Second / 60) }
	frame()
	check := func(what string, want ...bool) {
		t.Helper()
		got := cw.take()
		if len(got) != len(want) || len(got) == 1 && got[0] != want[0] {
			t.Fatalf("%s, the driver heard %v, want %v", what, got, want)
		}
	}
	check("with nothing focused")
	// As the focus moves, with no frame between.
	w.ui.Focus(term)
	check("as a node that claims the keys took the focus", true)
	frame()
	check("a frame with nothing changed")
	w.ui.Focus(plain)
	check("as another took it", false)
	w.ui.Focus(calm)
	check("as one that claims none now took it")
	// As a key and a press change the node's mind, with no frame between.
	w.Input(input.KeyPress{Key: input.KeySpace})
	check("as a key had it claim them", true)
	w.Input(input.KeyPress{Key: input.KeyEnter})
	check("a key that changed nothing")
	w.Input(input.KeyPress{Key: input.KeySpace})
	check("as a key had it let go", false)
	// A change the program makes reaches the driver with the next frame.
	calm.on = true
	w.ui.Invalidate()
	frame()
	check("as the program had it claim them", true)
	// From one that claims them to another, nothing changes.
	w.ui.Focus(term)
	check("between two that claim them")
	w.ui.Focus(nil)
	check("as the focus left", false)
	// Taken out of the tree while it has the focus.
	w.ui.Focus(term)
	check("as it took the focus again", true)
	w.ui.Remove(term)
	frame()
	check("as it left the tree", false)
}

// TestOnlyTheFocusedNodeClaims: a node inside one that claims the keys has the keyboard
// to itself.
func TestOnlyTheFocusedNodeClaims(t *testing.T) {
	w := NewOffscreen(geom.Sz(800, 600), nil)
	outer, inner := &claims{on: true}, &recorder{}
	w.ui.Insert(w.ui.Root(), outer)
	w.ui.Insert(outer, inner)
	w.Frame(time.Second / 60)
	w.ui.Focus(inner)
	if w.Offscreen().KeysClaimed() {
		t.Error("a node inside one that claims the keys claims them too")
	}
	w.ui.Focus(outer)
	if !w.Offscreen().KeysClaimed() {
		t.Error("the node that claims the keys does not")
	}
}

type askedToClose struct{}

func init() { RegisterType[askedToClose]("test.asked-to-close") }

// TestCloseAsksIsApartFromAskToClose: what the close button sends leaves the platform's
// own question as it is, which only SetCloseAsks asks for.
func TestCloseAsksIsApartFromAskToClose(t *testing.T) {
	w := NewOffscreenWith(WindowOptions{AskToClose: askedToClose{}})
	if w.Offscreen().CloseAsks() {
		t.Error("a window with AskToClose has the platform ask the user too")
	}
	w.ui.SetCloseAsks(true)
	if !w.Offscreen().CloseAsks() {
		t.Error("the driver was not told to ask the user")
	}
	w.Input(driver.CloseAsked{})
	select {
	case <-w.done:
		t.Fatal("a window with AskToClose closed unasked")
	default:
	}
	w.ui.SetCloseAsks(false)
	if w.Offscreen().CloseAsks() {
		t.Error("the driver was not told to ask no more")
	}
	// With no AskToClose, the close button closes, whatever the platform asks.
	plain := NewOffscreenWith(WindowOptions{})
	plain.ui.SetCloseAsks(true)
	plain.Input(driver.CloseAsked{})
	select {
	case <-plain.done:
	default:
		t.Error("a window with no AskToClose stayed open")
	}
}

// TestTheApplicationHearsTheUserLeaveFullScreen: as the driver says so, and at no other
// time.
func TestTheApplicationHearsTheUserLeaveFullScreen(t *testing.T) {
	w := NewOffscreenWith(WindowOptions{})
	c := w.Client()
	w.ui.SetFullScreen(true)
	w.ui.SetFullScreen(false)
	w.Frame(time.Second / 60)
	select {
	case e := <-c.Intents():
		t.Fatalf("the application leaving it sent %v", e.Intent)
	default:
	}
	w.Input(driver.FullScreenLeft{})
	if !w.ui.needsFrame() {
		t.Error("the window draws no frame as the user leaves")
	}
	if _, ok := As[FullScreenLeft](take(t, c)); !ok {
		t.Error("the application heard something else")
	}
}

// TestCancelDragGivesUpOnlyTheNodesOwn: another node's drag goes on.
func TestCancelDragGivesUpOnlyTheNodesOwn(t *testing.T) {
	w := NewOffscreen(geom.Sz(800, 600), nil)
	src, other := &recorder{}, &recorder{}
	w.ui.Insert(w.ui.Root(), src)
	w.ui.Insert(w.ui.Root(), other)
	w.Frame(time.Second / 60)
	w.ui.CancelDrag(src)
	w.ui.StartDrag(src, "data", nil, geom.Point{})
	w.ui.CancelDrag(other)
	w.ui.CancelDrag(nil)
	if w.ui.drag == nil {
		t.Fatal("a drag was given up by a node that did not start it")
	}
	w.ui.CancelDrag(src)
	if w.ui.drag != nil {
		t.Error("the drag goes on")
	}
	var nilUI *UI
	nilUI.CancelDrag(src)
}
