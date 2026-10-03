package main

import (
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// cellAnim is how one cell moves.
type cellAnim struct {
	// pop lands a candy: it springs from small to full past its size and
	// back, and the jelly squash follows how far past it is.
	pop *anim.Float
	// glow lights the cell, as a hint lands in it.
	glow *anim.Float
	// wait holds a bounce until its turn in a wave across a group, and
	// drop says it drops in from nothing, rather than hop.
	wait float32
	drop bool
	// again hops it once more this long after, for a celebration.
	again float32
	// gone is a candy taken off by undo, shrinking away, and out its
	// size.
	gone int8
	out  *anim.Float
}

// ghost is a wrong candy: it shakes, flushes red, and shrinks away.
type ghost struct {
	cell  int
	digit int8
	age   float32
	burst bool
}

// sweep is a light running across a finished group.
type sweep struct {
	unit int
	age  float32
}

// board draws the cells and their candies, and takes taps on them.
type board struct {
	anim.Group
	root   *gameRoot
	cells  [81]cellAnim
	ghosts []ghost
	sweeps []sweep
	// shown is the board last drawn, to tell candies taken off by undo.
	shown Grid
	// pulse turns for the candies of the digit selected to breathe.
	pulse float64
	// hover is the cell under the mouse, or -1.
	hover int
	size  geom.Size
}

func newBoard(r *gameRoot) *board {
	b := &board{root: r, hover: -1}
	for i := range b.cells {
		c := &b.cells[i]
		c.pop, c.glow, c.out = anim.NewFloat(1), anim.NewFloat(0), anim.NewFloat(0)
		b.Add(c.pop, c.glow, c.out)
	}
	return b
}

// show takes the board as it is now: candies gone from it shrink away.
func (b *board) show(g Game, fresh bool) {
	for c := range 81 {
		if fresh {
			b.cells[c].gone = 0
			// A new game's candies drop in, a wave from the top left.
			if g.Cells[c] != 0 {
				b.bounce(c, float32(c/9+c%9)*0.035, true)
			}
			continue
		}
		if b.shown[c] != 0 && g.Cells[c] == 0 {
			b.cells[c].gone = b.shown[c]
			b.cells[c].out.Jump(1)
			b.cells[c].out.Animate(0, anim.Spring{Response: 0.25, Damping: 1})
		}
	}
	b.shown = g.Cells
}

// bounce lands cell c's candy after delay seconds: from nothing when
// drop, or with a hop where it stands.
func (b *board) bounce(c int, delay float32, drop bool) {
	k := &b.cells[c]
	k.wait, k.drop = delay, drop
	if drop {
		// Unseen until its turn.
		k.pop.Jump(0)
	}
	if delay <= 0 {
		k.start()
	}
}

// start sets the cell's bounce going.
func (k *cellAnim) start() {
	k.wait = 0
	if k.drop {
		k.pop.Jump(0)
		k.pop.Animate(1, anim.Spring{Response: 0.42, Damping: 0.38})
		return
	}
	k.pop.Jump(0.8)
	k.pop.Animate(1, anim.Spring{Response: 0.3, Damping: 0.3})
}

// land drops a candy just placed into cell c.
func (b *board) land(c int) { b.bounce(c, 0, true) }

// wrong shows digit d bouncing off cell c.
func (b *board) wrong(c int, d int8) { b.ghosts = append(b.ghosts, ghost{cell: c, digit: d}) }

// done runs the light across unit u, and hops its candies in turn.
func (b *board) done(u int) {
	b.sweeps = append(b.sweeps, sweep{unit: u})
	for i, c := range units[u] {
		b.bounce(c, 0.05*float32(i), false)
	}
}

// celebrate hops every candy, a ripple out from the middle, twice.
func (b *board) celebrate() {
	for c := range 81 {
		dx, dy := float64(c%9-4), float64(c/9-4)
		d := float32(math.Hypot(dx, dy))
		b.bounce(c, 0.15+0.07*d, false)
		b.cells[c].again = 0.75 + 0.07*d
	}
}

// hinted lights cell c as a hint lands its candy.
func (b *board) hinted(c int) {
	b.land(c)
	b.cells[c].glow.Jump(1)
	b.cells[c].glow.Animate(0, anim.Tween{Duration: 1400 * time.Millisecond})
}

// Step implements [gunim.Animator].
func (b *board) Step(dt time.Duration) bool {
	s := float32(dt.Seconds())
	moving := false
	for i := range b.cells {
		k := &b.cells[i]
		if k.wait > 0 {
			k.wait -= s
			moving = true
			if k.wait <= 0 {
				k.start()
			}
		}
		if k.again > 0 {
			k.again -= s
			moving = true
			if k.again <= 0 {
				k.again, k.drop = 0, false
				k.start()
			}
		}
	}
	if b.Group.Step(dt) {
		moving = true
	}
	ghosts := b.ghosts[:0]
	for _, g := range b.ghosts {
		g.age += s
		if g.age > 0.42 && !g.burst {
			g.burst = true
			at := b.root.cellCenter(g.cell)
			b.root.fx.burst(at, 14, dot, 320, candyOf(g.digit).color, rgb(0xff, 0x4d, 0x6d))
		}
		if g.age < 0.6 {
			ghosts = append(ghosts, g)
		}
	}
	b.ghosts = ghosts
	sweeps := b.sweeps[:0]
	for _, w := range b.sweeps {
		w.age += s
		if w.age < 0.9 {
			sweeps = append(sweeps, w)
		}
	}
	b.sweeps = sweeps
	if b.root.selected >= 0 || b.root.armed > 0 {
		b.pulse += dt.Seconds()
		moving = true
	}
	return moving || len(b.ghosts) > 0 || len(b.sweeps) > 0
}

// Layout implements [gunim.Node].
func (b *board) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	b.size = c.Max
	return c.Max
}

