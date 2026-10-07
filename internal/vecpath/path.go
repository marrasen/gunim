// Package vecpath reads SVG path data and draws it into coverage masks: the outlines package icon strokes and
// package shape fills and strokes.
package vecpath

import (
	"fmt"
	"math"
	"strconv"
)

// Pt is a point on a path's grid, or in pixels once mapped.
type Pt struct{ X, Y float32 }

// Add is a+b.
func (a Pt) Add(b Pt) Pt { return Pt{a.X + b.X, a.Y + b.Y} }

// Sub is a-b.
func (a Pt) Sub(b Pt) Pt { return Pt{a.X - b.X, a.Y - b.Y} }

// Lerp is the point t of the way from a to b.
func (a Pt) Lerp(b Pt, t float32) Pt { return Pt{a.X + (b.X-a.X)*t, a.Y + (b.Y-a.Y)*t} }

// Seg is a line from P[0] to P[3], or a cubic Bézier through P[0] to P[3] when Curve is set.
type Seg struct {
	P     [4]Pt
	Curve bool
}

// Subpath is a run of joined segments.
type Subpath []Seg

// Affine maps a point (x, y) to (A*x + C*y + E, B*x + D*y + F), as SVG's matrix(a b c d e f) does.
type Affine struct{ A, B, C, D, E, F float32 }

// Identity maps each point to itself.
var Identity = Affine{A: 1, D: 1}

// Apply maps p.
func (m Affine) Apply(p Pt) Pt { return Pt{m.A*p.X + m.C*p.Y + m.E, m.B*p.X + m.D*p.Y + m.F} }

// Then is m followed by n.
func (m Affine) Then(n Affine) Affine {
	return Affine{
		A: n.A*m.A + n.C*m.B, B: n.B*m.A + n.D*m.B,
		C: n.A*m.C + n.C*m.D, D: n.B*m.C + n.D*m.D,
		E: n.A*m.E + n.C*m.F + n.E, F: n.B*m.E + n.D*m.F + n.F,
	}
}

// Transform returns subs with every point mapped by m. A Bézier mapped point by point is the Bézier of the mapped
// curve, so the result is exact.
func Transform(subs []Subpath, m Affine) []Subpath {
	out := make([]Subpath, len(subs))
	for i, sp := range subs {
		out[i] = make(Subpath, len(sp))
		for j, s := range sp {
			for k := range s.P {
				s.P[k] = m.Apply(s.P[k])
			}
			out[i][j] = s
		}
	}
	return out
}

// Bounds returns the smallest box holding subs and their control points, which holds the curves, and whether
// there are any points.
func Bounds(subs []Subpath) (lo, hi Pt, ok bool) {
	for _, sp := range subs {
		for _, s := range sp {
			for _, p := range s.P {
				if !ok {
					lo, hi, ok = p, p, true
					continue
				}
				lo = Pt{min(lo.X, p.X), min(lo.Y, p.Y)}
				hi = Pt{max(hi.X, p.X), max(hi.Y, p.Y)}
			}
		}
	}
	return lo, hi, ok
}

// pathParser reads SVG path data.
type pathParser struct {
	s string
	i int
}

