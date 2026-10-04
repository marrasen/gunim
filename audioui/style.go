package audioui

import (
	"image/color"
	"math"

	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
)

// The colours the pieces draw in.
var (
	// Ink is the colour of text and of guides, drawn faint.
	Ink = theme.Foreground("audio.ink", rgb(0xec, 0xee, 0xf4))
	// Ground is the colour behind a meter, a scope or a spectrum.
	Ground = theme.Color("audio.ground", rgb(0x0c, 0x0e, 0x13))
	// Raised is the colour of a control's cap, such as a fader's.
	Raised = theme.Color("audio.raised", rgb(0x1d, 0x21, 0x2b))
	// Sound is the colour of what plays: a waveform, a spectrum, a
	// reading on its mark.
	Sound = theme.Color("audio.sound", rgb(0x4f, 0xd6, 0xc0))
	// Near is the colour of a reading near its mark, and Over of one
	// past it.
	Near = theme.Color("audio.near", rgb(0xff, 0xc8, 0x57))
	Over = theme.Color("audio.over", rgb(0xff, 0x6b, 0x5f))
	// Spread is the colour of a range, such as the loudness range.
	Spread = theme.Color("audio.spread", rgb(0x5c, 0xb8, 0xff))
	// Short is the colour of the short-term loudness's curve.
	Short = theme.Color("audio.short", rgb(0xff, 0x6a, 0xd5))
)

func rgb(r, g, b uint8) color.NRGBA { return color.NRGBA{R: r, G: g, B: b, A: 0xff} }

// Faded is c at alpha a, from 0 to 1, of its own.
func Faded(c color.NRGBA, a float32) color.NRGBA {
	c.A = uint8(float32(c.A) * min(max(a, 0), 1))
	return c
}

// Mix blends a toward b by t.
func Mix(a, b color.NRGBA, t float32) color.NRGBA { return anim.Mix(anim.ColorCodec, a, b, t) }

// Segment draws a straight line from a to b, width wide, with round
// ends.
func Segment(p *paint.Painter, a, b geom.Point, width float32, c color.NRGBA) {
	d := b.Sub(a)
	l := float32(math.Hypot(float64(d.X), float64(d.Y)))
	if l < 0.01 {
		return
	}
	end := p.Push(paint.Rotate(float32(math.Atan2(float64(d.Y), float64(d.X))), a))
	p.RRect(geom.Rc(a.X-width/2, a.Y-width/2, l+width, width), width/2, paint.Solid(c))
	end()
}

// runs holds text shaped already, for pieces that paint every frame.
// It is reached from the UI goroutine alone.
var runs = map[runKey]text.Run{}

type runKey struct {
	s    string
	size float32
	bold bool
	mono bool
}

// Shaped is s shaped at size, kept for the next frame: in the
// monospaced face where mono says, for figures that change, so they
// hold still. Call it from the UI goroutine alone.
func Shaped(s string, size float32, bold, mono bool) text.Run {
	k := runKey{s, size, bold, mono}
	if r, ok := runs[k]; ok {
		return r
	}
	if len(runs) > 4000 {
		clear(runs)
	}
	face := text.GoSans(bold, false)
	if mono {
		face = text.GoMono(bold, false)
	}
	r := face.Shape(s, size)
	runs[k] = r
	return r
}

// DB is a level, 1 at full scale, in decibels, -180 for silence.
func DB(v float64) float64 { return 20 * math.Log10(max(v, 1e-9)) }
