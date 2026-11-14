package calibrate

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/marrasen/gunim/audio"
)

// listener taps along with a calibration on a clock of its own. The speaker reports latency, and the sound reaches
// the listener delay after that; the listener taps habit after each beat, give or take jitter.
type listener struct {
	t       *testing.T
	c       *Calibration
	mix     *audio.Mixer
	t0, now time.Time
	latency time.Duration
	delay   time.Duration
	habit   time.Duration
	jitter  time.Duration
	rng     *rand.Rand
}

func newListener(t *testing.T, o Options) *listener {
	t.Helper()
	l := &listener{t: t, mix: audio.NewMixer(), t0: time.Unix(1000, 0), latency: 40 * time.Millisecond,
		rng: rand.New(rand.NewPCG(1, 2))}
	l.now = l.t0
	// The frame heard at t, as the speaker reports it: the mixer works latency ahead.
	o.HeardAt = func(t time.Time) int64 {
		return max(0, audio.FramesAt(t.Sub(l.t0)-l.latency, l.mix.Rate()))
	}
	l.c = New(l.mix, nil, o)
	l.c.Start(l.now)
	// The mixer reads the clicks once, which starts them at frame 0.
	l.mix.Mix(make([]float32, 2*256))
	return l
}

// beatAt is when beat k of the pass is heard, as the speaker reports it.
func (l *listener) beatAt(k int64) time.Time {
	c := l.c
	if !c.ready(l.now) {
		l.t.Fatal("the calibration never found its first beat")
	}
	f := c.src.start.Load() + (c.from+k)*c.per
	return l.t0.Add(audio.DurationAt(f, l.mix.Rate()) + l.latency)
}

// pass taps along through the stage running, beat by beat: tap says how far from its beat the listener taps on
// beat k, past the habit and the jitter, and whether they tap at all. It returns once the stage changes.
func (l *listener) pass(tap func(k int64) (time.Duration, bool)) {
	l.t.Helper()
	s := l.c.Stage()
	for k := int64(0); l.c.Stage() == s; k++ {
		if k > 200 {
			l.t.Fatalf("the %v pass never ended", s)
		}
		at := l.beatAt(k).Add(l.habit)
		if s == Listen {
			at = at.Add(l.delay)
		}
		if off, ok := tap(k); ok {
			l.now = at.Add(off + time.Duration(l.rng.NormFloat64()*float64(l.jitter)))
			l.c.Tap(l.now)
		}
		l.now = at.Add(l.c.o.period() / 2)
		l.c.Update(l.now)
	}
}

func everyBeat(int64) (time.Duration, bool) { return 0, true }

func near(a, b, by time.Duration) bool { return (a - b).Abs() <= by }

// Steady taps find the delay and the habit exactly, and a tight Within.
func TestSteadyTapsFindTheDelay(t *testing.T) {
	l := newListener(t, Options{})
	l.delay, l.habit = 230*time.Millisecond, -35*time.Millisecond
	if l.c.Stage() != Watch {
		t.Fatalf("a calibration starts at %v, want watch", l.c.Stage())
	}
	l.pass(everyBeat)
	if l.c.Stage() != Listen {
		t.Fatalf("after the watch pass the calibration is at %v, want listen", l.c.Stage())
	}
	l.pass(everyBeat)
	r := l.c.Result()
	if l.c.Stage() != Done || !r.Sure() {
		t.Fatalf("the calibration ended at %v with %+v, want done and sure", l.c.Stage(), r)
	}
	if !near(r.Delay, 230*time.Millisecond, time.Millisecond) || !near(r.Habit, -35*time.Millisecond, time.Millisecond) {
		t.Fatalf("Delay %v and Habit %v, want 230ms and -35ms", r.Delay, r.Habit)
	}
	if len(r.Watch.Offsets) != 16 || len(r.Listen.Offsets) != 16 || r.Within > 2*time.Millisecond {
		t.Fatalf("kept %d and %d taps, within %v; want 16 each, within 2ms", len(r.Watch.Offsets),
			len(r.Listen.Offsets), r.Within)
	}
}

