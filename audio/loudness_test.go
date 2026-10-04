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

// steps writes twenty seconds of a 1 kHz tone at 48 kHz, at amplitude
// a, to the meters.
func steps(a float64, ms ...*LoudnessMeter) {
	buf := make([]float32, 2*48000)
	for s := range 20 {
		for i := range 48000 {
			v := float32(a * math.Sin(2*math.Pi*1000*float64(s*48000+i)/48000))
			buf[2*i], buf[2*i+1] = v, v
		}
		for _, m := range ms {
			m.Write(buf)
		}
	}
}

func TestTheLoudnessRangeIsAsFFmpegMeasuresIt(t *testing.T) {
	// Twenty seconds of a tone, and twenty of it 12 dB down: ffmpeg's
	// ebur128 reads 12.0 LU.
	m := NewLoudnessMeter(48000)
	steps(0.5, m)
	steps(0.125, m)
	r, ok := m.Range()
	if !ok || math.Abs(r-12.0) > 0.1 {
		t.Fatalf("the range is %.2f LU (%v), want 12.0", r, ok)
	}
	if _, ok := NewLoudnessMeter(48000).Range(); ok {
		t.Fatal("nothing written has a range")
	}
}

func TestTwoSoundsMeasuredTogetherAreAsOne(t *testing.T) {
	loud, quiet, both := NewLoudnessMeter(48000), NewLoudnessMeter(48000), NewLoudnessMeter(48000)
	steps(0.5, loud, both)
	steps(0.125, quiet, both)
	i, _ := Integrated(append(loud.Blocks(), quiet.Blocks()...))
	want, _ := both.Integrated()
	if math.Abs(i-want) > 0.1 {
		t.Fatalf("measured together, the sounds are %.2f LUFS; as one, %.2f", i, want)
	}
	// The range together leaves out the three seconds across the change
	// of level that one sound has: the levels' difference, 12 dB.
	low, high, _ := LoudnessRange(append(loud.ShortTerms(), quiet.ShortTerms()...))
	if math.Abs(high-low-12.04) > 0.1 || math.Abs(high-(-6.0)) > 0.1 {
		t.Fatalf("measured together, the range is %.2f to %.2f LUFS, want -18.04 to -6.0", low, high)
	}
}
