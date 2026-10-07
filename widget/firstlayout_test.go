package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
)

// mountSet mounts root in a window, with set run in the view's update before the first layout, and returns what
// runs a frame.
func mountSet(t *testing.T, root gunim.Node, set func(u *gunim.UI)) func() {
	t.Helper()
	w := gunimtest.New(t, geom.Sz(600, 400), nil)
	gunim.RegisterView(w, "v", func(struct{}) gunim.Node { return root },
		func(_ gunim.Node, _ struct{}, u *gunim.UI) { set(u) })
	if err := w.Client().Mount(gunim.Root, "v", "v", struct{}{}); err != nil {
		t.Fatal(err)
	}
	return func() { w.Frame(time.Second / 60) }
}

// held asserts on every frame of a second that got gives want, as a value set before the first layout shows at once.
func held(t *testing.T, frame func(), what string, want float32, got func() float32) {
	t.Helper()
	for i := range 60 {
		frame()
		if g := got(); abs32(g-want) > 1e-4 {
			t.Fatalf("frame %d: %s is at %v, want %v from the start", i, what, g, want)
		}
	}
}

func TestValuesSetBeforeTheFirstLayoutShowAtOnce(t *testing.T) {
	t.Run("slider", func(t *testing.T) {
		s := NewSlider(0, 100)
		step := mountSet(t, &frame{child: s, size: geom.Sz(300, 30)}, func(u *gunim.UI) { s.SetValue(60, u) })
		held(t, step, "the slider's knob", 0.6, s.at.Value)
	})
	t.Run("progress", func(t *testing.T) {
		b := NewProgressBar()
		step := mountSet(t, &frame{child: b, size: geom.Sz(300, 30)}, func(u *gunim.UI) { b.Set(0.4, u) })
		held(t, step, "the progress bar", 0.4, b.Value)
	})
	t.Run("drawer", func(t *testing.T) {
		d := NewDrawer(NewLabel("main"), NewLabel("panel"))
		step := mountSet(t, d, func(u *gunim.UI) { d.SetOpen(true, u) })
		held(t, step, "the drawer", 1, d.open.Value)
	})
	t.Run("tone curve", func(t *testing.T) {
		c := NewToneCurve()
		step := mountSet(t, &frame{child: c, size: geom.Sz(200, 200)}, func(u *gunim.UI) {
			c.SetPoints([]geom.Point{{X: 0, Y: 0.2}, {X: 1, Y: 0.8}}, u)
		})
		held(t, step, "the curve's morph", 1, c.morph.Value)
		if c.from != c.to {
			t.Fatal("the curve set before the first layout morphs from another shape")
		}
	})
	t.Run("slider row", func(t *testing.T) {
		s := NewSlider(-100, 100)
		s.Rest, s.HasRest = 0, true
		s.Set(80)
		row := NewSliderRow("Contrast", s)
		step := mountSet(t, &frame{child: row, size: geom.Sz(300, 28)}, func(*gunim.UI) {})
		held(t, step, "the reset mark", 1, row.off.Value)
	})
}
