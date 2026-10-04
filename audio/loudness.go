package audio

import "math"

// A LoudnessMeter measures how loud a sound is as the ear hears it, as
// ITU-R BS.1770 defines it, and EBU R128 and ReplayGain 2 use it: its
// integrated loudness, in LUFS, over the whole of what it is given.
// Give it a sound's frames with Write, then read Integrated. Tools
// such as bs1770gain measure the same.
//
// The sound is weighted first, as the ear weighs pitches: its bass
// counts for less and its highs a little more. Its power is taken in
// blocks of 400 ms, each starting 100 ms after the last. Blocks under
// -70 LUFS are silence, and pass uncounted; so do blocks more than 10
// LU under the loudness of the rest, so a song's quiet passages
// barely lower it.
//
// The zero LoudnessMeter measures sound at [SampleRate];
// [NewLoudnessMeter] makes one for another rate.
type LoudnessMeter struct {
	// shelf and high are the weighting's two filters, each a channel's
	// two values of state, with their coefficients at the meter's rate,
	// and quarter is 100 ms of frames at it.
	shelf, high   [2][2]float64
	kShelf, kHigh biquad
	quarter       int
	// recent holds the last 30 100 ms parts, three seconds, for
	// ShortTerm.
	recent []float64
	// sum is the weighted power of the 100 ms gathering now, over n
	// frames; quarters holds the last four 100 ms parts, and blocks the
	// power of every 400 ms block.
	sum      float64
	n        int
	quarters []float64
	blocks   []float64
	peak     float32
}

// NewLoudnessMeter returns a meter of sound at rate.
func NewLoudnessMeter(rate int) *LoudnessMeter {
	m := &LoudnessMeter{}
	m.at(rate)
	return m
}

// at sets the meter's weighting for sound at rate, from the analog
// filters BS.1770 specifies at 48 kHz, as libebur128 works them out:
// a shelf lifting the highs about 4 dB, and a cut under about 38 Hz.
func (m *LoudnessMeter) at(rate int) {
	fs := float64(rate)
	k := math.Tan(math.Pi * 1681.974450955533 / fs)
	vh := math.Pow(10, 3.999843853973347/20)
	vb := math.Pow(vh, 0.4996667741545416)
	q := 0.7071752369554196
	a0 := 1 + k/q + k*k
	m.kShelf = biquad{b0: (vh + vb*k/q + k*k) / a0, b1: 2 * (k*k - vh) / a0, b2: (vh - vb*k/q + k*k) / a0,
		a1: 2 * (k*k - 1) / a0, a2: (1 - k/q + k*k) / a0}
	k = math.Tan(math.Pi * 38.13547087602444 / fs)
	q = 0.5003270373238773
	a0 = 1 + k/q + k*k
	m.kHigh = biquad{b0: 1, b1: -2, b2: 1, a1: 2 * (k*k - 1) / a0, a2: (1 - k/q + k*k) / a0}
	m.quarter = rate / 10
}

// Write measures frames, interleaved stereo at the meter's rate.
func (m *LoudnessMeter) Write(frames []float32) {
	if m.quarter == 0 {
		m.at(SampleRate)
	}
	quarter := m.quarter
	kShelf, kHigh := m.kShelf, m.kHigh
	for i := 0; i+1 < len(frames); i += 2 {
		var power float64
		for ch := range 2 {
			x := float64(frames[i+ch])
			m.peak = max(m.peak, float32(math.Abs(x)))
			x = m.filter(&m.shelf[ch], kShelf, x)
			x = m.filter(&m.high[ch], kHigh, x)
			power += x * x
		}
		m.sum += power
		m.n++
		if m.n < quarter {
			continue
		}
		q := m.sum / float64(quarter)
		m.quarters = append(m.quarters, q)
		m.recent = append(m.recent, q)
		if len(m.recent) > 30 {
			m.recent = m.recent[len(m.recent)-30:]
		}
		m.sum, m.n = 0, 0
		if k := len(m.quarters); k >= 4 {
			last := m.quarters[k-4:]
			m.blocks = append(m.blocks, (last[0]+last[1]+last[2]+last[3])/4)
			m.quarters = m.quarters[k-3:]
		}
	}
}

// filter runs x through section s, with state st, in transposed
// direct form II.
func (*LoudnessMeter) filter(st *[2]float64, s biquad, x float64) float64 {
	y := s.b0*x + st[0]
	st[0] = s.b1*x - s.a1*y + st[1]
	st[1] = s.b2*x - s.a2*y
	return y
}

// lufs is the loudness of a mean weighted power.
func lufs(power float64) float64 { return -0.691 + 10*math.Log10(power) }

// Integrated returns the loudness of everything written, in LUFS, and
// false where there was too little that was not silence to say: under
// 400 ms of it.
func (m *LoudnessMeter) Integrated() (float64, bool) {
	gated := func(floor float64) (mean float64, n int) {
		var sum float64
		for _, b := range m.blocks {
			if b > 0 && lufs(b) > floor {
				sum += b
				n++
			}
		}
		if n == 0 {
			return 0, 0
		}
		return sum / float64(n), n
	}
	mean, n := gated(-70)
	if n == 0 {
		return math.Inf(-1), false
	}
	mean, n = gated(lufs(mean) - 10)
	if n == 0 {
		return math.Inf(-1), false
	}
	return lufs(mean), true
}

// Momentary returns the loudness of the last 400 ms written, in LUFS,
// as a meter shows it moving; minus infinity before there is 400 ms.
func (m *LoudnessMeter) Momentary() float64 {
	if len(m.blocks) == 0 {
		return math.Inf(-1)
	}
	return lufs(m.blocks[len(m.blocks)-1])
}

// ShortTerm returns the loudness of the last three seconds written, in
// LUFS, or of all written before there are three.
func (m *LoudnessMeter) ShortTerm() float64 {
	if len(m.recent) == 0 {
		return math.Inf(-1)
	}
	var sum float64
	for _, q := range m.recent {
		sum += q
	}
	return lufs(sum / float64(len(m.recent)))
}

// Peak returns the loudest sample written, 1 at full scale.
func (m *LoudnessMeter) Peak() float32 { return m.peak }