// The board's room: the padding inside its edge, and the gaps between
// its cells and between its boxes, as fractions of a cell.
const (
	boardPad = 0.22
	cellGap  = 0.06
	boxGap   = 0.22
)

// cellSize returns the side of a cell on a board of side w.
func cellSize(w float32) float32 { return w / (9 + 2*boardPad + 8*cellGap + 2*(boxGap-cellGap)) }

// cellRect returns where cell c lies on a board of side w.
func cellRect(c int, w float32) geom.Rect {
	s := cellSize(w)
	at := func(i int) float32 {
		return s*boardPad + float32(i)*s*(1+cellGap) + float32(i/3)*s*(boxGap-cellGap)
	}
	return geom.Rc(at(c%9), at(c/9), s, s)
}

// cellAt returns the cell at p, or -1.
func (b *board) cellAt(p geom.Point) int {
	for c := range 81 {
		r := cellRect(c, b.size.W)
		// Taps between cells go to the nearer.
		r = geom.Rect{Min: r.Min.Sub(geom.Pt(4, 4)), Max: r.Max.Add(geom.Pt(4, 4))}
		if r.Contains(p) {
			return c
		}
	}
	return -1
}

// Handle implements [gunim.Handler]: a tap picks a cell.
func (b *board) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		if !e.Touch {
			b.hover = b.cellAt(e.Pos)
		}
	case input.PointerLeave:
		b.hover = -1
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		if c := b.cellAt(e.Pos); c >= 0 {
			b.root.tapCell(c, u)
		}
	default:
		return false
	}
	u.Invalidate()
	return true
}

