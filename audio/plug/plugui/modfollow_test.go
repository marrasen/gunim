package plugui

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/paint"
)

// source is the modulation a host hands a knob for a frame at time at since the source began, and false for none.
type source func(at time.Duration) (plug.Modulation, bool)

// heard is what a frame asked of a knob's mark and what it drew: the turns of the value heard and of the reach's ends.
type heard struct {
	drawn              mark
	at, from, to       float64
	on                 bool
	dt                 time.Duration
	atStep, reachStep  float64
	drawnAt, drawnFrom float64
}

// timings are the frames a test plays, by name: even ones at a rate, and uneven ones.
func timings(name string, d time.Duration) []time.Duration {
	uneven := []time.Duration{8, 33, 12, 16, 50, 8, 16, 16, 25}
	var out []time.Duration
	for i, at := 0, time.Duration(0); at < d; i++ {
		step := uneven[i%len(uneven)] * time.Millisecond
		if name != "uneven" {
			var hz float64
			fmt.Sscan(name, &hz)
			step = time.Duration(float64(time.Second) / hz)
		}
		out = append(out, step)
		at += step
	}
	return out
}

// play shows a knob of p at v, held still by what src gives at 0 until it settles, and then hands it src at each
// frame of dts, and returns what each frame asked and drew.
func play(t *testing.T, p plug.Param, v float64, dts []time.Duration, src source) []heard {
	t.Helper()
	h := &menuHost{host: host{values: map[uint32]float64{p.ID: v}}, mod: map[uint32]plug.Modulation{}}
	if m, ok := src(0); ok {
		h.mod[p.ID] = m
	}
	k := NewKnob(h, p)
	w := mountKnob(t, k)
	frames(w, 60)
	ask := func(m plug.Modulation) (at, from, to float64) {
		n := k.p.Normal(v)
		return k.turnOf(m.Now), k.round(n + m.Low), k.round(n + m.High)
	}
	var out []heard
	last := heard{drawn: markOf(t, w)}
	last.at, last.from, last.to = ask(h.mod[p.ID])
	elapsed := time.Duration(0)
	for _, dt := range dts {
		elapsed += dt
		m, ok := src(elapsed)
		if ok {
			h.mod[p.ID] = m
		} else {
			delete(h.mod, p.ID)
		}
		w.Frame(dt)
		got := heard{drawn: markOf(t, w), on: ok, dt: dt}
		got.at, got.from, got.to = ask(m)
		got.atStep, got.reachStep = math.Abs(got.at-last.at), max(math.Abs(got.from-last.from), math.Abs(got.to-last.to))
		got.drawnAt, got.drawnFrom = math.Abs(got.drawn.at-last.drawn.at), max(math.Abs(got.drawn.from-last.drawn.from),
			math.Abs(got.drawn.to-last.drawn.to))
		out = append(out, got)
		last = got
	}
	return out
}

// degrees is a share of the knob's turn in degrees.
func degrees(turn float64) float64 { return turn * knobSweep * 180 / math.Pi }

// followsTruly checks what a source asked of a knob's mark against what it drew: the dot and the ring never move
// further in a frame than the source's own largest step, and both end where the source does.
func followsTruly(t *testing.T, what string, hs []heard) {
	t.Helper()
	srcAt, srcReach, drawnAt, drawnReach := 0.0, 0.0, 0.0, 0.0
	for _, h := range hs {
		srcAt, srcReach = max(srcAt, h.atStep), max(srcReach, h.reachStep)
		drawnAt, drawnReach = max(drawnAt, h.drawnAt), max(drawnReach, h.drawnFrom)
	}
	if drawnAt > srcAt+1e-4 {
		t.Errorf("%s: the dot moved %.2f° in a frame, where the source's largest step is %.2f°", what,
			degrees(drawnAt), degrees(srcAt))
	}
	if drawnReach > srcReach+1e-4 {
		t.Errorf("%s: the ring moved %.2f° in a frame, where the source's largest step is %.2f°", what,
			degrees(drawnReach), degrees(srcReach))
	}
	end := hs[len(hs)-1]
	if math.Abs(end.drawn.at-end.at) > 1e-3 || math.Abs(end.drawn.from-end.from) > 1e-3 ||
		math.Abs(end.drawn.to-end.to) > 1e-3 {
		t.Errorf("%s: settled, the mark is drawn at %.4f from %.4f to %.4f, want %.4f from %.4f to %.4f", what,
			end.drawn.at, end.drawn.from, end.drawn.to, end.at, end.from, end.to)
	}
}

