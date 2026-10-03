package audio

import (
	"math"
	"math/cmplx"
)

// An Analyzer measures the sound a [Mixer] is playing as it is heard:
// how loud it is, and how loud each band of pitches is, for visuals
// that move with the music. Measure it once a frame of the display.
// An Analyzer is for one goroutine at a time.
type Analyzer struct {
	m      *Mixer
	size   int
	window []float32
	in     []float32
	fft    []complex128
	// edges are the bins each band starts at, one more than the bands.
	edges []int
	// Floor is the quietest level a band shows, in decibels: a band
	// that loud or quieter is at 0. It is -70 by default.
	Floor float32
	// Tilt lifts each band by this many decibels for each octave it
	// lies above 40 Hz. Music has less in its higher pitches, so a
	// tilt of 3 to 4.5 shows them as tall as the bass; zero, the
	// default, shows each band as loud as it is.
	Tilt float32
}

// NewAnalyzer returns an analyzer of m's sound that splits it into
// the given number of bands of pitch, from 40 Hz to 16 kHz, each as wide as the next
// to the ear: as many bands in each octave.
func NewAnalyzer(m *Mixer, bands int) *Analyzer {
	const size = 4096
	a := &Analyzer{
		m:      m,
		size:   size,
		window: make([]float32, size),
		in:     make([]float32, size),
		fft:    make([]complex128, size),
		Floor:  -70,
	}
	for i := range a.window {
		// A Hann window, so a pitch between two bins stays in them.
		a.window[i] = float32(0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(size-1)))
	}
	lo, hi := math.Log2(40), math.Log2(16000)
	binHz := float64(SampleRate) / size
	a.edges = make([]int, bands+1)
	prev := 0
	for i := range a.edges {
		hz := math.Exp2(lo + (hi-lo)*float64(i)/float64(bands))
		b := max(1, int(math.Round(hz/binHz)))
		if i > 0 {
			// Each band takes at least one bin of its own.
			b = max(b, prev+1)
		}
		a.edges[i] = min(b, size/2)
		prev = a.edges[i]
	}
	return a
}

// Bands fills out, one value a band, with how loud each band is being
// heard, from 0, at Floor or quieter, to 1, as loud as a full-scale
// sine. It returns the level of the whole sound the same way.
func (a *Analyzer) Bands(out []float32) (level float32) {
	a.m.recent(a.in)
	var sum float64
	for i, s := range a.in {
		sum += float64(s) * float64(s)
		a.fft[i] = complex(float64(s*a.window[i]), 0)
	}
	fft(a.fft)
	// A full-scale sine through the Hann window peaks at size/4.
	ref := float64(a.size) / 4
	for b := range out {
		if b+1 >= len(a.edges) {
			out[b] = 0
			continue
		}
		peak := 0.0
		for k := a.edges[b]; k < max(a.edges[b+1], a.edges[b]+1) && k < a.size/2; k++ {
			peak = max(peak, cmplx.Abs(a.fft[k]))
		}
		amp := peak / ref
		if a.Tilt != 0 {
			octaves := math.Log2(float64(a.edges[b]+a.edges[b+1]) / 2 * SampleRate / float64(a.size) / 40)
			amp *= math.Pow(10, float64(a.Tilt)*max(octaves, 0)/20)
		}
		out[b] = a.scale(amp)
	}
	rms := math.Sqrt(sum / float64(len(a.in)))
	// A full-scale sine has an RMS of 1/√2.
	return a.scale(rms * math.Sqrt2)
}

// scale maps an amplitude, 1 full scale, to 0 to 1 over the decibels
// from Floor to 0.
func (a *Analyzer) scale(amp float64) float32 {
	if amp <= 0 {
		return 0
	}
	db := 20 * math.Log10(amp)
	return float32(max(0, min(1, 1-db/float64(a.Floor))))
}

// Wave fills out with the last len(out) samples heard, mono, oldest
// first, for drawing the wave itself. It holds at most a little under
// a second.
func (a *Analyzer) Wave(out []float32) { a.m.recent(out[:min(len(out), historyFrames/2)]) }

// recent fills out with the last len(out) frames heard, as mono.
func (m *Mixer) recent(out []float32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	end := m.heardLocked()
	start := end - int64(len(out))
	for i := range out {
		f := start + int64(i)
		if f < 0 || f < m.played-historyFrames {
			out[i] = 0
			continue
		}
		out[i] = m.history[f&(historyFrames-1)]
	}
}

// fft transforms x in place; len(x) is a power of two.
func fft(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Exp(complex(0, -2*math.Pi/float64(size)))
		for start := 0; start < n; start += size {
			wk := complex(1, 0)
			for k := range size / 2 {
				u, t := x[start+k], wk*x[start+k+size/2]
				x[start+k], x[start+k+size/2] = u+t, u-t
				wk *= w
			}
		}
	}
}
