package widget

import (
	"bytes"
	"slices"
	"testing"
)

// boxSizes are cells in device pixels: a terminal's at common text sizes and display scales, odd and even, and some
// as narrow and as large as any.
var boxSizes = [][2]int{{5, 10}, {7, 15}, {8, 16}, {8, 17}, {9, 18}, {10, 21}, {11, 20}, {13, 24}, {13, 27}, {17, 32}, {21, 43}, {34, 64}}

// eachBox runs test for every character drawn in code from first to last, at every size.
func eachBox(t *testing.T, first, last rune, test func(t *testing.T, r rune, m []byte, w, h int)) {
	t.Helper()
	for _, size := range boxSizes {
		for r := first; r <= last; r++ {
			m := boxDrawing(r, size[0], size[1])
			if m == nil {
				t.Errorf("%c, %U, is left to the font", r, r)
				continue
			}
			test(t, r, m, size[0], size[1])
		}
	}
}

func TestEveryBoxCharacterIsDrawnInCode(t *testing.T) {
	drawn := func(t *testing.T, r rune, m []byte, w, h int) {
		t.Helper()
		if len(m) != w*h {
			t.Fatalf("%c at %dx%d has %d pixels", r, w, h, len(m))
		}
		for _, a := range m {
			if a > 60 {
				return
			}
		}
		t.Errorf("%c, %U, shows nothing at %dx%d", r, r, w, h)
	}
	eachBox(t, 0x2500, 0x259f, drawn)
	eachBox(t, 0xe0b0, 0xe0bf, drawn)
	for _, r := range []rune{0, ' ', 'x', 0x24ff, 0x25a0, 0xe0af, 0xe0c0} {
		if boxDrawing(r, 8, 16) != nil {
			t.Errorf("%U is drawn in code", r)
		}
	}
}

// A line reaches the cell's edge at full strength, where the line of the same weight in the next cell goes on from it,
// and nothing else touches the edge.
func TestBoxLinesMeetAcrossCells(t *testing.T) {
	eachBox(t, 0x2500, 0x257f, func(t *testing.T, r rune, m []byte, w, h int) {
		t.Helper()
		bl, ok := boxLines[r]
		switch r {
		case 0x256d:
			bl = boxLine{down: wLight, right: wLight}
		case 0x256e:
			bl = boxLine{down: wLight, left: wLight}
		case 0x256f:
			bl = boxLine{up: wLight, left: wLight}
		case 0x2570:
			bl = boxLine{up: wLight, right: wLight}
		default:
			if !ok || bl.dashes > 0 {
				return
			}
		}
		edge := func(name string, weight uint8, size int, at func(i int) byte) {
			s := strokes(weight, size, strokeWidth(h))
			for i := range size {
				var want byte
				if weight != wNone && (i >= s[0].lo && i < s[0].hi || i >= s[1].lo && i < s[1].hi) {
					want = 255
				}
				if got := at(i); got != want {
					t.Errorf("%c at %dx%d: pixel %d of its %s edge is %d, want %d", r, w, h, i, name, got, want)
					return
				}
			}
		}
		edge("top", bl.up, w, func(i int) byte { return m[i] })
		edge("bottom", bl.down, w, func(i int) byte { return m[(h-1)*w+i] })
		edge("left", bl.left, h, func(i int) byte { return m[i*w] })
		edge("right", bl.right, h, func(i int) byte { return m[i*w+w-1] })
	})
}

// pieces counts the parts a mask's ink falls into, where pixels touch by their sides.
func pieces(m []byte, w, h int) int {
	seen := make([]bool, len(m))
	n := 0
	for start := range m {
		if seen[start] || m[start] == 0 {
			continue
		}
		n++
		todo := []int{start}
		seen[start] = true
		for len(todo) > 0 {
			i := todo[len(todo)-1]
			todo = todo[:len(todo)-1]
			next := func(j int, ok bool) {
				if ok && !seen[j] && m[j] != 0 {
					seen[j] = true
					todo = append(todo, j)
				}
			}
			next(i-1, i%w > 0)
			next(i+1, i%w < w-1)
			next(i-w, i >= w)
			next(i+w, i < w*(h-1))
		}
	}
	return n
}

