package icon

import (
	"fmt"

	"github.com/marrasen/gunim/internal/vecpath"
)

// outline is an icon parsed: its stroked subpaths and its filled ones, in drawing order.
type outline struct {
	strokes, fills []vecpath.Subpath
}

// parse reads SVG path data into subpaths. On an error it returns the subpaths read so far.
func parse(d string) ([]vecpath.Subpath, error) {
	subs, err := vecpath.Parse(d)
	if err != nil {
		return subs, fmt.Errorf("icon: %w", err)
	}
	return subs, nil
}

// rasterize draws an outline's strokes, width units wide, into w by h coverage bytes, as far as progress along
// their length. Each stroke is the union of round-capped segments, so its coverage comes from the distance to the
// nearest one.
func rasterize(o *outline, w, h int, width, progress float32) []byte {
	k := float32(min(w, h)) / 24
	off := vecpath.Pt{X: (float32(w) - 24*k) / 2, Y: (float32(h) - 24*k) / 2}
	px := func(p vecpath.Pt) vecpath.Pt { return vecpath.Pt{X: p.X*k + off.X, Y: p.Y*k + off.Y} }
	lines := vecpath.Flatten(o.strokes, px, false, nil)
	lines = vecpath.Flatten(o.fills, px, true, lines)

	var total float32
	for _, l := range lines {
		for i := 1; i < len(l.Pts); i++ {
			total += vecpath.Length(l.Pts[i].Sub(l.Pts[i-1]))
		}
	}
	left := progress * total
	hw := width * k / 2
	dist := vecpath.NewDistances(w, h)
	inside := make([]bool, w*h)
	for _, l := range lines {
		if progress < 1 && left <= 0 {
			break
		}
		whole := true
		for i := 1; i < len(l.Pts); i++ {
			a, b := l.Pts[i-1], l.Pts[i]
			n := vecpath.Length(b.Sub(a))
			if progress < 1 {
				if left <= 0 {
					whole = false
					break
				}
				if n > left {
					b, whole = a.Lerp(b, left/n), false
				}
				left -= n
			}
			vecpath.Segment(dist, w, h, a, b, hw)
		}
		if l.Fill && whole {
			vecpath.Inside(inside, w, h, l.Pts)
		}
	}
	return vecpath.Coverage(dist, inside, hw)
}
