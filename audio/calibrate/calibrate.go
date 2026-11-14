// Package calibrate measures how much later than the speaker reports a
// sound reaches the listener's ears. The listener taps along with a
// beat, and each tap is held against the beat as the speaker says it
// is heard. An application sets what it finds with
// [speaker.Speaker.SetDelay], and from then on everything that counts
// the frames heard agrees with the listener's ears.
//
// Most outputs need none of it: the speaker counts their delay. It is
// for the ones it can't count, as a Bluetooth headset on Windows, which
// reports a few hundred milliseconds less than it holds. An application
// may offer it anywhere, and stores the result itself, keyed by what
// the speaker's [speaker.State] says plays: its Driver and Route, and
// Info.Driver where Info is set.
//
// # Two passes
//
// People tap a little ahead of a beat they follow, or behind it, each
// by a habit of their own of up to 50 ms or so. One pass of taps along
// with a click holds that habit along with the delay. So a calibration
// runs two passes of the same beat, one after the other:
//
//   - Watch: a beat shown, in silence. A ball falls and bounces, and the
//     listener taps as it lands. The taps hold the habit alone.
//   - Listen: the same beat heard as clicks, with nothing shown. The
//     taps hold the habit and the delay the speaker leaves out.
//
// The difference of the two passes' medians is the delay. A bouncing
// ball, unlike a flash, is a beat people follow about as closely as a
// sound, so the habit carries over from one pass to the other. The time
// a tap takes to arrive is in both passes too, and drops out. What
// stays is the screen's own delay past the frame's present time, which
// gunim can't see, as a television's picture processing; it moves the
// result earlier by as much, which keeps picture and sound together for
// the listener.
//
// Each pass starts with a few practice beats, whose taps count for
// nothing, then counts a tap on each beat until it has [Options].Taps
// of them. A tap far from the rest is put aside, and of two taps on one
// beat, the one nearer the rest counts. At the default 800 ms beat, a
// calibration takes about 35 seconds.
//
// A listener whose habit is known from before, from a pass on an output
// that needs no calibration, skips the watch with [Options].SkipWatch
// and [Options].Habit.
//
// Package audioui's Calibrator draws a calibration and takes its taps.
package calibrate

import (
	"math"
	"slices"
	"sync/atomic"
	"time"

	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/speaker"
)

// Stage is where a calibration has got to.
type Stage int

const (
	// Ready is before [Calibration.Start].
	Ready Stage = iota
	// Watch is the pass of taps along with a beat shown, in silence.
	Watch
	// Listen is the pass of taps along with clicks heard.
	Listen
	// Done is after the passes: [Calibration.Result] holds what they
	// found, and the clicks go on, for a check by eye.
	Done
)

// String names the stage: "ready", "watch", "listen" or "done".
func (s Stage) String() string {
	switch s {
	case Watch:
		return "watch"
	case Listen:
		return "listen"
	case Done:
		return "done"
	case Ready:
	}
	return "ready"
}

// Options say how a calibration runs. The zero Options run the default.
type Options struct {
	// Period is the time from one beat to the next. Zero means 800 ms.
	// A tap may land up to 30% of it before the beat it is for, and 70%
	// after, so a delay of up to half a second is found at 800 ms.
	Period time.Duration
	// Practice is how many beats start each pass with taps that count
	// for nothing. Zero means 3.
	Practice int
	// Taps is how many taps each pass counts. Zero means 16.
	Taps int
	// SkipWatch leaves out the watch pass, and takes Habit as the
	// listener's habit: the median of their taps on a beat they hear in
	// time, as [Result].Habit gave on another calibration, or one tapped
	// along with an output known to need none.
	SkipWatch bool
	Habit     time.Duration
	// HeardAt, where set, is the mixer frame heard at t, as the
	// speaker reports it, for an application that keeps a clock of its
	// own. Where it is nil, the speaker's [speaker.Speaker.Anchor] gives
	// it, or, without a speaker, the mixer's [audio.Mixer.HeardAt].
	HeardAt func(t time.Time) int64
}

func (o Options) period() time.Duration {
	if o.Period <= 0 {
		return 800 * time.Millisecond
	}
	return o.Period
}

func (o Options) practice() int {
	if o.Practice <= 0 {
		return 3
	}
	return o.Practice
}

func (o Options) taps() int {
	if o.Taps <= 0 {
		return 16
	}
	return o.Taps
}

// A Pass is what one pass of taps found.
type Pass struct {
	// Offsets are the taps counted, in the order they came: how long
	// after its beat each landed, earlier where negative.
	Offsets []time.Duration
	// Dropped counts the taps put aside: far from the rest, or a second
	// tap on a beat.
	Dropped int
	// Median is the median of Offsets, and Spread how widely they lie
	// about it: a standard deviation, taken from the median distance
	// from the median, so a wild tap moves it little.
	Median, Spread time.Duration
}

