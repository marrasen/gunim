package audio

import (
	"math"
	"testing"
	"time"

	"github.com/marrasen/gunim/anim"
)

// constant returns a clip of n frames at l and r.
func constant(n int, l, r float32) *Clip {
	s := make([]float32, 2*n)
	for i := range n {
		s[2*i], s[2*i+1] = l, r
	}
	return NewClip(s)
}

// mix mixes n frames of m.
func mix(m *Mixer, n int) []float32 {
	out := make([]float32, 2*n)
	m.Mix(out)
	return out
}

func TestTwoSoundsAddUp(t *testing.T) {
	m := NewMixer()
	m.Play(constant(1000, 0.25, 0.1).Source(), Options{})
	m.Play(constant(1000, 0.25, 0.2).Source(), Options{})
	out := mix(m, 100)
	if !near(float64(out[0]), 0.5, 1e-6) || !near(float64(out[1]), 0.3, 1e-6) {
		t.Errorf("two sounds mix to %v, %v; want 0.5, 0.3", out[0], out[1])
	}
}

func TestASoundEndsAndItsVoiceIsDone(t *testing.T) {
	m := NewMixer()
	v := m.Play(constant(300, 0.5, 0.5).Source(), Options{})
	out := mix(m, 1000)
	if out[2*299] != 0.5 || out[2*300] != 0 {
		t.Errorf("frames 299 and 300 are %v and %v, want the sound's last and then silence", out[2*299], out[2*300])
	}
	select {
	case <-v.Done():
	default:
		t.Error("the voice is not done once its sound ended")
	}
	if m.Playing() != 0 {
		t.Errorf("%d voices playing after the sound ended, want none", m.Playing())
	}
}

func TestPanSendsASoundToOneSpeaker(t *testing.T) {
	m := NewMixer()
	m.Play(constant(1000, 0.5, 0.5).Source(), Options{Pan: -1})
	out := mix(m, 10)
	if out[0] != 0.5 || out[1] != 0 {
		t.Errorf("panned left the frame is %v, %v; want 0.5, 0", out[0], out[1])
	}
}

func TestAVolumeChangeGlidesWithoutSteps(t *testing.T) {
	m := NewMixer()
	v := m.Play(constant(SampleRate, 1, 1).Source(), Options{})
	mix(m, block)
	v.SetVolume(0, anim.Tween{Duration: 100 * time.Millisecond, Ease: anim.Linear})
	out := mix(m, int(Frames(200*time.Millisecond)))
	// No two frames differ by more than a linear fade's step allows.
	step := 1 / float64(Frames(100*time.Millisecond))
	for i := 1; i < len(out)/2; i++ {
		if d := math.Abs(float64(out[2*i] - out[2*i-2])); d > 2*step {
			t.Fatalf("frames %d and %d differ by %v, a step in the fade", i-1, i, d)
		}
	}
	half := out[2*Frames(50*time.Millisecond)]
	if !near(float64(half), 0.5, 0.02) {
		t.Errorf("halfway through the fade the sound is at %v, want 0.5", half)
	}
	if end := out[len(out)-2]; end != 0 {
		t.Errorf("after the fade the sound is at %v, want silence", end)
	}
}

func TestFadeInRisesFromSilence(t *testing.T) {
	m := NewMixer()
	m.Play(constant(SampleRate, 1, 1).Source(), Options{FadeIn: 100 * time.Millisecond})
	out := mix(m, int(Frames(150*time.Millisecond)))
	if out[0] > 0.01 {
		t.Errorf("the first frame of a fade in is %v, want silence", out[0])
	}
	if mid := out[2*Frames(50*time.Millisecond)]; !near(float64(mid), 0.5, 0.02) {
		t.Errorf("halfway in the sound is at %v, want 0.5", mid)
	}
	if end := out[len(out)-2]; end != 1 {
		t.Errorf("after the fade in the sound is at %v, want 1", end)
	}
}

func TestStopFadesOutAndEnds(t *testing.T) {
	m := NewMixer()
	v := m.Play(constant(SampleRate, 1, 1).Source(), Options{})
	v.Stop(0)
	out := mix(m, int(Frames(20*time.Millisecond)))
	if out[0] < 0.9 {
		t.Errorf("a stop cut the sound at once to %v; want it to fade", out[0])
	}
	if end := out[len(out)-2]; end != 0 {
		t.Errorf("after a stop the sound is at %v, want silence", end)
	}
	select {
	case <-v.Done():
	default:
		t.Error("a stopped voice is not done")
	}
}

