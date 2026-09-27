package text

import (
	"math"
	"slices"
	"sort"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
)

// hintMax is the largest device size, in pixels, that glyphs are
// hinted at.
const hintMax = 36

// A zone is a height that the edges of many glyphs share, such as the
// baseline or the x-height, in font units. ref is the height of flat
// edges, and over the height round ones overshoot to. A top zone takes
// the tops of strokes, and the others their bottoms.
type zone struct {
	ref, over float32
	top       bool
}

// zonesLocked returns f's zones, measuring them from its Latin letters
// on first use. A face with none of them has the baseline alone. It
// runs with mu held.
func (f *Face) zonesLocked() []zone {
	if f.zonesSet {
		return f.zones
	}
	f.zonesSet = true
	extent := func(r rune) (top, bottom float32, ok bool) {
		gid, ok := f.face.NominalGlyph(r)
		if !ok {
			return 0, 0, false
		}
		e, ok := f.face.GlyphExtents(gid)
		if !ok || e.Height == 0 {
			return 0, 0, false
		}
		return e.YBearing, e.YBearing + e.Height, true
	}
	base := zone{}
	if _, b, ok := extent('o'); ok && b < 0 && b > -0.05*f.upem {
		base.over = b
	}
	f.zones = append(f.zones, base)
	for _, letters := range [][2]rune{{'x', 'o'}, {'H', 'O'}, {'h', 'h'}} {
		t, _, ok := extent(letters[0])
		if !ok || t <= 0 {
			continue
		}
		z := zone{ref: t, over: t, top: true}
		if o, _, ok := extent(letters[1]); ok && o > t && o-t < 0.05*f.upem {
			z.over = o
		}
		f.zones = append(f.zones, z)
	}
	if _, b, ok := extent('p'); ok && b < 0 {
		f.zones = append(f.zones, zone{ref: b, over: b})
	}
	return f.zones
}

// A hinter maps a glyph's heights in font units to heights in device
// pixels, y up, with its horizontal edges on whole pixels. ys are the
// edges it placed, in order, and ts where each went.
type hinter struct {
	ys, ts []float32
	scale  float32
}

// edge is a horizontal edge of a glyph: a flat stretch of outline, or
// the flat top or bottom of a curve. top says the ink is below it.
type edge struct {
	y, x0, x1 float32
	top       bool
}

// newHinter finds o's horizontal edges and places them: edges in a
// zone on the zone's pixel, both edges of a stroke a whole number of
// pixels apart, and the rest on the nearest pixel. An edge that would
// cross or close up on one already placed stays where it was.
func newHinter(o font.GlyphOutline, zones []zone, upem, scale float32) hinter {
	h := hinter{scale: scale}
	edges := findEdges(o, upem)
	target := make([]float32, len(edges))
	placed := make([]bool, len(edges))

	tol := 0.015 * upem
	for i, e := range edges {
		for _, z := range zones {
			lo, hi := min(z.ref, z.over), max(z.ref, z.over)
			if z.top != e.top || e.y < lo-tol || e.y > hi+tol {
				continue
			}
			ref := float32(math.Round(float64(z.ref * scale)))
			if z.top {
				// Round the x-height and the like up a little sooner
				// than down, which keeps small text open.
				ref = float32(math.Floor(float64(z.ref*scale) + 0.6))
			}
			target[i], placed[i] = ref+float32(math.Round(float64((e.y-z.ref)*scale))), true
			h.add(e.y, target[i])
			break
		}
	}

	maxStem := 0.15 * upem
	slack := 0.02 * upem
	for i, t := range edges {
		if !t.top {
			continue
		}
		b := -1
		for j, e := range edges {
			if e.top || e.y >= t.y || t.y-e.y > maxStem || e.x0 > t.x1+slack || t.x0 > e.x1+slack {
				continue
			}
			if b < 0 || e.y > edges[b].y {
				b = j
			}
		}
		if b < 0 {
			continue
		}
		width := max(1, float32(math.Round(float64((t.y-edges[b].y)*scale))))
		switch {
		case placed[i] && placed[b]:
			continue
		case placed[i]:
			target[b] = target[i] - width
		case placed[b]:
			target[i] = target[b] + width
		default:
			mid := (t.y + edges[b].y) / 2 * scale
			target[b] = float32(math.Round(float64(mid - width/2)))
			target[i] = target[b] + width
		}
		placed[i], placed[b] = true, true
		h.add(edges[b].y, target[b])
		h.add(t.y, target[i])
	}

	for i, e := range edges {
		if !placed[i] {
			h.add(e.y, float32(math.Round(float64(e.y*scale))))
		}
	}
	return h
}

