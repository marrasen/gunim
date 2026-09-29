package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
)

// twoLists are two lists that can be clicked, in a group, with a button after it.
type twoLists struct {
	Top, Bottom []Key
}

// groupStage is a group of two lists over a button, in a window, with what runs frames and reports the node with the
// keyboard.
type groupStage struct {
	w       *gunim.Window
	g       *Group
	a, b    *List
	after   *Button
	run     func(int)
	focused func() gunim.Node
}

// newGroupStage mounts a group of two lists of rows 40 tall, the first of top and the second of bottom, over a
// button.
func newGroupStage(t *testing.T, top, bottom int) groupStage {
	t.Helper()
	var (
		w       *gunim.Window
		g       *Group
		a, b    *List
		after   *Button
		run     func(int)
		focused func() gunim.Node
	)
	a, b = NewList(), NewList()
	for _, l := range []*List{a, b} {
		l.OnClick = func(k Key) gunim.Intent { return rowClicked{k} }
	}
	lists := Column(a, b)
	lists.Cross = CrossStretch
	g = NewGroup(Vertical, lists)
	after = NewButton("After")
	col := Column(g, after)
	col.Cross = CrossStretch
	w = gunimtest.New(t, geom.Sz(300, 600), nil)
	var has gunim.Node
	gunim.RegisterView(w, "g", func(twoLists) gunim.Node { return col },
		func(_ gunim.Node, s twoLists, u *gunim.UI) {
			for _, p := range []struct {
				l    *List
				keys []Key
			}{{a, s.Top}, {b, s.Bottom}} {
				Sync(p.l, u, p.keys, func(k Key) Key { return k },
					func(k Key) *block { return &block{key: k, h: 40} }, nil)
			}
		})
	gunim.RegisterPatch(w, "g", func(_ gunim.Node, _ probeFocus, u *gunim.UI) { has = u.Focused() })
	named := func(prefix string, n int) []Key {
		out := make([]Key, n)
		for i := range out {
			out[i] = Key(prefix + string(rune('0'+i)))
		}
		return out
	}
	if err := w.Client().Mount(gunim.Root, "g", "g", twoLists{named("a", top), named("b", bottom)}); err != nil {
		t.Fatal(err)
	}
	run = func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(60)
	focused = func() gunim.Node {
		t.Helper()
		if err := w.Client().Patch("g", probeFocus{}); err != nil {
			t.Fatal(err)
		}
		run(1)
		return has
	}
	return groupStage{w, g, a, b, after, run, focused}
}

func TestAGroupIsOneTabStopAndTheArrowsWalkThroughIt(t *testing.T) {
	st := newGroupStage(t, 2, 2)
	w, g, a, b, after, run, focused := st.w, st.g, st.a, st.b, st.after, st.run, st.focused
	press := func(k input.Key, mods input.Mods) {
		w.Input(input.KeyPress{Key: k, Mods: mods})
		w.Input(input.KeyRelease{Key: k, Mods: mods})
		run(20)
	}
	press(input.KeyTab, 0)
	if f := focused(); f != a {
		t.Fatalf("Tab put the keyboard on %T, want the group's first list", f)
	}
	if g.ring.Value() < 0.9 {
		t.Fatal("the group shows no ring with the keyboard in it")
	}
	// Down past the first list's last row goes on to the second list's first
	press(input.KeyDown, 0)
	press(input.KeyDown, 0)
	if f := focused(); f != b {
		t.Fatalf("Down past the first list put the keyboard on %T, want the second list", f)
	}
	if k, _ := b.Cursor(); k != "b0" {
		t.Fatalf("the second list's cursor is on %q, want its first row", k)
	}
	// Up from there goes back to the first list's last row
	press(input.KeyUp, 0)
	if k, _ := a.Cursor(); focused() != a || k != "a1" {
		t.Fatalf("Up from the second list left the keyboard on %T, row %q; want the first list's last row", focused(), k)
	}
	// Tab leaves the group, and Shift+Tab comes back to the list it left
	press(input.KeyDown, 0)
	press(input.KeyTab, 0)
	if focused() != after {
		t.Fatal("Tab did not leave the group for the button after it")
	}
	press(input.KeyTab, input.ModShift)
	if f := focused(); f != b {
		t.Fatalf("Shift+Tab back into the group put the keyboard on %T, want the list it left", f)
	}
}