// Taps that wander, miss beats, land twice on one or go wild still find the delay to within a few milliseconds,
// and the wild ones are put aside.
func TestNoisyAndMissedTapsStillFindTheDelay(t *testing.T) {
	l := newListener(t, Options{})
	l.delay, l.habit, l.jitter = 180*time.Millisecond, 20*time.Millisecond, 15*time.Millisecond
	messy := func(k int64) (time.Duration, bool) {
		switch {
		case k%5 == 4:
			return 0, false // missed
		case k == 8:
			return 250 * time.Millisecond, true // wild
		case k == 11:
			return -200 * time.Millisecond, true // wild
		}
		return 0, true
	}
	l.pass(messy)
	l.pass(messy)
	r := l.c.Result()
	t.Logf("delay %v within %v, dropped %d and %d", r.Delay, r.Within, r.Watch.Dropped, r.Listen.Dropped)
	if !r.Enough || r.Watch.Dropped < 2 || r.Listen.Dropped < 2 {
		t.Fatalf("result %+v: want enough taps, and the wild ones dropped", r)
	}
	if !near(r.Delay, 180*time.Millisecond, 15*time.Millisecond) {
		t.Fatalf("Delay %v, want 180ms within 15", r.Delay)
	}
	if r.Within < 5*time.Millisecond || r.Within > 25*time.Millisecond {
		t.Fatalf("Within %v for taps 15ms apart, want between 5ms and 25ms", r.Within)
	}
	if !near(r.Delay, 180*time.Millisecond, r.Within) {
		t.Fatalf("Delay %v lies further than its Within %v from 180ms", r.Delay, r.Within)
	}
}

// A second tap on one beat counts once: the one nearer the rest.
func TestTwoTapsOnOneBeatCountOnce(t *testing.T) {
	taps := []tapAt{{0, 0.010}, {1, 0.012}, {2, 0.008}, {2, 0.200}, {3, 0.011}, {4, 0.009}}
	p := summarize(taps)
	if len(p.Offsets) != 5 || p.Dropped != 1 || !near(p.Median, 10*time.Millisecond, 0) {
		t.Fatalf("summarize = %+v, want 5 kept, 1 dropped, median 10ms", p)
	}
}

// Taps before the pass count for nothing, and those on its first beats are practice.
func TestPracticeAndEarlyTapsCountForNothing(t *testing.T) {
	l := newListener(t, Options{})
	if got, _ := l.c.Tap(l.beatAt(-2)); got != Early {
		t.Fatalf("a tap before the pass counted as %v, want early", got)
	}
	for k := range int64(3) {
		if got, _ := l.c.Tap(l.beatAt(k)); got != Practice {
			t.Fatalf("a tap on beat %d counted as %v, want practice", k, got)
		}
	}
	if got, off := l.c.Tap(l.beatAt(3).Add(12 * time.Millisecond)); got != Counted || !near(off, 12*time.Millisecond, 0) {
		t.Fatalf("a tap on beat 3, 12ms late, counted as %v %v", got, off)
	}
	if n, of, _ := l.c.Progress(l.now); n != 1 || of != 16 {
		t.Fatalf("Progress = %d of %d, want 1 of 16", n, of)
	}
}

// A listener who stops tapping lets the beats run out: each pass ends with what it has, and the result says it
// had too few.
func TestAPassEndsAsItsBeatsRunOut(t *testing.T) {
	l := newListener(t, Options{})
	l.pass(func(k int64) (time.Duration, bool) { return 0, k < 7 })
	l.pass(func(k int64) (time.Duration, bool) { return 0, k < 20 })
	r := l.c.Result()
	if l.c.Stage() != Done || r.Enough || r.Sure() || len(r.Watch.Offsets) != 4 {
		t.Fatalf("at %v, result %+v: want done, with too few taps", l.c.Stage(), r)
	}
}

// A habit known from before skips the watch pass.
func TestAKnownHabitSkipsTheWatch(t *testing.T) {
	l := newListener(t, Options{SkipWatch: true, Habit: -40 * time.Millisecond, Taps: 10})
	l.delay, l.habit = 300*time.Millisecond, -40*time.Millisecond
	if l.c.Stage() != Listen {
		t.Fatalf("with the watch skipped, the calibration starts at %v", l.c.Stage())
	}
	l.pass(everyBeat)
	if r := l.c.Result(); !near(r.Delay, 300*time.Millisecond, time.Millisecond) || len(r.Listen.Offsets) != 10 {
		t.Fatalf("result %+v, want 300ms from 10 taps", r)
	}
}

