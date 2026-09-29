package widget

import (
	"strings"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/text"
)

// openPicker stages a button that opens an emoji picker, clicks it, and returns the picker, the window, and what
// was picked so far.
func openPicker(t *testing.T) (*EmojiPicker, *gunim.Window, func(int), *[]string) {
	t.Helper()
	if !text.EmojiShows("\U0001F44D") {
		t.Skip("no colour emoji font here")
	}
	var picked []string
	picker := &EmojiPicker{Pick: func(s string, _ *gunim.UI) { picked = append(picked, s) }}
	button := NewButton("Emoji")
	button.OnActivate(func(u *gunim.UI) { picker.Open(button, geom.Rc(0, 0, 80, 36), u) })
	w, run := stage(t, &frame{child: button, size: geom.Sz(120, 36)})
	w.Input(input.PointerDown{Pos: geom.Pt(20, 18), Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: geom.Pt(20, 18), Button: input.ButtonPrimary})
	run(10)
	if !picker.IsOpen() {
		t.Fatal("the picker did not open")
	}
	return picker, w, run, &picked
}

func TestTheEmojiPickerShowsTheGroups(t *testing.T) {
	picker, _, _, _ := openPicker(t)
	c := picker.card
	if len(c.tabs.groups) < 5 {
		t.Fatalf("%d group tabs, want the groups of emoji", len(c.tabs.groups))
	}
	if c.first.Text != "\U0001F600" {
		t.Fatalf("the first emoji is %q, want the grinning face", c.first.Text)
	}
	if _, ok := c.rows["h:0"]; !ok {
		t.Fatal("the first group has no title row")
	}
}

func TestTypingNarrowsThePickerAndEnterPicks(t *testing.T) {
	picker, w, run, picked := openPicker(t)
	w.Input(input.TextInput{Text: "thumbs"})
	run(3)
	for k := range picker.card.rows {
		if !strings.HasPrefix(string(k), "s:") {
			t.Fatalf("row %q shows while searching", k)
		}
	}
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(3)
	if len(*picked) != 1 || (*picked)[0] != "\U0001F44D" {
		t.Fatalf("picked %q, want thumbs up", *picked)
	}
	if picker.IsOpen() {
		t.Fatal("the picker is still open after picking")
	}
}

func TestEscapeClosesThePicker(t *testing.T) {
	picker, w, run, picked := openPicker(t)
	w.Input(input.KeyPress{Key: input.KeyEscape})
	run(3)
	if picker.IsOpen() || len(*picked) != 0 {
		t.Fatalf("after Escape: open %v, picked %q", picker.IsOpen(), *picked)
	}
}
