package speaker

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/marrasen/gunim/audio"
)

// TestASoundReachesTheSpeakers plays a quiet 1 kHz tone for a second.
// It runs with GUNIM_SPEAKER=1, as it makes a sound; record the
// system's output meanwhile to hear it.
func TestASoundReachesTheSpeakers(t *testing.T) {
	if os.Getenv("GUNIM_SPEAKER") != "1" {
		t.Skip("set GUNIM_SPEAKER=1 to play a tone through the speakers")
	}
	m := audio.NewMixer()
	lat, _ := time.ParseDuration(os.Getenv("GUNIM_SPEAKER_LATENCY"))
	s, err := Open(m, Options{Name: "gunim speaker test", Latency: lat})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	// The speaker finds how far ahead it must work on this machine.
	time.Sleep(1500 * time.Millisecond)
	t.Logf("working %v ahead", s.Latency())
	tone := make([]float32, 2*audio.SampleRate)
	for i := range audio.SampleRate {
		v := float32(0.1 * math.Sin(2*math.Pi*1000*float64(i)/audio.SampleRate))
		tone[2*i], tone[2*i+1] = v, v
	}
	start := time.Now()
	v := m.Play(audio.NewClip(tone).Source(), audio.Options{})
	select {
	case <-v.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("the tone never finished playing")
	}
	took := time.Since(start)
	// Done means mixed; the speakers play the last of it after.
	time.Sleep(s.Latency() + 50*time.Millisecond)
	if took < 900*time.Millisecond || took > 1500*time.Millisecond {
		t.Errorf("a second's tone took %v to play", took)
	}
	if err := s.Err(); err != nil {
		t.Error(err)
	}
	t.Logf("played a second's tone in %v", took)
}

// TestSetOpensTheSpeakersAgain plays through the system's sound at
// 48 kHz, sets 44.1 kHz, and plays on at it. It runs with
// GUNIM_SPEAKER=1, as it makes a sound.
func TestSetOpensTheSpeakersAgain(t *testing.T) {
	if os.Getenv("GUNIM_SPEAKER") != "1" {
		t.Skip("set GUNIM_SPEAKER=1 to play through the speakers")
	}
	m := audio.NewMixer()
	o := Options{Name: "gunim speaker test", Rate: 48000, Bits: 16, Dither: true}
	s, err := Open(m, o)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	time.Sleep(300 * time.Millisecond)
	o.Rate = 44100
	if err := s.Set(o); err != nil {
		t.Fatal(err)
	}
	select {
	case <-s.Changed():
	default:
		t.Error("Changed told nothing of the speakers opening again")
	}
	st := s.State()
	if m.Rate() != 44100 || st.Rate != 44100 {
		t.Errorf("the mixer plays at %d Hz and the speakers at %d, want 44100", m.Rate(), st.Rate)
	}
	before := m.Mixed()
	time.Sleep(500 * time.Millisecond)
	if got := m.Mixed() - before; got < 44100/4 || got > 44100 {
		t.Errorf("half a second mixed %d frames at 44.1 kHz", got)
	}
	t.Logf("state %+v", st)
	if err := s.Err(); err != nil {
		t.Error(err)
	}
}
