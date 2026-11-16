package plugui

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
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
			in := len(o.Points) >= 4
			for _, pt := range o.Points {
				d := pt.At.Sub(c)
				if rad := float32(math.Hypot(float64(d.X), float64(d.Y))); rad < r+3 || rad > r+7 {
					in = false
					break
				}
			}
			if !in {
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

// largestStepIn is the largest share of its way a spring moves in one frame dt long, and how many frames it takes to
// come within a thousandth of the way.
func largestStepIn(sp anim.Spring, dt time.Duration) (share float64, settle int) {
	f := anim.NewFloat(0)
	f.RealTime()
	f.Animate(1, sp)
	most, last, n := 0.0, float32(0), 0
	for moving := true; moving; {
		moving = f.Step(dt)
		n++
		most, last = max(most, float64(f.Value()-last)), f.Value()
		if settle == 0 && 1-f.Value() < 1e-3 {
			settle = n
		}
	}
	return most, settle
}

// wanted is the mark a knob drawing m, as master does, draws at its needle now.
func wanted(k *Knob, m plug.Modulation) mark {
	n := k.normalAt(k.needle())
	from, to := k.round(n+m.Low), k.round(n+m.High)
	if m.Sets {
		from, to = 0, 1
	}
	return mark{alpha: 1, at: k.turnOf(m.Now), from: min(from, to), to: max(from, to)}
}

// samePlace reports whether got stands where want does: the dot, and the ring where want's spans 1/40 or more.
func samePlace(got, want mark) bool {
	const near = 1e-4
	if math.Abs(got.at-want.at) > near {
		return false
	}
	if want.to-want.from < 1.0/40 {
		return !got.ring
	}
	return got.ring && math.Abs(got.from-want.from) < near && math.Abs(got.to-want.to) < near
}

// timings are frames over d, by name: even ones at a rate, or uneven ones.
func timings(name string, d time.Duration) []time.Duration {
	uneven := []int{8, 33, 12, 16, 50, 8, 16, 16, 25}
	var out []time.Duration
	for i, at := 0, time.Duration(0); at < d; i++ {
		step := time.Duration(uneven[i%len(uneven)]) * time.Millisecond
		if name != "uneven" {
			hz, err := strconv.ParseFloat(name, 64)
			if err != nil {
				panic(err)
			}
			step = time.Duration(float64(time.Second) / hz)
		}
		out = append(out, step)
		at += step
	}
	return out
}

var (
	unit   = plug.Param{ID: 1, Name: "Mix", Max: 1}
	steps8 = plug.Param{ID: 5, Name: "Count", Max: 8, Steps: 8}
)

// A modulation's mark stands where the value handed stands on every frame, as master draws it: a sine handed every
// frame and once per audio block, stepped sources, a count on frames in a row, an attack, and the knob dragged.
func TestAKnobsMarkStandsWhereItsSourceIs(t *testing.T) {
	const block = 10700 * time.Microsecond
	sine := func(hz float64, per time.Duration) func(time.Duration) plug.Modulation {
		return func(at time.Duration) plug.Modulation {
			if per > 0 {
				at = at / per * per
			}
			return plug.Modulation{Low: -0.3, High: 0.3, Now: 0.5 + 0.4*math.Sin(2*math.Pi*hz*at.Seconds())}
		}
	}
	stepped := func(every time.Duration) func(time.Duration) plug.Modulation {
		return func(at time.Duration) plug.Modulation {
			i := int(at / every)
			return plug.Modulation{Low: -0.2, High: 0.2, Sets: i%3 == 0, Now: float64((i*37)%10) / 10}
		}
	}
	cases := []struct {
		name string
		p    plug.Param
		src  func(time.Duration) plug.Modulation
		drag bool
	}{
		{"a 5 Hz sine handed every frame", unit, sine(5, 0), false},
		{"a 5 Hz sine handed once per block", unit, sine(5, block), false},
		{"a stepped source every 59 ms", unit, stepped(59 * time.Millisecond), false},
		{"a stepped source every 8 ms", unit, stepped(8 * time.Millisecond), false},
		{"a count on frames in a row", steps8, func(at time.Duration) plug.Modulation {
			return plug.Modulation{Low: -0.5, High: 0.5, Now: float64(min(int(at/(16*time.Millisecond)), 8))}
		}, false},
		{"an attack of 50 ms", unit, func(at time.Duration) plug.Modulation {
			return plug.Modulation{High: 1, Now: min(at.Seconds()/0.05, 1)}
		}, false},
		{"the knob dragged under a held value", unit, func(time.Duration) plug.Modulation {
			return plug.Modulation{Low: -0.1, High: 0.4, Now: 0.8}
		}, true},
	}
	for _, rate := range []string{"60", "120", "144", "240", "uneven"} {
		for _, c := range cases {
			h := &menuHost{host: host{values: map[uint32]float64{c.p.ID: c.p.Max / 2}},
				mod: map[uint32]plug.Modulation{c.p.ID: c.src(0)}}
			k := NewKnob(h, c.p)
			w := mountPainted(t, k)
			frames(w, 30)
			if c.drag {
				w.Input(input.PointerMove{Pos: geom.Pt(KnobW/2, 21), Time: time.Now()})
				w.Input(input.PointerDown{Pos: geom.Pt(KnobW/2, 21), Button: input.ButtonPrimary, Clicks: 1,
					Time: time.Now()})
			}
			worst, at := 0.0, time.Duration(0)
			for i, dt := range timings(rate, time.Second) {
				at += dt
				m := c.src(at)
				h.mod[c.p.ID] = m
				if c.drag {
					w.Input(input.PointerMove{Pos: geom.Pt(KnobW/2, 21-float32(i%40)), Time: time.Now()})
				}
				w.Frame(dt)
				got, want := markOf(t, w), wanted(k, m)
				if !samePlace(got, want) || got.alpha != 1 {
					worst = max(worst, math.Abs(got.at-want.at), math.Abs(got.from-want.from), math.Abs(got.to-want.to))
					if worst == 0 {
						worst = 1
					}
				}
			}
			if worst > 0 {
				t.Errorf("%s at %s Hz: the mark stands up to %.2f° from where the value handed stands", c.name, rate,
					worst*knobSweep*180/math.Pi)
			}
		}
	}
}

// mountPainted shows k alone in a window that paints every frame, as a window on a screen does.
func mountPainted(t *testing.T, k *Knob) *gunim.Window {
	t.Helper()
	return mountThemed(t, k, theme.Theme{})
}

// slowQuick is a theme whose quick spring takes about 0.6 s.
var slowQuick = theme.Make("slow", theme.Set(widget.Quick, anim.Spring{Response: 0.6, Damping: 1}))

// mountThemed shows k alone in a window that wears th, where th has a name, from its first frame.
func mountThemed(t *testing.T, k *Knob, th theme.Theme) *gunim.Window {
	t.Helper()
	w := gunimtest.New(t, geom.Sz(KnobW, KnobH), nil)
	w.PaintWhenLooked(false)
	if th.Name != "" {
		w.RegisterTheme(th)
		if err := w.Client().SetTheme(th.Name); err != nil {
			t.Fatal(err)
		}
	}
	gunim.RegisterView(w, "knob", func(struct{}) gunim.Node { return k }, nil)
	if err := w.Client().Mount(gunim.Root, "knob", "knob", nil); err != nil {
		t.Fatal(err)
	}
	frames(w, 1)
	return w
}

// fades checks the mark's alpha a frame at a time to want, from what the last frame drew: no frame moves it further
// than the spring's own largest step of a whole fade, it comes near its end no later than the spring does, and it ends
// on want. Each frame its place must be where at says.
func fades(t *testing.T, what string, w *gunim.Window, sp anim.Spring, want float32, at func() mark) {
	t.Helper()
	from := markOf(t, w).alpha
	share, settle := largestStepIn(sp, time.Second/60)
	most := share + 1e-2
	last, near := from, 0
	for f := range 3 * settle {
		frames(w, 1)
		got := markOf(t, w)
		if d := math.Abs(float64(got.alpha - last)); d > most {
			t.Fatalf("%s: frame %d moved the mark's strength %.3f at once, more than the spring's own %.3f", what, f+1,
				d, most)
		}
		if got.alpha > 0 && !samePlace(got, at()) {
			t.Fatalf("%s: frame %d drew the mark %+v, want it at %+v", what, f+1, got, at())
		}
		if near == 0 && math.Abs(float64(got.alpha-want)) < 0.01 {
			near = f + 1
		}
		last = got.alpha
	}
	if last != want {
		t.Fatalf("%s: settled, the mark's strength is %v, want %v", what, last, want)
	}
	if near > settle+1 {
		t.Fatalf("%s: the mark's strength came near %v on frame %d, later than the spring's own %d", what, want, near,
			settle)
	}
}

// A modulation's mark fades in as control takes the parameter and out as it lets go, on the theme's quick spring, a
// slow one too, standing where the value handed stands as it fades in and where it last stood as it fades out; and
// settled, the knob asks for no frames.
func TestAKnobsModulationMarkFades(t *testing.T) {
	for _, th := range []theme.Theme{{}, slowQuick} {
		sp := widget.Quick.Default()
		if th.Name != "" {
			sp = widget.Quick.In(th)
		}
		h := &menuHost{host: host{values: map[uint32]float64{3: 100}}, mod: map[uint32]plug.Modulation{}}
		k := NewKnob(h, release)
		w := mountThemed(t, k, th)
		frames(w, 30)
		if m := markOf(t, w); m.alpha != 0 || m.ring {
			t.Fatalf("with no control, the knob draws a mark %+v", m)
		}
		on := plug.Modulation{Low: -0.2, High: 0.2, Now: 400}
		h.mod[3] = on
		fades(t, "fading in, "+th.Name, w, sp, 1, func() mark { return wanted(k, on) })
		for range 10 {
			if k.Step(time.Second / 60) {
				t.Fatal("settled, with control holding still, the knob asks for frames")
			}
		}
		delete(h.mod, 3)
		fades(t, "fading out, "+th.Name, w, sp, 0, func() mark { return wanted(k, on) })
		if m := markOf(t, w); m.ring {
			t.Fatal("faded out, the knob still draws the reach's ring")
		}
	}
}

// A modulation that ends and another that begins while the first still fades out glide in strength only: the mark
// stands where the new value stands from its first frame.
func TestAModulationThatEndsAndBeginsWithinTheFadeGlidesInStrength(t *testing.T) {
	h := &menuHost{host: host{values: map[uint32]float64{1: 0.5}},
		mod: map[uint32]plug.Modulation{1: {Low: -0.1, High: 0.1, Now: 0.2}}}
	k := NewKnob(h, unit)
	w := mountPainted(t, k)
	frames(w, 30)
	delete(h.mod, 1)
	frames(w, 4)
	if a := markOf(t, w).alpha; a <= 0.05 || a >= 0.95 {
		t.Fatalf("four frames after control let go, the mark's strength is %v, want part way", a)
	}
	again := plug.Modulation{Low: -0.2, High: 0.3, Now: 0.9}
	h.mod[1] = again
	fades(t, "begun again within the fade", w, widget.Quick.Default(), 1, func() mark { return wanted(k, again) })
}

// shelf holds a knob put on and taken off with Insert and Remove, or, where lazy, built and dropped in layout.
type shelf struct {
	k          *Knob
	lazy, show bool
}

func (s *shelf) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	if s.lazy {
		switch {
		case s.show && kids.Len() == 0:
			kid := kids.Build(s.k)
			kid.Layout(gunim.Tight(geom.Sz(KnobW, KnobH)))
			kid.Place(geom.Point{})
			return c.Max
		case !s.show && kids.Len() > 0 && kids.At(0).Presence() != gunim.Exiting:
			kids.Drop(s.k)
		}
	}
	for k := range kids.All {
		k.Layout(gunim.Tight(geom.Sz(KnobW, KnobH)))
		k.Place(geom.Point{})
	}
	return c.Max
}

