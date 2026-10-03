package main

import (
	"image/color"
	"math"
	"math/rand/v2"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// The kinds of particle.
const (
	// sparkle is a four-pointed glint that twinkles as it fades.
	sparkle = iota
	// dot is a round crumb that falls.
	dot
	// confetti is a paper strip that tumbles as it falls.
	confetti
	// starBit is a small candy star, spinning.
	starBit
	// rocket climbs, trailing sparks, and bursts as its life ends.
	rocket
)

// A particle is one bit of an effect, in window space.
type particle struct {
	kind     int
	pos, vel geom.Point
	// spin is how fast it turns, in radians a second, and angle where.
	angle, spin float32
	size        float32
	color       color.NRGBA
	// age and life are how long it has lived and will.
	age, life float32
	// gravity pulls it down, in pixels a second squared, and drag slows
	// it, a fraction of its speed lost a second.
	gravity, drag float32
	// delay holds it unseen until it is due.
	delay float32
}

// A popText is a word that bursts out and floats up, as "Sweet!".
type popText struct {
	// shout marks a combo's word: a new one takes its place.
	shout bool
	s     string
	at    geom.Point
	size  float32
	color color.NRGBA
	scale *anim.Float
	age   float32
	life  float32
}

// fx is the effects drawn over the whole window: particles and words.
type fx struct {
	parts []particle
	texts []*popText
	rng   *rand.Rand
	// boom hears each rocket burst, for its sound.
	boom func(at geom.Point)
}

func newFX() *fx { return &fx{rng: rand.New(rand.NewPCG(1, 2))} }

func (f *fx) rand(lo, hi float32) float32 { return lo + (hi-lo)*f.rng.Float32() }

// burst throws n particles of kind out from at, in colours cs.
func (f *fx) burst(at geom.Point, n, kind int, speed float32, cs ...color.NRGBA) {
	for range n {
		a := f.rand(0, 2*math.Pi)
		v := speed * f.rand(0.35, 1)
		p := particle{
			kind: kind, pos: at,
			vel:   geom.Pt(v*float32(math.Cos(float64(a))), v*float32(math.Sin(float64(a)))),
			angle: f.rand(0, 2*math.Pi), spin: f.rand(-8, 8),
			size: f.rand(4, 9), color: cs[f.rng.IntN(len(cs))],
			life: f.rand(0.5, 0.9), drag: 2.2, gravity: 500,
		}
		switch kind {
		case sparkle:
			p.gravity, p.size, p.life = 40, f.rand(5, 11), f.rand(0.4, 0.8)
		case confetti:
			p.gravity, p.drag, p.life, p.size = 700, 1.2, f.rand(1.6, 2.6), f.rand(6, 11)
		case starBit:
			p.size, p.life = f.rand(7, 13), f.rand(0.7, 1.1)
		}
		f.parts = append(f.parts, p)
	}
}

// rain lets confetti fall from the top of a box w wide, over a while.
func (f *fx) rain(w float32, n int, cs ...color.NRGBA) {
	for range n {
		f.parts = append(f.parts, particle{
			kind: confetti, pos: geom.Pt(f.rand(0, w), f.rand(-60, -10)),
			vel:   geom.Pt(f.rand(-60, 60), f.rand(60, 220)),
			angle: f.rand(0, 6), spin: f.rand(-9, 9), size: f.rand(7, 12),
			color: cs[f.rng.IntN(len(cs))], life: f.rand(3, 4.5), drag: 0.6, gravity: 160,
			delay: f.rand(0, 1.6),
		})
	}
}

// fireworks sends n rockets up from the bottom of a box of size, one
// after another, to burst in colours cs.
func (f *fx) fireworks(size geom.Size, n int, cs ...color.NRGBA) {
	for i := range n {
		f.parts = append(f.parts, particle{
			kind: rocket, pos: geom.Pt(size.W*f.rand(0.15, 0.85), size.H+10),
			vel:  geom.Pt(f.rand(-60, 60), -size.H*f.rand(0.95, 1.25)),
			size: 4, color: cs[f.rng.IntN(len(cs))],
			life: f.rand(0.7, 0.95), gravity: size.H * 0.9, drag: 0.3,
			delay: 0.3 + 0.45*float32(i) + f.rand(0, 0.2),
		})
	}
}

// say bursts s out at at.
func (f *fx) say(s string, at geom.Point, size float32, c color.NRGBA) {
	f.texts = append(f.texts, newPop(s, at, size, c))
}

// shout bursts a combo's word out at at, taking the last one's place,
// which leaves at once.
func (f *fx) shout(s string, at geom.Point, size float32, c color.NRGBA) {
	for _, t := range f.texts {
		if t.shout {
			t.age = max(t.age, t.life-0.12)
		}
	}
	t := newPop(s, at, size, c)
	t.shout = true
	f.texts = append(f.texts, t)
}

func newPop(s string, at geom.Point, size float32, c color.NRGBA) *popText {
	t := &popText{s: s, at: at, size: size, color: c, scale: anim.NewFloat(0.2), life: 1.3}
	t.scale.Animate(1, anim.Spring{Response: 0.35, Damping: 0.45})
	return t
}

// step moves everything on, and reports whether anything is left.
func (f *fx) step(dt time.Duration) bool {
	s := float32(dt.Seconds())
	// Bursts and trails made while stepping join after.
	old := f.parts
	f.parts = nil
	live := old[:0]
	for _, p := range old {
		if p.delay > 0 {
			p.delay -= s
			live = append(live, p)
			continue
		}
		p.age += s
		if p.age >= p.life {
			if p.kind == rocket {
				// It bursts: a ring of sparks, crumbs and stars.
				c := p.color
				f.burst(p.pos, 26, dot, 380, c, lighter(c, 0.5))
				f.burst(p.pos, 14, sparkle, 300, white, lighter(c, 0.6))
				f.burst(p.pos, 8, starBit, 260, c, gold)
				if f.boom != nil {
					f.boom(p.pos)
				}
			}
			continue
		}
		if p.kind == rocket && f.rng.Float32() < 0.6 {
			// It trails sparks as it climbs.
			f.parts = append(f.parts, particle{
				kind: dot, pos: p.pos, vel: geom.Pt(f.rand(-30, 30), f.rand(20, 60)),
				size: f.rand(2, 4), color: lighter(p.color, 0.5), life: f.rand(0.2, 0.4), gravity: 200, drag: 2,
			})
		}
		p.vel = p.vel.Mul(max(0, 1-p.drag*s))
		p.vel.Y += p.gravity * s
		p.pos = p.pos.Add(p.vel.Mul(s))
		p.angle += p.spin * s
		live = append(live, p)
	}
	clear(old[len(live):])
	f.parts = append(live, f.parts...)
	texts := f.texts[:0]
	for _, t := range f.texts {
		t.age += s
		t.scale.Step(dt)
		if t.age < t.life {
			texts = append(texts, t)
		}
	}
	f.texts = texts
	return len(f.parts) > 0 || len(f.texts) > 0
}

// paint draws the effects.
func (f *fx) paint(p *paint.Painter) {
	for i := range f.parts {
		pt := &f.parts[i]
		if pt.delay > 0 {
			continue
		}
		left := 1 - pt.age/pt.life
		a := min(1, left*2.5)
		s := pt.size
		switch pt.kind {
		case sparkle:
			// Two thin bars crossed, twinkling.
			tw := 0.6 + 0.4*float32(math.Sin(float64(pt.age*30)))
			l := s * tw * (0.6 + 0.4*left)
			func() {
				defer p.Push(paint.Rotate(pt.angle*0.2, pt.pos))()
				c := faded(pt.color, a)
				p.RRect(geom.Rc(pt.pos.X-l, pt.pos.Y-l*0.12, 2*l, l*0.24), l*0.12, paint.Solid(c))
				p.RRect(geom.Rc(pt.pos.X-l*0.12, pt.pos.Y-l, l*0.24, 2*l), l*0.12, paint.Solid(c))
				p.RRect(geom.Rc(pt.pos.X-l*0.25, pt.pos.Y-l*0.25, l*0.5, l*0.5), l*0.25, paint.Solid(faded(rgb(0xff, 0xff, 0xff), a)))
			}()
		case dot:
			r := s * 0.5 * (0.5 + 0.5*left)
			p.RRect(geom.Rc(pt.pos.X-r, pt.pos.Y-r, 2*r, 2*r), r, paint.Solid(faded(pt.color, a)))
		case confetti:
			// A strip seen turning: its width swings as it tumbles.
			w := s * float32(math.Abs(math.Cos(float64(pt.angle*1.3))))
			func() {
				defer p.Push(paint.Rotate(pt.angle, pt.pos))()
				p.RRect(geom.Rc(pt.pos.X-w/2, pt.pos.Y-s*0.3, max(w, 1), s*0.6), 1, paint.Solid(faded(pt.color, a)))
			}()
		case rocket:
			p.RRect(geom.Rc(pt.pos.X-3, pt.pos.Y-3, 6, 6), 3, paint.Solid(lighter(pt.color, 0.6)))
		case starBit:
			r := geom.Rc(pt.pos.X-s, pt.pos.Y-s, 2*s, 2*s)
			func() {
				defer p.Push(paint.Rotate(pt.angle, pt.pos))()
				p.Mask(candyMask{shape: shapeStar}, r, faded(pt.color, a))
			}()
		}
	}
	for _, t := range f.texts {
		run := shaped(t.s, t.size, true)
		left := 1 - t.age/t.life
		a := min(1, left*3)
		rise := t.age * 40
		at := geom.Pt(t.at.X, t.at.Y-rise)
		func() {
			defer p.Push(paint.Scale(t.scale.Value(), at))()
			x := at.X - run.Advance/2
			y := at.Y - t.size/2
			// A thick dark edge, then the word, as candy lettering.
			edge := faded(rgb(0x4a, 0x10, 0x60), a)
			for _, o := range [...]geom.Point{{X: -2, Y: 0}, {X: 2, Y: 0}, {X: 0, Y: -2}, {X: 0, Y: 3}, {X: 2, Y: 3}, {X: -2, Y: 3}} {
				run.Paint(p, geom.Pt(x+o.X, y+o.Y), edge)
			}
			run.Paint(p, geom.Pt(x, y), faded(t.color, a))
			run.Paint(p, geom.Pt(x, y-1), faded(lighter(t.color, 0.5), a*0.5))
		}()
	}
}

// fxNode paints the effects over the whole window.
type fxNode struct{ f *fx }

// Covers implements [gunim.Shaped]: the effects let every tap through.
func (n *fxNode) Covers(geom.Point) bool { return false }

// Step implements [gunim.Animator].
func (n *fxNode) Step(dt time.Duration) bool { return n.f.step(dt) }

// Layout implements [gunim.Node].
func (n *fxNode) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}

// Paint implements [gunim.Node].
func (n *fxNode) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, _ gunim.Children) { n.f.paint(p) }
