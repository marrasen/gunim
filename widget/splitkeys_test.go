package widget

import (
	"testing"

	"github.com/marrasen/gunim"
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