// add places the edge at y on pixel t, unless that would cross an edge
// already placed, or close the gap to one more than half a pixel away.
func (h *hinter) add(y, t float32) {
	i := sort.Search(len(h.ys), func(i int) bool { return h.ys[i] >= y })
	if i < len(h.ys) && h.ys[i] == y {
		return
	}
	keeps := func(yLo, tLo, yHi, tHi float32) bool {
		return tHi > tLo || tHi == tLo && (yHi-yLo)*h.scale < 0.5
	}
	if i > 0 && !keeps(h.ys[i-1], h.ts[i-1], y, t) {
		return
	}
	if i < len(h.ys) && !keeps(y, t, h.ys[i], h.ts[i]) {
		return
	}
	h.ys = slices.Insert(h.ys, i, y)
	h.ts = slices.Insert(h.ts, i, t)
}

// mapY returns where height y, in font units, lands in device pixels:
// on its edge's pixel, in proportion between two placed edges, and
// moved with the nearest one beyond them.
func (h *hinter) mapY(y float32) float32 {
	n := len(h.ys)
	if n == 0 {
		return y * h.scale
	}
	i := sort.Search(n, func(i int) bool { return h.ys[i] >= y })
	switch i {
	case 0:
		return h.ts[0] + (y-h.ys[0])*h.scale
	case n:
		return h.ts[n-1] + (y-h.ys[n-1])*h.scale
	}
	y0, y1, t0, t1 := h.ys[i-1], h.ys[i], h.ts[i-1], h.ts[i]
	return t0 + (y-y0)*(t1-t0)/(y1-y0)
}

// outlinePoint is a point of a contour, on the curve or a control point.
type outlinePoint struct {
	x, y float32
	on   bool
}

// findEdges returns o's horizontal edges, merged where two share a
// height and a side.
func findEdges(o font.GlyphOutline, upem float32) []edge {
	var contours [][]outlinePoint
	for _, seg := range o.Segments {
		a := seg.Args
		if seg.Op == opentype.SegmentOpMoveTo || len(contours) == 0 {
			contours = append(contours, nil)
		}
		c := &contours[len(contours)-1]
		switch seg.Op {
		case opentype.SegmentOpMoveTo, opentype.SegmentOpLineTo:
			*c = append(*c, outlinePoint{a[0].X, a[0].Y, true})
		case opentype.SegmentOpQuadTo:
			*c = append(*c, outlinePoint{a[0].X, a[0].Y, false}, outlinePoint{a[1].X, a[1].Y, true})
		case opentype.SegmentOpCubeTo:
			*c = append(*c, outlinePoint{a[0].X, a[0].Y, false}, outlinePoint{a[1].X, a[1].Y, false},
				outlinePoint{a[2].X, a[2].Y, true})
		}
	}

	// The sign of the area says which way the outer contours run, and
	// so which side of an edge the ink is on.
	var area float32
	for i, c := range contours {
		if n := len(c); n > 1 && c[0] == c[n-1] {
			contours[i] = c[:n-1]
		}
		c = contours[i]
		for j, p := range c {
			q := c[(j+1)%len(c)]
			area += p.x*q.y - q.x*p.y
		}
	}
	clockwise := area < 0

	flat := func(dx, dy float32) bool { return abs(dy) <= 0.07*abs(dx) }
	minLen := 0.01 * upem
	var edges []edge
	for _, c := range contours {
		n := len(c)
		for i, p := range c {
			if !p.on {
				continue
			}
			next := c[(i+1)%n]
			if dx := next.x - p.x; next.on && abs(dx) > minLen && flat(dx, next.y-p.y) {
				edges = append(edges, edge{
					y: (p.y + next.y) / 2, x0: min(p.x, next.x), x1: max(p.x, next.x), top: (dx > 0) == clockwise,
				})
			}
			// The top or bottom of a curve: the control points either
			// side sit level with it, one to the left and one to the
			// right.
			prev := c[(i-1+n)%n]
			if prev.on && next.on {
				continue
			}
			din, dout := p.x-prev.x, next.x-p.x
			if din*dout <= 0 || !flat(din, p.y-prev.y) || !flat(dout, next.y-p.y) {
				continue
			}
			edges = append(edges, edge{
				y: p.y, x0: min(prev.x, next.x), x1: max(prev.x, next.x), top: (dout > 0) == clockwise,
			})
		}
	}

	slices.SortFunc(edges, func(a, b edge) int {
		switch {
		case a.y < b.y:
			return -1
		case a.y > b.y:
			return 1
		}
		return 0
	})
	merged := edges[:0]
	for _, e := range edges {
		if k := len(merged) - 1; k >= 0 && merged[k].top == e.top && e.y-merged[k].y < 0.5 {
			merged[k].x0, merged[k].x1 = min(merged[k].x0, e.x0), max(merged[k].x1, e.x1)
			continue
		}
		merged = append(merged, e)
	}
	return merged
}
