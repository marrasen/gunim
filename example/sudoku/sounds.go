package main

import (
	"io"
	"math"
	"time"

	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio"
)

// hz is the sample rate as a float.
const hz = float64(audio.SampleRate)

// sfx plays the game's sounds, made in code: each digit is a note, so
// filling the board plays tunes. It is used from the UI goroutine.
type sfx struct {
	m     *audio.Mixer
	music *audio.Voice
	muted bool
	cache map[string]*audio.Clip
}

// musicVolume is how loud the music plays under the sounds.
const musicVolume = 0.32

func newSFX(m *audio.Mixer) *sfx {
	s := &sfx{m: m, cache: map[string]*audio.Clip{}}
	if m != nil {
		s.music = m.Play(&tune{}, audio.Options{Volume: musicVolume, FadeIn: 2 * time.Second, Loop: true})
	}
	return s
}

// setMuted silences everything, or brings it back; the music fades.
func (s *sfx) setMuted(on bool) {
	s.muted = on
	if s.music == nil {
		return
	}
	to := float32(musicVolume)
	if on {
		to = 0
	}
	s.music.SetVolume(to, anim.Spring{Response: 0.6, Damping: 1})
}

// play plays the clip named key, made by mk the first time, at vol and
// pan, after delay.
func (s *sfx) play(key string, mk func() *audio.Clip, vol, pan float32) {
	if s.m == nil || s.muted {
		return
	}
	c := s.cache[key]
	if c == nil {
		c = mk()
		s.cache[key] = c
	}
	s.m.Play(c.Source(), audio.Options{Volume: vol, Pan: max(-1, min(1, pan))})
}

// notes are the digits' pitches: C major's pentatonic scale over two
// octaves, so any digits together sound well.
var notes = [9]float64{523.25, 587.33, 659.26, 783.99, 880, 1046.5, 1174.66, 1318.51, 1567.98}

func noteOf(d int8) float64 { return notes[max(1, min(d, 9))-1] }

// placed plays digit d's note as its candy lands, and a run up the
// scale for a combo.
func (s *sfx) placed(d int8, combo int, pan float32) {
	s.play("pop", popClip, 0.4, pan)
	s.play("note"+string(rune('0'+d)), func() *audio.Clip { return render(0.9, func(t float64) float64 { return 0.28 * bar(t, noteOf(d)) }) }, 0.65, pan)
	if combo >= 2 {
		n := min(combo, 6)
		key := "combo" + string(rune('0'+n)) + string(rune('0'+d))
		s.play(key, func() *audio.Clip {
			return render(0.12*float64(n)+0.8, func(t float64) float64 {
				v := 0.0
				for i := range n {
					// Up the scale from the digit placed.
					idx := (int(d)-1+2+i*2)%9 + 1
					f := noteOf(int8(idx)) * 2
					v += bell(t-0.12-0.07*float64(i), f)
				}
				return 0.12 * v
			})
		}, 0.55, pan)
	}
}

// wrong plays a wooden bonk that sags, and a crack, as a heart breaks.
func (s *sfx) wrong(pan float32) {
	s.play("bonk", func() *audio.Clip {
		return render(0.7, func(t float64) float64 {
			// The pitch falls as the bonk wobbles.
			f := 210 * (1 - 0.25*min(t/0.4, 1))
			ph := 2 * math.Pi * f * t
			body := (math.Sin(ph) + 0.4*math.Sin(2.01*ph)) * math.Exp(-t/0.16) * (1 + 0.3*math.Sin(2*math.Pi*9*t))
			crack := noise(int64(t*hz)) * math.Exp(-math.Max(t-0.18, 0)/0.03) * step(t-0.18)
			return 0.3*body + 0.12*crack
		})
	}, 0.9, pan)
}

// done plays a bell's run up the scale, sweeping across with the light.
func (s *sfx) done(unit int) {
	pan := float32(0)
	key := "done-box"
	switch {
	case unit < 9:
		key = "done-row"
	case unit < 18:
		key = "done-col"
	}
	s.play(key, func() *audio.Clip {
		return renderStereo(1.6, func(t float64) (float64, float64) {
			l, r := 0.0, 0.0
			for i, f := range []float64{1046.5, 1318.51, 1567.98, 2093} {
				at := 0.08 * float64(i)
				v := bell(t-at, f)
				// The run sweeps left to right, as the light does.
				p := -0.7 + 0.47*float64(i)
				if key != "done-row" {
					p = 0
				}
				l += v * (1 - p) / 2
				r += v * (1 + p) / 2
			}
			return 0.16 * l, 0.16 * r
		})
	}, 0.8, pan)
}

// digitDone plays a pop and a shimmer as a digit is used up.
func (s *sfx) digitDone(d int8) {
	s.play("shimmer"+string(rune('0'+d)), func() *audio.Clip {
		return render(1.2, func(t float64) float64 {
			v := 0.0
			for i := range 6 {
				v += bell(t-0.05*float64(i), noteOf(int8((int(d)+i*2)%9+1))*2)
			}
			return 0.07 * v
		})
	}, 0.8, 0)
}

