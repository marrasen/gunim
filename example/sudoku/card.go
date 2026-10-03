package main

import (
	"fmt"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// card ends a game: it springs up over the board with the stars won,
// or with a heart broken, and a button to go on.
type card struct {
	anim.Group
	root *gameRoot
	// in brings it in, and stars fills its stars one after another.
	in      *anim.Float
	starsIn [3]*anim.Float
	waits   [3]float32
	// wait holds the card back while the board celebrates.
	wait     float32
	shown    bool
	win      bool
	stars    int
	score    int
	press    *anim.Float
	pressing bool
	size     geom.Size
}

func newCard(r *gameRoot) *card {
	c := &card{root: r, in: anim.NewFloat(0), press: anim.NewFloat(0)}
	c.Add(c.in, c.press)
	for i := range c.starsIn {
		c.starsIn[i] = anim.NewFloat(0)
		c.Add(c.starsIn[i])
	}
	return c
}

func (c *card) won(stars, score int) {
	c.shown, c.win, c.stars, c.score = true, true, stars, score
	c.in.Jump(0)
	// The board's celebration and the first fireworks show first.
	c.wait = 1.4
	for i := range c.starsIn {
		c.starsIn[i].Jump(0)
		c.waits[i] = c.wait + 0.6 + 0.35*float32(i)
	}
}

func (c *card) lost() {
	c.shown, c.win = true, false
	c.in.Jump(0)
	c.in.Animate(1, anim.Spring{Response: 0.5, Damping: 0.7})
}

func (c *card) hide() {
	c.shown, c.wait = false, 0
	c.in.Animate(0, anim.Spring{Response: 0.3, Damping: 1})
}

// Step implements [gunim.Animator]: the stars land in turn, each with a
// burst.
func (c *card) Step(dt time.Duration) bool {
	moving := false
	if c.wait > 0 {
		moving = true
		c.wait -= float32(dt.Seconds())
		if c.wait <= 0 && c.shown {
			c.in.Animate(1, anim.Spring{Response: 0.5, Damping: 0.62})
		}
	}
	if c.shown && c.win {
		for i := range c.waits {
			if c.waits[i] <= 0 {
				continue
			}
			moving = true
			c.waits[i] -= float32(dt.Seconds())
			if c.waits[i] <= 0 && i < c.stars {
				c.starsIn[i].Animate(1, anim.Spring{Response: 0.4, Damping: 0.4})
				c.root.fx.burst(c.starCenter(i), 24, starBit, 380, gold, white)
				c.root.sfx.star(i)
				c.root.fx.burst(c.starCenter(i), 10, sparkle, 260, white, gold)
			}
		}
	}
	return c.Group.Step(dt) || moving
}

// panel returns where the card lies in the window.
func (c *card) panel() geom.Rect {
	w := min(c.size.W-40, 360)
	h := float32(300)
	return geom.Rc((c.size.W-w)/2, (c.size.H-h)/2, w, h)
}

func (c *card) starCenter(i int) geom.Point {
	pr := c.panel()
	return geom.Pt(pr.Min.X+pr.Size().W/2+float32(i-1)*84, pr.Min.Y+120-map[bool]float32{true: 14, false: 0}[i == 1])
}

func (c *card) button() geom.Rect {
	pr := c.panel()
	return geom.Rc(pr.Min.X+40, pr.Max.Y-78, pr.Size().W-80, 54)
}

// Covers implements [gunim.Shaped]: an open card takes every tap, so
// the board under it stays still, and a hidden one none.
func (c *card) Covers(geom.Point) bool { return c.shown }

// Handle implements [gunim.Handler].
func (c *card) Handle(e input.Event, u *gunim.UI) bool {
	if !c.shown {
		return false
	}
	switch e := e.(type) {
	case input.PointerDown:
		if c.button().Contains(e.Pos) {
			c.pressing = true
			c.press.Animate(1, anim.Spring{Response: 0.1, Damping: 1})
		}
	case input.PointerUp:
		if !c.pressing {
			return true
		}
		c.pressing = false
		c.press.Animate(0, anim.Spring{Response: 0.4, Damping: 0.35})
		if c.button().Contains(e.Pos) {
			c.root.sfx.tick(0)
			if c.win {
				// Back to the map, where the next level opens.
				u.Send(c.root, ShowMap{})
			} else {
				u.Send(c.root, Start{Level: c.root.state.Level})
			}
		}
	}
	u.Invalidate()
	return true
}

// Layout implements [gunim.Node].
func (c *card) Layout(cs gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	c.size = cs.Max
	return cs.Max
}

// Paint implements [gunim.Node].
func (c *card) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	in := c.in.Value()
	if in <= 0.01 {
		return
	}
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(faded(rgb(0x1a, 0x05, 0x30), 0.5*min(in, 1))))
	pr := c.panel()
	mid := geom.Pt(pr.Min.X+pr.Size().W/2, pr.Min.Y+pr.Size().H/2)
	defer p.Push(paint.Scale(0.6+0.4*in, mid))()
	p.ShadowRRect(pr, 28, paint.Fill{Gradient: &paint.Gradient{
		From: pr.Min, To: geom.Pt(pr.Min.X, pr.Max.Y),
		Start: rgb(0xff, 0xf0, 0xfa), End: rgb(0xff, 0xc6, 0xe8),
	}}, paint.Shadow{Blur: 40, Offset: geom.Pt(0, 16), Color: faded(rgb(0x20, 0, 0x40), 0.6)})
	p.RRectStroke(pr, 28, paint.Solid(white), paint.Stroke{Width: 3})
	title, label := "Sweet victory!", "Continue"
	if !c.win {
		title, label = "Out of hearts", "Try again"
	}
	t := shaped(title, 28, true)
	t.Paint(p, geom.Pt(mid.X-t.Advance/2, pr.Min.Y+26), plum)
	if c.win {
		for i := range 3 {
			at := c.starCenter(i)
			s := float32(34)
			if i == 1 {
				s = 42
			}
			p.Mask(candyMask{shape: shapeStar}, geom.Rc(at.X-s, at.Y-s, 2*s, 2*s), faded(plum, 0.2))
			if v := c.starsIn[i].Value(); v > 0.01 {
				func() {
					defer p.Push(paint.Scale(v, at))()
					p.Mask(candyMask{shape: shapeStar, grow: 0.05}, geom.Rc(at.X-s, at.Y-s, 2*s, 2*s), rgb(0xd9, 0x8a, 0x10))
					p.Mask(candyMask{shape: shapeStar}, geom.Rc(at.X-s, at.Y-s, 2*s, 2*s), gold)
					p.Mask(candyMask{shape: shapeStar, grow: -0.12}, geom.Rc(at.X-s*0.85, at.Y-s*0.9, 1.7*s, 1.7*s), rgb(0xff, 0xe8, 0x8a))
				}()
			}
		}
		sc := shaped(fmt.Sprintf("Score %d", c.score), 18, true)
		sc.Paint(p, geom.Pt(mid.X-sc.Advance/2, pr.Min.Y+172), plum)
	} else {
		s := float32(48)
		hr := geom.Rc(mid.X-s, pr.Min.Y+80, 2*s, 2*s)
		p.Mask(candyMask{shape: shapeHeart}, hr, faded(plum, 0.25))
		msg := shaped("The candies will wait for you.", 15, false)
		msg.Paint(p, geom.Pt(mid.X-msg.Advance/2, pr.Min.Y+186), faded(plum, 0.8))
	}
	br := c.button()
	bm := geom.Pt(br.Min.X+br.Size().W/2, br.Min.Y+br.Size().H/2)
	func() {
		defer p.Push(paint.Scale(1-0.08*c.press.Value(), bm))()
		p.ShadowRRect(br, 27, paint.Fill{Gradient: &paint.Gradient{
			From: br.Min, To: geom.Pt(br.Min.X, br.Max.Y),
			Start: rgb(0x6c, 0xe8, 0x7a), End: rgb(0x1f, 0xb3, 0x4a),
		}}, paint.Shadow{Blur: 10, Offset: geom.Pt(0, 5), Color: faded(rgb(0x0a, 0x50, 0x20), 0.5)})
		p.RRect(geom.Rc(br.Min.X+10, br.Min.Y+5, br.Size().W-20, 16), 8, paint.Solid(faded(white, 0.3)))
		l := shaped(label, 20, true)
		l.Paint(p, geom.Pt(bm.X-l.Advance/2, bm.Y-12+2), faded(rgb(0x0a, 0x40, 0x18), 0.6))
		l.Paint(p, geom.Pt(bm.X-l.Advance/2, bm.Y-12), white)
	}()
}
