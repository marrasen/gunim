package plugui

import (
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio/plug"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// tone is a [plug.Sound] of a steady level: while it plays, it plays a
// sixtieth of a second more each time it is asked how far it has played.
type tone struct {
	at      int64
	stopped bool
}

// Heard implements [plug.Sound].
func (s *tone) Heard(dst []float32) (frames int, at int64, rate int) {
	const hz = 48000
	if len(dst) == 0 {
		if !s.stopped {
			s.at += hz / 60
		}
		return 0, s.at, hz
	}
	n := min(len(dst)/2, int(s.at))
	for i := range 2 * n {
		dst[i] = 0.5
	}
	return n, s.at, hz
}

// soundHost is a host whose sound plays, and whose processor keeps how
// far down says each ID turns each frame down.
type soundHost struct {
	host
	out  *tone
	down func(id uint32, f int64) float32
}

func (h *soundHost) Sound() (in, out plug.Sound) { return h.out, h.out }

func (h *soundHost) Traced(id uint32, f int64) (float32, bool) {
	if h.down == nil {
		return 0, false
	}
	return h.down(id, f), true
}

// steady returns traces that turn every frame down by db, whatever the ID.
func steady(db float32) func(uint32, int64) float32 {
	return func(uint32, int64) float32 { return db }
}

func newSoundHost(down func(uint32, int64) float32) *soundHost {
	return &soundHost{host: host{values: map[uint32]float64{3: 10, 4: 1}}, out: &tone{}, down: down}
}

var barColor = theme.Color("test.bar", color.NRGBA{R: 0xff, A: 0xff})

// stageSection shows s alone in a window width wide.
func stageSection(t *testing.T, s *Section, width float32) *gunim.Window {
	t.Helper()
	w := gunimtest.New(t, geom.Sz(width, SectionH), nil)
	gunim.RegisterView(w, "n", func(struct{}) gunim.Node { return s }, nil)
	if err := w.Client().Mount(gunim.Root, "n", "n", nil); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	return w
}

// boundsOf returns where n is in w.
func boundsOf(t *testing.T, w *gunim.Window, n gunim.Node) geom.Rect {
	t.Helper()
	var b geom.Rect
	gunim.RegisterPatch(w, "n", func(_ *Section, _ struct{}, u *gunim.UI) { b, _ = u.Bounds(n) })
	if err := w.Client().Patch("n", struct{}{}); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	return b
}

var meterDesc = plug.Descriptor{Params: []plug.Param{release, {ID: 4, Name: "Gate", Max: 1, Default: 1}}}

// Read sums its IDs frame by frame over the frames heard since it last read, and takes the most they were turned
// down; once the sound stops it holds that a moment, and then reads nothing.
func TestReducedReadsTheMostItsIDsTurnTheFramesHeardDown(t *testing.T) {
	// The tone plays 800 frames a reading. ID 7 is turned down 20 dB in the first 800, 6 dB about frame 1050, and
	// 1 dB elsewhere; ID 8 half a decibel throughout.
	h := newSoundHost(func(id uint32, f int64) float32 {
		switch {
		case id == 8:
			return -0.5
		case f < 800:
			return -20
		case f >= 1000 && f < 1100:
			return -6
		}
		return -1
	})
	r := NewReduced(h, 7, 8)
	const dt = time.Second / 60
	for i, want := range []float32{-20.5, -6.5, -1.5} {
		if db, live := r.Read(dt); db != want || !live {
			t.Fatalf("reading %d: %v dB, playing %v; want %v, playing", i, db, live, want)
		}
	}
	h.out.stopped = true
	if db, live := r.Read(dt); db != -1.5 || !live {
		t.Errorf("a frame after the sound stopped: %v dB, playing %v; want the last held, -1.5", db, live)
	}
	for range 20 {
		r.Read(dt)
	}
	if db, live := r.Read(dt); db != 0 || live {
		t.Errorf("with the sound stopped: %v dB, playing %v; want nothing", db, live)
	}
}

// The bar shows its first reading at once, glides to each after it a frame at a time, and glides back to nothing
// once the sound stops, and then asks for no frames.
func TestASectionsBarFollowsHowFarItsPartTurnsTheSoundDown(t *testing.T) {
	h := newSoundHost(steady(-1.5))
	s := NewSection(h, &meterDesc, "GATE", 4, 3).Meter(h, barColor, 40, 7, 8)
	w := stageSection(t, s, s.Width())
	from := -ChipScale(-3, 40)
	if got := s.bar.filled(); got != from {
		t.Fatalf("at its first reading the bar is %v full, want %v at once, for 3 dB of 40", got, from)
	}
	// Each ID turns the sound down 5 dB now: a quarter of the bar's 40, so half of it by the square root.
	h.down = steady(-5)
	const to = 0.5
	w.Frame(time.Second / 60)
	if got := s.bar.filled(); got <= from || got >= to {
		t.Fatalf("a frame after the reduction grew, the bar is %v full, want part way from %v to %v", got, from, to)
	}
	for i := range 90 {
		w.Frame(time.Second / 60)
		if got := s.bar.filled(); got < from || got > to+0.05 {
			t.Fatalf("frame %d: the bar is %v full, want it between %v and %v", i, got, from, to)
		}
	}
	if got := s.bar.filled(); math.Abs(float64(got-to)) > 1e-3 {
		t.Fatalf("settled, the bar is %v full, want %v", got, to)
	}
	if !s.bar.Step(time.Second / 60) {
		t.Error("settled, with the sound playing, the bar asks for no frames, so it would miss the next reading")
	}
	// The sound stops: the bar glides away, and rests.
	h.out.stopped = true
	glided := false
	for range 120 {
		w.Frame(time.Second / 60)
		if got := s.bar.filled(); got > 0.1 && got < to-0.1 {
			glided = true
		}
	}
	if !glided {
		t.Error("with the sound stopped, the bar jumped to nothing")
	}
	if got := s.bar.filled(); got != 0 {
		t.Errorf("with the sound stopped, the bar is %v full", got)
	}
	if s.bar.Step(time.Second / 60) {
		t.Error("with the sound stopped, the bar still asks for frames")
	}
}

// The bar draws its track across its box, and over it, from the right, as much as it is full, in its colour.
func TestASectionsBarFillsFromTheRightInItsColour(t *testing.T) {
	b := newSectionBar(NewReduced(newSoundHost(nil)), barColor, 40)
	box := geom.Sz(sectionBarMax, sectionBarH)
	rects := func() []*paint.RRectOp {
		var p paint.Painter
		b.Paint(&p, gunim.Frame{Scale: 1}, box, gunim.Children{})
		out := make([]*paint.RRectOp, 0, len(p.Ops()))
		for _, op := range p.Ops() {
			r, ok := op.(*paint.RRectOp)
			if !ok {
				t.Fatalf("the bar draws a %T", op)
			}
			out = append(out, r)
		}
		return out
	}
	whole := geom.Rect{Max: box.Point()}
	if got := rects(); len(got) != 1 || got[0].Rect != whole {
		t.Fatalf("empty, the bar draws %d parts, want its track alone, across its box", len(got))
	}
	b.shown.Jump(-10)
	got := rects()
	if len(got) != 2 {
		t.Fatalf("half full, the bar draws %d parts, want its track and its fill", len(got))
	}
	if want := geom.Rc(sectionBarMax/2, 0, sectionBarMax/2, sectionBarH); got[1].Rect != want {
		t.Errorf("half full, the fill is at %v, want the right half, %v", got[1].Rect, want)
	}
	if want := paint.Solid(barColor.Get(gunim.Frame{}.Theme)); got[1].Fill != want {
		t.Errorf("the fill is drawn in %v, want the bar's colour, %v", got[1].Fill, want)
	}
	// Turned down more than fills it, it stays in its box.
	b.shown.Jump(-90)
	if got := rects(); len(got) != 2 || got[1].Rect != whole {
		t.Errorf("past full, the bar draws %d parts, want its fill across its box", len(got))
	}
	// With no room, it draws nothing.
	box = geom.Sz(0, sectionBarH)
	if got := rects(); len(got) != 0 {
		t.Errorf("with no room the bar draws %d parts", len(got))
	}
}

// The bar stands at the right of the section's head, after the name: as wide as the room there, up to its most, and
// gone where a section is laid out with no room for it.
func TestASectionsBarStandsAtTheRightOfItsHead(t *testing.T) {
	h := newSoundHost(nil)
	for _, c := range []struct {
		name  string
		knobs []uint32
		// squeeze lays the section out that much narrower than its width.
		squeeze float32
	}{
		{name: "GATE", knobs: []uint32{3}},
		{name: "GATE", knobs: []uint32{3, 3, 3, 3}},
		{name: "A LONG NAME FOR A SECTION", knobs: []uint32{3}},
		{name: "GATE", knobs: []uint32{3}, squeeze: sectionBarMin + SectionPad},
	} {
		bare := NewSection(h, &meterDesc, c.name, 4, c.knobs...)
		s := NewSection(h, &meterDesc, c.name, 4, c.knobs...).Meter(h, barColor, 12, 7)
		if s.Width() < bare.Width() || s.Width() < s.nameEnd()+sectionBarMin+SectionPad {
			t.Errorf("%s: with a bar the section is %v wide; without, %v, and its head needs %v", c.name, s.Width(),
				bare.Width(), s.nameEnd()+sectionBarMin+SectionPad)
		}
		width := s.Width() - c.squeeze
		w := stageSection(t, s, width)
		b := boundsOf(t, w, s.bar)
		want := max(min(width-SectionPad-s.nameEnd(), sectionBarMax), 0)
		if got := b.Size().W; got != want {
			t.Errorf("%s, %d knobs, %v wide: the bar is %v wide, want %v", c.name, len(c.knobs), width, got, want)
		}
		if b.Max.X != width-SectionPad || (want > 0 && b.Min.X < s.nameEnd()) || b.Min.Y < 0 || b.Max.Y > SectionHead {
			t.Errorf("%s, %d knobs, %v wide: the bar stands at %v, want it in the head, ending %v across, after the name's end at %v",
				c.name, len(c.knobs), width, b, width-SectionPad, s.nameEnd())
		}
	}
}