// hint plays a magic rising shimmer.
func (s *sfx) hint(pan float32) {
	s.play("hint", func() *audio.Clip {
		return render(1.1, func(t float64) float64 {
			v := 0.0
			for i := range 10 {
				at := 0.05 * float64(i)
				f := 1046.5 * math.Pow(2, float64(i)/7)
				v += bell(t-at, f) * (1 - float64(i)/14)
			}
			return 0.06 * v * (1 + 0.3*math.Sin(2*math.Pi*12*t))
		})
	}, 0.9, pan)
}

// firework plays a soft boom and a crackle.
func (s *sfx) firework(pan float32) {
	s.play("boom", func() *audio.Clip {
		return render(1.0, func(t float64) float64 {
			boom := math.Sin(2*math.Pi*(55*t+80*0.05*(1-math.Exp(-t/0.05)))) * math.Exp(-t/0.25)
			n := int64(t * hz)
			crackle := 0.0
			// Pops scattered after the boom.
			if t > 0.15 && noise(n/400) > 0.55 {
				crackle = noise(n) * math.Exp(-math.Mod(t, 0.0083)/0.002)
			}
			return 0.3*boom + 0.06*crackle*math.Exp(-(t-0.15)/0.4)
		})
	}, 0.7, pan)
}

// tick plays a soft tick as a cell is picked.
func (s *sfx) tick(pan float32) {
	s.play("tick", func() *audio.Clip {
		return render(0.05, func(t float64) float64 {
			return 0.12 * math.Sin(2*math.Pi*2400*t) * math.Exp(-t/0.008)
		})
	}, 0.6, pan)
}

// star plays a bell for a star landing on the card, higher for each.
func (s *sfx) star(i int) {
	s.play("star"+string(rune('0'+i)), func() *audio.Clip {
		f := []float64{1046.5, 1318.51, 1567.98}[min(i, 2)]
		return render(1.4, func(t float64) float64 { return 0.22*bell(t, f) + 0.1*bell(t, f*2) })
	}, 0.9, float32(i-1)*0.5)
}

// won plays a fanfare: a run up the chord, and the chord rung together.
func (s *sfx) won() {
	s.play("won", func() *audio.Clip {
		return render(2.6, func(t float64) float64 {
			v := 0.0
			for i, f := range []float64{523.25, 659.26, 783.99, 1046.5} {
				v += bar(t-0.11*float64(i), f)
			}
			for _, f := range []float64{523.25, 659.26, 783.99, 1046.5, 1318.51} {
				v += 0.7 * bell(t-0.55, f)
			}
			return 0.13 * v
		})
	}, 0.9, 0)
}

// lost plays a sad slide down, as a muted trombone would: three short
// notes and a long one.
func (s *sfx) lost() {
	s.play("lost", func() *audio.Clip {
		return render(2.4, func(t float64) float64 {
			v := 0.0
			for i, f := range []float64{392, 370, 349.23, 329.63} {
				at := 0.42 * float64(i)
				long := 0.38
				if i == 3 {
					long = 1.1
				}
				u := t - at
				if u < 0 || u > long+0.2 {
					continue
				}
				// A muted brass: odd harmonics, with a slow wah.
				ff := f * (1 - 0.03*min(u/long, 1))
				ph := 2 * math.Pi * ff * u
				tone := math.Sin(ph) + 0.5*math.Sin(3*ph) + 0.25*math.Sin(5*ph)
				wah := 0.6 + 0.4*math.Sin(2*math.Pi*1.6*u)
				env := min(u/0.04, 1) * math.Exp(-math.Max(u-long, 0)/0.06)
				v += tone * wah * env
			}
			return 0.09 * v
		})
	}, 0.9, 0)
}

// render returns a clip d seconds long, each sample f of its time, the
// same in both speakers.
func render(d float64, f func(t float64) float64) *audio.Clip {
	return renderStereo(d, func(t float64) (float64, float64) {
		v := f(t)
		return v, v
	})
}

func renderStereo(d float64, f func(t float64) (float64, float64)) *audio.Clip {
	n := int(d * hz)
	s := make([]float32, 2*n)
	for i := range n {
		l, r := f(float64(i) / hz)
		// The last 10 ms fade, so nothing clicks off.
		fade := min(1, float64(n-i)/(0.01*hz))
		s[2*i], s[2*i+1] = float32(l*fade), float32(r*fade)
	}
	return audio.NewClip(s)
}

// popClip is a soft pop: a quick drop in pitch.
func popClip() *audio.Clip {
	return render(0.08, func(t float64) float64 {
		ph := 2 * math.Pi * (180*t + 600*0.01*(1-math.Exp(-t/0.01)))
		return 0.35 * math.Sin(ph) * math.Exp(-t/0.025)
	})
}

