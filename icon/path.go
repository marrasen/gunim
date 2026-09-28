package icon

import (
	"fmt"
	"math"
	"strconv"
)

// pt is a point on the icon's grid, or in pixels once scaled.
type pt struct{ x, y float32 }

func (a pt) add(b pt) pt             { return pt{a.x + b.x, a.y + b.y} }
func (a pt) sub(b pt) pt             { return pt{a.x - b.x, a.y - b.y} }
func (a pt) lerp(b pt, t float32) pt { return pt{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t} }

// seg is a line from p[0] to p[3], or a cubic Bézier through p[0] to p[3] when curve is set.
type seg struct {
	p     [4]pt
	curve bool
}

// subpath is a run of joined segments.
type subpath []seg

// outline is an icon parsed: its stroked subpaths and its filled ones, in drawing order.
type outline struct {
	strokes, fills []subpath
}

// pathParser reads SVG path data.
type pathParser struct {
	s string
	i int
}

// parse reads SVG path data into subpaths, with arcs and quadratic curves turned into cubic ones. On an error it
// returns the subpaths read so far.
func parse(d string) ([]subpath, error) {
	p := pathParser{s: d}
	var (
		out       []subpath
		cur       subpath
		at, start pt
		lastCtl   pt
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
	line := func(to pt) {
		cur = append(cur, seg{p: [4]pt{at, at, to, to}})
		at = to
	}
	cubic := func(c1, c2, to pt) {
		cur = append(cur, seg{p: [4]pt{at, c1, c2, to}, curve: true})
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
			return out, fmt.Errorf("icon: path data has %q at %d where a command belongs", c, p.i)
		}
		rel := cmd >= 'a'
		base := pt{}
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
		pair := func() pt { x := num(); return base.add(pt{x, num()}) }
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
				x -= at.x
			}
			line(pt{at.x + x, at.y})
		case 'v':
			y := num()
			if err != nil {
				return out, err
			}
			if !rel {
				y -= at.y
			}
			line(pt{at.x, at.y + y})
		case 'c':
			c1, c2, to := pair(), pair(), pair()
			if err != nil {
				return out, err
			}
			cubic(c1, c2, to)
		case 's':
			c1 := at
			if prev == 'c' || prev == 's' {
				c1 = at.add(at.sub(lastCtl))
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
				c = at.add(at.sub(lastCtl))
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
		return 0, fmt.Errorf("icon: path data wants a number at %d", p.i)
	}
	v, err := strconv.ParseFloat(p.s[p.i:j], 32)
	if err != nil {
		return 0, fmt.Errorf("icon: path data: %w", err)
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
	return false, fmt.Errorf("icon: path data wants an arc flag at %d", p.i)
}

// quad adds the quadratic Bézier from a through c to b as a cubic one.
func quad(a, c, b pt, cubic func(c1, c2, to pt)) {
	cubic(a.lerp(c, 2.0/3), b.lerp(c, 2.0/3), b)
}

// arc adds an SVG elliptical arc from a to b as cubic Béziers of at most a quarter turn each.
func arc(a, b pt, rx, ry, rotDeg float32, large, sweep bool, line func(pt), cubic func(c1, c2, to pt)) {
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
	dx, dy := float64(a.x-b.x)/2, float64(a.y-b.y)/2
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
	cx := cos*cx1 - sin*cy1 + float64(a.x+b.x)/2
	cy := sin*cx1 + cos*cy1 + float64(a.y+b.y)/2
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
			x3, y3 = float64(b.x), float64(b.y)
		}
		c1 := pt{float32(x0 + arm*dx0), float32(y0 + arm*dy0)}
		c2 := pt{float32(x3 - arm*dx3), float32(y3 - arm*dy3)}
		cubic(c1, c2, pt{float32(x3), float32(y3)})
		t += step
	}
}
