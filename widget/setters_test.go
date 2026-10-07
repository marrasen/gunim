package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
)

// glides asserts on every frame of a second that got leaves from and settles on to: the first frame shows it part
// way, each frame before it reaches to moves on toward it, and the last is at to. A bounce may carry it past to and back.
func glides(t *testing.T, run func(int), what string, from, to float32, got func() float32) {
	t.Helper()
	near := func(a, b float32) bool { return abs32(a-b) < 1e-3 }
	prev, past := from, false
	for i := range 60 {
		run(1)
		g := got()
		if i == 0 && (near(g, from) || near(g, to)) {
			t.Fatalf("frame 0: %s is at %v, want part way from %v to %v", what, g, from, to)
		}
		past = past || (g-to)*(from-to) <= 0
		if !past && abs32(g-to) > abs32(prev-to)+1e-4 {
			t.Fatalf("frame %d: %s went from %v back to %v, away from %v", i, what, prev, g, to)
		}
		prev = g
	}
	if !near(prev, to) {
		t.Fatalf("after a second %s is at %v, want %v", what, prev, to)
	}
}

// jumps asserts on every frame of a second that got is at want.
func jumps(t *testing.T, run func(int), what string, want float32, got func() float32) {
	t.Helper()
	for i := range 60 {
		run(1)
		if g := got(); abs32(g-want) > 1e-4 {
			t.Fatalf("frame %d: %s is at %v, want %v at once", i, what, g, want)
		}
	}
}

// quiet fails when the window sent the application anything.
func quiet(t *testing.T, w *gunim.Window, what string) {
	t.Helper()
	if got := sent(w); len(got) != 0 {
		t.Fatalf("%s sent %v, want nothing", what, got)
	}
}

func TestSetCheckedGlidesOnceLaidOutAndJumpsWithoutAUI(t *testing.T) {
	c := NewCheckbox("Tick")
	c.OnChange = func(on bool, u *gunim.UI) gunim.Intent { return flipped{on} }
	w, run := stage(t, &frame{child: c, size: geom.Sz(200, 28)})
	u := stageUI(t, w, run)
	c.SetChecked(true, u)
	if !c.Checked() {
		t.Fatal("SetChecked(true) left the box unticked")
	}
	glides(t, run, "the tick", 0, 1, c.lit.Value)
	c.SetChecked(false, nil)
	jumps(t, run, "the tick set with no UI", 0, c.lit.Value)
	quiet(t, w, "SetChecked")
}

// setterStage mounts n in a frame of size and returns the window, what runs frames, and the window's UI.
func setterStage(t *testing.T, n gunim.Node, size geom.Size) (*gunim.Window, func(int), *gunim.UI) {
	t.Helper()
	w, run := stage(t, &frame{child: n, size: size})
	return w, run, stageUI(t, w, run)
}