// bar is a struck wooden bar, as a marimba's, t seconds after it is
// struck: a sine with the bar's fourth partial dying sooner.
func bar(t, f float64) float64 {
	if t < 0 {
		return 0
	}
	w := 2 * math.Pi * f * t
	att := min(t/0.003, 1)
	return att * (math.Sin(w)*math.Exp(-t/0.35) + 0.35*math.Sin(3.93*w)*math.Exp(-t/0.05))
}

// bell is a small bell: a sine with a bright, inharmonic partial.
func bell(t, f float64) float64 {
	if t < 0 {
		return 0
	}
	w := 2 * math.Pi * f * t
	att := min(t/0.002, 1)
	return att * (math.Sin(w)*math.Exp(-t/0.45) + 0.3*math.Sin(2.76*w)*math.Exp(-t/0.12))
}

func step(x float64) float64 {
	if x < 0 {
		return 0
	}
	return 1
}

// noise is white noise for frame n, the same each time.
func noise(n int64) float64 {
	x := uint64(n) * 0x9e3779b97f4a7c15
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return float64(x>>11)/float64(1<<53)*2 - 1
}

// tune is the music: eight bars of marimba over a soft pad and a
// shaker, C Am F G, that loop without a seam. Each sample is a function
// of its time, so it costs nothing to hold.
type tune struct{ at int64 }

// tuneBPM is the music's tempo, and tuneBars its length.
const (
	tuneBPM  = 96
	tuneBars = 8
)

func tuneFrames() int64 { return int64(tuneBars * 4 * 60 / tuneBPM * hz) }

// tuneChords are the chords, a bar each, as semitones above C4.
var tuneChords = [4][3]int{{0, 4, 7}, {-3, 0, 4}, {-7, -3, 0}, {-5, -1, 2}}

// tuneMelody is the marimba's eighth notes over two bars, as steps of
// the chord, -1 for a rest.
var tuneMelody = [16]int{0, 1, 2, 4, 3, 2, 1, -1, 2, 3, 4, 5, 4, -1, 2, 1}

func (tn *tune) sample(t float64, n int64) (l, r float64) {
	spb := 60.0 / tuneBPM
	beat := t / spb
	bars := int(beat/4) % tuneBars
	chord := tuneChords[bars%4]
	c4 := 261.63
	note := func(semi, octave int) float64 { return c4 * math.Pow(2, float64(semi)/12+float64(octave)) }
	// The pad, swelling each bar.
	sw := 0.6 + 0.4*math.Sin(math.Pi*math.Mod(beat, 4)/4)
	for i, s := range chord {
		f := note(s, 0)
		a, b := math.Sin(2*math.Pi*f*1.003*t), math.Sin(2*math.Pi*f*0.997*t)
		l += 0.035 * sw * (a + 0.5*b) * (1 - 0.1*float64(i))
		r += 0.035 * sw * (b + 0.5*a) * (0.8 + 0.1*float64(i))
	}
	// The bass, on beats one and three.
	half := math.Mod(beat, 2)
	bf := note(chord[0], -2)
	bass := math.Sin(2*math.Pi*bf*half*spb) * math.Exp(-half*spb/0.5) * min(half*spb/0.01, 1)
	l += 0.12 * bass
	r += 0.12 * bass
	// The marimba's tune.
	pos := math.Mod(beat*2, 16)
	i := int(pos)
	if st := tuneMelody[i]; st >= 0 {
		semi := chord[st%3] + 12*(st/3)
		v := bar(math.Mod(pos, 1)*spb/2, note(semi, 1))
		pan := 0.15 * math.Sin(float64(i))
		l += 0.11 * v * (1 - pan)
		r += 0.11 * v * (1 + pan)
	}
	// A shaker on the off eighths.
	if e := math.Mod(beat*2, 1) * spb / 2; int(beat*2)%2 == 1 {
		sh := (noise(n) - noise(n-1)) * math.Exp(-e/0.03)
		l += 0.02 * sh
		r += 0.03 * sh
	}
	return l, r
}

// Read implements [audio.Source].
func (tn *tune) Read(dst []float32) (int, error) {
	total := tuneFrames()
	k := 0
	for k < len(dst)/2 && tn.at < total {
		l, r := tn.sample(float64(tn.at)/hz, tn.at)
		dst[2*k], dst[2*k+1] = float32(l), float32(r)
		k++
		tn.at++
	}
	if tn.at >= total {
		return k, io.EOF
	}
	return k, nil
}

// SeekFrame implements [audio.Seeker].
func (tn *tune) SeekFrame(f int64) error {
	tn.at = max(0, min(f, tuneFrames()))
	return nil
}

// Len implements [audio.Seeker].
func (tn *tune) Len() int64 { return tuneFrames() }

// The tune loops, which takes a Seeker.
var _ audio.Seeker = (*tune)(nil)
