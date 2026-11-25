package widget

import "math"

// Box-drawing and block-element characters, and the Powerline separators, are drawn in code rather than taken from a
// font, as gridterm draws them.
//
// A font's box glyphs are designed for that font's own advance width and
// line height, so in a terminal's cell box the strokes stop short of the
// edges and adjacent cells do not join up. Every table and framed TUI
// then renders as a field of disconnected ticks. Drawing them to the
// exact cell size is the only way to make the lines meet. And where a font
// lacks them, as every font in a browser does, they would not show at all.

// stroke weights used by the line-drawing characters.
const (
	wNone uint8 = iota
	wLight
	wHeavy
	wDouble
)

// boxLine describes one line-drawing character as the weight of its four
// arms, in the order up, right, down, left.
type boxLine struct {
	up, right, down, left uint8
	// dashes, when non-zero, splits the line into that many segments across the cell.
	dashes int
}

// boxLines maps the line-drawing characters made of straight arms. The rest of the block, the rounded corners and
// the diagonals, are drawn by their shapes.
var boxLines = map[rune]boxLine{
	// Horizontal and vertical, plain and dashed.
	0x2500: {right: wLight, left: wLight},
	0x2501: {right: wHeavy, left: wHeavy},
	0x2502: {up: wLight, down: wLight},
	0x2503: {up: wHeavy, down: wHeavy},
	0x2504: {right: wLight, left: wLight, dashes: 3},
	0x2505: {right: wHeavy, left: wHeavy, dashes: 3},
	0x2506: {up: wLight, down: wLight, dashes: 3},
	0x2507: {up: wHeavy, down: wHeavy, dashes: 3},
	0x2508: {right: wLight, left: wLight, dashes: 4},
	0x2509: {right: wHeavy, left: wHeavy, dashes: 4},
	0x250a: {up: wLight, down: wLight, dashes: 4},
	0x250b: {up: wHeavy, down: wHeavy, dashes: 4},
	0x254c: {right: wLight, left: wLight, dashes: 2},
	0x254d: {right: wHeavy, left: wHeavy, dashes: 2},
	0x254e: {up: wLight, down: wLight, dashes: 2},
	0x254f: {up: wHeavy, down: wHeavy, dashes: 2},

	// Corners.
	0x250c: {down: wLight, right: wLight},
	0x250d: {down: wLight, right: wHeavy},
	0x250e: {down: wHeavy, right: wLight},
	0x250f: {down: wHeavy, right: wHeavy},
	0x2510: {down: wLight, left: wLight},
	0x2511: {down: wLight, left: wHeavy},
	0x2512: {down: wHeavy, left: wLight},
	0x2513: {down: wHeavy, left: wHeavy},
	0x2514: {up: wLight, right: wLight},
	0x2515: {up: wLight, right: wHeavy},
	0x2516: {up: wHeavy, right: wLight},
	0x2517: {up: wHeavy, right: wHeavy},
	0x2518: {up: wLight, left: wLight},
	0x2519: {up: wLight, left: wHeavy},
	0x251a: {up: wHeavy, left: wLight},
	0x251b: {up: wHeavy, left: wHeavy},

	// Tees, left.
	0x251c: {up: wLight, down: wLight, right: wLight},
	0x251d: {up: wLight, down: wLight, right: wHeavy},
	0x251e: {up: wHeavy, down: wLight, right: wLight},
	0x251f: {up: wLight, down: wHeavy, right: wLight},
	0x2520: {up: wHeavy, down: wHeavy, right: wLight},
	0x2521: {up: wHeavy, down: wLight, right: wHeavy},
	0x2522: {up: wLight, down: wHeavy, right: wHeavy},
	0x2523: {up: wHeavy, down: wHeavy, right: wHeavy},

	// Tees, right.
	0x2524: {up: wLight, down: wLight, left: wLight},
	0x2525: {up: wLight, down: wLight, left: wHeavy},
	0x2526: {up: wHeavy, down: wLight, left: wLight},
	0x2527: {up: wLight, down: wHeavy, left: wLight},
	0x2528: {up: wHeavy, down: wHeavy, left: wLight},
	0x2529: {up: wHeavy, down: wLight, left: wHeavy},
	0x252a: {up: wLight, down: wHeavy, left: wHeavy},
	0x252b: {up: wHeavy, down: wHeavy, left: wHeavy},

	// Tees, down.
	0x252c: {left: wLight, right: wLight, down: wLight},
	0x252d: {left: wHeavy, right: wLight, down: wLight},
	0x252e: {left: wLight, right: wHeavy, down: wLight},
	0x252f: {left: wHeavy, right: wHeavy, down: wLight},
	0x2530: {left: wLight, right: wLight, down: wHeavy},
	0x2531: {left: wHeavy, right: wLight, down: wHeavy},
	0x2532: {left: wLight, right: wHeavy, down: wHeavy},
	0x2533: {left: wHeavy, right: wHeavy, down: wHeavy},

	// Tees, up.
	0x2534: {left: wLight, right: wLight, up: wLight},
	0x2535: {left: wHeavy, right: wLight, up: wLight},
	0x2536: {left: wLight, right: wHeavy, up: wLight},
	0x2537: {left: wHeavy, right: wHeavy, up: wLight},
	0x2538: {left: wLight, right: wLight, up: wHeavy},
	0x2539: {left: wHeavy, right: wLight, up: wHeavy},
	0x253a: {left: wLight, right: wHeavy, up: wHeavy},
	0x253b: {left: wHeavy, right: wHeavy, up: wHeavy},

	// Crosses.
	0x253c: {up: wLight, right: wLight, down: wLight, left: wLight},
	0x253d: {up: wLight, right: wLight, down: wLight, left: wHeavy},
	0x253e: {up: wLight, right: wHeavy, down: wLight, left: wLight},
	0x253f: {up: wLight, right: wHeavy, down: wLight, left: wHeavy},
	0x2540: {up: wHeavy, right: wLight, down: wLight, left: wLight},
	0x2541: {up: wLight, right: wLight, down: wHeavy, left: wLight},
	0x2542: {up: wHeavy, right: wLight, down: wHeavy, left: wLight},
	0x2543: {up: wHeavy, right: wLight, down: wLight, left: wHeavy},
	0x2544: {up: wHeavy, right: wHeavy, down: wLight, left: wLight},
	0x2545: {up: wLight, right: wLight, down: wHeavy, left: wHeavy},
	0x2546: {up: wLight, right: wHeavy, down: wHeavy, left: wLight},
	0x2547: {up: wHeavy, right: wHeavy, down: wLight, left: wHeavy},
	0x2548: {up: wLight, right: wHeavy, down: wHeavy, left: wHeavy},
	0x2549: {up: wHeavy, right: wLight, down: wHeavy, left: wHeavy},
	0x254a: {up: wHeavy, right: wHeavy, down: wHeavy, left: wLight},
	0x254b: {up: wHeavy, right: wHeavy, down: wHeavy, left: wHeavy},

	// Double lines.
	0x2550: {right: wDouble, left: wDouble},
	0x2551: {up: wDouble, down: wDouble},
	0x2552: {down: wLight, right: wDouble},
	0x2553: {down: wDouble, right: wLight},
	0x2554: {down: wDouble, right: wDouble},
	0x2555: {down: wLight, left: wDouble},
	0x2556: {down: wDouble, left: wLight},
	0x2557: {down: wDouble, left: wDouble},
	0x2558: {up: wLight, right: wDouble},
	0x2559: {up: wDouble, right: wLight},
	0x255a: {up: wDouble, right: wDouble},
	0x255b: {up: wLight, left: wDouble},
	0x255c: {up: wDouble, left: wLight},
	0x255d: {up: wDouble, left: wDouble},
	0x255e: {up: wLight, down: wLight, right: wDouble},
	0x255f: {up: wDouble, down: wDouble, right: wLight},
	0x2560: {up: wDouble, down: wDouble, right: wDouble},
	0x2561: {up: wLight, down: wLight, left: wDouble},
	0x2562: {up: wDouble, down: wDouble, left: wLight},
	0x2563: {up: wDouble, down: wDouble, left: wDouble},
	0x2564: {left: wDouble, right: wDouble, down: wLight},
	0x2565: {left: wLight, right: wLight, down: wDouble},
	0x2566: {left: wDouble, right: wDouble, down: wDouble},
	0x2567: {left: wDouble, right: wDouble, up: wLight},
	0x2568: {left: wLight, right: wLight, up: wDouble},
	0x2569: {left: wDouble, right: wDouble, up: wDouble},
	0x256a: {left: wDouble, right: wDouble, up: wLight, down: wLight},
	0x256b: {left: wLight, right: wLight, up: wDouble, down: wDouble},
	0x256c: {left: wDouble, right: wDouble, up: wDouble, down: wDouble},

	// Half lines and mixed-weight stubs.
	0x2574: {left: wLight},
	0x2575: {up: wLight},
	0x2576: {right: wLight},
	0x2577: {down: wLight},
	0x2578: {left: wHeavy},
	0x2579: {up: wHeavy},
	0x257a: {right: wHeavy},
	0x257b: {down: wHeavy},
	0x257c: {left: wLight, right: wHeavy},
	0x257d: {up: wLight, down: wHeavy},
	0x257e: {left: wHeavy, right: wLight},
	0x257f: {up: wHeavy, down: wLight},
}