// A delay set on the speaker before, here 500 ms too much, is left out of the measure: the calibration finds the
// whole delay the output leaves out. Once done, setting the delay found leaves the beat shown where it was.
func TestADelaySetBeforeIsLeftOutOfTheMeasure(t *testing.T) {
	l := newListener(t, Options{Taps: 8})
	output := l.c.heardAt
	set := 730 * time.Millisecond
	// The speaker's Heard counts the delay set: the frame the output reports, less it.
	heard := func(at time.Time) int64 { return output(at) - audio.FramesAt(set, l.mix.Rate()) }
	l.c.heardAt = atOutput(heard, func() time.Duration { return set }, l.mix.Rate)
	l.delay = 230 * time.Millisecond
	l.pass(everyBeat)
	l.pass(everyBeat)
	r := l.c.Result()
	if !near(r.Delay, 230*time.Millisecond, time.Millisecond) || !r.Sure() {
		t.Fatalf("with 730ms set before, the calibration found %v, sure %v: want 230ms, sure", r.Delay, r.Sure())
	}
	at := l.beatAt(30).Add(230 * time.Millisecond)
	k, phase, _ := l.c.Beat(at)
	if k != 30 || phase > 0.001 {
		t.Fatalf("once done, the beat 230ms past where the output plays beat 30 is beat %d at %.3f, want 30 at 0", k, phase)
	}
	set = r.Delay
	if k2, phase2, _ := l.c.Beat(at); k2 != k || math.Abs(phase2-phase) > 0.001 {
		t.Fatalf("setting the delay found moved the beat shown from %d at %.3f to %d at %.3f", k, phase, k2, phase2)
	}
}

// A delay near the end of what the taps' window reaches is marked unsure; one well inside it is sure.
func TestAMedianNearTheWindowsEdgeIsUnsure(t *testing.T) {
	for _, c := range []struct {
		delay time.Duration
		sure  bool
	}{{-230 * time.Millisecond, false}, {-150 * time.Millisecond, true}, {450 * time.Millisecond, true},
		{520 * time.Millisecond, false}} {
		l := newListener(t, Options{})
		l.delay = c.delay
		l.pass(everyBeat)
		l.pass(everyBeat)
		if r := l.c.Result(); r.Sure() != c.sure || r.Edge == c.sure {
			t.Errorf("a delay of %v measured %v, edge %v, sure %v: want sure %v", c.delay, r.Delay, r.Edge, r.Sure(), c.sure)
		}
	}
}

// The clicks are silent in the watch pass, and sound on the beat from the listen pass's first beat on, with their
// first frame at once loud.
func TestTheClicksSoundFromTheListenPass(t *testing.T) {
	l := newListener(t, Options{Taps: 4, Practice: 1})
	loud := func(n int) (first int64) {
		buf := make([]float32, 2*n)
		at := l.mix.Mixed()
		l.mix.Mix(buf)
		for i := range n {
			if math.Abs(float64(buf[2*i])) > 0.05 {
				return at + int64(i)
			}
		}
		return -1
	}
	per := int(l.c.per)
	if f := loud(3 * per); f >= 0 {
		t.Fatalf("a click sounded at frame %d in the watch pass", f)
	}
	l.pass(everyBeat)
	if l.c.Stage() != Listen {
		t.Fatal("the watch pass never ended")
	}
	first := l.c.src.start.Load() + l.c.from*l.c.per
	f := loud(int(first-l.mix.Mixed()) + per)
	if f < first || f > first+2 {
		t.Fatalf("the first click sounds at frame %d, want the pass's first beat at %d", f, first)
	}
}

// An output that reports more delay than it holds, by more than the taps' window reaches, has every tap match the
// beat before: the result lands near the window's edge, and must say it is unsure.
func TestADelayABeatOffIsUnsure(t *testing.T) {
	l := newListener(t, Options{})
	l.delay = -260 * time.Millisecond
	l.pass(everyBeat)
	l.pass(everyBeat)
	if r := l.c.Result(); r.Sure() {
		t.Fatalf("a delay of -260ms measured %v within %v, and says it is sure", r.Delay, r.Within)
	}
}
