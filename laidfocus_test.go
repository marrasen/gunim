package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// TestAnEditForTheTextBeforeIsTypedInTheNext: an edit a keyboard made of the text that had the focus, as a page is
// shown, types in the page what it types, and nothing of it changes the page's own text elsewhere.
func TestAnEditForTheTextBeforeIsTypedInTheNext(t *testing.T) {
	sw := &stateWindow{OffscreenWindow: driver.Offscreen(geom.Sz(800, 600))}
	w := newWindow(sw, nil)
	old, next := &box{size: geom.Sz(200, 40)}, &typedIn{box{size: geom.Sz(200, 40)}}
	old.text = "long before"
	w.ui.Insert(w.ui.Root(), old)
	w.ui.Insert(w.ui.Root(), next)
	w.Frame(time.Second / 60)
	w.ui.Focus(old)
	w.ui.FocusFirstLaidOut(next)
	w.Input(input.TextEdit{Seq: 1, Replace: [2]int{11, 11}, With: "a", Selection: [2]int{12, 12}})
	w.Input(input.TextEdit{Seq: 2, Replace: [2]int{4, 5}, Selection: [2]int{4, 4}})
	w.Input(input.TextEdit{Seq: 3, Replace: [2]int{12, 12}, With: "b", Selection: [2]int{13, 13}})
	if old.text != "long before" || next.text != "" {
		t.Fatalf("before a frame, the texts are %q and %q", old.text, next.text)
	}
	w.Frame(time.Second / 60)
	w.Frame(time.Second / 60)
	if old.text != "long before" || next.text != "ab" {
		t.Errorf("the texts are %q and %q, want the two letters in the next", old.text, next.text)
	}
	// The driver's copy is level with the edits it made.
	if got := sw.take(); len(got) == 0 || got[len(got)-1].seq != 3 || got[len(got)-1].state.Text != "ab" {
		t.Errorf("the driver heard %+v, want the next's text as of the third edit last", got)
	}
}

// typedIn is a box that takes typed text too.
type typedIn struct{ box }

func (n *typedIn) Handle(e input.Event, u *UI) bool {
	if ti, ok := e.(input.TextInput); ok {
		n.text += ti.Text
	}
	return n.box.Handle(e, u)
}

// TestKeysWaitForOneFrameAlone: for a node no frame lays out, they go where the keyboard is.
func TestKeysWaitForOneFrameAlone(t *testing.T) {
	w := NewOffscreen(geom.Sz(800, 600), nil)
	old, held, hidden := &box{size: geom.Sz(200, 40)}, &hides{}, &box{size: geom.Sz(200, 40)}
	w.ui.Insert(w.ui.Root(), old)
	w.ui.Insert(w.ui.Root(), held)
	w.ui.Insert(held, hidden)
	w.Frame(time.Second / 60)
	w.ui.Focus(old)
	w.ui.FocusFirstLaidOut(hidden)
	w.Input(input.KeyPress{Key: input.KeyA})
	if old.got(input.KeyPress{}) {
		t.Fatal("the key did not wait for the frame")
	}
	w.Frame(time.Second / 60)
	if !old.got(input.KeyPress{}) || hidden.got(input.KeyPress{}) {
		t.Fatal("the frame did not hand the key to the node with the keyboard")
	}
	old.events = nil
	w.Input(input.KeyPress{Key: input.KeyA})
	if !old.got(input.KeyPress{}) {
		t.Error("a key after the frame waited too")
	}
	// A press comes after the keys before it.
	w.ui.FocusFirstLaidOut(held)
	old.events = nil
	w.Input(input.KeyPress{Key: input.KeyA})
	w.Input(input.PointerDown{Pos: geom.Pt(700, 500), Button: input.ButtonPrimary, Clicks: 1})
	if !old.got(input.KeyPress{}) {
		t.Error("a press came before the key before it")
	}
}

// hides is a node that lays out none of its children.
type hides struct{ recorder }

func (*hides) Layout(Constraints, Frame, Children) geom.Size { return geom.Size{} }
