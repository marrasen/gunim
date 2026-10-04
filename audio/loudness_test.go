package audio

import (
	"math"
	"testing"
)

func TestASineMeasuresAsBS1770Says(t *testing.T) {
	// A 1 kHz sine at -20 dBFS in both channels is -20 LUFS: the
	// weighting lifts 1 kHz by the 0.691 dB the formula takes away.
	var m LoudnessMeter
	m.Write(sine(SampleRate*5, 1000, math.Pow(10, -20.0/20), 0))
	got, ok := m.Integrated()
	if !ok || math.Abs(got+20) > 0.1 {
		t.Fatalf("a 1 kHz sine at -20 dBFS measured %.2f LUFS (%v), want -20", got, ok)
	}
	if p := m.Peak(); math.Abs(float64(p)-0.1) > 0.001 {
		t.Fatalf("its peak is %v, want 0.1", p)
	}
}

func TestSilenceAndQuietPassagesBarelyLowerTheLoudness(t *testing.T) {
	loud := sine(SampleRate*5, 1000, 0.1, 0)
	var alone LoudnessMeter
	alone.Write(loud)
	want, _ := alone.Integrated()

	// The same, with silence and a passage 30 dB quieter: the gates
	// leave both out.
	var m LoudnessMeter
	m.Write(make([]float32, 2*SampleRate*3))
	m.Write(loud)
	m.Write(sine(SampleRate*3, 1000, 0.1*math.Pow(10, -30.0/20), 0))
	got, _ := m.Integrated()
	if math.Abs(got-want) > 0.3 {
		t.Fatalf("with silence and a quiet passage it measured %.2f LUFS, want about %.2f", got, want)
	}
	var none LoudnessMeter
	none.Write(make([]float32, 2*SampleRate))
	if _, ok := none.Integrated(); ok {
		t.Fatal("silence measured as a loudness")
	}
}

func TestTheWeightingCountsBassForLess(t *testing.T) {
	var low, mid LoudnessMeter
	low.Write(sine(SampleRate*3, 40, 0.1, 0))
	mid.Write(sine(SampleRate*3, 1000, 0.1, 0))
	l, _ := low.Integrated()
	m, _ := mid.Integrated()
	if m-l < 0.5 {
		t.Fatalf("40 Hz measured %.2f LUFS and 1 kHz %.2f, at one level; want 40 Hz quieter", l, m)
	}
}