func TestPauseHoldsTheSoundWhereItIs(t *testing.T) {
	m := NewMixer()
	ramp := make([]float32, 2*SampleRate)
	for i := range SampleRate {
		ramp[2*i] = float32(i) / SampleRate
	}
	v := m.Play(NewClip(ramp).Source(), Options{})
	mix(m, 4800)
	v.Pause()
	mix(m, 4800)
	if !v.Paused() {
		t.Fatal("not paused")
	}
	quiet := mix(m, 4800)
	if quiet[0] != 0 || quiet[len(quiet)-2] != 0 {
		t.Fatalf("a paused voice plays %v", quiet[0])
	}
	held := v.Position()
	v.Resume()
	out := mix(m, 4800)
	// It plays on from about where it paused: the pause's own fade
	// took a few milliseconds more.
	at := float64(out[len(out)-2]) * SampleRate
	want := float64(Frames(held) + 4800)
	if !near(at, want, float64(Frames(2*declick))) {
		t.Errorf("after the pause the sound is at frame %.0f, want about %.0f", at, want)
	}
}

func TestALoopingSoundStartsOver(t *testing.T) {
	m := NewMixer()
	m.Play(constant(100, 0.5, 0.5).Source(), Options{Loop: true})
	out := mix(m, 1000)
	for i := range 1000 {
		if out[2*i] != 0.5 {
			t.Fatalf("frame %d of a looping sound is %v, want it on", i, out[2*i])
		}
	}
}

func TestPositionCountsWhatTheSpeakersStillHold(t *testing.T) {
	m := NewMixer()
	held := int64(4800)
	m.SetLatency(func() int64 { return held })
	v := m.Play(constant(SampleRate, 0.5, 0.5).Source(), Options{})
	mix(m, 12000)
	if got, want := v.Position(), Duration(12000-4800); got != want {
		t.Errorf("position %v with %v in the speakers, want %v", got, Duration(held), want)
	}
	if err := v.Seek(500 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if got := v.Position(); got != 500*time.Millisecond {
		t.Errorf("position %v right after a seek to 500ms, want 500ms", got)
	}
	mix(m, 9600)
	if got, want := v.Position(), 500*time.Millisecond+Duration(9600-4800); got != want {
		t.Errorf("position %v after playing on, want %v", got, want)
	}
}

func TestReadGivesTheMixAsFloats(t *testing.T) {
	m := NewMixer()
	m.Play(constant(10, 0.5, -0.25).Source(), Options{})
	p := make([]byte, 8*4)
	if n, err := m.Read(p); n != len(p) || err != nil {
		t.Fatalf("Read: %d, %v", n, err)
	}
	if l, r := math.Float32frombits(uint32(p[0])|uint32(p[1])<<8|uint32(p[2])<<16|uint32(p[3])<<24),
		math.Float32frombits(uint32(p[4])|uint32(p[5])<<8|uint32(p[6])<<16|uint32(p[7])<<24); l != 0.5 || r != -0.25 {
		t.Errorf("first frame %v, %v; want 0.5, -0.25", l, r)
	}
}

func TestTheAnalyzerFindsAPitchInItsBand(t *testing.T) {
	m := NewMixer()
	sine := make([]float32, 2*SampleRate)
	for i := range SampleRate {
		v := float32(math.Sin(2 * math.Pi * 1000 * float64(i) / SampleRate))
		sine[2*i], sine[2*i+1] = v, v
	}
	m.Play(NewClip(sine).Source(), Options{})
	mix(m, SampleRate/2)
	a := NewAnalyzer(m, 32)
	bands := make([]float32, 32)
	level := a.Bands(bands)
	if !near(float64(level), 1, 0.02) {
		t.Errorf("a full-scale sine's level is %v, want 1", level)
	}
	// 1 kHz lies in the band whose edges hold it.
	want := -1
	for b := range 32 {
		lo, hi := float64(a.edges[b])*SampleRate/4096, float64(a.edges[b+1])*SampleRate/4096
		if lo <= 1000 && 1000 < hi {
			want = b
		}
	}
	loudest := 0
	for b := range bands {
		if bands[b] > bands[loudest] {
			loudest = b
		}
	}
	if loudest != want {
		t.Errorf("the loudest band is %d, want %d, which holds 1 kHz", loudest, want)
	}
	if bands[want] < 0.95 {
		t.Errorf("the band of a full-scale sine is at %v, want about 1", bands[want])
	}
	if bands[0] > 0.3 {
		t.Errorf("the lowest band, far from the sine, is at %v, want it quiet", bands[0])
	}
	// A tilt lifts a band above the bass by its octaves: 1 kHz is
	// nearly five octaves above 40 Hz, so 3 dB an octave lifts it about
	// 14 dB, a fifth of the scale's 70.
	low := make([]float32, 32)
	half := make([]float32, 2*SampleRate)
	for i := range half {
		half[i] = sine[i] * 0.05
	}
	m2 := NewMixer()
	m2.Play(NewClip(half).Source(), Options{})
	mix(m2, SampleRate/2)
	b2 := NewAnalyzer(m2, 32)
	b2.Bands(low)
	b2.Tilt = 3
	tilted := make([]float32, 32)
	b2.Bands(tilted)
	if d := tilted[want] - low[want]; !near(float64(d), 14.0/70, 0.02) {
		t.Errorf("a tilt of 3 dB an octave lifts 1 kHz by %v of the scale, want about %v", d, 14.0/70)
	}
}