// firstDrawn is the first character drawn in code.
const firstDrawn = 0x2500

// drawnInCode reports whether r may be one of the characters drawn in code.
func drawnInCode(r rune) bool {
	return r >= firstDrawn && r <= 0x259f || r >= 0xe0b0 && r <= 0xe0bf
}

// boxDrawing returns what r covers of w by h device pixels, one byte a pixel from the top, or nil for a character the
// font draws.
func boxDrawing(r rune, w, h int) []byte {
	if w <= 0 || h <= 0 {
		return nil
	}
	m := &mask{pix: make([]byte, w*h), w: w, h: h}
	if bl, ok := boxLines[r]; ok {
		m.line(bl)
		return m.pix
	}
	switch {
	case r >= 0x256d && r <= 0x2570:
		// Down and right, down and left, up and left, up and right.
		m.corner(r == 0x256d || r == 0x256e, r == 0x256d || r == 0x2570)
	case r >= 0x2571 && r <= 0x2573:
		m.diagonals(r != 0x2572, r != 0x2571)
	case r >= 0x2580 && r <= 0x259f:
		m.block(r)
	case r >= 0xe0b0 && r <= 0xe0bf:
		m.separator(r)
	default:
		return nil
	}
	return m.pix
}

// mask gathers what a character covers of a w by h cell.
type mask struct {
	pix  []byte
	w, h int
}

