package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

type (
	changed   struct{ Text string }
	submitted struct{ Text string }
)

// typer is a text field in an offscreen window, focused and ready.
type typer struct {
	t     *testing.T
	w     *gunim.Window
	run   func(int)
	field *TextField
	fr    *frame
}

func newTyper(t *testing.T) *typer {
	t.Helper()
	field := NewTextField()
	field.OnChange = func(s string) gunim.Intent { return changed{s} }
	field.OnSubmit = func(s string) gunim.Intent { return submitted{s} }
	fr := &frame{child: field, size: geom.Sz(300, 36)}
	w, run := stage(t, fr)
	w.Input(input.PointerDown{Pos: geom.Pt(20, 18), Clicks: 1})
	w.Input(input.PointerUp{Pos: geom.Pt(20, 18)})
	run(1)
	return &typer{t: t, w: w, run: run, field: field, fr: fr}
}

func (ty *typer) typeText(s string) {
	ty.w.Input(input.TextInput{Text: s})
	ty.run(1)
}

func (ty *typer) key(k input.Key, mods input.Mods) {
	ty.w.Input(input.KeyPress{Key: k, Mods: mods})
	ty.run(1)
}

func (ty *typer) want(text string, caret int) {
	ty.t.Helper()
	if got := ty.field.Text(); got != text {
		ty.t.Fatalf("text %q, want %q", got, text)
	}
	if ty.field.caret != caret {
		ty.t.Fatalf("caret at %d, want %d", ty.field.caret, caret)
	}
}

// intents drains the window's intents.
func (ty *typer) intents() []gunim.Intent {
	var out []gunim.Intent
	for {
		select {
		case e := <-ty.w.Client().Intents():
			out = append(out, e.Intent)
		default:
			return out
		}
	}
}

func TestTypingEditsTheText(t *testing.T) {
	ty := newTyper(t)
	ty.typeText("hello")
	ty.want("hello", 5)
	ty.key(input.KeyBackspace, 0)
	ty.want("hell", 4)
	ty.key(input.KeyLeft, 0)
	ty.key(input.KeyLeft, 0)
	ty.typeText("X")
	ty.want("heXll", 3)
	ty.key(input.KeyDelete, 0)
	ty.want("heXl", 3)
	ty.key(input.KeyEnd, 0)
	ty.want("heXl", 4)
}

func TestShiftSelectsAndTypingReplacesTheSelection(t *testing.T) {
	ty := newTyper(t)
	ty.typeText("hello world")
	ty.key(input.KeyHome, input.ModShift)
	if s, e := ty.field.Selection(); s != 0 || e != 11 {
		t.Fatalf("selection %d..%d, want all of it", s, e)
	}
	ty.typeText("bye")
	ty.want("bye", 3)
}

func TestCtrlMovesAndDeletesByWord(t *testing.T) {
	ty := newTyper(t)
	ty.typeText("one two three")
	ty.key(input.KeyLeft, input.ModControl)
	ty.want("one two three", 8)
	ty.key(input.KeyLeft, input.ModControl)
	ty.want("one two three", 4)
	ty.key(input.KeyBackspace, input.ModControl)
	ty.want("two three", 0)
	ty.key(input.KeyRight, input.ModControl)
	ty.want("two three", 3)
}

func TestClipboardCopiesCutsAndPastes(t *testing.T) {
	ty := newTyper(t)
	ty.typeText("abc")
	ty.key(input.KeyA, input.ModControl)
	ty.key(input.KeyC, input.ModControl)
	ty.key(input.KeyEnd, 0)
	ty.key(input.KeyV, input.ModControl)
	ty.want("abcabc", 6)
	ty.key(input.KeyHome, input.ModShift)
	ty.key(input.KeyX, input.ModControl)
	ty.want("", 0)
	ty.key(input.KeyV, input.ModControl)
	ty.want("abcabc", 6)
}

func TestChangesAndSubmitReachTheApplication(t *testing.T) {
	ty := newTyper(t)
	ty.intents()
	ty.typeText("hi")
	ty.key(input.KeyEnter, 0)
	got := ty.intents()
	if len(got) != 2 || got[0] != (changed{"hi"}) || got[1] != (submitted{"hi"}) {
		t.Fatalf("intents %v, want a change to hi and a submit of hi", got)
	}
}

func TestTypingStaysInTheField(t *testing.T) {
	ty := newTyper(t)
	ty.key(input.KeyT, 0)
	ty.typeText("t")
	if ty.fr.keys != 0 {
		t.Fatal("a typed letter reached the window's shortcuts")
	}
	ty.key(input.KeyT, input.ModControl)
	if ty.fr.keys != 1 {
		t.Fatal("Ctrl+T stopped at the field instead of reaching the window")
	}
}

func TestClicksPlaceTheCaretAndSelectWords(t *testing.T) {
	ty := newTyper(t)
	ty.typeText("one two three")
	ty.w.Input(input.PointerDown{Pos: geom.Pt(2, 18), Clicks: 1})
	ty.run(1)
	ty.want("one two three", 0)
	ty.w.Input(input.PointerDown{Pos: geom.Pt(290, 18), Clicks: 1})
	ty.run(1)
	ty.want("one two three", 13)

	// Double-click in the middle of "two", which starts at rune 4.
	mid := FieldPadding.Default() + (ty.field.shaped.run.CaretX(4)+ty.field.shaped.run.CaretX(7))/2
	ty.w.Input(input.PointerDown{Pos: geom.Pt(mid, 18), Clicks: 2})
	ty.run(1)
	if s, e := ty.field.Selection(); s != 4 || e != 7 {
		t.Fatalf("double-click selected %d..%d, want the word two at 4..7", s, e)
	}
}