func (s *shelf) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for k := range kids.All {
		k.Paint(p)
	}
}

type put struct{ On bool }

// mountShelf shows s, with no knob on it, and returns the window and what puts the knob on and takes it off.
func mountShelf(t *testing.T, s *shelf) (w *gunim.Window, show func(on bool)) {
	t.Helper()
	w = gunimtest.New(t, geom.Sz(KnobW, KnobH), nil)
	w.PaintWhenLooked(false)
	gunim.RegisterView(w, "shelf", func(struct{}) gunim.Node { return s }, nil)
	gunim.RegisterPatch(w, "shelf", func(_ gunim.Node, p put, u *gunim.UI) {
		switch {
		case s.lazy:
			s.show = p.On
			u.Invalidate()
		case p.On:
			u.Insert(s, s.k)
		default:
			u.Remove(s.k)
		}
	})
	if err := w.Client().Mount(gunim.Root, "shelf", "shelf", nil); err != nil {
		t.Fatal(err)
	}
	frames(w, 1)
	return w, func(on bool) {
		t.Helper()
		if err := w.Client().Patch("shelf", put{on}); err != nil {
			t.Fatal(err)
		}
	}
}

// A knob not drawn for longer than its mark's fade shows the mark as control has it on the frame it is drawn again,
// as master does: on its first frame, put back with Insert, and built again in layout after a drop; with control let
// go, taken up, and moved while it was not drawn.
func TestAKnobNotDrawnShowsItsMarkAsItIs(t *testing.T) {
	was := plug.Modulation{Low: -0.1, High: 0.1, Now: 0.2}
	moved := plug.Modulation{Low: -0.3, High: 0.2, Now: 0.9}
	changes := []struct {
		name         string
		before, then *plug.Modulation
	}{
		{"control let go", &was, nil},
		{"control taken up", nil, &moved},
		{"the value moved", &was, &moved},
	}
	set := func(h *menuHost, m *plug.Modulation) {
		if m == nil {
			delete(h.mod, 1)
		} else {
			h.mod[1] = *m
		}
	}
	check := func(what string, w *gunim.Window, k *Knob, m *plug.Modulation) {
		t.Helper()
		got := markOf(t, w)
		if m == nil {
			if got.alpha != 0 || got.ring {
				t.Errorf("%s: the first frame drawn draws a mark %+v, want none", what, got)
			}
			return
		}
		if got.alpha != 1 || !samePlace(got, wanted(k, *m)) {
			t.Errorf("%s: the first frame drawn draws the mark %+v, want it whole at %+v", what, got, wanted(k, *m))
		}
	}
	for _, c := range changes {
		h := &menuHost{host: host{values: map[uint32]float64{1: 0.5}}, mod: map[uint32]plug.Modulation{}}
		set(h, c.then)
		k := NewKnob(h, unit)
		check("the first frame of a knob with "+c.name, mountPainted(t, k), k, c.then)

		for _, how := range []string{"put back with Insert", "built again in layout"} {
			h := &menuHost{host: host{values: map[uint32]float64{1: 0.5}}, mod: map[uint32]plug.Modulation{}}
			set(h, c.before)
			k := NewKnob(h, unit)
			w, show := mountShelf(t, &shelf{k: k, lazy: how == "built again in layout"})
			show(true)
			frames(w, 60)
			show(false)
			frames(w, 30)
			set(h, c.then)
			frames(w, 30)
			show(true)
			frames(w, 1)
			check(how+", "+c.name+" while out", w, k, c.then)
		}
	}
}