// fill covers the rectangle by alpha, as far as it lies in the cell.
func (m *mask) fill(x0, y0, x1, y1 int, alpha uint8) {
	x0, y0 = max(x0, 0), max(y0, 0)
	x1, y1 = min(x1, m.w), min(y1, m.h)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			m.pix[y*m.w+x] = max(m.pix[y*m.w+x], alpha)
		}
	}
}

// cover covers the points inside holds, given in pixels from the cell's top left, antialiased.
func (m *mask) cover(inside func(x, y float64) bool) {
	const n = 8
	for y := range m.h {
		for x := range m.w {
			in := 0
			for j := range n {
				for i := range n {
					if inside(float64(x)+(float64(i)+0.5)/n, float64(y)+(float64(j)+0.5)/n) {
						in++
					}
				}
			}
			m.pix[y*m.w+x] = max(m.pix[y*m.w+x], uint8(in*255/(n*n)))
		}
	}
}

// span is a stretch of pixels across a stroke, from lo up to hi.
type span struct{ lo, hi int }

// strokeWidth is how wide a light line is, in a cell h pixels tall. A heavy line is twice as wide, and a double line
// two light ones with a light one's room between them.
func strokeWidth(h int) int { return max(h/14, 1) }

// strokes returns the stroke or strokes of a line of a weight that runs through the middle of size pixels: the lower
// and the higher of a double line, and otherwise the one stroke twice.
//
// Rounding down keeps a one-pixel stroke on the same side of the middle in every cell, which is what stops a long
// rule from looking ragged.
func strokes(weight uint8, size, t int) [2]span {
	switch weight {
	case wHeavy:
		t *= 2
	case wDouble:
		lo := size/2 - t/2
		return [2]span{{lo - t, lo}, {lo + t, lo + 2*t}}
	}
	lo := size/2 - t/2
	return [2]span{{lo, lo + t}, {lo, lo + t}}
}

// line draws a character made of straight arms. An arm runs from the cell's edge to where it meets the others, so the
// same character in the next cell continues the line exactly, and corners close.
func (m *mask) line(bl boxLine) {
	t := strokeWidth(m.h)
	if bl.dashes > 0 {
		m.dashed(bl, t)
		return
	}
	for side, s := range strokes(bl.up, m.w, t) {
		end := armEnd(true, bl.up, bl.down, [2]uint8{bl.left, bl.right}, side, m.h, t)
		m.fill(s.lo, 0, s.hi, end, 255)
	}
	for side, s := range strokes(bl.down, m.w, t) {
		end := armEnd(false, bl.down, bl.up, [2]uint8{bl.left, bl.right}, side, m.h, t)
		m.fill(s.lo, end, s.hi, m.h, 255)
	}
	for side, s := range strokes(bl.left, m.h, t) {
		end := armEnd(true, bl.left, bl.right, [2]uint8{bl.up, bl.down}, side, m.w, t)
		m.fill(0, s.lo, end, s.hi, 255)
	}
	for side, s := range strokes(bl.right, m.h, t) {
		end := armEnd(false, bl.right, bl.left, [2]uint8{bl.up, bl.down}, side, m.w, t)
		m.fill(end, s.lo, m.w, s.hi, 255)
	}
}

