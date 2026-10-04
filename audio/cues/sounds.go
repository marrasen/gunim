package cues

import (
	"math"
	"math/rand/v2"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio"
)

// Sounds returns the sounds a new [Player] plays: soft taps and plucks,
// a little lower or higher as a thing turns off or on, opens or
// closes. Each call makes them anew, for a player to change as it
// likes.
func Sounds() map[gunim.Cue]*audio.Clip {
	return map[gunim.Cue]*audio.Clip{
		gunim.CuePress:     tap(),
		gunim.CueToggleOn:  notes(0.16, 1319, 1760),
		gunim.CueToggleOff: notes(0.14, 1175, 880),
		gunim.CueSelect:    notes(0.16, 1568),
		gunim.CueTick:      tick(),
		gunim.CueOpen:      glide(0.11, 620, 1150),
		gunim.CueClose:     glide(0.09, 1050, 560),
		gunim.CueError:     buzz(),

		gunim.CueConnected:    notes(0.13, 784, 1175),
		gunim.CueDisconnected: notes(0.13, 1175, 784),
		gunim.CueDone:         notes(0.11, 1047, 1319, 1568),
		gunim.CueFailed:       notes(0.12, 988, 831, 659),
		gunim.CueBell:         bell(),
	}
}

// bell is a small bell struck once: a few partials at a bell's
// unharmonic ratios, the higher ones dying sooner, and a soft strike.
func bell() *audio.Clip {
	const hz = 1320
	partials := [...]struct{ ratio, gain, tau float64 }{
		{1, 1, 0.11}, {2.0, 0.45, 0.07}, {2.76, 0.3, 0.05}, {5.4, 0.12, 0.025},
	}
	return render(0.38, func(t float64) float64 {
		v := 0.0
		for _, p := range partials {
			v += p.gain * math.Sin(2*math.Pi*hz*p.ratio*t) * decay(t, 0.0015, p.tau)
		}
		// Faded out over its last 60 ms, so it ends in silence.
		if t > 0.32 {
			v *= (0.38 - t) / 0.06
		}
		return 0.09 * v
	})
}

// rate is the sample rate as a float.
const rate = float64(audio.SampleRate)

// render returns a clip d seconds long, each sample f of its time in
// seconds, the same in both speakers.
func render(d float64, f func(t float64) float64) *audio.Clip {
	n := int(d * rate)
	s := make([]float32, 2*n)
	for i := range n {
		v := float32(f(float64(i) / rate))
		s[2*i], s[2*i+1] = v, v
	}
	return audio.NewClip(s)
}

// decay is an envelope rising over attack seconds and falling with
// time constant tau.
func decay(t, attack, tau float64) float64 {
	if t < attack {
		return t / attack
	}
	return math.Exp(-(t - attack) / tau)
}

// pluck is a note of pitch hz at time t into it: a sine with a bright
// overtone that dies sooner, as a struck bar sounds.
func pluck(t, hz float64) float64 {
	if t < 0 {
		return 0
	}
	body := math.Sin(2*math.Pi*hz*t) * decay(t, 0.002, 0.045)
	shine := 0.3 * math.Sin(2*math.Pi*4*hz*t) * decay(t, 0.001, 0.012)
	return body + shine
}

// tap is a soft tap, a short drop in pitch over a breath of noise.
func tap() *audio.Clip {
	r := rand.New(rand.NewPCG(1, 2))
	noise := 0.0
	return render(0.05, func(t float64) float64 {
		// The pitch falls from 1.9 to 1.2 kHz; the phase is its integral.
		phase := 2 * math.Pi * (1200*t + 700*0.006*(1-math.Exp(-t/0.006)))
		tone := math.Sin(phase) * decay(t, 0.001, 0.009)
		// Noise through a gentle low-pass, only at the start.
		noise += 0.3 * (r.Float64()*2 - 1 - noise)
		return 0.17*tone + 0.05*noise*decay(t, 0.0005, 0.002)
	})
}

// notes is a pluck at each pitch, 45 ms apart, at gain.
func notes(gain float64, hz ...float64) *audio.Clip {
	const apart = 0.045
	return render(apart*float64(len(hz)-1)+0.2, func(t float64) float64 {
		v := 0.0
		for i, h := range hz {
			v += pluck(t-apart*float64(i), h)
		}
		return gain * v
	})
}

// tick is the shortest, quietest click, for steps.
func tick() *audio.Clip {
	return render(0.012, func(t float64) float64 {
		return 0.1 * math.Sin(2*math.Pi*3000*t) * decay(t, 0.0003, 0.0018)
	})
}

// glide is a soft swell gliding from one pitch to another over 80 ms,
// at gain.
func glide(gain, from, to float64) *audio.Clip {
	const d = 0.08
	phase := 0.0
	last := 0.0
	return render(d, func(t float64) float64 {
		hz := from + (to-from)*smooth(t/d)
		phase += 2 * math.Pi * hz * (t - last)
		last = t
		// A raised-cosine swell, so it starts and ends in silence.
		env := 0.5 - 0.5*math.Cos(2*math.Pi*t/d)
		return gain * env * (math.Sin(phase) + 0.15*math.Sin(2*phase))
	})
}

// smooth eases 0 to 1 in and out.
func smooth(x float64) float64 { return x * x * (3 - 2*x) }

// buzz is two low, falling notes with an edge, for a refusal.
func buzz() *audio.Clip {
	note := func(t, hz float64) float64 {
		if t < 0 {
			return 0
		}
		w := 2 * math.Pi * hz * t
		// A sine with its odd overtones, softer than a square.
		v := math.Sin(w) + math.Sin(3*w)/6 + math.Sin(5*w)/15
		return v * decay(t, 0.004, 0.06)
	}
	return render(0.32, func(t float64) float64 {
		return 0.14 * (note(t, 330) + note(t-0.1, 262))
	})
}
