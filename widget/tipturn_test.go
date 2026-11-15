package widget

import (
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// A control with no tooltip of its own, enabled again while it shows its reason, closes the tooltip, fading.
func TestAReasonsTooltipClosesAsTheControlIsEnabled(t *testing.T) {
	for _, m := range heldFamily() {
		m.c.Tooltip = ""
		m.c.Disabled, m.c.Why = true, why
		w, run, disable := familyStage(t, []member{m})
		w.Input(input.PointerMove{Pos: geom.Pt(6, 6), Time: time.Now()})
		run(60)
		card := m.c.tip.card
		if card == nil || card.text != why {
			t.Fatalf("resting on the disabled %s, no tooltip of its reason showed", m.name)
		}
		disable(false)
		run(4)
		if m.c.tip.popup != nil || m.c.tip.card != nil {
			t.Fatalf("enabled again with no tooltip of its own, the %s still shows %q", m.name, card.text)
		}
		if v := card.in.Value(); v <= 0.01 || v >= 0.99 {
			t.Errorf("four frames after the %s was enabled, its tooltip is %v in, want fading out", m.name, v)
		}
		run(60)
		if m.c.tip.popup != nil {
			t.Errorf("a second after the %s was enabled, a tooltip shows", m.name)
		}
	}
}

// cardSize is the size of the tooltip card t shows.
func cardSize(t *testing.T, tp *tipper) geom.Size {
	t.Helper()
	if tp.popup == nil {
		t.Fatal("no tooltip shows")
	}
	return tp.popup.Offscreen().Size()
}

// A tooltip's card glides between the sizes of its words as they change, between one line and many and back.
func TestATooltipsCardGlidesAsItsWordsChange(t *testing.T) {
	long := strings.Repeat("All of these words wrap ", 12)
	for _, c := range []struct{ from, to string }{{long, "Short"}, {"Short", long}, {long, long + "more"}} {
		tip := NewTooltip(NewButton("Hover me"), c.from)
		w, run := stage(t, &frame{child: tip, size: geom.Sz(200, 36)})
		gunim.RegisterPatch(w, "stage", func(_ gunim.Node, s setWhy, u *gunim.UI) {
			tip.Text = s.Why
			u.Invalidate()
		})
		w.Input(input.PointerMove{Pos: geom.Pt(20, 18), Time: time.Now()})
		run(60)
		from := cardSize(t, &tip.tip)
		if err := w.Client().Patch("stage", setWhy{c.to}); err != nil {
			t.Fatal(err)
		}
		run(60)
		to := cardSize(t, &tip.tip)
		if err := w.Client().Patch("stage", setWhy{c.from}); err != nil {
			t.Fatal(err)
		}
		run(60)
		if back := cardSize(t, &tip.tip); back != from {
			t.Fatalf("turned back to %.20q, the card is %v, want %v as it was", c.from, back, from)
		}
		if err := w.Client().Patch("stage", setWhy{c.to}); err != nil {
			t.Fatal(err)
		}
		last := from
		for f := range 60 {
			run(1)
			s := cardSize(t, &tip.tip)
			between := func(v, a, b float32) bool { return v >= min(a, b)-2 && v <= max(a, b)+2 }
			if !between(s.W, from.W, to.W) || !between(s.H, from.H, to.H) {
				t.Fatalf("frame %d from %.20q to %.20q, the card is %v, outside %v to %v", f+1, c.from, c.to, s,
					from, to)
			}
			// No frame takes more than half the way at once.
			if d := max(abs32(s.W-last.W)/max(abs32(to.W-from.W), 1), abs32(s.H-last.H)/max(abs32(to.H-from.H), 1)); d > 0.5 {
				t.Fatalf("frame %d from %.20q to %.20q, the card went from %v to %v at once", f+1, c.from, c.to,
					last, s)
			}
			last = s
		}
		if last != to {
			t.Fatalf("settled from %.20q to %.20q, the card is %v, want %v", c.from, c.to, last, to)
		}
	}
}

// pressDrag presses at from, moves through to, and leaves the button down.
func pressDrag(w *gunim.Window, run func(int), from geom.Point, to ...geom.Point) {
	w.Input(input.PointerMove{Pos: from, Time: time.Now()})
	w.Input(input.PointerDown{Pos: from, Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	run(1)
	for _, p := range to {
		w.Input(input.PointerMove{Pos: p, Time: time.Now()})
		run(1)
	}
}

// passOver moves the pointer along y with no button down.
func passOver(w *gunim.Window, run func(int), y float32) {
	for _, x := range []float32{10, 80, 160, 240, 160, 80} {
		w.Input(input.PointerMove{Pos: geom.Pt(x, y), Time: time.Now()})
		run(1)
	}
}

// A slider disabled in the middle of a drag ends the drag at once: one commit for the change made, and no more
// changes, then or after.
func TestASliderDisabledMidDragEndsTheDrag(t *testing.T) {
	s := NewSlider(0, 100)
	s.SetValue(50, nil)
	changes, commits := 0, 0
	s.OnChange = func(float32, *gunim.UI) gunim.Intent { changes++; return nil }
	s.OnCommit = func(float32, *gunim.UI) gunim.Intent { commits++; return nil }
	w, run := stage(t, &frame{child: Column(s), size: geom.Sz(300, 100)})
	y := s.size.H / 2
	pressDrag(w, run, geom.Pt(150, y), geom.Pt(170, y), geom.Pt(190, y))
	if changes == 0 || commits != 0 {
		t.Fatalf("the drag changed the slider %d times and committed %d, want changes and no commit yet", changes,
			commits)
	}
	was, at := changes, s.Value()
	s.Disabled = true
	run(1)
	if commits != 1 || s.held {
		t.Fatalf("a frame after it was disabled mid-drag, the slider committed %d times and is held %v", commits,
			s.held)
	}
	w.Input(input.PointerMove{Pos: geom.Pt(250, y), Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(250, y), Time: time.Now()})
	run(1)
	s.Disabled = false
	run(30)
	passOver(w, run, y)
	if changes != was || commits != 1 || s.Value() != at {
		t.Fatalf("after it was disabled mid-drag, the slider changed %d more times, committed %d, and stands at %v, "+
			"want %v", changes-was, commits, s.Value(), at)
	}
}

// A number field disabled in the middle of a drag ends the drag at once: one commit, one end of the drag, and no
// more changes.
func TestANumberFieldDisabledMidDragEndsTheDrag(t *testing.T) {
	n := NewNumberField(0, 1000)
	n.SetValue(500, nil)
	changes, commits, ends := 0, 0, 0
	n.OnChange = func(float64, *gunim.UI) gunim.Intent { changes++; return nil }
	n.OnCommit = func(float64, *gunim.UI) gunim.Intent { commits++; return nil }
	n.OnDragEnd = func(float64, *gunim.UI) gunim.Intent { ends++; return nil }
	w, run := stage(t, &frame{child: Column(n), size: geom.Sz(300, 100)})
	y := float32(14)
	pressDrag(w, run, geom.Pt(60, y), geom.Pt(60, y-10), geom.Pt(60, y-30), geom.Pt(60, y-50))
	if changes == 0 {
		t.Fatal("the drag changed the field none")
	}
	was, at := changes, n.Value()
	n.Disabled = true
	run(1)
	if commits != 1 || ends != 1 || n.drag.held {
		t.Fatalf("a frame after it was disabled mid-drag, the field committed %d times, ended %d drags, held %v",
			commits, ends, n.drag.held)
	}
	w.Input(input.PointerMove{Pos: geom.Pt(60, y-90), Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(60, y-90), Time: time.Now()})
	run(1)
	n.Disabled = false
	run(30)
	passOver(w, run, y)
	if changes != was || commits != 1 || ends != 1 || n.Value() != at {
		t.Fatalf("after it was disabled mid-drag, the field changed %d more times, committed %d, ended %d drags, "+
			"and stands at %v, want %v", changes-was, commits, ends, n.Value(), at)
	}
}