var (
	unit   = plug.Param{ID: 1, Name: "Mix", Max: 1}
	steps8 = plug.Param{ID: 5, Name: "Count", Max: 8, Steps: 8}
)

// A value a host hands once per audio block of 10.7 ms, a 5 Hz sine, is followed closely at every frame rate and
// with uneven frames: no lag once it moves, and no step larger than the source's own.
func TestAKnobFollowsAValueHandedOncePerBlock(t *testing.T) {
	const block = 10700 * time.Microsecond
	for _, rate := range []string{"60", "120", "144", "240", "uneven"} {
		hs := play(t, unit, 0.5, timings(rate, 2*time.Second), func(at time.Duration) (plug.Modulation, bool) {
			b := float64(at/block) * block.Seconds()
			return plug.Modulation{Low: -0.4, High: 0.4, Now: 0.5 + 0.4*math.Sin(2*math.Pi*5*b)}, true
		})
		lag := 0.0
		for i, h := range hs {
			if elapsedOf(hs[:i+1]) > 100*time.Millisecond {
				lag = max(lag, math.Abs(h.drawn.at-h.at))
			}
		}
		if degrees(lag) > 0.5 {
			t.Errorf("%s Hz: once moving, the dot lags the value heard by %.2f°", rate, degrees(lag))
		}
		followsTruly(t, rate+" Hz, handed once per block", hs)
	}
}

// elapsedOf is how long the frames of hs took.
func elapsedOf(hs []heard) time.Duration {
	var d time.Duration
	for _, h := range hs {
		d += h.dt
	}
	return d
}

// holding is a source that gives the values of steps on frames in a row from its second frame, the first being where it
// stands still before, and then holds the last.
func holding(p plug.Param, low, high float64, steps ...float64) func(dts []time.Duration) source {
	return func(dts []time.Duration) source {
		at := map[time.Duration]int{}
		e := time.Duration(0)
		for i, dt := range dts {
			e += dt
			at[e] = i + 1
		}
		return func(t time.Duration) (plug.Modulation, bool) {
			i := min(at[t], len(steps)-1)
			return plug.Modulation{Low: low, High: high, Now: steps[i]}, true
		}
	}
}

// Changes on frames in a row, steps of a count, a small step and then end to end, and an envelope's attack, never
// move the dot further in a frame than the source moves it, and it lands where the source does.
func TestAKnobsMarkNeverOutrunsItsSource(t *testing.T) {
	attack := func(d time.Duration) func([]time.Duration) source {
		return func([]time.Duration) source {
			return func(at time.Duration) (plug.Modulation, bool) {
				return plug.Modulation{High: 1, Now: min(at.Seconds()/d.Seconds(), 1)}, true
			}
		}
	}
	for _, rate := range []string{"60", "120", "uneven"} {
		for _, c := range []struct {
			name string
			p    plug.Param
			src  func([]time.Duration) source
		}{
			{"a count 4, 5, 6 on frames in a row", steps8, holding(steps8, -0.5, 0.5, 4, 5, 6)},
			{"a small step and then end to end", unit, holding(unit, -0.1, 0.1, 0, 0.03, 1)},
			{"an attack of 10 ms and a hold", unit, attack(10 * time.Millisecond)},
			{"an attack of 50 ms and a hold", unit, attack(50 * time.Millisecond)},
		} {
			dts := timings(rate, time.Second)
			hs := play(t, c.p, 0.5, dts, c.src(dts))
			followsTruly(t, fmt.Sprintf("%s at %s Hz", c.name, rate), hs)
		}
	}
}