// within is the standard error of p's median.
func (p Pass) within() float64 {
	if len(p.Offsets) == 0 {
		return math.Inf(1)
	}
	return 1.2533 * p.Spread.Seconds() / math.Sqrt(float64(len(p.Offsets)))
}

// A Result is what a calibration found.
type Result struct {
	// Delay is the delay to set with [speaker.Speaker.SetDelay]: the one
	// the speaker counted as the calibration ran, and what it measured
	// past it.
	Delay time.Duration
	// Measured is what the calibration measured past the delay the
	// speaker counted: the listen pass's median less the habit.
	Measured time.Duration
	// Within says how sure Delay is: about 19 times in 20, the true
	// delay lies within this either side of it. It counts how many taps
	// each pass kept and how closely they lay.
	Within time.Duration
	// Habit is the listener's own habit: how far after a beat they tap,
	// earlier where negative, from the watch pass, or Options.Habit
	// where it was skipped.
	Habit time.Duration
	// Watch and Listen are the two passes; Watch is empty where it was
	// skipped.
	Watch, Listen Pass
	// Enough says each pass counted at least half the taps it asked
	// for; the beats ran out on a pass with fewer.
	Enough bool
}

// Sure is the most Within may be for [Result.Sure].
const Sure = 20 * time.Millisecond

// Sure reports whether the result is worth keeping: each pass counted
// enough taps, and Delay is within [Sure] either side.
func (r Result) Sure() bool { return r.Enough && r.Within <= Sure }

// A Tap says how a tap counted.
type Tap int

const (
	// Early is a tap before the pass's first beat, or with no pass
	// running: it counts for nothing.
	Early Tap = iota
	// Practice is a tap on one of a pass's first beats.
	Practice
	// Counted is a tap the pass counted.
	Counted
)

// A Calibration runs one calibration of a speaker. Its methods run on
// one goroutine, as the UI's; the clicks it plays read only what they
// share with it through atomics.
type Calibration struct {
	o   Options
	mix *audio.Mixer
	spk *speaker.Speaker
	src *clicks
	v   *audio.Voice
	// rate is the mixer's rate as the clicks started, and per the
	// frames from one beat to the next at it.
	rate int
	per  int64
	// base is the delay the speaker counted as the calibration started.
	base  time.Duration
	stage Stage
	// from is the pass's first beat, which counts from the clicks'
	// start.
	from int64
	// taps are the pass's counted taps; dropped counts the taps the
	// pass put aside as they came, on two beats in one.
	taps  []tapAt
	watch Pass
	res   Result
	// heardAt is the mixer frame heard at t.
	heardAt func(t time.Time) int64
}

// tapAt is a tap counted: on beat k, off seconds after it.
type tapAt struct {
	k   int64
	off float64
}

// New returns a calibration of the sound mix plays. spk, where set,
// gives the delay it counts already and an even clock of the frames
// heard; without it the calibration counts by the mixer's
// [audio.Mixer.HeardAt].
func New(mix *audio.Mixer, spk *speaker.Speaker, o Options) *Calibration {
	c := &Calibration{o: o, mix: mix, spk: spk, heardAt: o.HeardAt}
	if c.heardAt != nil {
		return c
	}
	c.heardAt = func(t time.Time) int64 {
		if c.spk != nil {
			if a := c.spk.Anchor(); a.Period > 0 {
				return a.FrameAt(t)
			}
		}
		return c.mix.HeardAt(t)
	}
	return c
}

// Options returns the options the calibration runs with.
func (c *Calibration) Options() Options { return c.o }

// Start starts the calibration from its first pass, or again from any
// stage, and plays the beat. The pass's first beat falls a beat or two
// after t. Start it again as the speaker opens again, on
// [speaker.Speaker.Changed].
func (c *Calibration) Start(t time.Time) {
	if rate := c.mix.Rate(); c.v == nil || rate != c.rate {
		c.Stop()
		c.rate = rate
		c.per = audio.FramesAt(c.o.period(), rate)
		c.src = &clicks{rate: rate, per: c.per, mix: c.mix}
		c.src.start.Store(-1)
		c.src.from.Store(math.MaxInt64)
		c.v = c.mix.Play(c.src, audio.Options{})
	}
	c.base = 0
	if c.spk != nil {
		c.base = c.spk.Delay()
	}
	c.watch, c.res = Pass{}, Result{}
	if c.o.SkipWatch {
		c.begin(Listen, t)
	} else {
		c.begin(Watch, t)
	}
}

