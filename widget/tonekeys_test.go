package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// A ToneCurve takes the keyboard: Tab chooses the next point and leaves past the last, the arrows glide the chosen
// point, a move commits once the keys rest, and Delete takes out a point between the ends.
func TestAToneCurveWorksFromTheKeyboard(t *testing.T) {
	c := NewToneCurve()
	var commits int
	c.OnCommit = func(_ []geom.Point, _ *gunim.UI) gunim.Intent {
		commits++
		return nil
	}
	c.SetPoints([]geom.Point{{X: 0, Y: 0}, {X: 0.5, Y: 0.5}, {X: 1, Y: 1}}, nil)
	after := NewButton("After")
	w, run := stage(t, &frame{child: Column(c, after), size: geom.Sz(300, 400)})
	focused := focusProbe(t, w, run)
	tab(w, run, 0)
	if f := focused(); f != c {
		t.Fatalf("Tab put the keyboard on %T, want the curve", f)
	}
	if c.chosen != 0 {
		t.Fatalf("the keyboard came with point %d chosen, want the first", c.chosen)
	}
	tab(w, run, 0)
	if f := focused(); f != c || c.chosen != 1 {
		t.Fatalf("a second Tab left the keyboard on %T with point %d chosen, want the curve's middle point", f, c.chosen)
	}
	// The point glides up, and right with Shift, every frame nearer where it goes and never past it by much.
	for _, k := range []input.KeyPress{{Key: input.KeyUp}, {Key: input.KeyRight, Mods: input.ModShift}} {
		before := c.toBox(c.Points()[1])
		w.Input(k)
		run(1)
		target := c.toBox(c.Points()[1])
		if target == before {
			t.Fatalf("%v left the middle point where it was", k.Key)
		}
		gap := func() float32 {
			d := target.Sub(c.toBox(c.Points()[1]).Add(c.nudge.Value()))
			return d.X*d.X + d.Y*d.Y
		}
		last := gap()
		if last < 1 {
			t.Fatalf("a frame after %v, the point shows at its new place already: it jumped", k.Key)
		}
		for f := range 40 {
			run(1)
			g := gap()
			if g > last+0.5 {
				t.Fatalf("frame %d of %v, the point drew away from where it goes, %v to %v", f+1, k.Key, last, g)
			}
			last = g
		}
	}
	if p := c.Points()[1]; p.Y <= 0.5 || p.X <= 0.5 {
		t.Fatalf("Up, then Shift+Right, left the middle point at %v", p)
	}
	if commits != 2 {
		t.Fatalf("two moves by the keys, each let rest, committed %d times, want twice", commits)
	}
	// Keys pressed one after another commit once, as they rest.
	for range 3 {
		w.Input(input.KeyPress{Key: input.KeyDown})
		run(3)
	}
	if commits != 2 {
		t.Fatalf("the keys still moving the point, it committed %d times in all, want still 2", commits)
	}
	run(40)
	if commits != 3 {
		t.Fatalf("three quick moves committed %d times in all once they rested, want 3", commits)
	}
	w.Input(input.KeyPress{Key: input.KeyDelete})
	run(1)
	if n := len(c.Points()); n != 2 || commits != 4 {
		t.Fatalf("Delete on the middle point left %d points and %d commits, want 2 and 4", n, commits)
	}
	w.Input(input.KeyPress{Key: input.KeyDelete})
	run(1)
	if n := len(c.Points()); n != 2 {
		t.Fatalf("Delete on an end left %d points, want the ends kept", n)
	}
	tab(w, run, 0)
	if f := focused(); f != after {
		t.Fatalf("Tab on the last point put the keyboard on %T, want the button after the curve", f)
	}
}
