package text

import (
	"bytes"
	"testing"
)

// partialRows counts the rows of a mask's middle column that are
// neither empty nor solid.
func partialRows(m Mask) int {
	n := 0
	for y := range m.H {
		if c := m.Pix[y*m.W+m.W/2]; c > 8 && c < 247 {
			n++
		}
	}
	return n
}

func TestHintedStrokesLandOnWholePixels(t *testing.T) {
	f := Default()
	unhinted := 0
	for _, r := range []rune{'H', 'E', 'F', 'e'} {
		id := glyphOf(t, Default(), r)
		for _, size := range []float32{11, 12, 13, 14} {
			m := f.Rasterize(id, size, 0, Raster{Hint: true})
			if n := partialRows(m); n > 0 && r != 'e' {
				t.Errorf("%q at %v px: %d rows of its middle column are partly covered", r, size, n)
			}
			if r != 'e' && m.Offset.Y+m.H != 0 {
				t.Errorf("%q at %v px ends %d pixels below the baseline, want 0", r, size, m.Offset.Y+m.H)
			}
			unhinted += partialRows(f.Rasterize(id, size, 0, Raster{}))
		}
	}
	// Without hinting the same strokes fall across pixels.
	if unhinted == 0 {
		t.Fatal("no stroke falls across a pixel unhinted either, so the test proves nothing")
	}
}

func TestHintedXHeightIsTheSameForEveryLetter(t *testing.T) {
	f := Default()
	for _, size := range []float32{11, 12, 13, 14} {
		top := map[rune]int{}
		for _, r := range []rune{'x', 'z', 'u', 'v', 'w'} {
			top[r] = f.Rasterize(glyphOf(t, Default(), r), size, 0, Raster{Hint: true}).Offset.Y
		}
		for r, y := range top {
			if y != top['x'] {
				t.Errorf("at %v px %q starts at row %d and x at row %d", size, r, y, top['x'])
			}
		}
	}
}

func TestHintingStopsAtLargeSizes(t *testing.T) {
	f := Default()
	id := glyphOf(t, Default(), 'e')
	hinted := f.Rasterize(id, 48, 0.25, Raster{Hint: true})
	plain := f.Rasterize(id, 48, 0.25, Raster{})
	if !bytes.Equal(hinted.Pix, plain.Pix) || hinted.Offset != plain.Offset {
		t.Fatal("a 48 px glyph was hinted")
	}
}

func TestLCDFilterSumsToOne(t *testing.T) {
	sum := 0
	for _, w := range lcdFilter {
		sum += w
	}
	if sum != 256 {
		t.Fatalf("the LCD filter sums to %d/256, want 1", sum)
	}
}

func TestLCDMaskHasThreeChannels(t *testing.T) {
	f := Default()
	id := glyphOf(t, Default(), 'H')
	grey := f.Rasterize(id, 32, 0, Raster{})
	m := f.Rasterize(id, 32, 0, Raster{LCD: true})
	if !m.LCD || len(m.Pix) != 3*m.W*m.H {
		t.Fatalf("LCD mask %dx%d holds %d bytes, want 3 a pixel", m.W, m.H, len(m.Pix))
	}
	if m.W != grey.W+2 || m.Offset.X != grey.Offset.X-1 || m.H != grey.H {
		t.Fatalf("LCD mask %dx%d at %v, want a pixel wider each side of %dx%d at %v",
			m.W, m.H, m.Offset, grey.W, grey.H, grey.Offset)
	}
	// Across a stem's edge the thirds of a pixel differ, which is what
	// the panel's subpixels show.
	row := m.Pix[(m.H/4)*m.W*3 : (m.H/4+1)*m.W*3]
	coloured, solid := false, false
	for x := range m.W {
		r, g, b := row[3*x], row[3*x+1], row[3*x+2]
		coloured = coloured || r != g || g != b
		solid = solid || r == 255 && g == 255 && b == 255
	}
	if !coloured || !solid {
		t.Fatalf("a row across the H's stems is coloured %v and solid %v, want both", coloured, solid)
	}
	// Filtering moves coverage about but keeps all of it.
	var lcd, gray int
	for _, c := range m.Pix {
		lcd += int(c)
	}
	for _, c := range grey.Pix {
		gray += int(c)
	}
	if d := float64(lcd)/3 - float64(gray); d > 0.02*float64(gray) || d < -0.02*float64(gray) {
		t.Fatalf("LCD coverage %v, greyscale %d: want the same ink", float64(lcd)/3, gray)
	}
}