// Parse reads SVG path data into subpaths, with arcs and quadratic curves turned into cubic ones. On an error it
// returns the subpaths read so far.
func Parse(d string) ([]Subpath, error) {
	p := pathParser{s: d}
	var (
		out       []Subpath
		cur       Subpath
		at, start Pt
		lastCtl   Pt
		lastCmd   byte
		cmd       byte
		hasCmd    bool
	)
	flush := func() {
		if len(cur) > 0 {
			out = append(out, cur)
		}
		cur = nil
	}
	line := func(to Pt) {
		cur = append(cur, Seg{P: [4]Pt{at, at, to, to}})
		at = to
	}
	cubic := func(c1, c2, to Pt) {
		cur = append(cur, Seg{P: [4]Pt{at, c1, c2, to}, Curve: true})
		lastCtl, at = c2, to
	}
	for {
		p.skip()
		if p.i >= len(p.s) {
			break
		}
		if c := p.s[p.i]; isCommand(c) {
			cmd, hasCmd = c, true
			p.i++
		} else if !hasCmd {
			return out, fmt.Errorf("path data has %q at %d where a command belongs", c, p.i)
		}
		rel := cmd >= 'a'
		base := Pt{}
		if rel {
			base = at
		}
		var err error
		num := func() float32 {
			if err != nil {
				return 0
			}
			var v float32
			v, err = p.number()
			return v
		}
		pair := func() Pt { x := num(); return base.Add(Pt{x, num()}) }
		prev := lastCmd
		lastCmd = cmd | 0x20
		switch cmd | 0x20 {
		case 'm':
			to := pair()
			if err != nil {
				return out, err
			}
			flush()
			at, start = to, to
			// Pairs after a moveto are linetos.
			cmd = 'L' | (cmd & 0x20)
			lastCmd = 'l'
		case 'l':
			to := pair()
			if err != nil {
				return out, err
			}
			line(to)
		case 'h':
			x := num()
			if err != nil {
				return out, err
			}
			if !rel {
				x -= at.X
			}
			line(Pt{at.X + x, at.Y})
		case 'v':
			y := num()
			if err != nil {
				return out, err
			}
			if !rel {
				y -= at.Y
			}
			line(Pt{at.X, at.Y + y})
		case 'c':
			c1, c2, to := pair(), pair(), pair()
			if err != nil {
				return out, err
			}
			cubic(c1, c2, to)
		case 's':
			c1 := at
			if prev == 'c' || prev == 's' {
				c1 = at.Add(at.Sub(lastCtl))
			}
			c2, to := pair(), pair()
			if err != nil {
				return out, err
			}
			cubic(c1, c2, to)
		case 'q':
			c, to := pair(), pair()
			if err != nil {
				return out, err
			}
			quad(at, c, to, cubic)
			lastCtl = c
		case 't':
			c := at
			if prev == 'q' || prev == 't' {
				c = at.Add(at.Sub(lastCtl))
			}
			to := pair()
			if err != nil {
				return out, err
			}
			quad(at, c, to, cubic)
			lastCtl = c
		case 'a':
			rx, ry, rot := num(), num(), num()
			var large, sweep bool
			if err == nil {
				large, err = p.flag()
			}
			if err == nil {
				sweep, err = p.flag()
			}
			to := pair()
			if err != nil {
				return out, err
			}
			arc(at, to, rx, ry, rot, large, sweep, line, cubic)
		case 'z':
			if at != start {
				line(start)
			}
			flush()
			at = start
			hasCmd = false
			lastCmd = 'z'
		}
	}
	flush()
	return out, nil
}

// isCommand reports whether c is an SVG path command letter.
func isCommand(c byte) bool {
	switch c | 0x20 {
	case 'm', 'l', 'h', 'v', 'c', 's', 'q', 't', 'a', 'z':
		return true
	}
	return false
}

// skip passes spaces and commas.
func (p *pathParser) skip() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r', ',':
			p.i++
		default:
			return
		}
	}
}

// number reads a number, which may run straight on from the one before, as in "1.5.5" or "2-3".
func (p *pathParser) number() (float32, error) {
	p.skip()
	j := p.i
	if j < len(p.s) && (p.s[j] == '-' || p.s[j] == '+') {
		j++
	}
	digits, dot := false, false
	for ; j < len(p.s); j++ {
		c := p.s[j]
		if c == '.' && !dot {
			dot = true
			continue
		}
		if c < '0' || c > '9' {
			break
		}
		digits = true
	}
	if digits && j < len(p.s) && (p.s[j] == 'e' || p.s[j] == 'E') {
		k := j + 1
		if k < len(p.s) && (p.s[k] == '-' || p.s[k] == '+') {
			k++
		}
		if k < len(p.s) && p.s[k] >= '0' && p.s[k] <= '9' {
			for j = k; j < len(p.s) && p.s[j] >= '0' && p.s[j] <= '9'; j++ {
			}
		}
	}
	if !digits {
		return 0, fmt.Errorf("path data wants a number at %d", p.i)
	}
	v, err := strconv.ParseFloat(p.s[p.i:j], 32)
	if err != nil {
		return 0, fmt.Errorf("path data: %w", err)
	}
	p.i = j
	return float32(v), nil
}

