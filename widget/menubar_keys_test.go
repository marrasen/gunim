package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// key presses k with mods, and lets it go.
func key(w *gunim.Window, run func(int), k input.Key, mods input.Mods) {
	w.Input(input.KeyPress{Key: k, Mods: mods})
	w.Input(input.KeyRelease{Key: k, Mods: mods})
	run(1)
}

// tapAlt presses Alt and lets it go.
func tapAlt(w *gunim.Window, run func(int)) {
	w.Input(input.KeyPress{Key: input.KeyLeftAlt, Mods: input.ModAlt})
	w.Input(input.KeyRelease{Key: input.KeyLeftAlt})
	run(1)
}

func TestAnAmpersandMarksAnAccessKey(t *testing.T) {
	for _, c := range []struct {
		in, shown string
		key       rune
		at        int
	}{
		{"&File", "File", 'f', 0},
		{"E&xit", "Exit", 'x', 1},
		{"Save && Quit", "Save & Quit", 's', 0},
		{"Go", "Go", 'g', 0},
		{"…", "…", 0, -1},
	} {
		shown, key, at := accessKey(c.in)
		if shown != c.shown || key != c.key || at != c.at {
			t.Errorf("accessKey(%q) = %q, %q, %d; want %q, %q, %d", c.in, shown, key, at, c.shown, c.key, c.at)
		}
	}
}

func TestAltAndATitlesKeyOpenItsMenuAndALetterPicks(t *testing.T) {
	w, b, field, picks, run := newBarStage(t)
	b.Menus[1].Title = "&Edit"
	w.Input(input.KeyPress{Key: input.KeyLeftAlt, Mods: input.ModAlt})
	run(1)
	if !b.altDown {
		t.Fatal("Alt held does not underline the access keys")
	}
	key(w, run, input.KeyE, input.ModAlt)
	w.Input(input.KeyRelease{Key: input.KeyLeftAlt})
	run(1)
	// Edit's Copy is disabled, so the keys start on Paste.
	if b.open != 1 || b.menu.Highlighted() != 1 || !b.menu.cues {
		t.Fatalf("Alt+E left menu %d open, item %d lit, underlined %v; want Edit, Paste, underlined",
			b.open, b.menu.Highlighted(), b.menu.cues)
	}
	// Alt+V goes across to View.
	key(w, run, input.KeyV, input.ModAlt)
	if b.open != 2 {
		t.Fatalf("Alt+V with Edit open left menu %d open, want View", b.open)
	}
	key(w, run, input.KeyS, 0)
	run(20)
	if len(*picks) != 1 || (*picks)[0] != (barPick{2, 0}) || b.IsOpen() {
		t.Fatalf("S picked %v, the bar open %v; want View's Sidebar, and the menu shut", *picks, b.IsOpen())
	}
	w.Input(input.TextInput{Text: "hi"})
	run(1)
	if field.Text() != "hi" {
		t.Fatalf("after the pick, typing reached %q, want the field", field.Text())
	}
}

func TestAltAloneLightsTheFirstTitleForTheArrows(t *testing.T) {
	w, b, field, _, run := newBarStage(t)
	tapAlt(w, run)
	if !b.armed || b.IsOpen() || b.lit != 0 {
		t.Fatalf("Alt alone left the bar armed %v, open %v, title %d lit; want File lit, no menu open", b.armed,
			b.IsOpen(), b.lit)
	}
	key(w, run, input.KeyRight, 0)
	key(w, run, input.KeyDown, 0)
	if b.open != 1 || b.menu.Highlighted() != 1 {
		t.Fatalf("Right and Down left menu %d open, item %d lit; want Edit's Paste", b.open, b.menu.Highlighted())
	}
	// Escape goes back to the bar with Edit lit, and again gives the keyboard back
	key(w, run, input.KeyEscape, 0)
	run(20)
	if b.IsOpen() || !b.armed || b.lit != 1 {
		t.Fatalf("Escape left a menu open %v, the bar armed %v, title %d lit; want no menu, Edit lit", b.IsOpen(),
			b.armed, b.lit)
	}
	key(w, run, input.KeyEscape, 0)
	run(20)
	if b.IsOpen() || b.armed {
		t.Fatal("a second Escape left the bar with the keyboard")
	}
	w.Input(input.TextInput{Text: "hi"})
	run(1)
	if field.Text() != "hi" {
		t.Fatalf("after Escape, typing reached %q, want the field", field.Text())
	}

	// F10 does as Alt alone does, and Alt alone again gives the keyboard back.
	key(w, run, input.KeyF10, 0)
	if !b.armed {
		t.Fatal("F10 left the bar without the keyboard")
	}
	tapAlt(w, run)
	if b.armed {
		t.Fatal("Alt alone a second time left the bar with the keyboard")
	}
}

func TestAltWithAnotherKeyLeavesTheMenubarAlone(t *testing.T) {
	w, b, _, _, run := newBarStage(t)
	w.Input(input.KeyPress{Key: input.KeyLeftAlt, Mods: input.ModAlt})
	key(w, run, input.KeyLeft, input.ModAlt)
	w.Input(input.KeyRelease{Key: input.KeyLeftAlt})
	run(1)
	if b.armed || b.IsOpen() || b.altDown {
		t.Fatalf("Alt+Left left the bar armed %v, open %v, underlined %v", b.armed, b.IsOpen(), b.altDown)
	}
}

func TestAMenusLetterMovesAmongItemsThatShareIt(t *testing.T) {
	m := NewMenu(Labels("Save", "Save &as", "Sort"))
	m.AccessKeys = true
	var picked []int
	m.Pick = func(i int, _ *gunim.UI) { picked = append(picked, i) }
	m.Key(input.KeyPress{Key: input.KeyS}, nil)
	m.Key(input.KeyPress{Key: input.KeyS}, nil)
	if m.Highlighted() != 2 || len(picked) != 0 {
		t.Fatalf("S twice, which Save and Sort share, lit item %d and picked %v; want Sort lit, nothing picked",
			m.Highlighted(), picked)
	}
	m.Key(input.KeyPress{Key: input.KeyA}, nil)
	if len(picked) != 1 || picked[0] != 1 {
		t.Fatalf("A picked %v, want Save as", picked)
	}
	if got := m.label(1); got != "Save as" {
		t.Fatalf("the item reads %q, want the & left out", got)
	}
}

func TestHeldAltUnderlinesTheTitles(t *testing.T) {
	b := NewMenubar(BarMenu{Title: "&File"}, BarMenu{Title: "&Edit"})
	f := gunim.Frame{Scale: 1}
	size := b.Layout(gunim.Constraints{Max: geom.Sz(400, 40)}, f, gunim.Children{})
	plain := len(painted(b, size))
	b.altDown = true
	if cued := len(painted(b, size)); cued != plain+2 {
		t.Fatalf("with Alt held the bar drew %d ops, want %d: two more for the underlines", cued, plain+2)
	}
}

func TestAltAndATitleThenUpGoesToTheMenusLastItem(t *testing.T) {
	w, b, _, _, run := newBarStage(t)
	w.Input(input.KeyPress{Key: input.KeyLeftAlt, Mods: input.ModAlt})
	key(w, run, input.KeyF, input.ModAlt)
	w.Input(input.KeyRelease{Key: input.KeyLeftAlt})
	run(1)
	key(w, run, input.KeyUp, 0)
	if b.open != 0 || b.menu.Highlighted() != 2 {
		t.Fatalf("Alt+F and Up left menu %d open with item %d lit; want File's last, Quit", b.open, b.menu.Highlighted())
	}
}
