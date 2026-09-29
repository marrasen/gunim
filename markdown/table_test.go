package markdown

import (
	"reflect"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

func TestParseReadsATable(t *testing.T) {
	got := parse("| Name | Count |\n|:--|--:|\n| apples | 3 |\n| **pears** | 12 |", true)
	want := []block{{kind: table, aligns: []align{alignStart, alignEnd}, rows: [][][]span{
		{{{text: "Name"}}, {{text: "Count"}}},
		{{{text: "apples"}}, {{text: "3"}}},
		{{{text: "pears", style: bold}}, {{text: "12"}}},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parse gave\n%+v\nwant\n%+v", got, want)
	}
	if got, want := Plain("| a | b |\n|--|--|\n| 1 | 2 |"), "a\tb\n1\t2"; got != want {
		t.Fatalf("Plain = %q, want %q", got, want)
	}
}

func TestFitColumnsNarrowsTheWidest(t *testing.T) {
	got := fitColumns([]float32{50, 300, 100}, 250, 20)
	if got[0] != 50 || got[2] != 100 || got[1] < 99 || got[1] > 100.5 {
		t.Fatalf("fitColumns = %v, want the widest narrowed to about 100", got)
	}
}

func TestATableCopiesWithTabsAndSetsItsCellsSideBySide(t *testing.T) {
	v := New("| Name | Count |\n|:--|--:|\n| apples | 3 |")
	w, run := stage(t, v)
	if len(v.paras) != 4 {
		t.Fatalf("%d paragraphs, want a cell each", len(v.paras))
	}
	if v.paras[1].at.Y != v.paras[0].at.Y || v.paras[1].at.X <= v.paras[0].at.X {
		t.Fatal("a row's cells are not side by side")
	}
	// The Count column sets its text at its end.
	left, right := v.paras[1].at.X+v.paras[1].p.Size.W, v.paras[3].at.X+v.paras[3].p.Size.W
	if d := left - right; d > 0.5 || d < -0.5 {
		t.Fatalf("the Count column's cells end at %v and %v, want the same", left, right)
	}
	at := centre(v, 0)
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary})
	w.Input(input.KeyPress{Key: input.KeyA, Mods: input.ModControl})
	run(1)
	if got, want := v.SelectedText(), "Name\tCount\napples\t3"; got != want {
		t.Fatalf("selected %q, want %q", got, want)
	}
	// A press over the second column puts the caret in its cell.
	second := centre(v, 1)
	w.Input(input.PointerDown{Pos: second, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: second, Button: input.ButtonPrimary})
	run(1)
	if s, _ := v.Selection(); s < v.paras[1].base || s > v.paras[1].base+v.paras[1].n {
		t.Fatalf("a press on Count put the caret at %d, outside its cell", s)
	}
}

func TestALongCodeLineScrollsSideways(t *testing.T) {
	long := "go test ./... -run TestSyncAfterReconnectKeepsTheOrderOfEveryEventThatArrivedWhileOffline -count 50"
	v := New("```\n" + long + "\n```")
	w, run := stage(t, v)
	lp := v.paras[0]
	if lp.over <= 0 {
		t.Fatal("the long line wrapped, or fits, and does not scroll")
	}
	if len(lp.p.Lines) != 1 {
		t.Fatalf("the code has %d lines, want its one line unwrapped", len(lp.p.Lines))
	}
	in := lp.clip.Center()
	w.Input(input.Scroll{Pos: in, Delta: geom.Pt(0, -80)})
	run(1)
	if v.scrolls[0] != 0 {
		t.Fatal("the wheel alone scrolled the code sideways")
	}
	w.Input(input.Scroll{Pos: in, Delta: geom.Pt(-80, 0)})
	run(1)
	if v.scrolls[0] != 80 {
		t.Fatalf("scrolled to %v, want 80", v.scrolls[0])
	}
	w.Input(input.Scroll{Pos: in, Delta: geom.Pt(0, -1e5), Mods: input.ModShift})
	run(1)
	if v.scrolls[0] != lp.over {
		t.Fatalf("Shift and the wheel scrolled to %v, want the end at %v", v.scrolls[0], lp.over)
	}
}
