package plugui

import (
	"math"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// mark is a knob's modulation as its last frame drew it: the dot's alpha, from 0 to 1, and how far round it stands,
// and how far round the reach's ring runs from and to. A mark not drawn has alpha 0.
type mark struct {
	alpha        float32
	at, from, to float64
	ring         bool
}

// markOf reads the modulation the window's last frame drew on a knob of cap radius 17 at its top left.
func markOf(t *testing.T, w *gunim.Window) mark {
	t.Helper()
	c, r := geom.Pt(KnobW/2, 4+17), float32(17)
	turn := func(p geom.Point) float64 {
		a := math.Atan2(float64(p.Y-c.Y), float64(p.X-c.X))
		for a < knobFrom-0.3 {
			a += 2 * math.Pi
		}
		return (a - knobFrom) / knobSweep
	}
	var m mark
	for _, op := range w.Offscreen().Ops() {
		switch o := op.(type) {
		case *paint.RRectOp:
			if s := o.Rect.Size(); math.Abs(float64(s.W-6)) < 0.01 && math.Abs(float64(s.H-6)) < 0.01 {
				m.alpha, m.at = float32(o.Fill.Solid.A)/255, turn(o.Rect.Center())
			}
		case *paint.RibbonOp:
			first := true
			for _, pt := range o.Points {
				d := pt.At.Sub(c)
				if rad := float32(math.Hypot(float64(d.X), float64(d.Y))); rad < r+3 || rad > r+7 {
					first = false
					break
				}
			}
			if !first || len(o.Points) < 4 {
				continue
			}
			m.ring = true
			m.from, m.to = math.Inf(1), math.Inf(-1)
			for _, pt := range o.Points[2 : len(o.Points)-2] {
				m.from, m.to = min(m.from, turn(pt.At)), max(m.to, turn(pt.At))
			}
		}
	}
	return m
}

// largestStep is the largest share of its way a spring moves in one frame at 60 Hz, and how many frames it takes to
// come within a thousandth of the way.
func largestStep(sp anim.Spring) (share float64, settle int) { return largestStepIn(sp, time.Second/60) }

// largestStepIn is largestStep with frames dt long.
func largestStepIn(sp anim.Spring, dt time.Duration) (share float64, settle int) {
	f := anim.NewFloat(0)
	f.Animate(1, sp)
	most, last := 0.0, float32(0)
	for n := 1; f.Step(dt); n++ {
		most, last = max(most, float64(f.Value()-last)), f.Value()
		if settle == 0 && 1-f.Value() < 1e-3 {
			settle = n
		}
	}
	if settle == 0 {
		settle = 1
	}
	return max(most, float64(1-last)), settle
}

// markGlides checks a value a frame at a time from its read at the frame before to want: no frame moves it further than
// the spring's own largest step of the way, it comes within a thousandth of the way no later than the spring does,
// and it ends on want.
func markGlides(t *testing.T, what string, w *gunim.Window, sp anim.Spring, read func(mark) float64, want float64) {
	t.Helper()
	from := read(markOf(t, w))
	way := math.Abs(want - from)
	share, settle := largestStep(sp)
	most := share*way + 1e-3
	last, near := from, 0
	for f := range 60 {
		frames(w, 1)
		got := read(markOf(t, w))
		if d := math.Abs(got - last); d > most {
			t.Fatalf("%s: frame %d moved the mark %.4f of %.4f at once, more than the spring's own %.4f", what, f+1, d,
				way, most)
		}
		if near == 0 && math.Abs(got-want) <= 1e-3*max(way, 1) {
			near = f + 1
		}
		last = got
	}
	if math.Abs(last-want) > 1e-3 {
		t.Fatalf("%s: settled, the mark stands at %.4f, want %.4f", what, last, want)
	}
	if near > settle+1 {
		t.Fatalf("%s: the mark came near its end on frame %d, later than the spring's own %d", what, near, settle)
	}
}

// A knob's modulation mark fades in as control takes the parameter, glides as the value heard and the reach step,
// and fades out as control lets go; a knob shown with its modulation on draws it at once; and settled, it asks for
// no frames.
func TestAKnobsModulationMarkGlides(t *testing.T) {
	h := &menuHost{host: host{values: map[uint32]float64{3: 100}}, mod: map[uint32]plug.Modulation{}}
	k := NewKnob(h, release)
	w := mountKnob(t, k)
	frames(w, 30)
	if m := markOf(t, w); m.alpha != 0 || m.ring {
		t.Fatalf("with no control, the knob draws a mark %+v", m)
	}
	quick, glide := widget.Quick.Default(), KnobGlide.Default()
	alpha := func(m mark) float64 { return float64(m.alpha) }
	h.mod[3] = plug.Modulation{Low: -0.2, High: 0.2, Now: 400}
	markGlides(t, "fading in", w, quick, alpha, 1)
	at, from := func(m mark) float64 { return m.at }, func(m mark) float64 { return m.from }
	h.mod[3] = plug.Modulation{Low: -0.2, High: 0.2, Now: 100}
	markGlides(t, "the value heard stepping", w, glide, at, k.turnOf(100))
	h.mod[3] = plug.Modulation{Low: -0.4, High: 0.2, Now: 100}
	markGlides(t, "the reach stepping", w, glide, from, k.round(k.p.Normal(100)-0.4))
	for range 10 {
		if k.Step(time.Second / 60) {
			t.Fatal("settled, with control holding still, the knob asks for frames")
		}
	}
	delete(h.mod, 3)
	markGlides(t, "fading out", w, quick, alpha, 0)
	if m := markOf(t, w); m.ring {
		t.Fatal("faded out, the knob still draws the reach's ring")
	}

	shown := NewKnob(&menuHost{host: host{values: map[uint32]float64{3: 100}},
		mod: map[uint32]plug.Modulation{3: {Low: -0.2, High: 0.2, Now: 400}}}, release)
	w = mountKnob(t, shown)
	if m := markOf(t, w); m.alpha != 1 || math.Abs(m.at-shown.turnOf(400)) > 1e-3 {
		t.Fatalf("shown with control on, the knob's first frame draws the mark %+v, want it whole at %.4f", m,
			shown.turnOf(400))
	}
}

// A knob follows a modulation that moves on every frame, a 5 Hz and a 20 Hz sine, from its fourth frame on: from rest it
// glides into the movement, as the needle does, and catches up by no more than the sine's own step.
func TestAKnobFollowsAMovingModulation(t *testing.T) {
	for _, hz := range []float64{5, 20} {
		h := &menuHost{host: host{values: map[uint32]float64{3: 100}}, mod: map[uint32]plug.Modulation{}}
		k := NewKnob(h, release)
		w := mountKnob(t, k)
		frames(w, 30)
		lag := 0.0
		for f := range 120 {
			now := 100 * math.Pow(10, 0.5*math.Sin(2*math.Pi*hz*float64(f)/60))
			h.mod[3] = plug.Modulation{Low: -0.2, High: 0.2, Now: now}
			frames(w, 1)
			if f >= 3 {
				lag = max(lag, math.Abs(markOf(t, w).at-k.turnOf(now))*knobSweep*180/math.Pi)
			}
		}
		t.Logf("%v Hz: the mark lags the value heard by at most %.2f° of the knob's turn", hz, lag)
		if lag > 0.5 {
			t.Errorf("%v Hz: the mark lags the value heard by %.2f° of the knob's turn", hz, lag)
		}
	}
}
