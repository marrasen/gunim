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
