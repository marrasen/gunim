//go:build linux || windows || darwin || (js && wasm)

package render

import (
	"math"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// An area draws as a ribbon does, in the batch with everything else and
// through the same pixels of the draw program: one quad for each stretch
// between two of its points, from its line to its base. The quads are
// cut on the CPU, in device pixels.
//
// Each corner of a quad carries how far its pixel lies inside the
// stretch's two edges, as the two numbers a ribbon's does: half the sum
// of the two distances, the half width, and half their difference, how
// far across. Both are even over the stretch, as a distance from a
// straight edge is, so the shader works out from them how much of each
// pixel the area covers, and the edges stay smooth however steep the
// line runs. An area thinner than a pixel fades out evenly.
//
// A quad reaches past its two edges for their smoothing, along the lines
// from its points' At to their Base, which the quads either side of a
// point share to the corner. So no two quads overlap, and no pixel falls
// between two, whatever the line does. The colour is carried on past the
// edge at the pace it changes across the area, so the colour within the
// area is the one asked for.
//
// The ends are smoothed as a ribbon's are: a short quad at each end
// fades the colour in, from nothing half a pixel outside the end to the
// whole of it half a pixel inside.

// areaFeather is how far past an edge a quad reaches at least, in
// device pixels, for the edge's smoothing: the half pixel it needs, and
// a little more.
const areaFeather = 1

// areaReach is how far a quad reaches past an edge at most, in device
// pixels, along the line from a point's At to its Base. An edge that
// runs nearly along that line, as a graph's line does where it jumps,
// would reach without end; cut short, the last of its smoothing is lost
// where it is steepest, by less than a pixel's width.
const areaReach = 16

// areaThin is the least an area's height at a point, in device pixels,
// that still says which way across the area lies there. A point where
// it is thinner takes the way of the point before it.
const areaThin = 0.05

// areaCorner is an area's point made ready for its quads: its two
// points in device pixels and its colour at each, premultiplied; the
// way from its base to its line, and how far apart they lie; and its
// quads' two corners, past the line and past the base, with the colours
// there.
type areaCorner struct {
	a, b       geom.Point
	ca, cb     [4]float32
	up         geom.Point
	tall       float32
	outA, outB geom.Point
	colA, colB [4]float32
}

// areaSide is a stretch of an area between two of its points: the ways
// into it from its line and from its base, each across its edge, and
// whether it has any area to draw.
type areaSide struct {
	na, nb geom.Point
	ok     bool
}

// area queues an area's quads.
func (r *Renderer) area(op *paint.AreaOp) {
	pts := op.Points
	if len(pts) < 2 {
		return
	}
	t := op.Transform
	cs := r.areaCorners[:0]
	for _, q := range pts {
		ca, cb := rgba(q.Color), rgba(q.BaseColor)
		cs = append(cs, areaCorner{
			a:  t.Apply(q.At).Mul(r.scale),
			b:  t.Apply(q.Base).Mul(r.scale),
			ca: [4]float32{ca[0] * ca[3], ca[1] * ca[3], ca[2] * ca[3], ca[3]},
			cb: [4]float32{cb[0] * cb[3], cb[1] * cb[3], cb[2] * cb[3], cb[3]},
		})
	}
	r.areaCorners = cs
	if !areaUps(cs) {
		return
	}
	// The capped corners go after the points' own, in the same buffer.
	all := capArea(cs, cs)
	r.areaCorners = all[:0]
	cs = all[len(cs):]
	if !areaUps(cs) {
		return
	}
	sides := areaSides(r.areaSides[:0], cs)
	r.areaSides = sides
	areaOuts(cs, sides)
	add := op.Blend == paint.BlendAdd
	for i, s := range sides {
		if !s.ok {
			continue
		}
		c, d := &cs[i], &cs[i+1]
		if c.ca[3] == 0 && c.cb[3] == 0 && d.ca[3] == 0 && d.cb[3] == 0 {
			continue
		}
		quad := [4]geom.Point{c.outA, c.outB, d.outB, d.outA}
		if !r.cull.Empty() {
			lo, hi := quad[0], quad[0]
			for _, p := range quad[1:] {
				lo = geom.Pt(min(lo.X, p.X), min(lo.Y, p.Y))
				hi = geom.Pt(max(hi.X, p.X), max(hi.Y, p.Y))
			}
			if r.outside(geom.Rect{Min: lo, Max: hi}) {
				continue
			}
		}
		r.uses(0)
		for k, col := range [4][4]float32{c.colA, c.colB, d.colB, d.colA} {
			p := quad[k]
			// How far inside the stretch's line and its base the corner
			// lies: less than nothing, past the edge it reaches over.
			in, up := dot(p.Sub(c.a), s.na), dot(p.Sub(c.b), s.nb)
			r.bandVert(p, (up-in)/2, (up+in)/2, col, add)
		}
	}
}

// dot returns the dot product of a and b.
func dot(a, b geom.Point) float32 { return a.X*b.X + a.Y*b.Y }

// areaUps sets each corner's way from its base to its line and how far
// apart the two lie, and reports false where the area has no height
// anywhere, which draws nothing. A corner too thin to say takes the way
// of the one before it, or of the first one that says.
func areaUps(cs []areaCorner) bool {
	var last geom.Point
	found := false
	for i := range cs {
		c := &cs[i]
		d := c.a.Sub(c.b)
		c.tall = float32(math.Hypot(float64(d.X), float64(d.Y)))
		if c.tall > areaThin {
			u := d.Mul(1 / c.tall)
			if !found {
				// The thin corners at the start take this one's.
				for j := range i {
					cs[j].up = u
				}
			}
			last, found = u, true
		}
		c.up = last
	}
	return found
}

// areaAdvance returns how far the area runs on from c to d, across the
// line from c's base to its line.
func areaAdvance(c, d *areaCorner) float32 {
	m := geom.Pt(-c.up.Y, c.up.X)
	l := (dot(d.a.Sub(c.a), m) + dot(d.b.Sub(c.b), m)) / 2
	return float32(math.Abs(float64(l)))
}

// capArea appends to dst the corners of cs with a fading quad at each
// end, and returns the longer dst, as capRibbon does for a ribbon: each
// end moves half a pixel in along the area, or halfway along a shorter
// one, the corners it passes left out, and a corner of no colour a pixel
// further out ends it.
func capArea(dst, cs []areaCorner) []areaCorner {
	n := len(cs)
	var length float32
	for i := range n - 1 {
		length += areaAdvance(&cs[i], &cs[i+1])
	}
	in := min(0.5, length/2)
	start, first, startOut := capAreaEnd(cs, in, 1)
	end, last, endOut := capAreaEnd(cs, in, -1)
	dst = append(dst, startOut, start)
	if first <= last {
		dst = append(dst, cs[first:last+1]...)
	}
	return append(dst, end, endOut)
}

// capAreaEnd returns the corner in along cs from its first corner, for
// way 1, or from its last, for way -1; the index of the next corner on
// from there; and the corner of no colour a pixel back out from it.
func capAreaEnd(cs []areaCorner, in float32, way int) (at areaCorner, next int, beyond areaCorner) {
	i, j := 0, 1
	if way < 0 {
		i, j = len(cs)-1, len(cs)-2
	}
	for ; j >= 0 && j < len(cs); i, j = j, j+way {
		c, d := &cs[i], &cs[j]
		l := areaAdvance(c, d)
		if l < in || l == 0 {
			in -= l
			continue
		}
		t := in / l
		lerp := func(a, b float32) float32 { return a + (b-a)*t }
		at = *c
		at.a = geom.Pt(lerp(c.a.X, d.a.X), lerp(c.a.Y, d.a.Y))
		at.b = geom.Pt(lerp(c.b.X, d.b.X), lerp(c.b.Y, d.b.Y))
		for k := range at.ca {
			at.ca[k], at.cb[k] = lerp(c.ca[k], d.ca[k]), lerp(c.cb[k], d.cb[k])
		}
		// Back out along the two edges, so the fading quad's edges run
		// on from the stretch's own.
		beyond = at
		beyond.a = at.a.Add(c.a.Sub(d.a).Mul(1 / l))
		beyond.b = at.b.Add(c.b.Sub(d.b).Mul(1 / l))
		beyond.ca, beyond.cb = [4]float32{}, [4]float32{}
		return at, j, beyond
	}
	// Rounding left in a hair past the area's length: its far end.
	at = cs[i]
	return at, i + way, at
}

// areaSides appends to dst the stretches between the corners of cs.
func areaSides(dst []areaSide, cs []areaCorner) []areaSide {
	const none = 1e-6
	for i := range len(cs) - 1 {
		c, d := &cs[i], &cs[i+1]
		var s areaSide
		ea, eb := d.a.Sub(c.a), d.b.Sub(c.b)
		la := float32(math.Hypot(float64(ea.X), float64(ea.Y)))
		lb := float32(math.Hypot(float64(eb.X), float64(eb.Y)))
		// The way across the stretch, from its base to its line: that
		// of its two corners, which holds where the stretch is too thin
		// to say.
		w := c.up.Add(d.up)
		if (la > none || lb > none) && (c.tall > none || d.tall > none) && dot(w, w) > none {
			// An edge of no length, as where two points share a base,
			// runs the way the other one does.
			if la <= none {
				ea, la = eb, lb
			}
			if lb <= none {
				eb, lb = ea, la
			}
			s.na, s.nb = geom.Pt(-ea.Y/la, ea.X/la), geom.Pt(-eb.Y/lb, eb.X/lb)
			if dot(s.na, w) > 0 {
				s.na = s.na.Mul(-1)
			}
			if dot(s.nb, w) < 0 {
				s.nb = s.nb.Mul(-1)
			}
			s.ok = true
		}
		dst = append(dst, s)
	}
	return dst
}

// areaOuts sets each corner's two corners of its quads, past its line
// and past its base, and the colours there. Each lies far enough out
// along the corner's way that both stretches at the corner reach
// areaFeather past their edge.
func areaOuts(cs []areaCorner, sides []areaSide) {
	for i := range cs {
		c := &cs[i]
		// How fast a step out along the way leaves each edge behind:
		// the slower of the two stretches.
		ka, kb := float32(1), float32(1)
		for _, j := range [2]int{i - 1, i} {
			if j < 0 || j >= len(sides) || !sides[j].ok {
				continue
			}
			ka, kb = min(ka, -dot(c.up, sides[j].na)), min(kb, dot(c.up, sides[j].nb))
		}
		sa := areaFeather / max(ka, float32(areaFeather)/areaReach)
		sb := areaFeather / max(kb, float32(areaFeather)/areaReach)
		c.outA, c.outB = c.a.Add(c.up.Mul(sa)), c.b.Sub(c.up.Mul(sb))
		c.colA, c.colB = c.ca, c.cb
		if c.tall > areaThin {
			// The colour runs on past the edges as it runs between them.
			for k := range c.ca {
				step := (c.ca[k] - c.cb[k]) / c.tall
				c.colA[k], c.colB[k] = c.ca[k]+step*sa, c.cb[k]-step*sb
			}
		}
	}
}