// A reach that steps while the value moves, and a value that steps while the reach moves, move each as far as its
// own source does, and no further.
func TestAKnobsValueAndReachFollowApart(t *testing.T) {
	sine := func(at time.Duration) float64 { return math.Sin(2 * math.Pi * 2 * at.Seconds()) }
	for _, rate := range []string{"60", "120", "uneven"} {
		hs := play(t, unit, 0.5, timings(rate, time.Second), func(at time.Duration) (plug.Modulation, bool) {
			reach := 0.1
			if at > 400*time.Millisecond {
				reach = 0.4
			}
			if at > 700*time.Millisecond {
				return plug.Modulation{Low: -reach, High: reach, Now: 0.5}, true
			}
			return plug.Modulation{Low: -reach, High: reach, Now: 0.5 + 0.3*sine(at)}, true
		})
		followsTruly(t, "a reach that steps while the value moves at "+rate+" Hz", hs)
		glidesAlone(t, "a reach that steps while the value moves at "+rate+" Hz", hs, func(h heard) (float64, float64) {
			return h.reachStep, h.drawnFrom
		})
		hs = play(t, unit, 0.5, timings(rate, time.Second), func(at time.Duration) (plug.Modulation, bool) {
			now := 0.2
			if at > 400*time.Millisecond {
				now = 0.9
			}
			if at > 700*time.Millisecond {
				return plug.Modulation{Low: -0.2, High: 0.2, Now: now}, true
			}
			return plug.Modulation{Low: -0.2 + 0.1*sine(at), High: 0.2 + 0.1*sine(at), Now: now}, true
		})
		followsTruly(t, "a value that steps while the reach moves at "+rate+" Hz", hs)
		glidesAlone(t, "a value that steps while the reach moves at "+rate+" Hz", hs, func(h heard) (float64, float64) {
			return h.atStep, h.drawnAt
		})
	}
}

// glidesAlone checks the one step a part of the mark takes, its source's step and its drawn steps as steps reads
// them, glides on the knob's spring: no drawn step is more than the spring's own largest share of it in the longest
// frame.
func glidesAlone(t *testing.T, what string, hs []heard, steps func(heard) (src, drawn float64)) {
	t.Helper()
	longest := time.Duration(0)
	for _, h := range hs {
		longest = max(longest, h.dt)
	}
	share, _ := largestStepIn(KnobGlide.Default(), longest)
	src, drawn := 0.0, 0.0
	for _, h := range hs {
		s, d := steps(h)
		src, drawn = max(src, s), max(drawn, d)
	}
	if drawn > share*src+1e-3 {
		t.Errorf("%s: the step of %.2f° moved %.2f° in a frame, where a glide moves %.2f° at most", what, degrees(src),
			degrees(drawn), degrees(share*src))
	}
}

// shelf holds a knob that comes and goes, and builds one in its first layout where build is set.
type shelf struct {
	build *Knob
	built bool
}

func (s *shelf) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	lay := func(k gunim.Child) {
		k.Layout(gunim.Tight(geom.Sz(KnobW, KnobH)))
		k.Place(geom.Point{})
	}
	if s.build != nil && !s.built {
		s.built = true
		lay(kids.Build(s.build))
	}
	for k := range kids.All {
		lay(k)
	}
	return c.Max
}

func (s *shelf) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for k := range kids.All {
		k.Paint(p)
	}
}

type put struct{ On bool }

