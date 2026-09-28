package icon

import (
	"math"
)

// flatness is how far, in pixels, a flattened curve may stray from the true one.
const flatness = 0.05

// polyline is a subpath flattened to points in pixels. fill says it is filled as well as stroked.
type polyline struct {
	pts  []pt
	fill bool
}

// flatten turns subpaths into polylines in pixels, mapping a grid point p to p*k+off.
func flatten(subs []subpath, k float32, off pt, fill bool, out []polyline) []polyline {
	for _, sp := range subs {
		px := func(p pt) pt { return pt{p.x*k + off.x, p.y*k + off.y} }
		pl := polyline{pts: []pt{px(sp[0].p[0])}, fill: fill}
		for _, s := range sp {
			if !s.curve {
				pl.pts = append(pl.pts, px(s.p[3]))
				continue
			}
			p0, p1, p2, p3 := px(s.p[0]), px(s.p[1]), px(s.p[2]), px(s.p[3])
			// A cubic's second difference bounds how far its chords stray.
			dd := max(length(p0.sub(p1.scale2()).add(p2)), length(p1.sub(p2.scale2()).add(p3)))
			n := max(1, int(math.Ceil(math.Sqrt(float64(0.75*dd/flatness)))))
			for i := 1; i <= n; i++ {
				t := float32(i) / float32(n)
				a, b, c := p0.lerp(p1, t), p1.lerp(p2, t), p2.lerp(p3, t)
				d, e := a.lerp(b, t), b.lerp(c, t)
				pl.pts = append(pl.pts, d.lerp(e, t))
			}
		}
		out = append(out, pl)
	}
	return out
}

func (a pt) scale2() pt { return pt{2 * a.x, 2 * a.y} }

func length(a pt) float32 { return float32(math.Hypot(float64(a.x), float64(a.y))) }

// rasterize draws an outline's strokes, width units wide, into w by h coverage bytes, as far as progress along
// their length. Each stroke is the union of round-capped segments, so its coverage comes from the distance to the
// nearest one.
func rasterize(o *outline, w, h int, width, progress float32) []byte {
	k := float32(min(w, h)) / 24
	off := pt{(float32(w) - 24*k) / 2, (float32(h) - 24*k) / 2}
	lines := flatten(o.strokes, k, off, false, nil)
	lines = flatten(o.fills, k, off, true, lines)

	var total float32
	for _, l := range lines {
		for i := 1; i < len(l.pts); i++ {
			total += length(l.pts[i].sub(l.pts[i-1]))
		}
	}
	left := progress * total
	hw := width * k / 2
	dist := make([]float32, w*h)
	for i := range dist {
		dist[i] = math.MaxFloat32
	}
	inside := make([]bool, w*h)
	for _, l := range lines {
		if progress < 1 && left <= 0 {
			break
		}
		whole := true
		for i := 1; i < len(l.pts); i++ {
			a, b := l.pts[i-1], l.pts[i]
			n := length(b.sub(a))
			if progress < 1 {
				if left <= 0 {
					whole = false
					break
				}
				if n > left {
					b, whole = a.lerp(b, left/n), false
				}
				left -= n
			}
			segment(dist, w, h, a, b, hw)
		}
		if l.fill && whole {
			fill(inside, w, h, l.pts)
		}
	}

	out := make([]byte, w*h)
	for i, d := range dist {
		if d == math.MaxFloat32 && !inside[i] {
			continue
		}
		d = float32(math.Sqrt(float64(d)))
		// The share of a pixel a band 2*hw wide covers, the pixel d from its middle.
		c := min(d+0.5, hw) - max(d-0.5, -hw)
		if inside[i] {
			c = max(c, d+0.5)
		}
		out[i] = uint8(min(max(c, 0), 1)*255 + 0.5)
	}
	return out
}

// segment lowers each pixel's squared distance in dist to that from the segment a b, over the pixels it can cover.
func segment(dist []float32, w, h int, a, b pt, hw float32) {
	r := hw + 1
	x0 := max(0, int(math.Floor(float64(min(a.x, b.x)-r))))
	y0 := max(0, int(math.Floor(float64(min(a.y, b.y)-r))))
	x1 := min(w, int(math.Ceil(float64(max(a.x, b.x)+r))))
	y1 := min(h, int(math.Ceil(float64(max(a.y, b.y)+r))))
	ab := b.sub(a)
	ll := ab.x*ab.x + ab.y*ab.y
	for y := y0; y < y1; y++ {
		row := dist[y*w : (y+1)*w]
		cy := float32(y) + 0.5
		for x := x0; x < x1; x++ {
			ap := pt{float32(x) + 0.5 - a.x, cy - a.y}
			t := float32(0)
			if ll > 0 {
				t = min(max((ap.x*ab.x+ap.y*ab.y)/ll, 0), 1)
			}
			dx, dy := ap.x-t*ab.x, ap.y-t*ab.y
			if d := dx*dx + dy*dy; d < row[x] {
				row[x] = d
			}
		}
	}
}

// fill marks the pixels whose centres lie inside the closed polygon pts, by the nonzero rule.
func fill(inside []bool, w, h int, pts []pt) {
	lo, hi := pts[0], pts[0]
	for _, p := range pts {
		lo = pt{min(lo.x, p.x), min(lo.y, p.y)}
		hi = pt{max(hi.x, p.x), max(hi.y, p.y)}
	}
	y0, y1 := max(0, int(lo.y)), min(h, int(math.Ceil(float64(hi.y))))
	x0, x1 := max(0, int(lo.x)), min(w, int(math.Ceil(float64(hi.x))))
	for y := y0; y < y1; y++ {
		cy := float32(y) + 0.5
		for x := x0; x < x1; x++ {
			cx := float32(x) + 0.5
			wind := 0
			for i := range pts {
				a, b := pts[i], pts[(i+1)%len(pts)]
				if (a.y <= cy) == (b.y <= cy) {
					continue
				}
				// Which side of the edge the centre lies on, counted by the edge's direction.
				if side := (b.x-a.x)*(cy-a.y) - (cx-a.x)*(b.y-a.y); a.y <= cy && side > 0 {
					wind++
				} else if a.y > cy && side < 0 {
					wind--
				}
			}
			if wind != 0 {
				inside[y*w+x] = true
			}
		}
	}
}
