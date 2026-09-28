package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// focusProbe returns what reports the node with the keyboard in w, the stage's window.
func focusProbe(t *testing.T, w *gunim.Window, run func(int)) func() gunim.Node {
	t.Helper()
	var focused gunim.Node
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, _ probeFocus, u *gunim.UI) { focused = u.Focused() })
	return func() gunim.Node {
		t.Helper()
		if err := w.Client().Patch("stage", probeFocus{}); err != nil {
			t.Fatal(err)
		}
		run(1)
		return focused
	}
}

type enable struct{}

func TestADisabledButtonTakesNoClickKeyOrFocus(t *testing.T) {
	a, b, c := NewButton("A"), NewButton("B"), NewButton("C")
	b.On = pressed{1}
	b.Disabled = true
	w, run := stage(t, &frame{child: Row(a, b, c), size: geom.Sz(400, 36)})
	focused := focusProbe(t, w, run)
	run(1)
	// B sits after A and the row's gap.
	mid := geom.Pt(a.size.W+8+b.size.W/2, 18)
	w.Input(input.PointerMove{Pos: mid})
	w.Input(input.PointerDown{Pos: mid, Clicks: 1})
	w.Input(input.PointerUp{Pos: mid})
	run(1)
	if n := len(sent(w)); n != 0 {
		t.Fatalf("clicked while disabled, the button sent %d intents", n)
	}
	tab(w, run, 0)
	tab(w, run, 0)
	if f := focused(); f != c {
		t.Fatalf("two Tabs put the keyboard on %T %p, want C past the disabled B", f, f)
	}
	if d := b.dim.Value(); d != 1 {
		t.Fatalf("the button made disabled is %v of the way faint, want all the way from the start", d)
	}

	// Enabled again, it fades back and takes the click.
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, _ enable, _ *gunim.UI) { b.Disabled = false })
	if err := w.Client().Patch("stage", enable{}); err != nil {
		t.Fatal(err)
	}
	run(2)
	if d := b.dim.Value(); d <= 0 || d >= 1 {
		t.Fatalf("a frame after enabling, the button is %v of the way faint, want part way back", d)
	}
	w.Input(input.PointerDown{Pos: mid, Clicks: 1})
	w.Input(input.PointerUp{Pos: mid})
	run(1)
	if n := len(sent(w)); n != 1 {
		t.Fatalf("clicked once enabled, the button sent %d intents, want 1", n)
	}
}

func TestAButtonThatKeepsFocusLeavesTheKeyboardOnAClick(t *testing.T) {
	field, b := NewTextField(), NewButton("Go")
	b.KeepFocus = true
	b.On = pressed{1}
	w, run := stage(t, &frame{child: Column(field, b), size: geom.Sz(300, 200)})
	focused := focusProbe(t, w, run)
	click(w, 20, 10)
	run(1)
	top := FieldHeight.Default() + Gap.Default()
	click(w, 20, top+10)
	run(1)
	if f := focused(); f != field {
		t.Fatalf("a click on the button moved the keyboard to %T, want it left in the field", f)
	}
	if got := sent(w); len(got) != 1 {
		t.Fatalf("the click sent %v, want the button's intent", got)
	}
	tab(w, run, 0)
	if f := focused(); f != b {
		t.Fatalf("Tab from the field put the keyboard on %T, want the button", f)
	}
}
