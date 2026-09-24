package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// writer is a text area in an offscreen window, focused and ready.
type writer struct {
	t    *testing.T
	w    *gunim.Window
	run  func(int)
	area *TextArea
}

func newWriter(t *testing.T, width float32) *writer {
	t.Helper()
	area := NewTextArea()
	w, run := stage(t, &frame{child: area, size: geom.Sz(width, 400)})
	w.Input(input.PointerDown{Pos: geom.Pt(20, 20), Clicks: 1})
	w.Input(input.PointerUp{Pos: geom.Pt(20, 20)})
	run(1)
	return &writer{t: t, w: w, run: run, area: area}
}

func (wr *writer) typeText(s string) {
	wr.w.Input(input.TextInput{Text: s})
	wr.run(1)
}

func (wr *writer) key(k input.Key, mods input.Mods) {
	wr.w.Input(input.KeyPress{Key: k, Mods: mods})
	wr.run(1)
}

func (wr *writer) want(text string, caret int) {
	wr.t.Helper()
	if got := wr.area.Text(); got != text {
		wr.t.Fatalf("text %q, want %q", got, text)
	}
	if wr.area.caret != caret {
		wr.t.Fatalf("caret at %d, want %d", wr.area.caret, caret)
	}
}

func TestEnterStartsALine(t *testing.T) {
	wr := newWriter(t, 400)
	wr.typeText("one")
	wr.key(input.KeyEnter, 0)
	wr.typeText("two")
	wr.want("one\ntwo", 7)
	if n := len(wr.area.para.Lines); n != 2 {
		t.Fatalf("%d lines, want 2", n)
	}
}

func TestUpAndDownKeepTheColumn(t *testing.T) {
	wr := newWriter(t, 400)
	wr.typeText("abcdef")
	wr.key(input.KeyEnter, 0)
	wr.typeText("ab")
	wr.key(input.KeyEnter, 0)
	wr.typeText("abcdef")
	wr.want("abcdef\nab\nabcdef", 16)

	wr.key(input.KeyUp, 0)
	wr.want("abcdef\nab\nabcdef", 9) // the short line's end
	wr.key(input.KeyUp, 0)
	wr.want("abcdef\nab\nabcdef", 6) // back in the column it came from
	wr.key(input.KeyDown, input.ModShift)
	if s, e := wr.area.Selection(); s != 6 || e != 9 {
		t.Fatalf("Shift+Down selected %d..%d, want 6..9", s, e)
	}
	wr.key(input.KeyUp, 0)
	wr.key(input.KeyUp, 0)
	wr.want("abcdef\nab\nabcdef", 0) // Up from the first line goes to its start
}

func TestHomeAndEndWorkOnTheLineOnScreen(t *testing.T) {
	wr := newWriter(t, 140)
	wr.typeText("one two three four five six")
	if len(wr.area.para.Lines) < 2 {
		t.Fatal("setup: the text should wrap")
	}
	wr.key(input.KeyHome, input.ModControl)
	wr.key(input.KeyEnd, 0)
	first := wr.area.para.Lines[0].Run
	if wr.area.caret != first.End-1 {
		t.Fatalf("End on a wrapped line put the caret at %d, want %d, before the break", wr.area.caret, first.End-1)
	}
	if line, _ := wr.area.para.Caret(wr.area.caret); line != 0 {
		t.Fatalf("after End, the caret shows on line %d, want the first", line)
	}
	wr.key(input.KeyEnd, input.ModControl)
	wr.want("one two three four five six", 27)
	wr.key(input.KeyHome, 0)
	if last := wr.area.para.Lines[len(wr.area.para.Lines)-1].Run; wr.area.caret != last.Start {
		t.Fatalf("Home put the caret at %d, want the last line's start %d", wr.area.caret, last.Start)
	}
}

func TestPastingKeepsLineBreaksInAnArea(t *testing.T) {
	wr := newWriter(t, 400)
	wr.typeText("a\nb")
	wr.key(input.KeyA, input.ModControl)
	wr.key(input.KeyC, input.ModControl)
	wr.key(input.KeyEnd, input.ModControl)
	wr.key(input.KeyV, input.ModControl)
	wr.want("a\nba\nb", 6)

	ty := newTyper(t)
	ty.typeText("a\nb")
	ty.want("a b", 3)
}

func TestAClickInAnAreaFindsItsLine(t *testing.T) {
	wr := newWriter(t, 400)
	wr.typeText("abc\ndef")
	pad := FieldPadding.Default()
	lh := wr.area.para.LineHeight
	wr.w.Input(input.PointerDown{Pos: geom.Pt(pad+1000, pad+lh*1.5), Clicks: 1})
	wr.run(1)
	wr.want("abc\ndef", 7)
	wr.w.Input(input.PointerDown{Pos: geom.Pt(pad, pad+lh*0.5), Clicks: 1})
	wr.run(1)
	wr.want("abc\ndef", 0)
}

func TestAnAreaScrollsToKeepTheCaretInView(t *testing.T) {
	wr := newWriter(t, 400)
	for range 12 {
		wr.typeText("line")
		wr.key(input.KeyEnter, 0)
	}
	wr.run(120)
	lh := wr.area.para.LineHeight
	_, at := wr.area.para.Caret(wr.area.caret)
	top := wr.area.scroll.Value()
	if at.Y < top || at.Y+lh > top+wr.area.view+0.5 {
		t.Fatalf("caret line at %v..%v, view %v..%v", at.Y, at.Y+lh, top, top+wr.area.view)
	}
	wr.key(input.KeyHome, input.ModControl)
	wr.run(120)
	if top := wr.area.scroll.Value(); top != 0 {
		t.Fatalf("at the start of the text, the view is scrolled to %v, want 0", top)
	}
}

func TestLeftAndRightStepInAField(t *testing.T) {
	ty := newTyper(t)
	ty.typeText("abc")
	ty.key(input.KeyLeft, 0)
	ty.key(input.KeyLeft, 0)
	ty.want("abc", 1)
	ty.key(input.KeyRight, input.ModShift)
	if s, e := ty.field.Selection(); s != 1 || e != 2 {
		t.Fatalf("Shift+Right selected %d..%d, want 1..2", s, e)
	}
	ty.key(input.KeyLeft, 0)
	ty.want("abc", 1) // collapses to the selection's start
}
