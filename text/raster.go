package text

import (
	"image"
	"math"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/font/opentype"
	"golang.org/x/image/vector"
)

// A Mask is one glyph's coverage at one device size: 0 outside the
// glyph, 255 inside, and antialiased in between.
type Mask struct {
	// Pix holds W*H coverage bytes, row by row, or three bytes a pixel
	// when LCD is set.
	Pix  []byte
	W, H int
	// Offset is where the mask's top-left corner sits relative to the
	// glyph's origin on the baseline, in device pixels, y down.
	Offset image.Point
	// LCD says each pixel holds the coverage of its left, middle and
	// right thirds, in that order.
	LCD bool
	// Color says each pixel holds a colour, four bytes of premultiplied
	// red, green, blue and alpha, as [Face.RasterizeColor] draws.
	Color bool
}

// Raster says how [Face.Rasterize] renders a glyph.
type Raster struct {
	// Hint snaps the glyph's heights to whole pixels, as
	// [HintingLight] describes, at device sizes up to 36 pixels.
	Hint bool
	// LCD renders a coverage for each third of a pixel, filtered
	// across neighbouring thirds to limit colour fringes.
	LCD bool
}

// lcdFilter is FreeType's default LCD filter, in 256ths: it spreads
// each third of a pixel over its neighbours, so a stroke does not
// light one colour alone.
var lcdFilter = [5]int{0x08, 0x4D, 0x56, 0x4D, 0x08}

// Rasterize renders glyph id at sizePx device pixels, with its origin
// shifted right by dx, a fraction of a pixel. A driver keeps a few
// shifts of each glyph so text can sit at any fractional position and
// still look the same.
//
// A glyph with no outline, such as a space, returns an empty mask.
func (f *Face) Rasterize(id uint32, sizePx, dx float32, o Raster) Mask {
	hint := o.Hint && sizePx <= hintMax
	mu.Lock()
	outline, ok := f.face.GlyphDataOutline(font.GID(id))
	var zones []zone
	if hint {
		zones = f.zonesLocked()
	}
	mu.Unlock()
	if !ok || len(outline.Segments) == 0 {
		return Mask{}
	}

	scale := sizePx / f.upem
	mapY := func(y float32) float32 { return y * scale }
	if hint {
		h := newHinter(outline, zones, f.upem, scale)
		mapY = h.mapY
	}
	pt := func(p font.SegmentPoint) (float32, float32) { return p.X*scale + dx, -mapY(p.Y) }

	minX, minY := float32(math.Inf(1)), float32(math.Inf(1))
	maxX, maxY := float32(math.Inf(-1)), float32(math.Inf(-1))
	for _, seg := range outline.Segments {
		for _, a := range seg.ArgsSlice() {
			x, y := pt(a)
			minX, maxX = min(minX, x), max(maxX, x)
			minY, maxY = min(minY, y), max(maxY, y)
		}
	}
	x0, y0 := int(math.Floor(float64(minX))), int(math.Floor(float64(minY)))
	x1, y1 := int(math.Ceil(float64(maxX))), int(math.Ceil(float64(maxY)))
	if x1-x0 <= 0 || y1-y0 <= 0 {
		return Mask{}
	}
	sub := 1
	if o.LCD {
		// The filter spreads coverage two thirds of a pixel each way.
		x0, x1, sub = x0-1, x1+1, 3
	}
	w, h := x1-x0, y1-y0

	r := vector.NewRasterizer(w*sub, h)
	local := func(p font.SegmentPoint) (float32, float32) {
		x, y := pt(p)
		return (x - float32(x0)) * float32(sub), y - float32(y0)
	}
	// MoveTo leaves the previous contour open, so each one is closed
	// before the next begins.
	open := false
	for _, seg := range outline.Segments {
		switch seg.Op {
		case opentype.SegmentOpMoveTo:
			if open {
				r.ClosePath()
			}
			open = true
			r.MoveTo(local(seg.Args[0]))
		case opentype.SegmentOpLineTo:
			r.LineTo(local(seg.Args[0]))
		case opentype.SegmentOpQuadTo:
			bx, by := local(seg.Args[0])
			cx, cy := local(seg.Args[1])
			r.QuadTo(bx, by, cx, cy)
		case opentype.SegmentOpCubeTo:
			bx, by := local(seg.Args[0])
			cx, cy := local(seg.Args[1])
			dx, dy := local(seg.Args[2])
			r.CubeTo(bx, by, cx, cy, dx, dy)
		}
	}
	r.ClosePath()
	dst := image.NewAlpha(image.Rect(0, 0, w*sub, h))
	r.Draw(dst, dst.Bounds(), image.Opaque, image.Point{})
	m := Mask{Pix: dst.Pix, W: w, H: h, Offset: image.Pt(x0, y0)}
	if o.LCD {
		m.Pix, m.LCD = filterLCD(dst.Pix, w*sub, h), true
	}
	return m
}

// filterLCD runs lcdFilter along each row of a coverage mask w thirds
// of a pixel wide and h rows tall.
func filterLCD(pix []byte, w, h int) []byte {
	out := make([]byte, len(pix))
	for y := range h {
		row := pix[y*w : (y+1)*w]
		for x := range w {
			sum := 0
			for k, wt := range lcdFilter {
				if i := x + k - len(lcdFilter)/2; i >= 0 && i < w {
					sum += wt * int(row[i])
				}
			}
			out[y*w+x] = byte((sum + 128) >> 8)
		}
	}
	return out
}
