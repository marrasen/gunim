package maximizer

import "math"

// True peaks: the peaks of the sound as it would be between its samples,
// as a converter rebuilds it, which can rise over every sample, as ITU-R
// BS.1770 measures them: the sound eight times over, through a filter
// that draws the curve between the samples, and the peak of a parabola
// through its loudest point.

// The oversampling filter: tpTaps taps for each of tpPhases phases, a
// Kaiser-windowed sinc; it delays the sound tpDelay frames. phases is
// written out for eight phases.
const (
	tpPhases = 8
	tpTaps   = 64
	tpBeta   = 7.0
	tpDelay  = tpTaps / 2
)

// Fail to build unless tpPhases is the eight phases is written out for.
var _ = [1]struct{}{}[tpPhases-8]

// tpFilter is the filter's taps, by phase: phase p draws the sound
// p/tpPhases of a frame after the middle sample, each phase scaled to
// pass a steady level unchanged.
var tpFilter = func() [tpPhases][tpTaps]float64 {
	var f [tpPhases][tpTaps]float64
	mid := float64(tpTaps/2 - 1)
	for p := range tpPhases {
		sum := 0.0
		for k := range tpTaps {
			d := mid + float64(p)/tpPhases - float64(k)
			sinc := 1.0
			if d != 0 {
				sinc = math.Sin(math.Pi*d) / (math.Pi * d)
			}
			r := d / (tpTaps / 2)
			f[p][k] = sinc * bessel0(tpBeta*math.Sqrt(max(0, 1-r*r))) / bessel0(tpBeta)
			sum += f[p][k]
		}
		for k := range tpTaps {
			f[p][k] /= sum
		}
	}
	return f
}()

// bessel0 is the modified Bessel function of the first kind, of order 0.
func bessel0(x float64) float64 {
	s, term := 1.0, 1.0
	for k := 1; k < 40; k++ {
		term *= (x / 2 / float64(k)) * (x / 2 / float64(k))
		s += term
	}
	return s
}

// truePeak finds a channel's true peaks: the last tpSpan samples, at
// in a ring held twice over, end to end, so the last tpSpan are always
// side by side, from at on.
type truePeak struct {
	ring [2 * tpSpan]float64
	at   int
	// last is the last phase of the frame before; stale says it is still to work out.
	last  float64
	stale bool
	loud  loudest
}

// tpSpan is the samples a frame's true peak reads: the filter's window and the sample before it.
const tpSpan = tpTaps + 1

// tpBound is a little over the most a true peak can be, as a ratio of the loudest sample it reads.
var tpBound = func() float64 {
	most := 1.0
	for p := range tpPhases {
		s := 0.0
		for _, v := range tpFilter[p] {
			s += math.Abs(v)
		}
		most = max(most, s)
	}
	// The parabola rises at most a quarter over the phases it is drawn through.
	return 1.25 * most * (1 + 1e-9)
}()

// push takes the next sample, and returns the true peak from the sample
// tpDelay before it to the next, a ratio of full scale.
func (t *truePeak) push(x float64) float64 {
	t.loud.push(x)
	return t.peak(x)
}

// peak is push without following the loudest sample.
func (t *truePeak) peak(x float64) float64 {
	v := t.phases(x)
	m := 1
	for i := 2; i <= tpPhases; i++ {
		if math.Abs(v[i]) > math.Abs(v[m]) {
			m = i
		}
	}
	// A parabola through the loudest point and its neighbours finds the
	// peak between them.
	s := 1.0
	if v[m] < 0 {
		s = -1
	}
	y0, a, b := s*v[m], s*v[m-1], s*v[m+1]
	if c := a - 2*y0 + b; c < 0 && a <= y0 && b <= y0 {
		return y0 - (a-b)*(a-b)/(8*c)
	}
	return y0
}

// phases takes the next sample, and returns the sound drawn from the
// sample tpDelay before it to the next, at every phase in turn, after
// the last phase before it.
//
// The filter is symmetric: phase tpPhases-q is phase q backwards, and
// phase 0 is the middle sample itself. So each pair of phases is worked
// out from the sums and differences of the samples, each a pair of them
// as far from the middle of the window: half the products.
func (t *truePeak) phases(x float64) (v [tpPhases + 2]float64) {
	w, before := t.add(x)
	if t.stale {
		t.last, t.stale = lastPhase(before), false
	}
	var e1, o1, e2, o2, e3, o3, e4 float64
	for k := range tpTaps / 2 {
		s, d := w[k]+w[tpTaps-1-k], w[k]-w[tpTaps-1-k]
		h := &tpHalves[k]
		e1 += h[0] * s
		o1 += h[1] * d
		e2 += h[2] * s
		o2 += h[3] * d
		e3 += h[4] * s
		o3 += h[5] * d
		e4 += h[6] * s
	}
	v = [tpPhases + 2]float64{t.last, w[tpTaps/2-1], e1 + o1, e2 + o2, e3 + o3, e4, e3 - o3, e2 - o2, e1 - o1,
		w[tpTaps/2]}
	t.last = v[tpPhases]
	return v
}

