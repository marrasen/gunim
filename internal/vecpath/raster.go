package vecpath

import (
	"math"
	"slices"
)

// flatness is how far, in pixels, a flattened curve may stray from the true one.
const flatness = 0.05

// maxSteps is the most chords a curve is flattened into.
const maxSteps = 1024

// Polyline is a subpath flattened to points in pixels. Fill says it is filled as well as stroked.
type Polyline struct {
	Pts  []Pt
	Fill bool
}

// Flatten turns subpaths into polylines in pixels, mapping each point by m, and appends them to out.
func Flatten(subs []Subpath, m func(Pt) Pt, fill bool, out []Polyline) []Polyline {
	for _, sp := range subs {
		if len(sp) == 0 {
			continue
		}
		pl := Polyline{Pts: []Pt{m(sp[0].P[0])}, Fill: fill}
		for _, s := range sp {
			if !s.Curve {
				pl.Pts = append(pl.Pts, m(s.P[3]))
				continue
			}
			p0, p1, p2, p3 := m(s.P[0]), m(s.P[1]), m(s.P[2]), m(s.P[3])
			// A cubic's second difference bounds how far its chords stray.
			dd := max(Length(p0.Sub(p1.Scale2()).Add(p2)), Length(p1.Sub(p2.Scale2()).Add(p3)))
			// Past maxSteps a curve is far bigger than any mask, and its points are lost off the edges anyway.
			n := maxSteps
			if f := math.Ceil(math.Sqrt(float64(0.75 * dd / flatness))); f < maxSteps {
				n = max(1, int(f))
			}
			for i := 1; i <= n; i++ {
				t := float32(i) / float32(n)
				a, b, c := p0.Lerp(p1, t), p1.Lerp(p2, t), p2.Lerp(p3, t)
				d, e := a.Lerp(b, t), b.Lerp(c, t)
				pl.Pts = append(pl.Pts, d.Lerp(e, t))
			}
		}
		out = append(out, pl)
	}
	return out
}

// Scale2 is a doubled.
func (a Pt) Scale2() Pt { return Pt{2 * a.X, 2 * a.Y} }

// Length is a's distance from the origin.
func Length(a Pt) float32 { return float32(math.Hypot(float64(a.X), float64(a.Y))) }

// NewDistances returns w*h squared distances, each as far as can be, for [Segment] to lower.
func NewDistances(w, h int) []float32 {
	dist := make([]float32, w*h)
	for i := range dist {
		dist[i] = math.MaxFloat32
	}
	return dist
}

// Segment lowers each pixel's squared distance in dist to that from the segment a b, over the pixels a band hw
// either side of it can cover.
func Segment(dist []float32, w, h int, a, b Pt, hw float32) {
	r := hw + 1
	x0 := max(0, int(math.Floor(float64(min(a.X, b.X)-r))))
	y0 := max(0, int(math.Floor(float64(min(a.Y, b.Y)-r))))
	x1 := min(w, int(math.Ceil(float64(max(a.X, b.X)+r))))
	y1 := min(h, int(math.Ceil(float64(max(a.Y, b.Y)+r))))
	ab := b.Sub(a)
	ll := ab.X*ab.X + ab.Y*ab.Y
	for y := y0; y < y1; y++ {
		row := dist[y*w : (y+1)*w]
		cy := float32(y) + 0.5
		for x := x0; x < x1; x++ {
			ap := Pt{float32(x) + 0.5 - a.X, cy - a.Y}
			t := float32(0)
			if ll > 0 {
				t = min(max((ap.X*ab.X+ap.Y*ab.Y)/ll, 0), 1)
			}
			dx, dy := ap.X-t*ab.X, ap.Y-t*ab.Y
			if d := dx*dx + dy*dy; d < row[x] {
				row[x] = d
			}
		}
	}
}

