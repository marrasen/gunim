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
type LoudnessMeter struct {
	// shelf and high are the weighting's two filters, each a channel's
	// two values of state.
	shelf, high [2][2]float64
	// sum is the weighted power of the 100 ms gathering now, over n
	// frames; quarters holds the last four 100 ms parts, and blocks the
	// power of every 400 ms block.
	sum      float64
	n        int
	quarters []float64
	blocks   []float64
	peak     float32
}

// The weighting's filters at 48 kHz, from BS.1770: a shelf lifting the
// highs about 4 dB, and a cut under about 38 Hz.
var (
	kShelf = biquad{b0: 1.53512485958697, b1: -2.69169618940638, b2: 1.19839281085285,
		a1: -1.69065929318241, a2: 0.73248077421585}
	kHigh = biquad{b0: 1, b1: -2, b2: 1, a1: -1.99004745483398, a2: 0.99007225036621}
)

// quarter is 100 ms of frames.
const quarter = SampleRate / 10

// Write measures frames, interleaved stereo at [SampleRate].
func (m *LoudnessMeter) Write(frames []float32) {
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
		m.quarters = append(m.quarters, m.sum/quarter)
		m.sum, m.n = 0, 0
		if k := len(m.quarters); k >= 4 {
			q := m.quarters[k-4:]
			m.blocks = append(m.blocks, (q[0]+q[1]+q[2]+q[3])/4)
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

// Peak returns the loudest sample written, 1 at full scale.
func (m *LoudnessMeter) Peak() float32 { return m.peak }
