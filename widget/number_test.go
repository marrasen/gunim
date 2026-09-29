package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

type numbered struct{ V float64 }

func numberStage(t *testing.T, n *NumberField) (*gunim.Window, func(int)) {
	t.Helper()
	w, run := stage(t, &frame{child: n, size: geom.Sz(200, 32)})
	click(w, 100, 16)
	run(1)
	return w, run
}

func TestNumberFieldStepsWithTheArrows(t *testing.T) {
	n := NewNumberField(0, 512)
	n.SetValue(10)
	n.OnChange = func(v float64) gunim.Intent { return numbered{v} }
	w, run := numberStage(t, n)

	w.Input(input.KeyPress{Key: input.KeyUp})
	run(1)
	if n.Value() != 11 || n.Text() != "11" {
		t.Fatalf("up gave %v and %q, want 11", n.Value(), n.Text())
	}
	w.Input(input.KeyPress{Key: input.KeyPageDown})
	run(1)
	if n.Value() != 1 {
		t.Fatalf("page down gave %v, want 1", n.Value())
	}
	got := sent(w)
	if last := got[len(got)-1]; last != (numbered{1}) {
		t.Fatalf("last intent %v, want numbered{1}", last)
	}
}

func TestNumberFieldHoldsToItsBounds(t *testing.T) {
	n := NewNumberField(1, 16)
	n.SetValue(16)
	w, run := numberStage(t, n)
	for range 3 {
		w.Input(input.KeyPress{Key: input.KeyUp})
	}
	run(1)
	if n.Value() != 16 {
		t.Fatalf("stepping past the top gave %v, want 16", n.Value())
	}
	n.SetValue(99)
	if n.Value() != 16 {
		t.Fatalf("SetValue past the top gave %v, want 16", n.Value())
	}
}

func TestNumberFieldLeavesHalfTypedTextAlone(t *testing.T) {
	// Clearing the field to retype it must not snap back to the
	// minimum under the fingers.
	n := NewNumberField(1, 512)
	n.SetValue(120)
	w, run := numberStage(t, n)
	for range 3 {
		w.Input(input.KeyPress{Key: input.KeyBackspace})
	}
	run(1)
	if n.Text() != "" {
		t.Fatalf("the text reads %q after clearing it, want it empty", n.Text())
	}
	w.Input(input.TextInput{Text: "8"})
	run(1)
	if n.Value() != 8 {
		t.Fatalf("typing 8 gave %v", n.Value())
	}
}

func TestNumberFieldSettlesWhenItLosesFocus(t *testing.T) {
	n := NewNumberField(10, 100)
	n.SetValue(50)
	w, run := numberStage(t, n)
	for range 2 {
		w.Input(input.KeyPress{Key: input.KeyBackspace})
	}
	w.Input(input.TextInput{Text: "5"})
	run(1)
	if n.Value() != 5 {
		t.Fatalf("typing 5 gave %v; below the minimum is allowed while typing", n.Value())
	}
	// Leaving the field holds it to the bounds and rewrites the text.
	if err := w.Client().Focus(""); err != nil {
		t.Fatal(err)
	}
	run(1)
	if n.Value() != 10 || n.Text() != "10" {
		t.Fatalf("leaving the field gave %v and %q, want 10", n.Value(), n.Text())
	}
}

func TestNumberFieldTurnsWithTheWheel(t *testing.T) {
	n := NewNumberField(0, 255)
	n.SetValue(100)
	n.Step = 5
	w, run := numberStage(t, n)
	w.Input(input.Scroll{Pos: geom.Pt(100, 16), Delta: geom.Pt(0, -1)})
	run(1)
	if n.Value() != 105 {
		t.Fatalf("a wheel turn up gave %v, want 105", n.Value())
	}
	w.Input(input.Scroll{Pos: geom.Pt(100, 16), Delta: geom.Pt(0, 2)})
	run(1)
	if n.Value() != 100 {
		t.Fatalf("a wheel turn down gave %v, want 100", n.Value())
	}
}

func TestNumberFieldWritesDecimalsAndASuffix(t *testing.T) {
	n := NewNumberField(20, 999)
	n.Decimals, n.Suffix, n.Step = 2, " BPM", 0.5
	n.SetValue(128)
	if n.Text() != "128.00 BPM" {
		t.Fatalf("the text reads %q", n.Text())
	}
	// The suffix is not part of what is read back.
	if v, ok := parseNumber("128.00 BPM"); !ok || v != 128 {
		t.Fatalf("parseNumber gave %v, %v", v, ok)
	}
}

func TestParseNumber(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"42", 42, true},
		{" 42 ", 42, true},
		{"-7", -7, true},
		{"1.5", 1.5, true},
		{"1,5", 1.5, true}, // a European keyboard's decimal point
		{"250 ms", 250, true},
		{"", 0, false},
		{"ms", 0, false},
		{"--", 0, false},
	}
	for _, c := range cases {
		got, ok := parseNumber(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("parseNumber(%q) = %v, %v; want %v, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}