// Stop stops the clicks. The result stays.
func (c *Calibration) Stop() {
	if c.v != nil {
		c.v.Stop(0)
		c.v = nil
	}
	if c.stage != Done {
		c.stage = Ready
	}
}

// begin starts stage s, whose first beat is one clear beat from the
// next the mixer mixes: the listener hears it at least a beat away.
func (c *Calibration) begin(s Stage, t time.Time) {
	c.stage, c.taps, c.from = s, c.taps[:0], -1
	c.src.from.Store(math.MaxInt64)
	c.ready(t)
}

// ready finds the stage's first beat once the clicks have started, and
// has the clicks sound from it in the listen pass and after.
func (c *Calibration) ready(t time.Time) bool {
	if c.from >= 0 {
		return true
	}
	start := c.src.start.Load()
	if start < 0 || c.stage == Ready {
		return false
	}
	ahead := max(c.mix.Mixed(), c.heardAt(t)) - start
	c.from = ahead/c.per + 2
	if c.stage != Watch {
		c.src.from.Store(c.from)
	}
	return true
}

// Running reports whether the beat plays: from Start to Stop.
func (c *Calibration) Running() bool { return c.v != nil }

// Stage returns where the calibration has got to.
func (c *Calibration) Stage() Stage { return c.stage }

// Beat returns the beat at t as the listener meets it, counted from
// the pass's first beat, and how far through it t is, from 0 on the
// beat to 1 at the next. In the watch pass it is the beat shown; once
// done, the beat heard, moved by the delay measured. Before the clicks
// start, ok is false.
func (c *Calibration) Beat(t time.Time) (k int64, phase float64, ok bool) {
	if c.stage == Ready || !c.ready(t) {
		return 0, 0, false
	}
	x := c.beats(t)
	if c.stage == Done {
		x -= c.res.Measured.Seconds() / c.o.period().Seconds()
	}
	k = int64(math.Floor(x))
	return k, x - float64(k), true
}

// beats is the beat heard at t, counted from the pass's first beat,
// with its fraction.
func (c *Calibration) beats(t time.Time) float64 {
	return float64(c.heardAt(t)-c.src.start.Load()-c.from*c.per) / float64(c.per)
}

// Progress returns the taps the pass counted, and how many it counts
// in all; and the practice beats left before it counts, from t on.
func (c *Calibration) Progress(t time.Time) (counted, of, practice int) {
	of = c.o.taps()
	switch c.stage {
	case Done:
		return of, of, 0
	case Ready:
		return 0, of, c.o.practice()
	case Watch, Listen:
	}
	practice = c.o.practice()
	if k, _, ok := c.Beat(t); ok {
		practice = max(0, min(practice, practice-int(k)))
	}
	return len(c.taps), of, practice
}

// Taps returns the offsets of the taps the pass counted so far.
func (c *Calibration) Taps() []time.Duration {
	out := make([]time.Duration, len(c.taps))
	for i, t := range c.taps {
		out[i] = secs(t.off)
	}
	return out
}

// Tap takes a tap at t, which the input event's time gives, and says
// how it counted, and how long after its beat it landed. It may end
// the pass, and start the next.
func (c *Calibration) Tap(t time.Time) (Tap, time.Duration) {
	if (c.stage != Watch && c.stage != Listen) || !c.ready(t) {
		return Early, 0
	}
	// A tap may land 30% of a beat before its beat, round the habit in
	// the listen pass, and 70% after; in the watch pass, half either way.
	lead, centre := 0.5, 0.0
	if c.stage == Listen {
		lead, centre = 0.3, c.habit().Seconds()/c.o.period().Seconds()
	}
	x := c.beats(t)
	k := int64(math.Floor(x - centre + lead))
	off := (x - float64(k)) * c.o.period().Seconds()
	switch {
	case k < 0:
		return Early, secs(off)
	case k < int64(c.o.practice()):
		return Practice, secs(off)
	}
	c.taps = append(c.taps, tapAt{k: k, off: off})
	if beatsTapped(c.taps) >= c.o.taps() {
		c.next(t)
	}
	return Counted, secs(off)
}

// Update moves the calibration on at t: a pass whose beats ran out,
// with taps missed, ends with what it has. It reports whether the stage
// changed. Call it once a frame.
func (c *Calibration) Update(t time.Time) bool {
	if c.stage != Watch && c.stage != Listen {
		return false
	}
	k, _, ok := c.Beat(t)
	if !ok || k < int64(c.o.practice()+2*c.o.taps()) {
		return false
	}
	c.next(t)
	return true
}

// habit is the listener's habit: the watch pass's median, or the
// options' where it was skipped.
func (c *Calibration) habit() time.Duration {
	if c.o.SkipWatch {
		return c.o.Habit
	}
	return c.watch.Median
}