// Inside marks the pixels whose centres lie inside the closed polygon pts, by the nonzero rule.
func Inside(inside []bool, w, h int, pts []Pt) {
	lo, hi := pts[0], pts[0]
	for _, p := range pts {
		lo = Pt{min(lo.X, p.X), min(lo.Y, p.Y)}
		hi = Pt{max(hi.X, p.X), max(hi.Y, p.Y)}
	}
	y0, y1 := max(0, int(lo.Y)), min(h, int(math.Ceil(float64(hi.Y))))
	x0, x1 := max(0, int(lo.X)), min(w, int(math.Ceil(float64(hi.X))))
	for y := y0; y < y1; y++ {
		cy := float32(y) + 0.5
		for x := x0; x < x1; x++ {
			cx := float32(x) + 0.5
			wind := 0
			for i := range pts {
				a, b := pts[i], pts[(i+1)%len(pts)]
				if (a.Y <= cy) == (b.Y <= cy) {
					continue
				}
				// Which side of the edge the centre lies on, counted by the edge's direction.
				if side := (b.X-a.X)*(cy-a.Y) - (cx-a.X)*(b.Y-a.Y); a.Y <= cy && side > 0 {
					wind++
				} else if a.Y > cy && side < 0 {
					wind--
				}
			}
			if wind != 0 {
				inside[y*w+x] = true
			}
		}
	}
}

// Coverage turns squared distances from strokes hw either side, and the pixels inside fills, into coverage bytes.
func Coverage(dist []float32, inside []bool, hw float32) []byte {
	out := make([]byte, len(dist))
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

// subRows is how many rows each pixel row of a fill is sampled at; across a row, coverage is exact.
const subRows = 5

// crossing is where an edge crosses a sampled row, and which way it runs.
type crossing struct {
	x   float32
	dir int
}

// Fill draws closed polygons into w by h coverage bytes, their edges smoothed: inside by the nonzero rule, or by
// the even-odd rule where evenOdd is set, so a ring's hole stays clear whichever way it runs.
func Fill(lines []Polyline, w, h int, evenOdd bool) []byte {
	acc := make([]float32, w*h)
	var xs []crossing
	for sy := range h * subRows {
		y := (float32(sy) + 0.5) / subRows
		xs = xs[:0]
		for _, l := range lines {
			pts := l.Pts
			for i := range pts {
				a, b := pts[i], pts[(i+1)%len(pts)]
				if (a.Y <= y) == (b.Y <= y) {
					continue
				}
				dir := 1
				if b.Y < a.Y {
					dir = -1
				}
				x := a.X + (y-a.Y)/(b.Y-a.Y)*(b.X-a.X)
				// An edge from a point past float32 crosses at no number; it is left out.
				if x-x != 0 {
					continue
				}
				xs = append(xs, crossing{x, dir})
			}
		}
		if len(xs) < 2 {
			continue
		}
		slices.SortFunc(xs, func(p, q crossing) int {
			switch {
			case p.x < q.x:
				return -1
			case p.x > q.x:
				return 1
			}
			return 0
		})
		row := acc[(sy/subRows)*w : (sy/subRows+1)*w]
		wind := 0
		for i := range len(xs) - 1 {
			wind += xs[i].dir
			in := wind != 0
			if evenOdd {
				in = i%2 == 0
			}
			if in {
				span(row, xs[i].x, xs[i+1].x, 1.0/subRows)
			}
		}
	}
	out := make([]byte, w*h)
	for i, c := range acc {
		out[i] = uint8(min(max(c, 0), 1)*255 + 0.5)
	}
	return out
}

// span adds weight to row for the run from x0 to x1, each pixel by the share of it the run covers.
func span(row []float32, x0, x1, weight float32) {
	x0, x1 = max(x0, 0), min(x1, float32(len(row)))
	if !(x0 < x1) {
		return
	}
	i0, i1 := int(x0), int(x1)
	if i0 == i1 {
		row[i0] += (x1 - x0) * weight
		return
	}
	row[i0] += (float32(i0+1) - x0) * weight
	for i := i0 + 1; i < i1; i++ {
		row[i] += weight
	}
	if i1 < len(row) {
		row[i1] += (x1 - float32(i1)) * weight
	}
}
