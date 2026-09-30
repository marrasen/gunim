package widget

import (
	"image/color"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

type pressed struct{ N int }

func TestAButtonFiresOnlyWhenReleasedOverIt(t *testing.T) {
	b := NewButton("Go")
	b.On = pressed{1}
	w, run := stage(t, &frame{child: b, size: geom.Sz(100, 36)})
	drain := func() int {
		n := 0
		for {
			select {
			case <-w.Client().Intents():
				n++
			default:
				return n
			}
		}
	}

	w.Input(input.PointerDown{Pos: geom.Pt(20, 18)})
	w.Input(input.PointerMove{Pos: geom.Pt(300, 18)})
	w.Input(input.PointerUp{Pos: geom.Pt(300, 18)})
	run(1)
	if n := drain(); n != 0 {
		t.Fatalf("released outside, the button fired %d times", n)
	}

	w.Input(input.PointerDown{Pos: geom.Pt(20, 18)})
	w.Input(input.PointerMove{Pos: geom.Pt(300, 18)})
	w.Input(input.PointerMove{Pos: geom.Pt(30, 18)})
	w.Input(input.PointerUp{Pos: geom.Pt(30, 18)})
	run(1)
	if n := drain(); n != 1 {
		t.Fatalf("dragged out and back, then released over it, the button fired %d times, want 1", n)
	}
}

func tab(w *gunim.Window, run func(int), mods input.Mods) {
	w.Input(input.KeyPress{Key: input.KeyTab, Mods: mods})
	run(1)
}

func TestTabVisitsFocusableNodesInOrder(t *testing.T) {
	a, field, c := NewButton("A"), NewTextField(), NewButton("C")
	label := NewLabel("not focusable")
	col := Column(a, label, field, c)
	var focused gunim.Node
	w, run := stage(t, &frame{child: col, size: geom.Sz(300, 400)})
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, _ probeFocus, u *gunim.UI) { focused = u.Focused() })
	check := func(want gunim.Node) {
		t.Helper()
		if err := w.Client().Patch("stage", probeFocus{}); err != nil {
			t.Fatal(err)
		}
		run(1)
		if focused != want {
			t.Fatalf("focus is on %T %p, want %T %p", focused, focused, want, want)
		}
	}

	tab(w, run, 0)
	check(a)
	tab(w, run, 0)
	check(field) // the label is skipped
	tab(w, run, 0)
	check(c) // Tab passes through the field
	tab(w, run, 0)
	check(a) // and wraps
	tab(w, run, input.ModShift)
	check(c)

	// A click on something that takes no focus drops it.
	w.Input(input.PointerDown{Pos: geom.Pt(250, 390)})
	check(nil)
}

type probeFocus struct{}

func TestTabBringsAFocusedNodeIntoView(t *testing.T) {
	buttons := make([]gunim.Node, 0, 20)
	for range 20 {
		buttons = append(buttons, NewButton("Row"))
	}
	sc := NewScroll(Column(buttons...))
	w, run := stage(t, &frame{child: sc, size: geom.Sz(200, 200)})
	for range 10 {
		tab(w, run, 0)
	}
	run(120)
	// The tenth button's top is 9 * (36 + 8) = 396 down the content;
	// its bottom must now show within 200.
	if off := sc.Offset(); off < 396+36-200 || off > 396 {
		t.Fatalf("offset %v; the tenth button, at 396..432, is out of view", off)
	}
}

func TestADialogKeepsTabAmongItsButtons(t *testing.T) {
	d := NewDialog("Sure?")
	w, run := stage(t, d)
	var focused gunim.Node
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, _ probeFocus, u *gunim.UI) { focused = u.Focused() })
	check := func(want gunim.Node) {
		t.Helper()
		if err := w.Client().Patch("stage", probeFocus{}); err != nil {
			t.Fatal(err)
		}
		run(1)
		if focused != want {
			t.Fatalf("focus is on %T %p, want %T %p", focused, focused, want, want)
		}
	}
	if err := w.Client().Focus("stage"); err != nil {
		t.Fatal(err)
	}
	run(30)
	tab(w, run, 0)
	check(d.cancel)
	tab(w, run, 0)
	check(d.ok)
	tab(w, run, 0)
	check(d.cancel)

	// A click on the panel's empty space keeps focus in the dialog, on
	// the dialog itself, where Escape reaches it.
	w.Input(input.PointerDown{Pos: geom.Pt(400, 300)})
	check(d)
}

// themeProbe records the Ink it is drawn with and handles keys with.
type themeProbe struct {
	spot
	drawn, handled color.NRGBA
}

func (p *themeProbe) Paint(pt *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	p.drawn = Ink.Get(f.Theme)
}

func (p *themeProbe) Handle(e input.Event, u *gunim.UI) bool {
	p.handled = Ink.Get(u.Theme())
	return true
}

func (p *themeProbe) Focusable() bool { return true }

func TestThemedGivesItsSubtreeItsOwnTheme(t *testing.T) {
	red := color.NRGBA{R: 0xff, A: 0xff}
	inside, outside := &themeProbe{spot: spot{size: geom.Sz(50, 20)}}, &themeProbe{spot: spot{size: geom.Sz(50, 20)}}
	col := Column(NewThemed(inside, theme.Make("red", theme.Set(Ink, red))), outside)
	w, run := stage(t, &frame{child: col, size: geom.Sz(300, 300)})
	run(1)
	if inside.drawn != red || outside.drawn != Ink.Default() {
		t.Fatalf("drawn with %v inside and %v outside, want red and the default", inside.drawn, outside.drawn)
	}
	w.Input(input.PointerDown{Pos: geom.Pt(10, 10)})
	if inside.handled != red {
		t.Fatalf("handled with %v inside, want red", inside.handled)
	}
}

func TestFocusingADialogFromCodeLeavesTheKeyboardInItsField(t *testing.T) {
	host := &frame{child: NewSpacer(), size: geom.Sz(800, 600)}
	w, run := stage(t, host)
	field := NewTextField()
	d := NewDialog("Go to a folder")
	d.Body = NewForm().Add("Folder", field)
	type open struct{}
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, _ open, u *gunim.UI) {
		// As an application does that inserts a dialog and then focuses it
		u.Insert(host, d)
		u.Focus(d)
	})
	if err := w.Client().Patch("stage", open{}); err != nil {
		t.Fatal(err)
	}
	run(5)
	w.Input(input.TextInput{Text: "rd"})
	run(1)
	if field.Text() != "rd" {
		t.Fatalf("typing reached the field as %q, want the dialog to have left the keyboard in it", field.Text())
	}
}
