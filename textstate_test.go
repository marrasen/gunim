package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// stateWindow is an offscreen window that keeps the text states the
// engine sends, as a phone's driver does.
type stateWindow struct {
	*driver.OffscreenWindow
	sent []sentState
}

type sentState struct {
	state *input.TextState
	seq   uint64
}

func (w *stateWindow) SetTextState(s *input.TextState, seq uint64) {
	w.sent = append(w.sent, sentState{s, seq})
}

// take returns what the engine has sent since the last take.
func (w *stateWindow) take() []sentState {
	s := w.sent
	w.sent = nil
	return s
}

// editsText is a node that keeps a text and makes the edits it is sent.
type editsText struct {
	recorder
	text string
}

func (*editsText) TakesText() bool { return true }

func (n *editsText) TextState() input.TextState {
	c := len(n.text)
	return input.TextState{Text: n.text, Selection: [2]int{c, c}, Composing: [2]int{c, c}}
}

func (n *editsText) Handle(e input.Event, u *UI) bool {
	if te, ok := e.(input.TextEdit); ok {
		n.text = n.text[:te.Replace[0]] + te.With + n.text[te.Replace[1]:]
	}
	return n.recorder.Handle(e, u)
}

func TestTheDriverHearsTheFocusedTextAndTheEditsItHolds(t *testing.T) {
	sw := &stateWindow{OffscreenWindow: driver.Offscreen(geom.Sz(800, 600))}
	w := newWindow(sw, nil)
	field, other := &editsText{text: "hi"}, &recorder{}
	w.ui.Insert(w.ui.Root(), field)
	w.ui.Insert(w.ui.Root(), other)
	frame := func() { w.Frame(time.Second / 60) }

	w.ui.Focus(field)
	got := sw.take()
	if len(got) != 1 || got[0].state == nil || got[0].state.Text != "hi" || got[0].seq != 0 {
		t.Fatalf("on focus the driver heard %+v, want hi at seq 0", got)
	}
	frame()
	if got := sw.take(); len(got) != 0 {
		t.Fatalf("a frame with nothing changed sent %+v", got)
	}

	// Two edits in flight: the state after the first is behind the
	// driver, and says so by its seq.
	s, _ := field.TextState().Apply(field.TextState().Commit("!"))
	w.Input(input.TextEdit{Replace: [2]int{2, 2}, With: "!", Selection: [2]int{3, 3}, Composing: [2]int{3, 3}, Seq: 1})
	frame()
	if got := sw.take(); len(got) != 1 || *got[0].state != s || got[0].seq != 1 {
		t.Fatalf("after edit 1 the driver heard %+v, want %+v at seq 1", got, s)
	}
	w.Input(input.TextEdit{Replace: [2]int{3, 3}, Selection: [2]int{3, 3}, Composing: [2]int{3, 3}, Seq: 2})
	frame()
	if got := sw.take(); len(got) != 1 || *got[0].state != s || got[0].seq != 2 {
		t.Fatalf("an edit that changed nothing told the driver %+v, want the same state at seq 2", got)
	}

	// Text the program sets reaches the driver at the seq it already has.
	field.text = "set"
	w.ui.Invalidate()
	frame()
	if got := sw.take(); len(got) != 1 || got[0].state.Text != "set" || got[0].seq != 2 {
		t.Fatalf("text set by the program reached the driver as %+v", got)
	}

	w.ui.Focus(other)
	if got := sw.take(); len(got) != 1 || got[0].state != nil {
		t.Fatalf("focus leaving the text told the driver %+v, want nil", got)
	}
}

func TestEachTextNodeTakingTheFocusStartsTheKeyboardOver(t *testing.T) {
	// Two empty fields have the same state, and the second still has to
	// reach the driver, as a field of its own.
	sw := &stateWindow{OffscreenWindow: driver.Offscreen(geom.Sz(800, 600))}
	w := newWindow(sw, nil)
	a, b := &editsText{}, &editsText{}
	w.ui.Insert(w.ui.Root(), a)
	w.ui.Insert(w.ui.Root(), b)
	w.ui.Focus(a)
	w.ui.Focus(b)
	if got := sw.take(); len(got) != 2 || got[0].state == nil || got[1].state == nil {
		t.Fatalf("the driver heard %+v, want a state for each field", got)
	}
}

// keyboardWindow is an offscreen window with a keyboard on the screen,
// that counts the times the engine asks for it.
type keyboardWindow struct {
	*driver.OffscreenWindow
	asked int
}

func (w *keyboardWindow) ShowKeyboard() { w.asked++ }

// box is a recorder of a fixed size, so presses can land on it or miss.
type box struct {
	editsText
	size geom.Size
}

func (b *box) Layout(Constraints, Frame, Children) geom.Size { return b.size }
func (*box) Focusable() bool                                 { return true }

func TestATapOnTheFocusedTextAsksForTheKeyboard(t *testing.T) {
	kw := &keyboardWindow{OffscreenWindow: driver.Offscreen(geom.Sz(800, 600))}
	w := newWindow(kw, nil)
	field := &box{size: geom.Sz(200, 40)}
	w.ui.Insert(w.ui.Root(), field)
	w.ui.Focus(field)
	w.Frame(time.Second / 60)
	if kw.asked != 0 {
		t.Fatalf("focus alone asked for the keyboard %d times, want none until a tap", kw.asked)
	}

	tap := func(p geom.Point) {
		w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: 1})
		w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary})
		w.Frame(time.Second / 60)
	}
	tap(geom.Pt(20, 20))
	if kw.asked != 1 {
		t.Fatalf("a tap on the focused field asked %d times, want once", kw.asked)
	}
	tap(geom.Pt(500, 500))
	if kw.asked != 1 {
		t.Fatalf("a tap beside the field asked for the keyboard, %d times in all", kw.asked)
	}
}