// Lines that meet are in one piece, and the strokes of double lines join as their corner or junction has them join:
// two nested corners, four of them in a cross, a stroke on its own opposite a branch.
func TestBoxLinesJoin(t *testing.T) {
	double := map[rune]int{
		'═': 2, '║': 2, '╔': 2, '╗': 2, '╚': 2, '╝': 2, '╠': 3, '╣': 3, '╦': 3, '╩': 3, '╬': 4,
		'╤': 2, '╧': 2, '╟': 2, '╢': 2,
	}
	eachBox(t, 0x2500, 0x257f, func(t *testing.T, r rune, m []byte, w, h int) {
		t.Helper()
		if boxLines[r].dashes > 0 {
			return
		}
		want := max(double[r], 1)
		if got := pieces(m, w, h); got != want {
			t.Errorf("%c at %dx%d is in %d pieces, want %d", r, w, h, got, want)
		}
	})
}

// The strokes of a double corner turn as two corners one inside the other, with nothing past either turn.
func TestDoubleCornersNest(t *testing.T) {
	for _, size := range boxSizes {
		w, h := size[0], size[1]
		st := strokeWidth(h)
		v, across := strokes(wDouble, w, st), strokes(wDouble, h, st)
		want := make([]byte, w*h)
		for k := range 2 {
			// The outer corner of strokes 0, then the inner one of strokes 1.
			for y := across[k].lo; y < h; y++ {
				for x := v[k].lo; x < w; x++ {
					if y < across[k].hi || x < v[k].hi {
						want[y*w+x] = 255
					}
				}
			}
		}
		if !bytes.Equal(boxDrawing('╔', w, h), want) {
			t.Errorf("╔ at %dx%d is not two corners, one in the other", w, h)
		}
	}
}

// Where two lines make a corner, the square they share is full, with no notch out of it.
func TestHeavyCornersHaveNoNotch(t *testing.T) {
	eachBox(t, 0x250c, 0x254b, func(t *testing.T, r rune, m []byte, w, h int) {
		t.Helper()
		bl := boxLines[r]
		st := strokeWidth(h)
		v, across := strokes(max(bl.up, bl.down), w, st)[0], strokes(max(bl.left, bl.right), h, st)[0]
		for _, y := range []int{across.lo, across.hi - 1} {
			for _, x := range []int{v.lo, v.hi - 1} {
				// A heavier arm on one side only fills its own half.
				if m[y*w+x] == 0 && bl.up == bl.down && bl.left == bl.right {
					t.Errorf("%c at %dx%d has a notch at %d, %d", r, w, h, x, y)
				}
				corner := (bl.up == wNone || bl.down == wNone) && (bl.left == wNone || bl.right == wNone)
				if m[y*w+x] == 0 && corner {
					t.Errorf("%c at %dx%d has a notch at %d, %d", r, w, h, x, y)
				}
			}
		}
	})
}

// A dashed line has as many dashes as its name says, each as long as the next, and the gaps between them, on to the
// next cell's too, differ by a pixel at most.
func TestDashedLinesAreEven(t *testing.T) {
	for r, bl := range boxLines {
		if bl.dashes == 0 {
			continue
		}
		for size := 2 * bl.dashes; size < 70; size++ {
			// The line runs along a cell size long, and its middle is where a light line's is.
			w, h := size, 2*size
			at := func(m []byte, i int) byte { return m[strokes(bl.left, h, strokeWidth(h))[0].lo*w+i] }
			if bl.up != wNone {
				w, h = max(size/2, 3), size
				at = func(m []byte, i int) byte { return m[i*w+strokes(bl.up, w, strokeWidth(h))[0].lo] }
			}
			m := boxDrawing(r, w, h)
			// Round the cell once, from the start of a dash.
			first := 0
			for first < size && (at(m, first) == 0 || at(m, (first+size-1)%size) != 0) {
				first++
			}
			var dashes, gaps []int
			for i := range size {
				on := at(m, (first+i)%size) != 0
				if on && (i == 0 || at(m, (first+i-1)%size) == 0) {
					dashes, gaps = append(dashes, 0), append(gaps, 0)
				}
				if on {
					dashes[len(dashes)-1]++
				} else {
					gaps[len(gaps)-1]++
				}
			}
			if len(dashes) != bl.dashes || slices.Min(dashes) != slices.Max(dashes) ||
				slices.Min(gaps) < 1 || slices.Max(gaps)-slices.Min(gaps) > 1 {
				t.Errorf("%c in %d px has dashes of %v and gaps of %v", r, size, dashes, gaps)
			}
		}
	}
}