// armEnd returns where an arm ends, in pixels along it. low says it comes from the cell's top or left rather than
// its bottom or right, and size is the cell's height or width along it. opposite is the weight of the arm on from it,
// and across those of the arms to its sides, the top or left one first. A double arm's stroke on that side is 0, and
// the other 1.
func armEnd(low bool, weight, opposite uint8, across [2]uint8, side, size, t int) int {
	mid := size / 2
	if weight == wNone {
		// No arm, so it ends at its own edge.
		if low {
			return 0
		}
		return size
	}
	// far is the far edge of a line across, and near the far edge of the first stroke of it the arm comes to, which
	// is the same but for a double line.
	far := func(w uint8) int {
		s := strokes(w, size, t)
		if low {
			return s[1].hi
		}
		return s[0].lo
	}
	near := func(w uint8) int {
		s := strokes(w, size, t)
		if low {
			return s[0].hi
		}
		return s[1].lo
	}
	if weight == wDouble {
		// Each stroke of a double arm meets what is on its own side.
		on, off := across[side], across[1-side]
		switch {
		case on == wDouble:
			return near(on)
		case on != wNone:
			return far(on)
		case opposite != wNone:
			return mid
		case off != wNone:
			return far(off)
		}
		return mid
	}
	// The widest single line across closes the corner with this one.
	if w := max(single(across[0]), single(across[1])); w != wNone {
		return far(w)
	}
	switch {
	case opposite != wNone:
		return mid
	case across[0] == wDouble && across[1] == wDouble:
		return near(wDouble)
	case across[0] == wDouble || across[1] == wDouble:
		return far(wDouble)
	}
	return mid
}

// single returns a weight but for a double line's, which it returns as none.
func single(weight uint8) uint8 {
	if weight == wDouble {
		return wNone
	}
	return weight
}

// dashed draws a line right across the cell in dashes, all as long as each other. The gaps between them, in the cell and
// on to the next cell's, differ by a pixel at most.
func (m *mask) dashed(bl boxLine, t int) {
	size := m.w
	if bl.up != wNone {
		size = m.h
	}
	n := bl.dashes
	// Two thirds dash to one gap, to the nearest pixel, with a pixel of gap where there is room for one.
	long := max(min((4*size+3*n)/(6*n), size/n-1), 1)
	for i := range n {
		lo := size*i/n + (size/n-long)/2
		hi := lo + long
		if bl.up != wNone {
			s := strokes(bl.up, m.w, t)[0]
			m.fill(s.lo, lo, s.hi, hi, 255)
			continue
		}
		s := strokes(bl.left, m.h, t)[0]
		m.fill(lo, s.lo, hi, s.hi, 255)
	}
}

// corner draws a rounded corner between the light lines down, or up, and to the right, or the left.
func (m *mask) corner(down, right bool) {
	v, h := strokes(wLight, m.w, strokeWidth(m.h))[0], strokes(wLight, m.h, strokeWidth(m.h))[0]
	x, y := float64(v.lo+v.hi)/2, float64(h.lo+h.hi)/2
	half := float64(v.hi-v.lo) / 2
	// The arc leaves the last pixel straight, for the line on from it to be as strong.
	r := max(min(x, float64(m.w)-x, y, float64(m.h)-y)-1, 0)
	// The arc's centre, and its ends straight up from it and straight across.
	sx, sy := -1.0, -1.0
	if right {
		sx = 1
	}
	if down {
		sy = 1
	}
	cx, cy := x+sx*r, y+sy*r
	m.cover(func(px, py float64) bool {
		switch {
		case (px-cx)*sx >= 0:
			return math.Abs(py-y) <= half
		case (py-cy)*sy >= 0:
			return math.Abs(px-x) <= half
		}
		return math.Abs(math.Hypot(px-cx, py-cy)-r) <= half
	})
}

// diagonals draws light lines from corner to corner, rising to the right, falling or both.
func (m *mask) diagonals(rising, falling bool) {
	w, h := float64(m.w), float64(m.h)
	half, long := float64(strokeWidth(m.h))/2, math.Hypot(w, h)
	m.cover(func(x, y float64) bool {
		return rising && math.Abs(x*h+y*w-w*h) <= half*long || falling && math.Abs(x*h-y*w) <= half*long
	})
}

