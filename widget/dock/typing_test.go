package dock

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// typist is a pane with two places to type, which keep what is typed in them.
type typist struct{ one, two *typing }

func newTypist() *typist { return &typist{&typing{}, &typing{}} }

func (p *typist) Children() []gunim.Node { return []gunim.Node{p.one, p.two} }

func (p *typist) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	for i := range 2 {
		k := kids.At(i)
		k.Layout(gunim.Tight(geom.Sz(c.Max.W, c.Max.H/2)))
		k.Place(geom.Pt(0, float32(i)*c.Max.H/2))
	}
	return c.Max
}

func (p *typist) Paint(pt *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for k := range kids.All {
		k.Paint(pt)
	}
}

// typing is a place to type.
type typing struct {
	pane
	said string
}

func (p *typing) Focusable() bool { return true }
func (p *typing) TakesText() bool { return true }

func (p *typing) Handle(e input.Event, _ *gunim.UI) bool {
	switch e := e.(type) {
	case input.TextInput:
		p.said += e.Text
		return true
	case input.KeyPress:
		return e.Mods == 0
	}
	return false
}

// typingDock shows a dock of typists, with the keyboard in a's second place.
func typingDock(t *testing.T, l Layout) (d *Dock, at map[string]*typist, w *gunim.Window, run func(int)) {
	t.Helper()
	at = map[string]*typist{}
	d = NewDock(0)
	d.Keys = true
	d.Content = func(item string) gunim.Node {
		at[item] = newTypist()
		return at[item]
	}
	w, run, _ = stage(t, d, l)
	run(30)
	r := d.items["a"]
	click(w, run, geom.Pt(r.Center().X, r.Max.Y-5), input.ButtonPrimary)
	say(w, "0")
	if at["a"].two.said != "0" {
		t.Fatal("a click in a pane does not give it the keyboard")
	}
	return d, at, w, run
}

// say types s.
func say(w *gunim.Window, s string) {
	w.Input(input.KeyPress{Key: input.KeyA})
	w.Input(input.TextInput{Text: s})
	w.Input(input.KeyRelease{Key: input.KeyA})
}

// TestWhatIsTypedRightAfterATabIsPickedIsItsPanes: with no frame between, and nothing is lost as the tab is pressed.
func TestWhatIsTypedRightAfterATabIsPickedIsItsPanes(t *testing.T) {
	d, at, w, run := typingDock(t, NewLayout(true, "a", "b", "c"))
	p := tabAt(t, d, "b")
	w.Input(input.PointerMove{Pos: p})
	w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: 1})
	say(w, "1")
	run(1)
	say(w, "2")
	w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary})
	say(w, "3")
	say(w, "4")
	run(1)
	say(w, "5")
	if a, b := at["a"].two.said, at["b"].one.said; a != "012" || b != "345" {
		t.Fatalf("typing across a click, a got %q and b %q", a, b)
	}
	// The tab in front leaves the keyboard where it is in its pane.
	r := d.items["b"]
	click(w, run, geom.Pt(r.Center().X, r.Max.Y-5), input.ButtonPrimary)
	click(w, run, p, input.ButtonPrimary)
	say(w, "6")
	run(1)
	if b := at["b"].two.said; b != "6" {
		t.Errorf("after a click on the tab in front, its pane got %q", b)
	}
	// With the keys, one after another.
	next := input.KeyPress{Key: input.KeyPageDown, Mods: input.ModControl}
	w.Input(next)
	say(w, "7")
	w.Input(next)
	say(w, "8")
	w.Input(input.KeyPress{Key: input.KeyPageUp, Mods: input.ModControl})
	say(w, "9")
	for range 4 {
		run(1)
	}
	if c, a := at["c"].one.said, at["a"].one.said; c != "79" || a != "8" {
		t.Errorf("typing as the keys step, c got %q and a %q, want 79 and 8", c, a)
	}
}

// TestATabPickedOnTheWayToAnotherKeepsTheKeyboard: there and back between two frames, and the other tab takes it only
// once it is picked again.
func TestATabPickedOnTheWayToAnotherKeepsTheKeyboard(t *testing.T) {
	d, at, w, run := typingDock(t, NewLayout(true, "a", "b"))
	for _, id := range []string{"b", "a"} {
		p := tabAt(t, d, id)
		w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: 1})
		w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary})
	}
	say(w, "1")
	run(1)
	say(w, "2")
	if got := at["a"].two.said; got != "012" {
		t.Fatalf("a got %q, want 012", got)
	}
	// Shown without the keyboard, b leaves it where it is.
	l := d.Arrangement()
	l.Show("b")
	d.SetLayout(l, nil)
	run(5)
	say(w, "3")
	if got := at["b"]; got != nil && got.one.said != "" {
		t.Errorf("b took the keyboard later, and got %q", got.one.said)
	}
}

// TestWhatIsTypedRightAfterCtrlTabIsTheNextPanes: in another group.
func TestWhatIsTypedRightAfterCtrlTabIsTheNextPanes(t *testing.T) {
	d, at, w, run := typingDock(t, splitLayout())
	w.Input(input.KeyPress{Key: input.KeyTab, Mods: input.ModControl})
	say(w, "1")
	run(1)
	say(w, "2")
	if got := at[d.Active()].one.said; d.Active() == "a" || got != "12" {
		t.Errorf("%s got %q, want 12", d.Active(), got)
	}
}

// TestWhatIsTypedAsThePaneGoesIsTheNextPanes: the view with the keyboard leaves as the tab is picked.
func TestWhatIsTypedAsThePaneGoesIsTheNextPanes(t *testing.T) {
	d, at, w, run := typingDock(t, NewLayout(true, "a", "b"))
	d.OnChange = func(c Change, u *gunim.UI) gunim.Intent {
		if _, ok := c.(Show); ok {
			u.Remove(at["a"])
		}
		return nil
	}
	p := tabAt(t, d, "b")
	w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary})
	say(w, "1")
	run(1)
	say(w, "2")
	if got := at["b"].one.said; got != "12" {
		t.Errorf("b got %q, want 12", got)
	}
}
