package main

import (
	"math"
	"sort"

	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// The board's antics: candies cheering as their digit is picked, a new
// level's candies running in to their cells, and Pac-Man eating a won
// board.

// cheerLife is how long a cheer lasts, in seconds.
const cheerLife = 0.9

// cheer sets the candies of digit d cheering: each hops, swells and
// wiggles, and light runs out of it along its row and column, the cells
// it keeps d from. They go in turn, out from cell from, or row by row
// from the top with from -1.
func (b *board) cheer(d int8, from int) {
	g := b.root.state
	i := 0
	for c := range 81 {
		if g.Cells[c] != d {
			continue
		}
		k := &b.cells[c]
		if k.entering {
			continue
		}
		wait := 0.045 * float32(i)
		if from >= 0 {
			dx, dy := float64(c%9-from%9), float64(c/9-from/9)
			wait = 0.05 * float32(math.Hypot(dx, dy))
		}
		k.cheering, k.cheerAt = true, -wait
		i++
	}
}

// cheerPose returns how a cheering candy stands t seconds in: how high
// it has hopped and how much it has grown, as fractions of a cell, the
// jelly squash as it lands, and how far it leans, in radians.
func cheerPose(t float32) (hop, grow, squash, lean float32) {
	if t <= 0 {
		return 0, 0, 0, 0
	}
	const up = 0.34
	if t < up {
		a := float32(math.Sin(math.Pi * float64(t/up)))
		hop, grow = 0.32*a, 0.28*a
	} else {
		u := float64(t - up)
		squash = float32(0.2 * math.Exp(-u*8) * math.Cos(u*32))
	}
	lean = float32(0.34 * math.Sin(float64(t)*26) * math.Exp(-float64(t)*3.2))
	return hop, grow, squash, lean
}

// cheerLight fills light with how lit each cell is by the cheers: a
// front runs out of each cheering candy along its row and column, and
// the cells it passes stay lit as it fades. It reports whether any is.
func (b *board) cheerLight(light *[81]float32) bool {
	*light = [81]float32{}
	any := false
	for c := range b.cells {
		k := &b.cells[c]
		if !k.cheering || k.cheerAt < 0 {
			continue
		}
		t := k.cheerAt
		// The front crosses the board in 0.4 seconds; the light fades
		// after.
		front := (t - 0.06) * 24
		fade := min(1, (cheerLife-t)/0.5)
		at := func(o int, d int) {
			a := lightAt(front, float32(d)) * fade
			if a > light[o] {
				light[o] = a
			}
		}
		row, col := c/9, c%9
		for i := range 9 {
			at(row*9+i, abs(i-col))
			at(i*9+col, abs(i-row))
		}
		any = true
	}
	return any
}

// lightAt is how lit a cell d cells out is with the front at front:
// bright as it arrives, fading slowly behind it.
func lightAt(front, d float32) float32 {
	if d > front {
		x := (d - front) / 0.6
		return float32(math.Exp(-float64(x * x)))
	}
	return float32(math.Exp(-float64(front-d) * 0.3))
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// How a new level's candies come in.
const (
	// enterAfter holds them back while the map zooms into the level.
	enterAfter = 0.35
	// enterRun is how fast they run, in cells a second, and enterStride
	// how far each hop of their run carries them, in cells.
	enterRun    = 13
	enterStride = 0.85
	// enterLeap is how far above the line to its cell a candy's leap
	// rises, in cells.
	enterLeap = 1.1
)

// enter brings the candies on board g in, as the characters of an
// arcade game come on: they run in along the foot of the board, from
// the left those of its left half and from the right the others, and
// leap up to their cells, the bottom rows first.
func (b *board) enter(g Game) {
	var order []int
	for c := range 81 {
		k := &b.cells[c]
		k.entering, k.cheering, k.hint = false, false, false
		if g.Cells[c] == 0 {
			continue
		}
		k.side = sideOf(c)
		order = append(order, c)
	}
	// From the bottom row up; in a row, those nearer their edge first,
	// so a runner passes behind those still to come.
	edge := func(c int) int {
		if b.cells[c].side < 0 {
			return c % 9
		}
		return 8 - c%9
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, c := order[i], order[j]
		if a/9 != c/9 {
			return a/9 > c/9
		}
		return edge(a) < edge(c)
	})
	step := min(0.045, 2.2/float32(max(len(order), 1)))
	for i, c := range order {
		k := &b.cells[c]
		k.entering, k.enterAt = true, -(enterAfter + step*float32(i))
		k.pop.Jump(1)
	}
}

// sideOf returns the side cell c's candy comes in from: -1 the left,
// for the board's left half, and 1 the right.
func sideOf(c int) float32 {
	if col := c % 9; col < 4 || col == 4 && (c/9)%2 == 0 {
		return -1
	}
	return 1
}

// enterPath returns the way cell c's candy comes in: where it sets off,
// beyond the window's edge; where it stops running, at the foot of the
// board beside its column; and its cell's middle. run and leap are how
// long it runs and leaps.
func (b *board) enterPath(c int) (from, stop, to geom.Point, run, leap float32) {
	w := b.size.W
	s := cellSize(w)
	r := cellRect(c, w)
	to = geom.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	floor := w - s*0.2
	side := b.cells[c].side
	edge := -b.root.boardAt.X - s
	if side > 0 {
		edge = b.root.size.W - b.root.boardAt.X + s
	}
	from = geom.Pt(edge, floor)
	stop = geom.Pt(to.X+side*s*0.9, floor)
	run = float32(math.Abs(float64(stop.X-from.X))) / (enterRun * s)
	rise := (floor - to.Y) / s
	leap = 0.3 + 0.035*rise
	return from, stop, to, run, leap
}

// enterPose returns where cell c's candy is t seconds after it set off,
// in the board's space, how much it stretches across and up, and how
// far it leans, in radians.
func (b *board) enterPose(c int, t float32) (at geom.Point, sx, sy, lean float32) {
	from, stop, to, run, leap := b.enterPath(c)
	s := cellSize(b.size.W)
	side := b.cells[c].side
	if t < run {
		u := t / run
		x := from.X + (stop.X-from.X)*u
		// It hops along as it runs, squashing each time it touches down.
		stride := math.Abs(float64(x-from.X)) / float64(enterStride*s)
		up := float32(math.Abs(math.Sin(math.Pi * stride)))
		down := (1 - up) * (1 - up)
		return geom.Pt(x, from.Y-up*s*0.25), 1 + 0.14*down, 1 - 0.14*down, -side * 0.14
	}
	u := min((t-run)/leap, 1)
	x := stop.X + (to.X-stop.X)*u
	y := stop.Y + (to.Y-stop.Y)*u - enterLeap*s*4*u*(1-u)
	// Stretched tall as it springs and as it falls, round at the top.
	v := float32(math.Abs(float64(1 - 2*u)))
	return geom.Pt(x, y), 1 - 0.1*v, 1 + 0.12*v, -side * 0.14 * (1 - u)
}

// pacSpeed is how fast Pac-Man goes, in cells a second: he takes his
// time.
const pacSpeed = 5

// pacWait holds Pac-Man back after a win while the board celebrates and
// the card comes up.
const pacWait = 2.6

// pacman is Pac-Man eating a won board.
type pacman struct {
	// on says he is out; wait holds him back, and dist is how far along
	// his path he has come, in the board's pixels.
	on   bool
	wait float32
	dist float32
	// full says he eats no more, as he fades away.
	full bool
	// bites counts the candies eaten, for the sound to go up and down.
	bites int
	fade  *anim.Float
}

// letPacmanOut sends Pac-Man out to eat the board, once the board has
// celebrated.
func (b *board) letPacmanOut() {
	b.pac.on, b.pac.wait, b.pac.dist, b.pac.full, b.pac.bites = true, pacWait, 0, false, 0
	b.pac.fade.Jump(1)
}

// sendPacmanAway stops Pac-Man eating, and fades him out, as the game
// goes back to the map.
func (b *board) sendPacmanAway() {
	if !b.pac.on {
		return
	}
	if b.pac.wait > 0 {
		b.pac.on = false
		return
	}
	b.pac.full = true
	b.pac.fade.Animate(0, anim.Spring{Response: 0.3, Damping: 1})
}

// pacPath returns Pac-Man's way over a board: in from the left along
// the top row, down and back along the next, a row at a time, and off
// to the right after the last.
func (b *board) pacPath() []geom.Point {
	w := b.size.W
	s := cellSize(w)
	rowY := func(r int) float32 {
		cr := cellRect(r*9, w)
		return (cr.Min.Y + cr.Max.Y) / 2
	}
	left, right := cellRect(0, w).Min.X-s*0.3, cellRect(8, w).Max.X+s*0.3
	pts := []geom.Point{geom.Pt(-b.root.boardAt.X-s, rowY(0))}
	for r := range 9 {
		x := right
		if r%2 == 1 {
			x = left
		}
		pts = append(pts, geom.Pt(x, rowY(r)))
		if r < 8 {
			pts = append(pts, geom.Pt(x, rowY(r+1)))
		}
	}
	return append(pts, geom.Pt(b.root.size.W-b.root.boardAt.X+s, rowY(8)))
}

// pathLen returns the length of a path.
func pathLen(pts []geom.Point) float32 {
	n := float32(0)
	for i := 1; i < len(pts); i++ {
		n += dist(pts[i-1], pts[i])
	}
	return n
}

func dist(a, b geom.Point) float32 {
	return float32(math.Hypot(float64(b.X-a.X), float64(b.Y-a.Y)))
}

// pathAt returns where along a path d is, and which way it heads there.
func pathAt(pts []geom.Point, d float32) (at, dir geom.Point) {
	for i := 1; i < len(pts); i++ {
		a, c := pts[i-1], pts[i]
		l := dist(a, c)
		if l == 0 {
			continue
		}
		dir = c.Sub(a).Mul(1 / l)
		if d <= l || i == len(pts)-1 {
			return a.Add(dir.Mul(min(d, l))), dir
		}
		d -= l
	}
	return pts[len(pts)-1], geom.Pt(1, 0)
}

// biteAt returns how far along path Pac-Man has come as he bites cell
// c's candy: as his mouth reaches it, a little before his middle does.
func (b *board) biteAt(pts []geom.Point, c int) float32 {
	s := cellSize(b.size.W)
	r := cellRect(c, b.size.W)
	mid := geom.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	d := float32(0)
	for i := 1; i < len(pts); i++ {
		a, e := pts[i-1], pts[i]
		lo, hi := min(a.X, e.X), max(a.X, e.X)
		if a.Y == e.Y && math.Abs(float64(a.Y-mid.Y)) < 1 && mid.X >= lo && mid.X <= hi {
			return d + float32(math.Abs(float64(mid.X-a.X))) - s*0.25
		}
		d += dist(a, e)
	}
	return d
}

// stepPacman moves Pac-Man on by s seconds, and eats the candies he
// reaches. It reports whether he is still out.
func (b *board) stepPacman(s float32) bool {
	pc := &b.pac
	if !pc.on {
		return false
	}
	if pc.full {
		if pc.fade.Value() < 0.01 && pc.fade.Target() == 0 {
			pc.on = false
		}
		pc.dist += pacSpeed * cellSize(b.size.W) * s
		return pc.on
	}
	if pc.wait > 0 {
		pc.wait -= s
		return true
	}
	pts := b.pacPath()
	pc.dist += pacSpeed * cellSize(b.size.W) * s
	g := b.root.state
	for c := range b.cells {
		k := &b.cells[c]
		if g.Cells[c] == 0 || k.eaten || b.biteAt(pts, c) > pc.dist {
			continue
		}
		k.eaten = true
		at := b.root.cellCenter(c)
		b.root.fx.burst(at, 6, dot, 160, candyOf(g.Cells[c]).color, white)
		b.root.sfx.chomp(pc.bites%2 == 1, b.root.pan(at))
		pc.bites++
	}
	if pc.dist >= pathLen(pts) {
		pc.on = false
	}
	return pc.on
}

// pacYellow is Pac-Man's colour.
var pacYellow = rgb(0xff, 0xe1, 0x2e)

// paintPacman draws Pac-Man, chomping as he goes, his mouth the way he
// heads.
func (b *board) paintPacman(p *paint.Painter) {
	pc := &b.pac
	if !pc.on || pc.wait > 0 {
		return
	}
	fade := pc.fade.Value()
	if fade < 0.01 {
		return
	}
	s := cellSize(b.size.W)
	at, dir := pathAt(b.pacPath(), pc.dist)
	rad := s * 0.58
	// His mouth opens and shuts every half a cell.
	open := math.Abs(math.Sin(float64(pc.dist/(s*0.5)) * math.Pi / 2))
	m := pacMask{mouth: int8(open*6 + 0.5)}
	box := geom.Rc(at.X-rad, at.Y-rad, 2*rad, 2*rad)
	switch {
	case dir.X < -0.5:
		// Facing left he is mirrored, so his eye stays on top.
		defer p.Push(paint.Transform{A: -1, C: 2 * at.X, E: 1})()
	case dir.Y > 0.5:
		defer p.Push(paint.Rotate(math.Pi/2, at))()
	}
	p.Mask(m, box.Add(geom.Pt(0, s*0.1)), faded(plum, 0.35*fade))
	p.Mask(m, box, faded(darker(pacYellow, 0.18), fade))
	func() {
		defer p.Push(paint.Scale(0.86, at.Add(geom.Pt(-rad*0.06, -rad*0.06))))()
		p.Mask(m, box, faded(pacYellow, fade))
	}()
	eye := geom.Pt(at.X+rad*0.08, at.Y-rad*0.52)
	er := rad * 0.13
	p.RRect(geom.Rc(eye.X-er, eye.Y-er, 2*er, 2*er), er, paint.Solid(faded(rgb(0x2a, 0x10, 0x30), fade)))
	p.RRect(geom.Rc(eye.X-er*0.2, eye.Y-er*0.7, er*0.6, er*0.6), er*0.3, paint.Solid(faded(white, 0.8*fade)))
}

// pacMask is Pac-Man's shape: a disc, its mouth open toward the right
// by mouth sixths of its widest. It is comparable, so each opening is a
// mask made once.
type pacMask struct{ mouth int8 }

// Settled implements [paint.Shape].
func (pacMask) Settled() bool { return true }

// Coverage implements [paint.Shape].
func (m pacMask) Coverage(w, h int) []byte {
	out := make([]byte, w*h)
	half := float64(min(w, h)) / 2
	ang := float64(m.mouth) / 6 * 0.8
	sa, ca := math.Sin(ang), math.Cos(ang)
	for y := range h {
		for x := range w {
			px := (float64(x) + 0.5 - float64(w)/2) / half
			py := (float64(y) + 0.5 - float64(h)/2) / half
			d := math.Hypot(px, py) - 0.94
			if m.mouth > 0 {
				// How far inside the mouth's wedge, from its nearer side
				// or its corner.
				qy := math.Abs(py)
				wedge := -sa*px + ca*qy
				if px*ca+qy*sa < 0 {
					wedge = math.Hypot(px, qy)
				}
				d = max(d, -wedge)
			}
			out[y*w+x] = uint8(255 * max(0, min(1, 0.5-d*half)))
		}
	}
	return out
}
