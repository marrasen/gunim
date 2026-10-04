package main

import (
	"math"
	"testing"

	"github.com/marrasen/gunim/audio"
)

func TestEachSoundIsLoudEnoughAndStaysUnderClipping(t *testing.T) {
	m := audio.NewMixer()
	s := newSFX(nil, nil)
	s.m = m
	plays := map[string]func(){
		"placed":    func() { s.placed(5, 1, 0) },
		"combo":     func() { s.placed(9, 6, 0) },
		"wrong":     func() { s.wrong(0) },
		"done":      func() { s.done(3) },
		"digitDone": func() { s.digitDone(9) },
		"hint":      func() { s.hint(0) },
		"star":      func() { s.star(2) },
		"won":       func() { s.won() },
		"lost":      func() { s.lost() },
	}
	for name, play := range plays {
		play()
		out := make([]float32, 2*3*audio.SampleRate)
		m.Mix(out)
		peak := 0.0
		for _, v := range out {
			peak = max(peak, math.Abs(float64(v)))
		}
		if peak < 0.05 || peak > 0.9 {
			t.Errorf("%s peaks at %.2f, want it heard and under 0.9", name, peak)
		}
	}
}

func TestTheTuneLoopsWithoutASeam(t *testing.T) {
	tn := &tune{}
	n := tuneFrames()
	l0, r0 := tn.sample(0, 0)
	l1, r1 := tn.sample(float64(n-1)/hz, n-1)
	if math.Abs(l1-l0) > 0.05 || math.Abs(r1-r0) > 0.05 {
		t.Errorf("the tune's last frame %.3f %.3f and first %.3f %.3f jump", l1, r1, l0, r0)
	}
	peak := 0.0
	buf := make([]float32, 2*4096)
	for {
		k, err := tn.Read(buf)
		for _, v := range buf[:2*k] {
			peak = max(peak, math.Abs(float64(v)))
		}
		if err != nil {
			break
		}
	}
	if peak > 0.9 || peak < 0.1 {
		t.Errorf("the tune peaks at %.2f", peak)
	}
}

func TestEachSoundSettingPlaysItsPart(t *testing.T) {
	m := audio.NewMixer()
	s := newSFX(m, nil)
	voices := func() int { return m.Playing() - 1 } // the music is one
	for _, c := range []struct {
		mode    Sound
		effects bool
		music   float32
	}{
		{SoundAll, true, musicVolume},
		{SoundMusic, false, musicVolume},
		{SoundEffects, true, 0},
		{SoundOff, false, 0},
	} {
		s.setMode(c.mode)
		before := voices()
		s.tick(0)
		if got := voices() > before; got != c.effects {
			t.Errorf("mode %v: a tick played %v, want %v", c.mode, got, c.effects)
		}
		if v := s.music.Volume(); v != c.music {
			t.Errorf("mode %v: the music heads for %v, want %v", c.mode, v, c.music)
		}
		s.cache = map[string]*audio.Clip{}
		m.Mix(make([]float32, 2*audio.SampleRate/2))
	}
}

func TestAwayEverythingStops(t *testing.T) {
	m := audio.NewMixer()
	s := newSFX(m, nil)
	s.setAway(true)
	m.Mix(make([]float32, 2*4800))
	if !s.music.Paused() {
		t.Fatal("away, the music plays on")
	}
	before := m.Playing()
	s.tick(0)
	if m.Playing() != before {
		t.Fatal("away, a sound played")
	}
	s.setAway(false)
	if s.music.Paused() {
		t.Fatal("back, the music stays paused")
	}
}
