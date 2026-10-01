package main

import (
	_ "embed"
	"image/color"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// Lesson 4: a node of your own.
//
// Node is Layout and Paint. Three more interfaces are opt-in: Handler
// for input, Animator for animated values, and Transitioner for
// entering and leaving. This lesson's ball is a Handler and an
// Animator: a click sends it there on a spring, and the engine keeps
// drawing frames while the spring is moving.
//
// Everything here stays in the window. The application hears nothing,
// which is right for motion that is the interface's own.

//go:embed lesson4.go
var lesson4Source string

var lesson4 = lesson{
	Title:  lessonTitles[3],
	File:   lessonFile(3),
	Source: lesson4Source,
	View:   "lesson4",
	Register: func(w *gunim.Window) {
		gunim.RegisterView(w, "lesson4", buildLesson4, nil)
	},
	State: func(*app) any { return struct{}{} },
}

const lesson4Intro = `A node is two methods, ` + "`Layout`" + ` and ` + "`Paint`" + `. Three more interfaces are opt-in. A **Handler** gets input aimed at it, with pointer positions already in its own space. An **Animator** owns animated values: its ` + "`Step`" + ` reports whether anything is still moving, and the window draws frames while any node says yes, then sleeps. A **Transitioner** animates as it enters and leaves, as the page you are reading does.

The ball below is a Handler and an Animator. A click sets the spring's target, and ` + "`anim.Group`" + `, embedded, steps the spring each frame. Animation runs on wall-clock time, so the motion looks the same at 60 Hz and 144 Hz.

Each click also sends a ripple out from where it landed and gives the ball its next colour. The colour is an ` + "`anim.Color`" + `, and it blends through Oklab, so it keeps its brightness on the way.

Click around the field. Click again before the ball arrives: the spring retargets and keeps the speed it had.`

// ballField is a field a ball springs about in. It is a leaf: it
// handles input itself and steps its own spring.
type ballField struct {
	// Group steps the values added to it, and reports whether any is
	// still moving. Embedding it makes the field an Animator.
	anim.Group
	// at is where the ball is, and where it is heading, and tint its
	// colour, blending to the next on each click.
	at   *anim.Point
	tint *anim.Color
	// ripple runs from 0 to 1 as a ring spreads from where the last
	// click landed, at from.
	ripple *anim.Float
	from   geom.Point
	clicks int
	// laid says at has a place: the first layout puts the ball in the
	// middle without animating.
	laid bool
}

// fieldHeight is how tall the field is, and ballSize the ball.
var (
	fieldHeight = theme.Length("tutorial.field.height", 220)
	ballSize    = theme.Length("tutorial.ball.size", 36)
	fieldFill   = theme.Color("tutorial.field.fill", color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0x30})
)

// rippleReach is how far a ripple spreads.
const rippleReach = 90

func newBallField() *ballField {
	b := &ballField{at: anim.NewPoint(geom.Point{}), tint: anim.NewColor(hueColors[0]), ripple: anim.NewFloat(1)}
	b.Add(b.at, b.tint, b.ripple)
	return b
}

// Layout implements [gunim.Node]: the field takes the width it is
// given and the theme's height.
func (b *ballField) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	size := c.Constrain(geom.Sz(c.Max.W, fieldHeight.Get(f.Theme)))
	if !b.laid {
		b.at.Jump(geom.Pt(size.W/2, size.H/2))
		b.laid = true
	}
	return size
}

// Paint implements [gunim.Node]. The painter's origin is the field's
// top left, and box the size Layout settled on.
func (b *ballField) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	p.RRect(geom.Rect{Max: box.Point()}, widget.CardRadius.Get(th), paint.Solid(fieldFill.Get(th)))
	tint := b.tint.Value()
	// The ripple: a ring that grows from the click and thins away.
	if rp := b.ripple.Value(); rp < 1 {
		rr := 12 + rippleReach*rp
		ring := geom.Rc(b.from.X-rr, b.from.Y-rr, 2*rr, 2*rr)
		p.RRectStroke(ring, rr, paint.Fill{}, paint.Stroke{Width: 3 * (1 - rp), Color: tinted(tint, 1-rp)})
	}
	r := ballSize.Get(th) / 2
	at := b.at.Value()
	ball := geom.Rc(at.X-r, at.Y-r, 2*r, 2*r)
	p.ShadowRRect(ball, r, paint.Solid(tint), paint.Shadow{
		Offset: geom.Pt(0, 4),
		Blur:   12,
		Color:  tinted(tint, 0.5),
	})
}

// Handle implements [gunim.Handler]: a press sends the ball there.
// Returning true keeps the event; false would offer it to the parent.
func (b *ballField) Handle(e input.Event, f *gunim.UI) bool {
	if e, ok := e.(input.PointerDown); ok && e.Button == input.ButtonPrimary {
		th := f.Theme()
		b.at.Animate(e.Pos, widget.Bounce.Get(th))
		b.clicks++
		b.tint.Animate(hueColors[b.clicks%len(hueColors)], widget.Settle.Get(th))
		b.from = e.Pos
		b.ripple.Jump(0)
		b.ripple.Animate(1, anim.Tween{Duration: 600 * time.Millisecond, Ease: anim.EaseOut})
		return true
	}
	return false
}

func buildLesson4(struct{}) *page {
	return newPage(3, lesson4Intro, newBallField(), lesson4Source)
}
