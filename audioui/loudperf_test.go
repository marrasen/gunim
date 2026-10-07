package audioui

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// powerOf is the mean weighted power of loudness l, in LUFS.
func powerOf(l float64) float64 { return math.Pow(10, (l+0.691)/10) }

// randomPowers returns n powers of loudness spread from -95 to +5 LUFS, some silent, and some exactly on the gates
// a steady sound at -30 LUFS sets.
func randomPowers(rng *rand.Rand, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		switch rng.IntN(20) {
		case 0:
			out[i] = 0
		case 1:
			out[i] = powerOf(-70)
		case 2:
			out[i] = powerOf(-40)
		default:
			out[i] = powerOf(-95 + 100*rng.Float64())
		}
	}
	return out
}

// The histogram reads the integrated loudness and the range as the audio package's pass over every power does.
func TestTheLoudnessHistogramReadsAsAPassOverEveryPower(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	for round := range 200 {
		powers := randomPowers(rng, 1+rng.IntN(3000))
		h := &loudHist{}
		for _, p := range powers {
			h.add(p)
		}
		want, wantOK := audio.Integrated(powers)
		got, ok := h.integrated()
		if ok != wantOK || ok && math.Abs(got-want) > 1e-9 {
			t.Fatalf("round %d: integrated %v %v, want %v %v", round, got, ok, want, wantOK)
		}
		wlo, whi, wok := audio.LoudnessRange(powers)
		lo, hi, rok := h.span()
		if rok != wok || rok && (lo != wlo || hi != whi) {
			t.Fatalf("round %d: ranged %v from %v to %v, want %v from %v to %v", round, rok, lo, hi, wok, wlo, whi)
		}
	}
}

// anHour returns an hour of blocks' powers, 100 ms apart, wandering about -14 LUFS.
func anHour() []float64 {
	rng := rand.New(rand.NewPCG(5, 6))
	out := make([]float64, 36_000)
	l := -14.0
	for i := range out {
		l = max(-40, min(0, l+rng.NormFloat64()))
		out[i] = powerOf(l)
	}
	return out
}

// BenchmarkLoudnessFrameAnHourIn times a frame of a loudness panel an hour into a sound: 1/60 s of it heard, the
// readings eased, and the panel drawn.
func BenchmarkLoudnessFrameAnHourIn(b *testing.B) {
	const r = 8000
	l := NewLoudness(r)
	second := sine(r, 1000, 0.3)
	for range 3600 {
		l.Write(second)
	}
	frame := second[:2*r/60]
	var p paint.Painter
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		l.Write(frame)
		l.Step(time.Second/60, true)
		p.Reset()
		l.Paint(&p, nil, geom.Rc(0, 0, 300, 200), -14)
	}
}

// BenchmarkCurvesFrame draws the loudness curves of an hour's sound, ten seconds of it in view, as each frame of
// a view of the sound does.
func BenchmarkCurvesFrame(b *testing.B) {
	blocks := anHour()
	c := &Curves{Blocks: blocks, Shorts: blocks, Running: RunningLoudness(blocks), LUFS: -14, Loud: true}
	v := CurveView{Area: geom.Rc(0, 0, 800, 200), X: func(t float64) float32 { return float32((t - 1800) * 80) },
		Fade: [4]float32{1, 1, 1, 1}, Alpha: 1, Target: -14, Right: 800}
	var p paint.Painter
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		p.Reset()
		c.Paint(&p, nil, v)
	}
}

// BenchmarkRunningLoudness reads the integrated loudness to each second of an hour's sound.
func BenchmarkRunningLoudness(b *testing.B) {
	blocks := anHour()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		RunningLoudness(blocks)
	}
}
