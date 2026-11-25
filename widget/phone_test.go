package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
)

// phoneStage mounts root on a surface in an offscreen window of size, keeping time by its frames as a phone's
// touch tests want, and returns the window, a grab of its UI and a function that runs n frames.
func phoneStage(t *testing.T, size geom.Size, root gunim.Node) (*gunim.Window, *grab, func(int)) {
	t.Helper()
	w := gunimtest.New(t, size, NewSurface())
	w.PaintWhenLooked(false)
	g := &grab{child: root}
	gunim.RegisterView(w, "phone", func(struct{}) gunim.Node { return g }, nil)
	if err := w.Client().Mount(gunim.Root, "phone", "phone", struct{}{}); err != nil {
		t.Fatal(err)
	}
	w.TimeByFrames(true)
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(2)
	return w, g, run
}

// fingerDrag lands a finger at from, drags it to the point to in steps over d, running a frame after each, waits hold, and
// lifts it. A hold of a tenth of a second or more lets go with no fling.
func fingerDrag(w *gunim.Window, run func(int), from, to geom.Point, steps int, d, hold time.Duration) {
	t0 := time.Now()
	w.Input(input.PointerMove{Pos: from, Touch: true, Time: t0})
	w.Input(input.PointerDown{Pos: from, Button: input.ButtonPrimary, Clicks: 1, Touch: true, Time: t0})
	run(1)
	for i := 1; i <= steps; i++ {
		f := float32(i) / float32(steps)
		p := geom.Pt(from.X+(to.X-from.X)*f, from.Y+(to.Y-from.Y)*f)
		w.Input(input.PointerMove{Pos: p, Touch: true, Time: t0.Add(d * time.Duration(i) / time.Duration(steps))})
		run(1)
	}
	end := t0.Add(d + hold)
	w.Input(input.PointerUp{Pos: to, Button: input.ButtonPrimary, Touch: true, Time: end})
	w.Input(input.PointerLeave{Time: end})
}

// tap lands a finger at p and lifts it.
func tap(w *gunim.Window, run func(int), p geom.Point) {
	touchDown(w, p)
	run(1)
	touchUp(w, p)
	run(1)
}
