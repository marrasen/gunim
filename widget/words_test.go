package widget

import (
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/theme"
)

func TestCtrlArrowsStopAtPunctuation(t *testing.T) {
	// A path or an address is no longer one long word: Ctrl+Left and
	// Ctrl+Right stop at its slashes and dots, as most text fields do.
	ty := newTyper(t)
	ty.typeText("/home/me/my_file.txt")
	for _, want := range []int{17, 9, 6, 1, 0, 0} {
		ty.key(input.KeyLeft, input.ModControl)
		ty.want("/home/me/my_file.txt", want)
	}
	for _, want := range []int{5, 8, 16, 20, 20} {
		ty.key(input.KeyRight, input.ModControl)
		ty.want("/home/me/my_file.txt", want)
	}
	ty.key(input.KeyBackspace, input.ModControl)
	ty.want("/home/me/my_file.", 17)
	ty.key(input.KeyBackspace, input.ModControl)
	ty.want("/home/me/", 9)
	ty.key(input.KeyHome, 0)
	ty.key(input.KeyDelete, input.ModControl)
	ty.want("/me/", 0)
}

func TestCtrlShiftArrowsSelectToTheStops(t *testing.T) {
	// With Shift, the same stops grow the selection, either way.
	ty := newTyper(t)
	ty.typeText("/home/me/my_file.txt")
	sel := func(start, end int) {
		t.Helper()
		if s, e := ty.field.Selection(); s != start || e != end {
			t.Fatalf("selection %d..%d, want %d..%d", s, e, start, end)
		}
	}
	ty.key(input.KeyLeft, input.ModControl|input.ModShift)
	sel(17, 20)
	ty.key(input.KeyLeft, input.ModControl|input.ModShift)
	sel(9, 20)
	ty.key(input.KeyRight, input.ModControl|input.ModShift)
	sel(16, 20)
	ty.key(input.KeyHome, 0)
	ty.key(input.KeyRight, input.ModControl|input.ModShift)
	sel(0, 5)
	ty.key(input.KeyRight, input.ModControl|input.ModShift)
	sel(0, 8)
	ty.key(input.KeyBackspace, 0)
	ty.want("/my_file.txt", 0)
}

func TestAThemeChoosesTheWordStops(t *testing.T) {
	// A themed scope round one field gives it stops of its own: here
	// only the slash, so the dot stays inside a word.
	field := NewTextField()
	th := theme.Make("stops", theme.Set(WordStops, "/"))
	fr := &frame{child: NewThemed(field, th), size: geom.Sz(300, 36)}
	w, run := stage(t, fr)
	w.Input(input.PointerDown{Pos: geom.Pt(20, 18), Clicks: 1})
	w.Input(input.PointerUp{Pos: geom.Pt(20, 18)})
	run(1)
	w.Input(input.TextInput{Text: "a.b/c.d"})
	run(1)
	for _, want := range []int{4, 0} {
		w.Input(input.KeyPress{Key: input.KeyLeft, Mods: input.ModControl})
		run(1)
		if field.caret != want {
			t.Fatalf("caret at %d, want %d", field.caret, want)
		}
	}
}

func TestWordsEndAtStopsAndSpaces(t *testing.T) {
	stops := WordStops.Default()
	for _, c := range []struct {
		text       string
		i          int
		start, end int
	}{
		{"foo.bar", 1, 0, 3},  // inside a word
		{"foo.bar", 3, 0, 3},  // just after a word, on a stop
		{"foo.bar", 5, 4, 7},  // the word after the stop
		{"a // b", 2, 2, 4},   // a run of stops
		{"my_file", 3, 0, 7},  // the underscore is part of a word
		{"one  two", 4, 4, 4}, // on a space after a space
		{"x = y", 2, 2, 3},    // a stop between spaces
	} {
		if s, e := wordAt([]rune(c.text), c.i, stops); s != c.start || e != c.end {
			t.Errorf("wordAt(%q, %d) = %d–%d, want %d–%d", c.text, c.i, s, e, c.start, c.end)
		}
	}
	if got := wordEnd([]rune("a.b c"), 0, ""); got != 3 {
		t.Errorf("with no stops, wordEnd ends at a space: got %d, want 3", got)
	}
	if got := wordStart([]rune("---"), 3, stops); got != 0 {
		t.Errorf("wordStart over stops alone = %d, want 0", got)
	}
}