// A knob that never has a modulation costs what it did before its mark faded: 10 allocations and 1 808 bytes to
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

type setTo struct{ V float64 }

// needleRuns plays the needle's cases and returns, for each, how far round the needle stood on every frame: a value
// set once, set on every frame, set once per audio block, and the knob dragged and reset with a double click.
func needleRuns(t *testing.T) map[string][]float32 {
	t.Helper()
	const block = 10700 * time.Microsecond
	out := map[string][]float32{}
	run := func(name string, each func(i int, at time.Duration, w *gunim.Window, set func(float64))) {
		h := &host{values: map[uint32]float64{1: 0.2}}
		k := NewKnob(h, unit)
		w := mountPainted(t, k)
		gunim.RegisterPatch(w, "knob", func(_ gunim.Node, s setTo, u *gunim.UI) { k.Follow(s.V, u) })
		set := func(v float64) {
			if err := w.Client().Patch("knob", setTo{v}); err != nil {
				t.Fatal(err)
			}
		}
		frames(w, 10)
		at := time.Duration(0)
		for i, dt := range timings("120", 700*time.Millisecond) {
			at += dt
			each(i, at, w, set)
			w.Frame(dt)
			out[name] = append(out[name], k.turned.Value())
		}
	}
	run("set once", func(i int, _ time.Duration, _ *gunim.Window, set func(float64)) {
		if i == 3 {
			set(0.9)
		}
	})
	run("set every frame", func(_ int, at time.Duration, _ *gunim.Window, set func(float64)) {
		set(0.5 + 0.4*math.Sin(2*math.Pi*5*at.Seconds()))
	})
	run("set once per block", func(_ int, at time.Duration, _ *gunim.Window, set func(float64)) {
		set(0.5 + 0.4*math.Sin(2*math.Pi*5*(at/block*block).Seconds()))
	})
	run("dragged and reset", func(i int, _ time.Duration, w *gunim.Window, _ func(float64)) {
		pt := func(y float32) geom.Point { return geom.Pt(KnobW/2, y) }
		switch {
		case i == 2:
			w.Input(input.PointerMove{Pos: pt(40), Time: time.Now()})
			w.Input(input.PointerDown{Pos: pt(40), Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
		case i > 2 && i < 30:
			w.Input(input.PointerMove{Pos: pt(40 - float32(i-2)*3), Time: time.Now()})
		case i == 30:
			w.Input(input.PointerUp{Pos: pt(-44), Button: input.ButtonPrimary, Time: time.Now()})
		case i == 50:
			w.Input(input.PointerDown{Pos: pt(40), Button: input.ButtonPrimary, Clicks: 2, Time: time.Now()})
			w.Input(input.PointerUp{Pos: pt(40), Button: input.ButtonPrimary, Time: time.Now()})
		}
	})
	return out
}

// The needle moves as it did before the modulation's mark faded, frame for frame: its every angle, in each case,
// equals the one recorded from master in testdata. GUNIM_WRITE_NEEDLE=1 records them anew.
func TestAKnobsNeedleMovesAsItDid(t *testing.T) {
	path := filepath.Join("testdata", "needle.json")
	got := needleRuns(t)
	if os.Getenv("GUNIM_WRITE_NEEDLE") == "1" {
		b, err := json.MarshalIndent(got, "", "\t")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want map[string][]float32
	if err := json.Unmarshal(b, &want); err != nil {
		t.Fatal(err)
	}
	for name, ws := range want {
		gs := got[name]
		if len(gs) != len(ws) {
			t.Errorf("%s: the needle stood on %d frames, want %d", name, len(gs), len(ws))
			continue
		}
		for i := range ws {
			if gs[i] != ws[i] {
				t.Errorf("%s: on frame %d the needle stands at %v, want %v as master had it", name, i, gs[i], ws[i])
				break
			}
		}
	}
}