// flag reads an arc flag, a lone 0 or 1.
func (p *pathParser) flag() (bool, error) {
	p.skip()
	if p.i < len(p.s) && (p.s[p.i] == '0' || p.s[p.i] == '1') {
		p.i++
		return p.s[p.i-1] == '1', nil
	}
	return false, fmt.Errorf("path data wants an arc flag at %d", p.i)
}

// quad adds the quadratic Bézier from a through c to b as a cubic one.
func quad(a, c, b Pt, cubic func(c1, c2, to Pt)) {
	cubic(a.Lerp(c, 2.0/3), b.Lerp(c, 2.0/3), b)
}

// arc adds an SVG elliptical arc from a to b as cubic Béziers of at most a quarter turn each.
func arc(a, b Pt, rx, ry, rotDeg float32, large, sweep bool, line func(Pt), cubic func(c1, c2, to Pt)) {
	if a == b {
		return
	}
	rx, ry = float32(math.Abs(float64(rx))), float32(math.Abs(float64(ry)))
	if rx == 0 || ry == 0 {
		line(b)
		return
	}
	phi := float64(rotDeg) * math.Pi / 180
	sin, cos := math.Sincos(phi)
	// The endpoints in the ellipse's own frame, centred between them.
	dx, dy := float64(a.X-b.X)/2, float64(a.Y-b.Y)/2
	x1 := cos*dx + sin*dy
	y1 := -sin*dx + cos*dy
	rxf, ryf := float64(rx), float64(ry)
	// Radii too small to reach are scaled up until they just do.
	if l := x1*x1/(rxf*rxf) + y1*y1/(ryf*ryf); l > 1 {
		s := math.Sqrt(l)
		rxf, ryf = rxf*s, ryf*s
	}
	num := rxf*rxf*ryf*ryf - rxf*rxf*y1*y1 - ryf*ryf*x1*x1
	den := rxf*rxf*y1*y1 + ryf*ryf*x1*x1
	k := math.Sqrt(math.Max(0, num/den))
	if large == sweep {
		k = -k
	}
	cx1, cy1 := k*rxf*y1/ryf, -k*ryf*x1/rxf
	cx := cos*cx1 - sin*cy1 + float64(a.X+b.X)/2
	cy := sin*cx1 + cos*cy1 + float64(a.Y+b.Y)/2
	angle := func(ux, uy, vx, vy float64) float64 { return math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy) }
	t1 := angle(1, 0, (x1-cx1)/rxf, (y1-cy1)/ryf)
	dt := angle((x1-cx1)/rxf, (y1-cy1)/ryf, (-x1-cx1)/rxf, (-y1-cy1)/ryf)
	if !sweep && dt > 0 {
		dt -= 2 * math.Pi
	} else if sweep && dt < 0 {
		dt += 2 * math.Pi
	}
	n := int(math.Ceil(math.Abs(dt) / (math.Pi / 2)))
	step := dt / float64(n)
	// The control arm of a cubic that follows a unit circle through step radians.
	arm := 4.0 / 3 * math.Tan(step/4)
	onEllipse := func(t float64) (x, y, ex, ey float64) {
		st, ct := math.Sincos(t)
		x = cx + rxf*ct*cos - ryf*st*sin
		y = cy + rxf*ct*sin + ryf*st*cos
		// The derivative, for the control arms.
		ex = -rxf*st*cos - ryf*ct*sin
		ey = -rxf*st*sin + ryf*ct*cos
		return x, y, ex, ey
	}
	t := t1
	for i := range n {
		x0, y0, dx0, dy0 := onEllipse(t)
		x3, y3, dx3, dy3 := onEllipse(t + step)
		if i == n-1 {
			x3, y3 = float64(b.X), float64(b.Y)
		}
		c1 := Pt{float32(x0 + arm*dx0), float32(y0 + arm*dy0)}
		c2 := Pt{float32(x3 - arm*dx3), float32(y3 - arm*dy3)}
		cubic(c1, c2, Pt{float32(x3), float32(y3)})
		t += step
	}
}