// mountShelf mounts a shelf, to show from the next frame, and returns the window, and what puts k on or takes it off.
func mountShelf(t *testing.T, s *shelf, k *Knob) (*gunim.Window, func(on bool)) {
	t.Helper()
	w := gunimtest.New(t, geom.Sz(KnobW, KnobH), nil)
	gunim.RegisterView(w, "shelf", func(struct{}) gunim.Node { return s }, nil)
	gunim.RegisterPatch(w, "shelf", func(_ gunim.Node, p put, u *gunim.UI) {
		if p.On {
			u.Insert(s, k)
		} else {
			u.Remove(k)
		}
	})
	if err := w.Client().Mount(gunim.Root, "shelf", "shelf", nil); err != nil {
		t.Fatal(err)
	}
	return w, func(on bool) {
		t.Helper()
		if err := w.Client().Patch("shelf", put{on}); err != nil {
			t.Fatal(err)
		}
	}
}

// A knob's first frame shows its mark as control has it: as it comes back into the tree after control let go or
// moved, and as it is built in layout with control on.
func TestAKnobsFirstFrameShowsItsMarkAsItIs(t *testing.T) {
	for _, c := range []struct {
		name  string
		while func(h *menuHost)
		want  mark
	}{
		{"control let go while it was out", func(h *menuHost) { delete(h.mod, 1) }, mark{}},
		{"the value heard moved while it was out", func(h *menuHost) {
			h.mod[1] = plug.Modulation{Low: -0.1, High: 0.1, Now: 0.9}
		}, mark{alpha: 1, at: 0.9}},
	} {
		h := &menuHost{host: host{values: map[uint32]float64{1: 0.5}},
			mod: map[uint32]plug.Modulation{1: {Low: -0.1, High: 0.1, Now: 0.2}}}
		k := NewKnob(h, unit)
		w, show := mountShelf(t, &shelf{}, k)
		frames(w, 1)
		show(true)
		frames(w, 60)
		show(false)
		frames(w, 30)
		c.while(h)
		show(true)
		frames(w, 1)
		got := markOf(t, w)
		if got.alpha != c.want.alpha || c.want.alpha > 0 && math.Abs(got.at-c.want.at) > 1e-3 {
			t.Errorf("%s: the first frame back draws the mark %+v, want %+v", c.name, got, c.want)
		}
	}
	h := &menuHost{host: host{values: map[uint32]float64{1: 0.5}},
		mod: map[uint32]plug.Modulation{1: {Low: -0.1, High: 0.1, Now: 0.2}}}
	k := NewKnob(h, unit)
	w, _ := mountShelf(t, &shelf{build: k}, nil)
	frames(w, 1)
	if got := markOf(t, w); got.alpha != 1 || math.Abs(got.at-0.2) > 1e-3 {
		t.Errorf("built in layout with control on, the knob's first frame draws the mark %+v, want it whole at 0.2", got)
	}
}

// A knob that never has a modulation costs what it did before the mark glided: 10 allocations and 1 808 bytes to
// make, and 3 allocations to step and paint.
func TestAKnobWithNoModulationCostsWhatItDid(t *testing.T) {
	h := &host{values: map[uint32]float64{3: 100}}
	if n := testing.AllocsPerRun(200, func() { NewKnob(h, release) }); n > 10 {
		t.Errorf("a new knob takes %v allocations, want 10 at most", n)
	}
	r := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			NewKnob(h, release)
		}
	})
	if n := r.AllocedBytesPerOp(); n > 1808 {
		t.Errorf("a new knob takes %d bytes, want 1 808 at most", n)
	}
	k := NewKnob(h, release)
	k.Step(0)
	k.Layout(gunim.Tight(geom.Sz(KnobW, KnobH)), gunim.Frame{}, gunim.Children{})
	var p paint.Painter
	if n := testing.AllocsPerRun(200, func() {
		p.Reset()
		k.Step(16 * time.Millisecond)
		k.Paint(&p, gunim.Frame{Scale: 1}, geom.Sz(KnobW, KnobH), gunim.Children{})
	}); n > 3 {
		t.Errorf("stepping and painting a knob with no modulation takes %v allocations, want 3 at most", n)
	}
}
