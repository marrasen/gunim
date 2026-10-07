package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// The divider takes the keyboard between the panes. Its keys glide it, frame by frame, and each move says so.
func TestASplitsDividerMovesByTheKeys(t *testing.T) {
	field := NewTextField()
	s := NewSplit(field, NewButton("After"))
	s.OnCommit = func(v float32, u *gunim.UI) gunim.Intent { return splitMoved{v} }
	w, run := stage(t, &frame{child: s, size: geom.Sz(406, 300)})
	focused := focusProbe(t, w, run)
	tab(w, run, 0)
	if f := focused(); f != field {
		t.Fatalf("the first Tab put the keyboard on %T, want the first pane's field", f)
	}
	tab(w, run, 0)
	if f := focused(); f != s.bar {
		t.Fatalf("the second Tab put the keyboard on %T, want the divider", f)
	}
	run(30)
	if r := s.bar.ring.Value(); r < 0.99 {
		t.Fatalf("on the divider, the ring is at %v", r)
	}
	space := s.length - s.gap
	sent(w)
	// glide presses k and follows the first pane's length every frame to want, and checks one move was sent.
	glide := func(k input.KeyPress, want float32) {
		t.Helper()
		from := s.firstLength()
		w.Input(k)
		run(1)
		last := from
		for f := range 40 {
			run(1)
			a := s.firstLength()
			// The glide may bounce a pixel or two past where it stops.
			if (want > from && a < last-2) || (want < from && a > last+2) {
				t.Fatalf("frame %d of %v, the pane went from %v to %v, away from %v", f+1, k.Key, last, a, want)
			}
			if f == 1 && want != from && (a == from || a == want) {
				t.Fatalf("two frames into %v, the pane is %v long: it jumped, or never moved, from %v to %v", k.Key, a,
					from, want)
			}
			last = a
		}
		if d := last - want; d < -1 || d > 1 {
			t.Fatalf("after %v the first pane is %v long, want %v", k.Key, last, want)
		}
		if got := sent(w); len(got) != 1 {
			t.Fatalf("%v sent %v, want one move", k.Key, got)
		}
	}
	glide(input.KeyPress{Key: input.KeyRight}, 200+space/50)
	glide(input.KeyPress{Key: input.KeyLeft, Mods: input.ModShift}, 200+space/50-space/10)
	glide(input.KeyPress{Key: input.KeyEnd}, space-splitMin)
	glide(input.KeyPress{Key: input.KeyHome}, splitMin)
	glide(input.KeyPress{Key: input.KeyEnter}, space/2)
	w.Input(input.KeyPress{Key: input.KeyUp})
	run(2)
	if got := sent(w); len(got) != 0 {
		t.Fatalf("Up on a split side by side sent %v", got)
	}
}

// A screen reader hears the divider once, as a slider of the first pane's share, and can move it.
func TestASplitsDividerReadsOnceAsASlider(t *testing.T) {
	s := NewSplit(NewTextField(), NewButton("After"))
	s.OnCommit = func(v float32, u *gunim.UI) gunim.Intent { return splitMoved{v} }
	w, run := stage(t, &frame{child: s, size: geom.Sz(406, 300)})
	w.Offscreen().ListenForAccess()
	run(2)
	dividers := findAll(w.Offscreen().AccessTree().Root, access.RoleSlider)
	if len(dividers) != 1 || dividers[0].Name != "Divider" || dividers[0].Range.Value != 0.5 {
		t.Fatalf("the tree has %d sliders for the divider, want one at 0.5: %+v", len(dividers), dividers)
	}
	w.Input(access.Request{ID: dividers[0].ID, SetValue: true, Value: 0.25})
	run(30)
	if got := sent(w); len(got) != 1 || got[0] != (splitMoved{0.25}) || s.Share() != 0.25 {
		t.Fatalf("set to 0.25 by a screen reader, the split is at %v and sent %v", s.Share(), got)
	}
}