// eighth is how many of size pixels n eighths are, to the nearest. A block takes the pixels before it or from it on, so
// two that share an edge leave no gap there, at any size.
func eighth(n, size int) int { return (size*n + 4) / 8 }

// quadrants are the quarters of the cell U+2596 to U+259F fill: 1 for the upper left, 2 the upper right, 4 the lower
// left and 8 the lower right.
var quadrants = [...]uint8{4, 8, 1, 1 | 4 | 8, 1 | 8, 1 | 2 | 4, 1 | 2 | 8, 2, 2 | 4, 2 | 4 | 8}

// block draws the block elements, the shades and the quadrants, U+2580 to U+259F. A shade is so much of the ink all
// over, which the painter blends over the cell's own background, where gridterm dithers.
func (m *mask) block(r rune) {
	w, h := m.w, m.h
	midX, midY := eighth(4, w), eighth(4, h)
	switch {
	case r == 0x2580: // upper half
		m.fill(0, 0, w, midY, 255)
	case r <= 0x2588: // lower eighths, 1/8 up to the full block
		m.fill(0, min(eighth(8-int(r-0x2580), h), h-1), w, h, 255)
	case r <= 0x258f: // left eighths, 7/8 down to 1/8
		m.fill(0, 0, max(eighth(8-int(r-0x2588), w), 1), h, 255)
	case r == 0x2590: // right half
		m.fill(midX, 0, w, h, 255)
	case r <= 0x2593: // light, medium and dark shade
		m.fill(0, 0, w, h, uint8(255*int(r-0x2590)/4))
	case r == 0x2594: // upper eighth
		m.fill(0, 0, w, max(eighth(1, h), 1), 255)
	case r == 0x2595: // right eighth
		m.fill(min(eighth(7, w), w-1), 0, w, h, 255)
	default:
		q := quadrants[r-0x2596]
		for i := range 4 {
			if q&(1<<i) == 0 {
				continue
			}
			x, y := span{0, midX}, span{0, midY}
			if i&1 != 0 {
				x = span{midX, w}
			}
			if i&2 != 0 {
				y = span{midY, h}
			}
			m.fill(x.lo, y.lo, x.hi, y.hi, 255)
		}
	}
}

// separator draws the Powerline separators: the triangles, the half circles and the cut corners, filled, and as the
// line along their edge. Each fills its cell's height, and its flat side the whole of the cell's edge.
func (m *mask) separator(r rune) {
	w, h := float64(m.w), float64(m.h)
	t := float64(strokeWidth(m.h))
	// flip turns the character that points right into the one that points left.
	flip := func(inside func(x, y float64) bool) func(x, y float64) bool {
		return func(x, y float64) bool { return inside(w-x, y) }
	}
	arrow := func(x, y float64) bool { return x*h/2 <= min(y, h-y)*w }
	arrowLine := func(x, y float64) bool {
		return math.Abs(x*h/2-min(y, h-y)*w) <= t/2*math.Hypot(w, h/2)
	}
	ellipse := func(x, y, rx, ry float64) bool {
		return rx > 0 && ry > 0 && x*x/(rx*rx)+(y-h/2)*(y-h/2)/(ry*ry) <= 1
	}
	round := func(x, y float64) bool { return ellipse(x, y, w, h/2) }
	roundLine := func(x, y float64) bool { return round(x, y) && !ellipse(x, y, w-t, h/2-t) }
	switch r {
	case 0xe0b0:
		m.cover(arrow)
	case 0xe0b1:
		m.cover(arrowLine)
	case 0xe0b2:
		m.cover(flip(arrow))
	case 0xe0b3:
		m.cover(flip(arrowLine))
	case 0xe0b4:
		m.cover(round)
	case 0xe0b5:
		m.cover(roundLine)
	case 0xe0b6:
		m.cover(flip(round))
	case 0xe0b7:
		m.cover(flip(roundLine))
	case 0xe0b8: // the lower left corner
		m.cover(func(x, y float64) bool { return y*w >= x*h })
	case 0xe0ba: // the lower right
		m.cover(func(x, y float64) bool { return y*w >= (w-x)*h })
	case 0xe0bc: // the upper left
		m.cover(func(x, y float64) bool { return y*w <= (w-x)*h })
	case 0xe0be: // the upper right
		m.cover(func(x, y float64) bool { return y*w <= x*h })
	case 0xe0b9, 0xe0bf:
		m.diagonals(false, true)
	case 0xe0bb, 0xe0bd:
		m.diagonals(true, false)
	}
}