// Blocks that share an edge fill the cell between them, with no gap and no overlap, at any size.
func TestBlocksFillTheCellBetweenThem(t *testing.T) {
	sets := []string{"▀▄", "▌▐", "▘▝▖▗", "▚▞", "▛▗", "▜▖", "▙▝", "▟▘", "▀▖▗", "▌▝▗"}
	for _, size := range boxSizes {
		w, h := size[0], size[1]
		for _, set := range sets {
			sum := make([]int, w*h)
			for _, r := range set {
				for i, a := range boxDrawing(r, w, h) {
					sum[i] += int(a)
				}
			}
			for i, a := range sum {
				if a != 255 {
					t.Errorf("%s at %dx%d cover pixel %d, %d by %d, want 255", set, w, h, i%w, i/w, a)
					break
				}
			}
		}
		// The eighths rise one by one to the whole block.
		last := 0
		for r := rune(0x2581); r <= 0x2588; r++ {
			n := 0
			for _, a := range boxDrawing(r, w, h) {
				n += int(a)
			}
			if n < last || r == 0x2588 && n != 255*w*h {
				t.Errorf("%c at %dx%d covers %d, after %d", r, w, h, n, last)
			}
			last = n
		}
	}
}

// The filled Powerline separators meet the cell's own edges: what points right and what points left are each other's
// mirror images, and the two triangles either side of a slant fill the cell.
func TestSeparatorsMeetTheirCells(t *testing.T) {
	for _, size := range boxSizes {
		w, h := size[0], size[1]
		for _, pair := range [][2]rune{{0xe0b0, 0xe0b2}, {0xe0b1, 0xe0b3}, {0xe0b4, 0xe0b6}, {0xe0b5, 0xe0b7}, {0xe0b8, 0xe0ba}} {
			a, b := boxDrawing(pair[0], w, h), boxDrawing(pair[1], w, h)
			for i := range a {
				if j := i/w*w + w - 1 - i%w; a[i] != b[j] {
					t.Errorf("%U and %U at %dx%d are not mirror images at %d, %d", pair[0], pair[1], w, h, i%w, i/w)
					break
				}
			}
		}
		for _, pair := range [][2]rune{{0xe0b8, 0xe0be}, {0xe0ba, 0xe0bc}} {
			a, b := boxDrawing(pair[0], w, h), boxDrawing(pair[1], w, h)
			for i := range a {
				// The points on the slant itself are in both.
				if s := int(a[i]) + int(b[i]); s < 254 || s > 255+255/4 {
					t.Errorf("%U and %U at %dx%d cover pixel %d, %d by %d", pair[0], pair[1], w, h, i%w, i/w, s)
					break
				}
			}
		}
		arrow := boxDrawing(0xe0b0, w, h)
		if mid := h / 2; arrow[mid*w] != 255 || arrow[mid*w+w-1] < 60 {
			t.Errorf("the arrow at %dx%d runs from %d to %d along its middle, want it across the cell", w, h, arrow[mid*w], arrow[mid*w+w-1])
		}
	}
}

// A wide character drawn in code, as some take the box lines to be, is one drawing across its two cells.
func TestAWideBoxCharacterSpansItsCells(t *testing.T) {
	w, g, run := newCellStage(t)
	g.SetRow(0, []Cell{{Rune: '╬', Wide: true}, {}, {Rune: 'x'}})
	run(1)
	row, ok := cellsRow(w.Offscreen().Ops(), 0)
	if !ok || row.Cells[0].Pattern == 0 || row.Cells[1].Pattern == 0 || row.Cells[0].Text {
		t.Fatalf("a wide ╬ is drawn as %+v", row.Cells[:2])
	}
	p := row.Patterns
	whole := boxDrawing('╬', 2*p.W, p.H)
	for y := range p.H {
		left, right := p.Masks[row.Cells[0].Pattern-1], p.Masks[row.Cells[1].Pattern-1]
		joined := append(append([]byte{}, left[y*p.W:(y+1)*p.W]...), right[y*p.W:(y+1)*p.W]...)
		if !bytes.Equal(joined, whole[y*2*p.W:(y+1)*2*p.W]) {
			t.Fatalf("row %d of a wide ╬'s halves is %v, want %v", y, joined, whole[y*2*p.W:(y+1)*2*p.W])
		}
	}
}
