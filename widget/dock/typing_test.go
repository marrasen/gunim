package dock

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
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

// keyed is a program's root, which takes the keyboard itself, over a toolbar with a place to type and a dock.
type keyed struct {
	typing
	focusable bool
	tools     *typing
	d         *Dock
}

func (k *keyed) Focusable() bool        { return k.focusable }
func (k *keyed) Children() []gunim.Node { return []gunim.Node{k.tools, k.d} }

func (k *keyed) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	tools, d := kids.At(0), kids.At(1)
	tools.Layout(gunim.Tight(geom.Sz(c.Max.W, 40)))
	tools.Place(geom.Point{})
	d.Layout(gunim.Tight(geom.Sz(c.Max.W, c.Max.H-40)))
	d.Place(geom.Pt(0, 40))
	return c.Max
}

func (k *keyed) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for kid := range kids.All {
		kid.Paint(p)
	}
}

// dull is a pane with nothing to put the keyboard on.
type dull struct{ pane }

func (*dull) Focusable() bool { return false }

// keyedDock shows a root over a dock of a, in front, and of b and c, which have nothing to put the keyboard on.
func keyedDock(t *testing.T, focusable bool, l Layout) (k *keyed, a *typist, w *gunim.Window, run func(int)) {
	t.Helper()
	a = newTypist()
	d := NewDock(0)
	d.Keys = true
	d.Content = func(item string) gunim.Node {
		if item == "a" {
			return a
		}
		return &dull{}
	}
	k = &keyed{focusable: focusable, tools: &typing{}, d: d}
	w = gunimtest.New(t, geom.Sz(800, 500), nil)
	w.PaintWhenLooked(false)
	gunim.RegisterView(w, "v", func(Layout) *keyed { return k }, func(k *keyed, l Layout, u *gunim.UI) { k.d.SetLayout(l, u) })
	gunim.RegisterPatch(w, "v", func(_ *keyed, call uiCall, u *gunim.UI) { call(u) })
	if err := w.Client().Mount(gunim.Root, "v", "v", l, "v"); err != nil {
		t.Fatal(err)
	}
	run = func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(30)
	return k, a, w, run
}

// TestAPickedPaneWithNothingToFocusLeavesTheKeyboardAboveTheDock: with the keyboard nowhere, in a toolbar and in a
// pane, by a click and by the keys, where what is above the dock takes it and where nothing does.
func TestAPickedPaneWithNothingToFocusLeavesTheKeyboardAboveTheDock(t *testing.T) {
	for _, focusable := range []bool{true, false} {
		for _, from := range []string{"nowhere", "the toolbar", "a pane", "above"} {
			for _, keys := range []bool{false, true} {
				k, a, w, run := keyedDock(t, focusable, NewLayout(true, "a", "b", "c"))
				on := map[string]gunim.Node{"nowhere": nil, "the toolbar": k.tools, "a pane": a.two, "above": k}[from]
				if from == "above" && !focusable || from != "a pane" && keys {
					continue
				}
				onUIDock(t, w, run, func(u *gunim.UI) { u.Focus(on) })
				p := tabAt(t, k.d, "b").Add(geom.Pt(0, 40))
				if keys {
					w.Input(input.KeyPress{Key: input.KeyPageDown, Mods: input.ModControl})
				} else {
					w.Input(input.PointerMove{Pos: p})
					w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: 1})
					w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary})
				}
				run(3)
				var focused gunim.Node
				onUIDock(t, w, run, func(u *gunim.UI) { focused = u.Focused() })
				var want gunim.Node
				if focusable {
					want = k
				}
				if focused != want || k.d.Active() != "b" {
					t.Errorf("focusable above %v, from %s, keys %v: the keyboard is on %T, want %T, and %s is picked", focusable, from, keys, focused, want, k.d.Active())
				}
				// One with something to focus takes it, from there.
				p = tabAt(t, k.d, "a").Add(geom.Pt(0, 40))
				w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: 1})
				w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary})
				say(w, "1")
				run(2)
				if a.one.said != "1" {
					t.Errorf("focusable above %v, from %s: back on a, it got %q", focusable, from, a.one.said)
				}
			}
		}
	}
}

// TestAPressOnATabMovesTheKeyboardOnlyFromOutsideThePanes: until its release.
func TestAPressOnATabMovesTheKeyboardOnlyFromOutsideThePanes(t *testing.T) {
	k, a, w, run := keyedDock(t, true, NewLayout(true, "a", "b", "c"))
	p := tabAt(t, k.d, "b").Add(geom.Pt(0, 40))
	for _, c := range []struct{ from, want gunim.Node }{{nil, k}, {k.tools, k}, {k, k}, {a.two, a.two}} {
		onUIDock(t, w, run, func(u *gunim.UI) { u.Focus(c.from) })
		w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: 1})
		run(1)
		var focused gunim.Node
		onUIDock(t, w, run, func(u *gunim.UI) { focused = u.Focused() })
		if focused != c.want {
			t.Errorf("with the keyboard on %T, a press on a tab has it on %T, want %T", c.from, focused, c.want)
		}
		// Let go elsewhere, it picks nothing.
		w.Input(input.PointerUp{Pos: geom.Pt(400, 400), Button: input.ButtonPrimary})
		run(1)
	}
}

// TestAPickedPaneWithNothingToFocusTakesTheKeyboardFromThePaneBeside: which shows still, and is typed in no more.
func TestAPickedPaneWithNothingToFocusTakesTheKeyboardFromThePaneBeside(t *testing.T) {
	for _, focusable := range []bool{true, false} {
		for _, step := range []bool{true, false} {
			k, a, w, run := keyedDock(t, focusable, splitLayout())
			r := k.d.items["a"]
			click(w, run, geom.Pt(r.Center().X, r.Max.Y+30), input.ButtonPrimary)
			say(w, "1")
			if a.two.said != "1" {
				t.Fatal("a click in a pane does not give it the keyboard")
			}
			if step {
				w.Input(input.KeyPress{Key: input.KeyTab, Mods: input.ModControl})
			} else {
				p := tabAt(t, k.d, "b").Add(geom.Pt(0, 40))
				w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: 1})
				w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary})
			}
			say(w, "2")
			run(3)
			say(w, "3")
			want := "23"
			if !focusable {
				want = ""
			}
			if a.two.said != "1" || k.d.Active() != "b" || k.said != want {
				t.Errorf("focusable above %v, with the keys %v: the pane got %q and what is above %q, want 1 and %q", focusable, step, a.two.said, k.said, want)
			}
		}
	}
}
