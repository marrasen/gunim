package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
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
	return newTyperWith(t, func(*TextField) {})
}

// newTyperWith is newTyper with the field set up by set first.
func newTyperWith(t *testing.T, set func(*TextField)) *typer {
	t.Helper()
	field := NewTextField()
	set(field)
	field.OnChange = func(s string, u *gunim.UI) gunim.Intent { return changed{s} }
	field.OnCommit = func(s string, u *gunim.UI) gunim.Intent { return submitted{s} }
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

func TestCtrlShiftLettersGoOnPastTheField(t *testing.T) {
	ty := newTyper(t)
	ty.typeText("abc")
	ty.key(input.KeyA, input.ModControl|input.ModShift)
	if a := ty.field.anchor; a != 3 {
		t.Fatalf("Ctrl+Shift+A moved the anchor to %d, want 3", a)
	}
	ty.key(input.KeyA, input.ModControl)
	for _, k := range []input.Key{input.KeyC, input.KeyX, input.KeyV} {
		ty.key(k, input.ModControl|input.ModShift)
		ty.want("abc", 3)
		if a := ty.field.anchor; a != 0 {
			t.Fatalf("Ctrl+Shift+%v moved the anchor to %d, want 0", k, a)
		}
	}
	if ty.fr.keys != 4 {
		t.Fatalf("%d of the four Ctrl+Shift shortcuts reached the window, want all four", ty.fr.keys)
	}
	// Ctrl+Shift+Z redoes.
	ty.key(input.KeyZ, input.ModControl)
	ty.want("", 0)
	ty.key(input.KeyZ, input.ModControl|input.ModShift)
	ty.want("abc", 3)
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
	mid := FieldPadding.Default() + (ty.field.line.CaretX(4)+ty.field.line.CaretX(7))/2
	ty.w.Input(input.PointerDown{Pos: geom.Pt(mid, 18), Clicks: 2})
	ty.run(1)
	if s, e := ty.field.Selection(); s != 4 || e != 7 {
		t.Fatalf("double-click selected %d..%d, want the word two at 4..7", s, e)
	}
}

func TestACompositionShowsInPlaceUntilItCommits(t *testing.T) {
	ty := newTyper(t)
	ty.typeText("ab")
	ty.key(input.KeyLeft, 0)
	// "にほ" is six bytes; the input method's caret is at its end.
	ty.w.Input(input.Composing{Text: "にほ", Selected: [2]int{6, 6}})
	ty.run(1)
	if got := ty.field.Text(); got != "ab" {
		t.Fatalf("text %q during composition, want it untouched", got)
	}
	shown, at := ty.field.shown()
	if string(shown) != "aにほb" || at != 1 {
		t.Fatalf("shown %q from %d, want aにほb from 1", string(shown), at)
	}

	ty.w.Input(input.Composing{})
	ty.w.Input(input.TextInput{Text: "日本"})
	ty.run(1)
	ty.want("a日本b", 3)
	if shown, _ := ty.field.shown(); string(shown) != "a日本b" {
		t.Fatalf("shown %q after the commit, want the committed text", string(shown))
	}
}

func TestACommitReplacesTheSelection(t *testing.T) {
	ty := newTyper(t)
	ty.typeText("hello")
	ty.key(input.KeyHome, input.ModShift)
	ty.w.Input(input.Composing{Text: "x", Selected: [2]int{1, 1}})
	if shown, _ := ty.field.shown(); string(shown) != "x" {
		t.Fatalf("shown %q, want the composition in place of the selection", string(shown))
	}
	ty.w.Input(input.TextInput{Text: "X"})
	ty.run(1)
	ty.want("X", 1)
}

func TestTheCaretJumpsForTypingAndGlidesForMoving(t *testing.T) {
	ty := newTyper(t)
	ty.typeText("abc")
	if ty.field.caretAt.Active() {
		t.Fatal("the caret is still gliding the frame after typing")
	}
	if got, want := ty.field.caretAt.Value(), ty.field.line.CaretX(3); got != want {
		t.Fatalf("after typing, the caret is at %v, want %v at once", got, want)
	}
	ty.key(input.KeyBackspace, 0)
	if got, want := ty.field.caretAt.Value(), ty.field.line.CaretX(2); got != want {
		t.Fatalf("after Backspace, the caret is at %v, want %v at once", got, want)
	}
	ty.key(input.KeyLeft, 0)
	if !ty.field.caretAt.Active() {
		t.Fatal("an arrow key moved the caret without a glide")
	}
}

func TestAltGrTypesWhereControlAndAltAreHeld(t *testing.T) {
	ty := newTyper(t)
	ty.typeText("ab")
	// AltGr+A types ą on a Polish keyboard, with Control and Alt held:
	// typing, and no select-all.
	ty.w.Input(input.KeyPress{Key: input.KeyA, Mods: input.ModControl | input.ModAlt, Typed: true})
	ty.typeText("ą")
	ty.want("abą", 3)
	// AltGr+Q types @ on a German one. The press stays in the field,
	// where a window's Control+Alt+Q would otherwise take it.
	ty.w.Input(input.KeyPress{Key: input.KeyQ, Mods: input.ModControl | input.ModAlt, Typed: true})
	ty.typeText("@")
	ty.want("abą@", 4)
	if ty.fr.keys != 0 {
		t.Fatalf("%d typing presses left the field", ty.fr.keys)
	}
}

func TestAFlashFadesOutSteadily(t *testing.T) {
	w := gunimtest.New(t, geom.Sz(300, 60), nil)
	f := NewTextField()
	gunim.RegisterView(w, "field", func(struct{}) gunim.Node { return f }, nil)
	if err := w.Client().Mount(gunim.Root, "field", "field", struct{}{}); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	f.Flash()
	last := f.flash.Value()
	if last != 1 {
		t.Fatalf("flashed, the tint starts at %v", last)
	}
	for i := range 60 {
		w.Frame(time.Second / 60)
		v := f.flash.Value()
		if v > last || v < 0 {
			t.Fatalf("frame %d: the tint went from %v to %v", i, last, v)
		}
		last = v
	}
	if last != 0 {
		t.Fatalf("a second on, the tint is at %v", last)
	}
}

// Tab or Right takes a ghost at the end of the text; without one, Tab
// is left to the fields around.
func TestTabTakesTheGhost(t *testing.T) {
	ty := newTyper(t)
	ty.field.OnChange = func(s string, _ *gunim.UI) gunim.Intent {
		ty.field.Ghost = ""
		if s == "/ho" {
			ty.field.Ghost = "me/"
		}
		return nil
	}
	ty.typeText("/ho")
	if ty.field.Ghost != "me/" {
		t.Fatalf("typed /ho, the ghost is %q", ty.field.Ghost)
	}
	ty.key(input.KeyTab, 0)
	ty.want("/home/", 6)
	if ty.field.Ghost != "" {
		t.Fatalf("taken, the ghost is still %q", ty.field.Ghost)
	}
	ty.w.Input(input.KeyPress{Key: input.KeyLeft})
	ty.run(1)
	ty.field.Ghost = "x"
	// Not at the end: Right moves, and the ghost stays a suggestion.
	ty.key(input.KeyRight, 0)
	ty.want("/home/", 6)
}

func TestSelectPicksARunOfText(t *testing.T) {
	ty := newTyper(t)
	ty.field.SetText("report.pdf", nil)
	ty.field.Select(0, 6)
	if start, end := ty.field.Selection(); start != 0 || end != 6 {
		t.Fatalf("selected %d to %d, want 0 to 6", start, end)
	}
	ty.typeText("summary")
	if got := ty.field.Text(); got != "summary.pdf" {
		t.Fatalf("typing over the selection left %q", got)
	}
	ty.field.Select(-4, 400)
	if start, end := ty.field.Selection(); start != 0 || end != len("summary.pdf") {
		t.Fatalf("a selection past both ends became %d to %d", start, end)
	}
}

// Keys hears a key before the field uses it, and takes it from the
// field when it reports true.
func TestAFieldsKeysComeFirst(t *testing.T) {
	f := NewTextField()
	f.SetText("ab", nil)
	took := 0
	f.Keys = func(e input.KeyPress, u *gunim.UI) bool {
		if e.Key == input.KeyLeft {
			took++
			return true
		}
		return false
	}
	w, run := stage(t, &frame{child: Row(f), size: geom.Sz(300, 100)})
	click(w, 10, 10)
	run(2)
	w.Input(input.KeyPress{Key: input.KeyEnd})
	w.Input(input.KeyPress{Key: input.KeyLeft})
	w.Input(input.KeyPress{Key: input.KeyBackspace})
	run(1)
	// Left was taken, so the caret stayed at the end.
	if took != 1 || f.Text() != "a" {
		t.Fatalf("Keys took %d, and the field holds %q", took, f.Text())
	}
}