// next ends the pass running and starts the next stage.
func (c *Calibration) next(t time.Time) {
	p := summarize(c.taps)
	if c.stage == Watch {
		c.watch = p
		c.begin(Listen, t)
		return
	}
	r := Result{Watch: c.watch, Listen: p, Habit: c.habit()}
	r.Measured = p.Median - r.Habit
	r.Delay = c.base + r.Measured
	w := p.within()
	if !c.o.SkipWatch {
		w = math.Hypot(w, c.watch.within())
	}
	r.Within = secs(2 * w)
	half := (c.o.taps() + 1) / 2
	r.Enough = len(p.Offsets) >= half && (c.o.SkipWatch || len(c.watch.Offsets) >= half)
	c.res = r
	c.stage = Done
	// The clicks sound on, every beat, for a check by eye.
	c.src.from.Store(0)
}

// Result returns what the calibration found, once done.
func (c *Calibration) Result() Result { return c.res }

// secs is s seconds, to the microsecond.
func secs(s float64) time.Duration {
	return time.Duration(math.Round(s*1e6)) * time.Microsecond
}

// beatsTapped counts the beats taps landed on.
func beatsTapped(taps []tapAt) int {
	n := 0
	for i, t := range taps {
		if !slices.ContainsFunc(taps[:i], func(u tapAt) bool { return u.k == t.k }) {
			n++
		}
	}
	return n
}

// spreadFactor turns the median distance from the median into a
// standard deviation, for taps spread normally.
const spreadFactor = 1.4826

// keepWithin is the least distance from the median past which a tap is
// put aside, however closely the rest lie.
const keepWithin = 0.030

// summarize finds a pass's median and spread from its taps. Of two taps
// on one beat, the one nearer the median of all counts. A tap more than
// three spreads from the median, and keepWithin, is put aside.
func summarize(taps []tapAt) Pass {
	var p Pass
	if len(taps) == 0 {
		return p
	}
	all := make([]float64, len(taps))
	for i, t := range taps {
		all[i] = t.off
	}
	m := median(all)
	// One tap a beat: the one nearest m.
	best := map[int64]tapAt{}
	var order []int64
	for _, t := range taps {
		b, seen := best[t.k]
		if !seen {
			order = append(order, t.k)
			best[t.k] = t
			continue
		}
		p.Dropped++
		if math.Abs(t.off-m) < math.Abs(b.off-m) {
			best[t.k] = t
		}
	}
	one := make([]float64, 0, len(order))
	for _, k := range order {
		one = append(one, best[k].off)
	}
	m = median(one)
	limit := max(3*spreadFactor*mad(one, m), keepWithin)
	kept := one[:0:0]
	for _, x := range one {
		if math.Abs(x-m) <= limit {
			kept = append(kept, x)
		} else {
			p.Dropped++
		}
	}
	m = median(kept)
	p.Median = secs(m)
	p.Spread = secs(spreadFactor * mad(kept, m))
	for _, x := range kept {
		p.Offsets = append(p.Offsets, secs(x))
	}
	return p
}

// median is the median of v, which it leaves as it is.
func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := slices.Clone(v)
	slices.Sort(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// mad is the median distance of v from m.
func mad(v []float64, m float64) float64 {
	d := make([]float64, len(v))
	for i, x := range v {
		d[i] = math.Abs(x - m)
	}
	return median(d)
}

// clicks plays the beat: a short click on each beat from the beat from
// says, and silence on the beats before it. It never ends.
type clicks struct {
	rate int
	per  int64
	n    int64
	mix  *audio.Mixer
	// start is the mixer frame the first beat falls on, or -1 before
	// the mixer first reads; from is the first beat that sounds.
	start, from atomic.Int64
}

// click is how long a click lasts, and its pitch.
const (
	click      = 25 * time.Millisecond
	clickPitch = 1800.0
)

func (s *clicks) Read(dst []float32) (int, error) {
	if s.start.Load() < 0 {
		s.start.Store(s.mix.Mixed())
	}
	long := audio.FramesAt(click, s.rate)
	from := s.from.Load()
	for i := 0; i+1 < len(dst); i += 2 {
		v := float32(0)
		if k, at := s.n/s.per, s.n%s.per; k >= from && at < long {
			// A sharp start, and a quick fall: the click is heard at its
			// first frame.
			x := float64(at) / float64(s.rate)
			v = float32(0.5 * math.Exp(-x/0.004) * math.Sin(2*math.Pi*clickPitch*x))
		}
		dst[i], dst[i+1] = v, v
		s.n++
	}
	return len(dst) / 2, nil
}
