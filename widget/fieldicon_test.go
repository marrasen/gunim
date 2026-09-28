package widget

import (
	"slices"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
)

// iconsDrawn returns the icons in the window's last frame, in drawing order.
func iconsDrawn(t *testing.T, w *gunim.Window) []*icon.Icon {
	t.Helper()
	ms := maskOps(w.Offscreen())
	out := make([]*icon.Icon, 0, len(ms))
	for _, m := range ms {
		out = append(out, strokeOf(t, m).Icon)
	}
	return out
}

func TestAFieldShowsItsIconAndPutsTheCaretAfterIt(t *testing.T) {
	ty := newTyperWith(t, func(f *TextField) { f.Icon = icon.Search })
	ms := maskOps(ty.w.Offscreen())
	if len(ms) != 1 || strokeOf(t, ms[0]).Icon != icon.Search {
		t.Fatalf("the field drew %d icons, want its search icon", len(ms))
	}
	pad, s, gap := FieldPadding.Default(), IconSize.Default(), IconGap.Default()
	if r := ms[0].Rect; r != geom.Rc(pad, (36-s)/2, s, s) {
		t.Errorf("the icon is at %v, want at the field's start", r)
	}
	if ms[0].Color != Placeholder.Default() {
		t.Errorf("the icon is %v, want the placeholder's colour", ms[0].Color)
	}
	if x := ty.field.TextCaret().Min.X; x != pad+s+gap {
		t.Fatalf("the caret is at %v, want %v, after the icon", x, pad+s+gap)
	}
	ty.typeText("hello")
	ty.run(30)
	// A click just after the icon puts the caret before the first letter, and one on the icon too.
	for _, x := range []float32{pad + s + gap + 1, pad + 2} {
		click(ty.w, x, 18)
		ty.run(1)
		ty.want("hello", 0)
		ty.key(input.KeyEnd, 0)
	}
	ty.run(30)
	run := ty.field.line
	if got, want := ty.field.TextCaret().Min.X, pad+s+gap+run.Advance; got-want > 0.5 || want-got > 0.5 {
		t.Fatalf("at the end the caret is at %v, want %v", got, want)
	}
}

func TestAClearableFieldShowsAnXThatEmptiesIt(t *testing.T) {
	ty := newTyperWith(t, func(f *TextField) { f.Clearable = true })
	if got := iconsDrawn(t, ty.w); len(got) != 0 {
		t.Fatalf("an empty field drew %d icons, want no X", len(got))
	}
	ty.typeText("abc")
	ty.run(30)
	ms := maskOps(ty.w.Offscreen())
	if len(ms) != 1 || strokeOf(t, ms[0]).Icon != icon.X {
		t.Fatalf("with text the field drew %d icons, want an X", len(ms))
	}
	x := ms[0].Rect.Center()
	if x.X < 300-FieldPadding.Default()-IconSize.Default() {
		t.Fatalf("the X is at %v, want at the field's end", x)
	}
	ty.intents()
	click(ty.w, x.X, x.Y)
	ty.run(1)
	ty.want("", 0)
	if got := ty.intents(); !slices.Contains(got, gunim.Intent(changed{""})) {
		t.Fatalf("clearing sent %v, want a change to nothing", got)
	}
	ty.typeText("d")
	ty.want("d", 1)
	ty.key(input.KeyEscape, 0)
	ty.want("", 0)
	if ty.fr.keys != 0 {
		t.Fatal("the Escape that cleared the field went on past it")
	}
	ty.key(input.KeyEscape, 0)
	if ty.fr.keys != 1 {
		t.Fatal("an Escape in an empty field stayed with it, want it passed on")
	}
	ty.typeText("e")
	ty.want("e", 1)
	ty.key(input.KeyBackspace, 0)
	ty.run(40)
	if got := iconsDrawn(t, ty.w); len(got) != 0 {
		t.Fatalf("emptied, the field still draws %v", got)
	}
}

func TestTheXAppearsOnASpring(t *testing.T) {
	ty := newTyperWith(t, func(f *TextField) { f.Clearable = true })
	ty.typeText("abc")
	ty.run(1)
	ms := maskOps(ty.w.Offscreen())
	if len(ms) != 1 {
		t.Fatalf("a frame after typing, the field drew %d icons, want the X arriving", len(ms))
	}
	if m := ms[0]; m.Transform.A >= 1 || m.Color.A == 0xff {
		t.Fatalf("a frame in, the X is scaled %v and %v opaque, want it growing and fading in", m.Transform.A, m.Color.A)
	}
	ty.run(60)
	if m := maskOps(ty.w.Offscreen())[0]; m.Transform.A != 1 || m.Color.A != Placeholder.Default().A {
		t.Fatalf("settled, the X is scaled %v at %v", m.Transform.A, m.Color)
	}
}

func TestTextScrollsClearOfTheIconAndTheX(t *testing.T) {
	ty := newTyperWith(t, func(f *TextField) { f.Icon, f.Clearable = icon.Search, true })
	ty.typeText("a long line of text that runs well past the end of the field")
	ty.run(60)
	pad, s, gap := FieldPadding.Default(), IconSize.Default(), IconGap.Default()
	if x := ty.field.TextCaret().Min.X; x > 300-pad-s-gap+0.5 || x < pad+s+gap {
		t.Fatalf("at the end of a long line the caret is at %v, under the X", x)
	}
	ty.key(input.KeyHome, 0)
	ty.run(60)
	if x := ty.field.TextCaret().Min.X; x != pad+s+gap {
		t.Fatalf("at the start the caret is at %v, want %v", x, pad+s+gap)
	}
}
