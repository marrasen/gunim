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
