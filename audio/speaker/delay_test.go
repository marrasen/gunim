package speaker

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/audio"
)

// A delay set on the speaker moves the frame heard earlier by as much, in the speaker's Heard, the mixer's and the
// anchor alike, and adds to the latency, while the output keeps working as far ahead as before.
func TestADelayCountsInEverythingHeard(t *testing.T) {
	m := audio.NewMixer()
	s, _ := stillSpeaker(m)
	m.SetLatency(s.held)
	m.Mix(make([]float32, 2*48000))
	if h := s.Heard(); h != 48000 {
		t.Fatalf("with no delay, Heard = %d, want 48000", h)
	}
	s.SetDelay(-10 * time.Millisecond)
	if h := s.Heard(); h != 48000 {
		t.Fatalf("a delay below what the output holds took Heard to %d, want it held at 48000", h)
	}
	s.SetDelay(250 * time.Millisecond)
	if s.Delay() != 250*time.Millisecond || s.State().Delay != 250*time.Millisecond {
		t.Fatalf("Delay = %v, State().Delay = %v, want 250ms", s.Delay(), s.State().Delay)
	}
	if l := s.Latency(); l != 250*time.Millisecond {
		t.Fatalf("Latency = %v, want the output's 0 and the delay's 250ms", l)
	}
	if l := s.State().Latency; l != 0 {
		t.Fatalf("State().Latency = %v, want the output's own 0", l)
	}
	// Heard never goes back: it stands until the sound catches up with the delay.
	if h := s.Heard(); h != 48000 {
		t.Fatalf("as the delay grew Heard went to %d, want it to stand at 48000", h)
	}
	m.Mix(make([]float32, 2*48000))
	if h, want := s.Heard(), int64(96000-12000); h != want || m.Heard() != want {
		t.Fatalf("a second on, Heard = %d and the mixer's %d, want %d", h, m.Heard(), want)
	}
	if f := s.Anchor().Frame; f != 96000-12000 {
		t.Fatalf("the anchor sits at frame %d, want %d", f, 96000-12000)
	}
}