func TestOnlyTabShowsTheRingsAndTheArrowsShowTheCursor(t *testing.T) {
	st := newGroupStage(t, 2, 2)
	w, g, a, run, focused := st.w, st.g, st.a, st.run, st.focused
	// The first list's first row is at the top
	clickAt(w, run, geom.Pt(100, 20))
	run(20)
	if focused() != a {
		t.Fatal("a click on a row did not give its list the keyboard")
	}
	if g.ring.Value() > 0.01 || a.ring.Value() > 0.01 {
		t.Fatal("a click showed a focus ring")
	}
	// After a click the arrows show the list's cursor, and no ring
	w.Input(input.KeyPress{Key: input.KeyDown})
	run(20)
	if g.ring.Value() > 0.01 || a.ring.Value() > 0.01 {
		t.Fatal("an arrow key showed a focus ring")
	}
	if a.cursorShown() < 0.9 {
		t.Fatal("the arrow key moved the list's cursor out of sight")
	}
	// Tab out and back in shows the group's ring
	w.Input(input.KeyPress{Key: input.KeyTab, Mods: input.ModShift})
	w.Input(input.KeyPress{Key: input.KeyTab})
	run(20)
	if focused() != a || g.ring.Value() < 0.9 {
		t.Fatal("Tab back into the group did not show its ring")
	}
	if a.whole {
		t.Fatal("a list in a group draws its own ring round all of it, as well as the group's")
	}
}

// A button the arrows walk to in a group lights, with no ring of its
// own; Tab into the group rings the group, and the button lights.
func TestAButtonTheArrowsReachLightsWithoutARing(t *testing.T) {
	one, two := NewButton("OK"), NewButton("Cancel")
	g := NewGroup(Horizontal, Row(one, two))
	before := NewButton("Before")
	w, run := stage(t, Column(before, g))
	var u *gunim.UI
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, _ probeFocus, ui *gunim.UI) { u = ui })
	if err := w.Client().Patch("stage", probeFocus{}); err != nil {
		t.Fatal(err)
	}
	run(2)
	u.Focus(one)
	run(20)
	w.Input(input.KeyPress{Key: input.KeyRight})
	run(20)
	if u.Focused() != two || two.walked.Value() < 0.9 || two.ring.Value() > 0.01 {
		t.Fatalf("Right put the keyboard on %T, lit %v, ring %v", u.Focused(), two.walked.Value(), two.ring.Value())
	}
	run(40)
	if one.walked.Value() > 0.01 {
		t.Fatal("the button left stays lit")
	}
	u.Focus(before)
	run(2)
	w.Input(input.KeyPress{Key: input.KeyTab})
	run(20)
	if f := u.Focused(); f != two || g.ring.Value() < 0.9 || two.ring.Value() > 0.01 || two.walked.Value() < 0.9 {
		t.Fatalf("Tab into the group: on %T, group ring %v, button ring %v, lit %v", f, g.ring.Value(), two.ring.Value(), two.walked.Value())
	}
}

// A button in a group given the keyboard by the application, not the
// arrows, is lit as the group's selected one; one on its own is not.
func TestAButtonGivenTheKeyboardInAGroupLights(t *testing.T) {
	one, two := NewButton("OK"), NewButton("Cancel")
	g := NewGroup(Horizontal, Row(one, two))
	alone := NewButton("Alone")
	w, run := stage(t, Column(alone, g))
	var u *gunim.UI
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, _ probeFocus, ui *gunim.UI) { u = ui })
	if err := w.Client().Patch("stage", probeFocus{}); err != nil {
		t.Fatal(err)
	}
	run(2)
	u.Focus(one)
	run(20)
	if one.walked.Value() < 0.9 || one.ring.Value() > 0.01 {
		t.Fatalf("given the keyboard in a group, the button is lit %v, ring %v", one.walked.Value(), one.ring.Value())
	}
	u.Focus(alone)
	run(20)
	if alone.walked.Value() > 0.01 {
		t.Fatal("a button on its own given the keyboard is lit")
	}
}
