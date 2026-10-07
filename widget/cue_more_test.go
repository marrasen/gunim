package widget

import (
	"slices"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// soundedPress reports whether l played the press cue since it was last asked, and forgets what it played.
func soundedPress(l *cueLog) bool { return slices.Contains(l.take(), gunim.CuePress) }

// Each of the lists sounds the press cue as Enter activates a row or a tile.
func TestTheListsSoundAsTheyActivate(t *testing.T) {
	enter := func(w *gunim.Window, run func(int)) {
		w.Input(input.KeyPress{Key: input.KeyEnter})
		run(1)
	}
	t.Run("list", func(t *testing.T) {
		w, _, _, run, _ := newKeyedList(t, 5)
		tab(w, run, 0)
		tab(w, run, 0)
		l := listens(w)
		enter(w, run)
		if !soundedPress(l) {
			t.Fatal("Enter on a list's row played no press")
		}
	})
	t.Run("tree", func(t *testing.T) {
		w, _, run, _ := newTreeStage(t)
		w.Input(input.KeyPress{Key: input.KeyDown})
		run(1)
		l := listens(w)
		enter(w, run)
		if !soundedPress(l) {
			t.Fatal("Enter on a tree's row played no press")
		}
	})
	t.Run("table", func(t *testing.T) {
		w, _, _, run := newTableStage(t, 5)
		l := listens(w)
		enter(w, run)
		if !soundedPress(l) {
			t.Fatal("Enter on a table's row played no press")
		}
	})
	t.Run("data grid", func(t *testing.T) {
		g := NewDataGrid(GridColumn{Title: "Message"})
		g.Row = func(int) (GridRow, bool) { return GridRow{Cells: [][]GridSpan{{{Text: "row"}}}}, true }
		g.OnActivate = func(i int, _ *gunim.UI) gunim.Intent { return nil }
		g.rows, g.selected = 5, 0
		w, run := stage(t, &frame{child: g, size: geom.Sz(400, 300)})
		tab(w, run, 0)
		l := listens(w)
		enter(w, run)
		if !soundedPress(l) {
			t.Fatal("Enter on a data grid's row played no press")
		}
	})
	t.Run("tile grid", func(t *testing.T) {
		g, w, run := tileStage(t, 5)
		g.cursor = 0
		tab(w, run, 0)
		l := listens(w)
		enter(w, run)
		if !soundedPress(l) {
			t.Fatal("Enter on a tile played no press")
		}
	})
}

// Toasts sound as they arrive and as they go; one taking another's place sounds once.
func TestToastsSoundAsTheyComeAndGo(t *testing.T) {
	ts := NewToasts()
	w, run := stage(t, &frame{child: ts, size: geom.Sz(400, 400)})
	u := stageUI(t, w, run)
	l := listens(w)
	ts.Show(Toast{Title: "Saved", Key: "s"}, u)
	run(1)
	wantCues(t, l, "a toast arriving", gunim.CueOpen)
	ts.Show(Toast{Title: "Saved again", Key: "s"}, u)
	run(1)
	wantCues(t, l, "a toast in another's place", gunim.CueOpen)
	ts.Close("s", u)
	run(1)
	wantCues(t, l, "a toast going", gunim.CueClose)
}

// A fold sounds as it opens and as it shuts.
func TestAFoldSoundsAsItOpensAndShuts(t *testing.T) {
	f := NewFold(NewLabel("Inside"), false)
	w, run := stage(t, &frame{child: f, size: geom.Sz(400, 400)})
	u := stageUI(t, w, run)
	l := listens(w)
	f.SetOpen(true, u)
	run(1)
	wantCues(t, l, "a fold opening", gunim.CueOpen)
	f.SetOpen(false, u)
	run(1)
	wantCues(t, l, "a fold shutting", gunim.CueClose)
}

// A palette sounds as it opens, as an item is picked, and as it goes with nothing picked.
func TestAPaletteSoundsAsItOpensAndPicks(t *testing.T) {
	w, _, run := newPaletteStage(t)
	focusOpener(w, run)
	l := listens(w)
	open := func() {
		w.Input(input.KeyPress{Key: input.KeyF1})
		run(5)
	}
	open()
	wantCues(t, l, "a palette opening", gunim.CueOpen)
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(5)
	wantCues(t, l, "a pick from the palette", gunim.CuePress)
	open()
	l.take()
	w.Input(input.KeyPress{Key: input.KeyEscape})
	run(5)
	wantCues(t, l, "Escape out of the palette", gunim.CueClose)
}

// The emoji picker sounds as an emoji is picked.
func TestTheEmojiPickerSoundsAsItPicks(t *testing.T) {
	_, w, run, _ := openPicker(t)
	l := listens(w)
	w.Input(input.TextInput{Text: "thumbs"})
	run(3)
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(3)
	if !soundedPress(l) {
		t.Fatal("picking an emoji played no press")
	}
}

// A chip sounds as it is removed.
func TestAChipSoundsAsItIsRemoved(t *testing.T) {
	c := NewChip("level", "error")
	w, run := stage(t, &frame{child: Row(c), size: geom.Sz(400, 40)})
	l := listens(w)
	cueClick(w, run, geom.Pt(c.crossX, 12))
	wantCues(t, l, "a click on the chip's cross", gunim.CuePress)
}