// tpHalves are phases 1 to tpPhases/2 of the filter, halved, by tap: for
// each phase in turn, the tap's mean with the one as far from the other
// end, and half their difference, but for the middle phase, whose taps
// are their own mirror. A phase's sum is the means times the sums of the
// samples they stand for, plus the half differences times their
// differences; its mirror's, minus.
var tpHalves = func() (h [tpTaps / 2][tpPhases - 1]float64) {
	for k := range tpTaps / 2 {
		for q := range tpPhases - 1 {
			a, b := tpFilter[q/2+1][k], tpFilter[q/2+1][tpTaps-1-k]
			h[k][q] = (a + b) / 2
			if q%2 == 1 {
				h[k][q] = (a - b) / 2
			}
		}
	}
	return h
}()

// lastPhase is phase tpPhases-1 of the window w, worked out as phases works it out.
func lastPhase(w *[tpTaps]float64) float64 {
	var e1, o1 float64
	for k := range tpTaps / 2 {
		s, d := w[k]+w[tpTaps-1-k], w[k]-w[tpTaps-1-k]
		h := &tpHalves[k]
		e1 += h[0] * s
		o1 += h[1] * d
	}
	return e1 - o1
}

// add takes the next sample in, and returns the last tpTaps samples and
// the tpTaps before the last, oldest first.
func (t *truePeak) add(x float64) (w, before *[tpTaps]float64) {
	t.ring[t.at], t.ring[t.at+tpSpan] = x, x
	t.at++
	if t.at == tpSpan {
		t.at = 0
	}
	return (*[tpTaps]float64)(t.ring[t.at+1 : t.at+1+tpTaps]), (*[tpTaps]float64)(t.ring[t.at : t.at+tpTaps])
}

// run pushes each of x in turn, and puts the true peak each returns in
// out, as long as x.
func (t *truePeak) run(x, out []float64) {
	out = out[:len(x)]
	for i, v := range x {
		out[i] = t.push(v)
	}
}

// over puts in out, for each of x, its true peak where it may pass ceiling, and 0 where it cannot.
func (t *truePeak) over(x, out []float64, ceiling float64) {
	out = out[:len(x)]
	for i, v := range x {
		t.loud.push(v)
		if t.loud.most()*tpBound <= ceiling {
			t.add(v)
			t.stale = true
			out[i] = 0
			continue
		}
		out[i] = t.peak(v)
	}
}

// most returns the most of floor and the true peaks of x.
func (t *truePeak) most(x []float64, floor float64) float64 {
	// A frame's true peak is at least the sample tpDelay before it, so the loudest of those starts the floor.
	for _, v := range x[:max(0, len(x)-tpDelay)] {
		floor = max(floor, math.Abs(v))
	}
	for _, v := range x {
		t.loud.push(v)
		if t.loud.most()*tpBound < floor {
			t.add(v)
			t.stale = true
			continue
		}
		floor = max(floor, t.peak(v))
	}
	return floor
}

// reset forgets the samples before.
func (t *truePeak) reset() { *t = truePeak{} }

// loudest follows, a little over, the loudest of the last tpSpan samples: the loudest of the run of tpRun samples
// coming in, and of the tpRuns runs before it, a NaN the loudest of all.
type loudest struct {
	runs [tpRuns]float64
	at   int
	cur  float64
	n    int
	past float64
}

// tpRun and tpRuns are how loudest splits the samples, in runs that span at least tpSpan.
const (
	tpRun  = 16
	tpRuns = (tpSpan + tpRun - 1) / tpRun
)

// push takes the next sample in.
func (l *loudest) push(x float64) {
	l.cur = max(l.cur, math.Abs(x))
	l.n++
	if l.n < tpRun {
		return
	}
	l.runs[l.at] = l.cur
	l.at++
	if l.at == tpRuns {
		l.at = 0
	}
	l.cur, l.n = 0, 0
	l.past = l.runs[0]
	for _, v := range l.runs[1:] {
		l.past = max(l.past, v)
	}
}

// most is at least the loudest of the last tpSpan samples.
func (l *loudest) most() float64 { return max(l.cur, l.past) }