func TestSettersGlideOnceLaidOutAndJumpWithoutAUI(t *testing.T) {
	t.Run("tabs", func(t *testing.T) {
		tabs := NewTabs([]string{"One", "Two", "Three"}, &recorder{}, &recorder{}, &recorder{})
		tabs.OnChange = func(i int, u *gunim.UI) gunim.Intent { return tabbed{i} }
		w, run, u := setterStage(t, tabs, geom.Sz(400, 300))
		tabs.SetSelected(1, u)
		glides(t, run, "the new page", 0, 1, tabs.slide.Value)
		tabs.SetSelected(2, nil)
		jumps(t, run, "the page set with no UI", 1, tabs.slide.Value)
		if tabs.Selected() != 2 || tabs.prev != -1 {
			t.Fatalf("tab %d chosen, %d leaving; want 2 and none", tabs.Selected(), tabs.prev)
		}
		quiet(t, w, "SetSelected")
	})
	t.Run("segmented", func(t *testing.T) {
		s := NewSegmented("Day", "Week", "Month")
		s.OnChange = func(i int, u *gunim.UI) gunim.Intent { return tabbed{i} }
		w, run, u := setterStage(t, s, geom.Sz(300, 30))
		s.SetSelected(2, u)
		glides(t, run, "the pill", 0, 2, s.pill.Value)
		s.SetSelected(0, nil)
		jumps(t, run, "the pill set with no UI", 0, s.pill.Value)
		quiet(t, w, "SetSelected")
	})
	t.Run("slider", func(t *testing.T) {
		s := NewSlider(0, 100)
		s.OnChange = func(v float32, u *gunim.UI) gunim.Intent { return slid{v} }
		w, run, u := setterStage(t, s, geom.Sz(300, 30))
		s.SetValue(80, u)
		glides(t, run, "the knob", 0, 0.8, s.at.Value)
		s.SetValue(20, nil)
		jumps(t, run, "the knob set with no UI", 0.2, s.at.Value)
		quiet(t, w, "SetValue")
	})
	t.Run("progress", func(t *testing.T) {
		b := NewProgressBar()
		_, run, u := setterStage(t, b, geom.Sz(300, 30))
		b.SetValue(0.6, u)
		if b.Value() != 0.6 {
			t.Fatalf("Value is %v while the fill glides, want 0.6", b.Value())
		}
		glides(t, run, "the fill", 0, 0.6, b.Shown)
		b.SetValue(0.1, nil)
		jumps(t, run, "the fill set with no UI", 0.1, b.Shown)
	})
	t.Run("split", func(t *testing.T) {
		s := NewSplit(&block{h: 10}, &block{h: 10})
		s.OnCommit = func(v float32, u *gunim.UI) gunim.Intent { return splitMoved{v} }
		w, run, u := setterStage(t, s, geom.Sz(400, 300))
		s.SetShare(0.8, u)
		glides(t, run, "the share", 0.5, 0.8, s.share.Value)
		s.SetShare(0.3, nil)
		jumps(t, run, "the share set with no UI", 0.3, s.share.Value)
		quiet(t, w, "SetShare")
	})
	t.Run("fold", func(t *testing.T) {
		f := NewFold(&block{h: 40}, false)
		_, run, u := setterStage(t, f, geom.Sz(300, 100))
		f.SetOpen(true, u)
		glides(t, run, "the fold", 0, 1, f.open.Value)
		f.SetOpen(false, nil)
		jumps(t, run, "the fold shut with no UI", 0, f.open.Value)
	})
	t.Run("data grid", func(t *testing.T) {
		g := NewDataGrid(GridColumn{Title: "Message"})
		g.Row = func(int) (GridRow, bool) { return GridRow{}, true }
		g.OnSelect = func(row int, u *gunim.UI) gunim.Intent { return gridSelected{row} }
		g.rows = 100
		w, run, u := setterStage(t, g, geom.Sz(400, 300))
		g.SetSelected(60, u)
		run(5)
		if g.Selected() != 60 || g.Top() != 0 {
			t.Fatalf("SetSelected left row %d selected and the view at %v; want 60, the view where it was", g.Selected(), g.Top())
		}
		g.Reveal(60, u)
		to := float32(61 - g.Visible())
		glides(t, run, "the view", 0, to, func() float32 { return float32(g.Top()) })
		g.SetSelected(5, nil)
		g.Reveal(5, nil)
		jumps(t, run, "the view revealed with no UI", 5, func() float32 { return float32(g.Top()) })
		quiet(t, w, "SetSelected and Reveal")
	})
}

func TestDropdownSetSelectedMovesTheOpenListsHighlightAndSendsNothing(t *testing.T) {
	w, run, d := newPicker(t) // open
	u := stageUI(t, w, run)
	run(60)
	from := d.menu.hotY.Value()
	d.SetSelected(2, u)
	if d.Selected() != 2 || d.menu.Highlighted() != 2 {
		t.Fatalf("SetSelected(2) left %d chosen and %d lit", d.Selected(), d.menu.Highlighted())
	}
	glides(t, run, "the highlight", from, d.menu.rowTop(2), d.menu.hotY.Value)
	click(w, 20, 14) // closes
	run(30)
	d.SetSelected(0, nil)
	click(w, 20, 14) // opens
	run(1)
	if d.menu.Highlighted() != 0 || d.menu.hotY.Value() != d.menu.rowTop(0) {
		t.Fatal("the list opened with the item chosen while closed unlit")
	}
	quiet(t, w, "SetSelected")
}

func TestTextSettersSendNothing(t *testing.T) {
	f := NewTextField()
	f.OnChange = func(s string, u *gunim.UI) gunim.Intent { return s }
	n := NewNumberField(0, 10)
	n.OnChange = func(v float64, u *gunim.UI) gunim.Intent { return v }
	w, run, u := setterStage(t, Column(f, n), geom.Sz(300, 100))
	f.SetText("hello", u)
	n.SetValue(12, u)
	f.SetText("again", nil)
	run(2)
	if f.Text() != "again" || n.Value() != 10 || n.Text() != "10" {
		t.Fatalf("the field holds %q and the number %v, %q", f.Text(), n.Value(), n.Text())
	}
	quiet(t, w, "SetText and SetValue")
}
