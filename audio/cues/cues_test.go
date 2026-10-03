package cues

import (
	"math"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio"
)

func TestEachSoundIsShortQuietAndStartsAndEndsInSilence(t *testing.T) {
	for c, clip := range Sounds() {
		s := clip.Samples()
		if d := audio.Duration(clip.Len()); d <= 0 || d > 400e6 {
			t.Errorf("%s lasts %v, want a short sound", c, d)
		}
		peak := 0.0
		for _, v := range s {
			peak = max(peak, math.Abs(float64(v)))
		}
		if peak < 0.03 || peak > 0.3 {
			t.Errorf("%s peaks at %.3f, want it quiet but there", c, peak)
		}
		// No click: the sound starts from silence and dies away.
		if first := math.Abs(float64(s[0])); first > 0.02 {
			t.Errorf("%s starts at %.3f, want silence", c, first)
		}
		if last := math.Abs(float64(s[len(s)-2])); last > 0.01 {
			t.Errorf("%s ends at %.3f, want silence", c, last)
		}
	}
}

func TestEveryCueOfTheWidgetsHasASound(t *testing.T) {
	s := Sounds()
	for _, c := range []gunim.Cue{gunim.CuePress, gunim.CueToggleOn, gunim.CueToggleOff, gunim.CueSelect,
		gunim.CueTick, gunim.CueOpen, gunim.CueClose, gunim.CueError} {
		if s[c] == nil {
			t.Errorf("no sound for %s", c)
		}
	}
}

func TestACueRepeatedAtOnceSoundsOnce(t *testing.T) {
	m := audio.NewMixer()
	p := New(m)
	p.PlayCue(gunim.CueTick, 0)
	p.PlayCue(gunim.CueTick, 0)
	p.PlayCue(gunim.CuePress, 0)
	if n := m.Playing(); n != 2 {
		t.Errorf("%d voices for a tick twice and a press, want 2", n)
	}
}

func TestACueSetToNothingIsSilent(t *testing.T) {
	m := audio.NewMixer()
	p := New(m)
	p.Set(gunim.CuePress, nil)
	p.PlayCue(gunim.CuePress, 0)
	p.SetVolume(0)
	p.PlayCue(gunim.CueOpen, 0)
	if n := m.Playing(); n != 0 {
		t.Errorf("%d voices for a silenced cue and a muted player, want none", n)
	}
}

func TestPanLeansACueToItsSide(t *testing.T) {
	m := audio.NewMixer()
	New(m).PlayCue(gunim.CuePress, -0.6)
	out := make([]float32, 2*1000)
	m.Mix(out)
	var l, r float64
	for i := range 1000 {
		l += math.Abs(float64(out[2*i]))
		r += math.Abs(float64(out[2*i+1]))
	}
	if !(l > 2*r) || r == 0 {
		t.Errorf("a cue at pan -0.6 sums %.2f left and %.2f right, want it leaning left but in both", l, r)
	}
}