// Paint implements [gunim.Node].
func (b *board) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	g := b.root.state
	w := box.W
	s := cellSize(w)
	whole := geom.Rect{Max: box.Point()}
	radius := s * 0.45
	// Frosted glass, with a light rim.
	p.ShadowRRect(whole, radius, paint.Solid(faded(rgb(0x2a, 0x0a, 0x4a), 0.25)),
		paint.Shadow{Blur: 30, Offset: geom.Pt(0, 10), Color: faded(rgb(0x1a, 0x00, 0x30), 0.5)})
	end := p.Layer(paint.LayerOpts{Bounds: whole, Opacity: 1, Backdrop: 16, Clip: true, Radius: radius})
	p.RRect(whole, radius, paint.Solid(faded(rgb(0xff, 0xff, 0xff), 0.14)))
	end()
	p.RRectStroke(whole, radius, paint.Solid(faded(rgb(0xff, 0xff, 0xff), 0.35)), paint.Stroke{Width: 1.5})

	sel := b.root.selected
	selDigit := int8(0)
	if sel >= 0 {
		selDigit = g.Cells[sel]
	}
	if b.root.armed > 0 {
		selDigit = b.root.armed
	}
	// The slots: lit along the selected cell's row, column and box.
	for c := range 81 {
		r := cellRect(c, w)
		fill := faded(rgb(0xff, 0xff, 0xff), 0.12)
		if sel >= 0 && sharesUnit(sel, c) {
			fill = faded(rgb(0xff, 0xf0, 0xff), 0.24)
		}
		if c == b.hover {
			fill = faded(rgb(0xff, 0xff, 0xff), 0.3)
		}
		p.RRect(r, s*0.22, paint.Solid(fill))
		if gl := b.cells[c].glow.Value(); gl > 0.01 {
			p.ShadowRRect(r, s*0.22, paint.Solid(faded(rgb(0xff, 0xe0, 0x7a), 0.5*gl)),
				paint.Shadow{Blur: s * 0.5, Color: faded(rgb(0xff, 0xd0, 0x40), gl)})
		}
	}
	// The light running across finished groups.
	for _, sw := range b.sweeps {
		for i, c := range units[sw.unit] {
			d := (sw.age*2.2 - float32(i)/8) / 0.18
			a := float32(math.Exp(-float64(d * d)))
			if a > 0.02 {
				p.RRect(cellRect(c, w), s*0.22, paint.Solid(faded(rgb(0xff, 0xff, 0xff), 0.55*a)))
			}
		}
	}
	// The selected cell: a ring that breathes.
	if sel >= 0 {
		r := cellRect(sel, w)
		breathe := 0.5 + 0.5*float32(math.Sin(b.pulse*5))
		grow := 2 + 2*breathe
		ring := geom.Rect{Min: r.Min.Sub(geom.Pt(grow, grow)), Max: r.Max.Add(geom.Pt(grow, grow))}
		p.ShadowRRect(ring, s*0.28, paint.Solid(faded(rgb(0xff, 0xff, 0xff), 0)),
			paint.Shadow{Blur: 10, Color: faded(rgb(0xff, 0xe6, 0x6e), 0.6+0.3*breathe)})
		p.RRectStroke(ring, s*0.28, paint.Solid(rgb(0xff, 0xf4, 0xb0)), paint.Stroke{Width: 2.5})
	}

	numbered := b.root.numbered
	for c := range 81 {
		r := cellRect(c, w)
		k := &b.cells[c]
		mid := geom.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
		if d := g.Cells[c]; d != 0 {
			pop := k.pop.Value()
			// Past full size it squashes wide and short, then tall.
			over := pop - 1
			sx, sy := pop*(1+0.9*over), pop*(1-0.9*over)
			// A dropped candy falls in from above as it grows.
			dy := (1 - min(pop, 1)) * -s * 0.6
			if d == selDigit && sel != c {
				// The same digits as the selected breathe with it.
				br := 1 + 0.05*float32(math.Sin(b.pulse*5+float64(c)*0.3))
				sx, sy = sx*br, sy*br
			}
			func() {
				// Scaled about its bottom, as jelly squashes on landing.
				base := geom.Pt(mid.X, r.Max.Y)
				defer p.Push(paint.Translate(geom.Pt(0, dy)))()
				defer p.Push(paint.Transform{A: sx, E: sy, C: base.X * (1 - sx), F: base.Y * (1 - sy)})()
				inset := s * 0.07
				cr := geom.Rect{Min: r.Min.Add(geom.Pt(inset, inset)), Max: r.Max.Sub(geom.Pt(inset, inset))}
				paintCandy(p, d, cr, min(1, pop*2), numbered)
			}()
			continue
		}
		if k.gone != 0 {
			if o := k.out.Value(); o > 0.01 {
				func() {
					defer p.Push(paint.Scale(o, mid))()
					inset := s * 0.07
					paintCandy(p, k.gone, geom.Rect{Min: r.Min.Add(geom.Pt(inset, inset)), Max: r.Max.Sub(geom.Pt(inset, inset))}, o, numbered)
				}()
			}
		}
		if notes := g.Notes[c]; notes != 0 {
			m := s / 3
			for d := int8(1); d <= 9; d++ {
				if notes&(1<<d) == 0 {
					continue
				}
				col, row := float32((d-1)%3), float32((d-1)/3)
				nr := geom.Rc(r.Min.X+col*m+m*0.12, r.Min.Y+row*m+m*0.12, m*0.76, m*0.76)
				if d == selDigit {
					p.RRect(geom.Rc(nr.Min.X-1, nr.Min.Y-1, m*0.76+2, m*0.76+2), m*0.4, paint.Solid(faded(rgb(0xff, 0xff, 0xff), 0.5)))
				}
				p.Mask(candyMask{shape: candyOf(d).shape}, nr, candyOf(d).color)
			}
		}
	}
	// Wrong candies, shaking, flushing red, then shrinking away.
	for _, gh := range b.ghosts {
		r := cellRect(gh.cell, w)
		mid := geom.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
		shake := float32(math.Sin(float64(gh.age)*55)) * s * 0.14 * max(0, 1-gh.age/0.45)
		scale := float32(1)
		if gh.age > 0.3 {
			scale = max(0, 1-(gh.age-0.3)/0.15)
		}
		p.RRect(r, s*0.22, paint.Solid(faded(rgb(0xff, 0x30, 0x50), 0.5*max(0, 1-gh.age/0.6))))
		func() {
			defer p.Push(paint.Translate(geom.Pt(shake, 0)))()
			defer p.Push(paint.Scale(scale, mid))()
			inset := s * 0.07
			paintCandy(p, gh.digit, geom.Rect{Min: r.Min.Add(geom.Pt(inset, inset)), Max: r.Max.Sub(geom.Pt(inset, inset))}, 1, numbered)
		}()
	}
}

// sharesUnit reports whether cells a and b share a row, column or box.
func sharesUnit(a, b int) bool {
	for _, u := range unitsOf[a] {
		if unitsOf[b][u/9] == u {
			return true
		}
	}
	return false
}
