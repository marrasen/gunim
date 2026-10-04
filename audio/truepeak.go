package audio

import "math"

// A TruePeakMeter finds a sound's true peak, as BS.1770 defines it: the
// loudest the sound gets between its samples too, as a converter plays
// it, found by playing it at four times its rate. A sound that never
// passes full scale at its samples may still pass it between them, and
// clip on the way out.
type TruePeakMeter struct {
	hist [2][truePeakTaps]float64
	at   int
	peak float64
}

// truePeakTaps is how many samples each of the four phases of the
// interpolating filter reads.
const truePeakTaps = 12

// truePeakFilter holds the four phases' coefficients: a windowed sinc
// interpolating at four times the rate, its passband flat to well
// above 20 kHz at 44.1 kHz.
var truePeakFilter = func() (f [4][truePeakTaps]float64) {
	const n = 4 * truePeakTaps
	for i := range n {
		x := float64(i) - float64(n-1)/2
		t := x / 4
		sinc := 1.0
		if t != 0 {
			sinc = math.Sin(math.Pi*t) / (math.Pi * t)
		}
		// A Blackman-Harris window.
		u := 2 * math.Pi * float64(i) / float64(n-1)
		w := 0.35875 - 0.48829*math.Cos(u) + 0.14128*math.Cos(2*u) - 0.01168*math.Cos(3*u)
		f[i%4][i/4] = sinc * w
	}
	// Each phase passes a steady sound as it is.
	for p := range f {
		var sum float64
		for _, c := range f[p] {
			sum += c
		}
		for k := range f[p] {
			f[p][k] /= sum
		}
	}
	return f
}()

// Write measures frames, interleaved stereo.
func (m *TruePeakMeter) Write(frames []float32) {
	for i := 0; i+1 < len(frames); i += 2 {
		m.at = (m.at + 1) % truePeakTaps
		for ch := range 2 {
			m.hist[ch][m.at] = float64(frames[i+ch])
			for p := range truePeakFilter {
				var y float64
				for k, c := range truePeakFilter[p] {
					y += c * m.hist[ch][(m.at-k+truePeakTaps)%truePeakTaps]
				}
				m.peak = max(m.peak, math.Abs(y))
			}
		}
	}
}

// Peak returns the true peak written, 1 at full scale.
func (m *TruePeakMeter) Peak() float64 { return m.peak }
