package shape

import (
	"image/color"
	"strings"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

func TestAFilledPathCoversItsInsideAndItsBoundsMapIntoAPlace(t *testing.T) {
	// A diamond on a 24-unit grid, from 4 to 20 each way.
	d := mustPath(t, "M12 4L20 12L12 20L4 12Z")
	if b := d.Bounds(); b != geom.Rc(4, 4, 16, 16) {
		t.Fatalf("bounds %v", b)
	}
	if at := d.Fill().In(geom.Rc(0, 0, 24, 24), geom.Rc(100, 0, 48, 48)); at != geom.Rc(108, 8, 32, 32) {
		t.Fatalf("the grid drawn twice its size at x 100 puts the diamond at %v", at)
	}
	m := d.Fill().Coverage(32, 32)
	if m[16*32+16] != 255 || m[0] != 0 || m[31*32+31] != 0 {
		t.Fatalf("middle %d, corners %d %d", m[16*32+16], m[0], m[31*32+31])
	}
	// Its edge runs through pixels, which are partly covered.
	partly := 0
	for _, c := range m {
		if c > 0 && c < 255 {
			partly++
		}
	}
	if partly < 32 {
		t.Fatalf("%d pixels on its edges are partly covered, want its edges smoothed", partly)
	}
	if a, b := d.Fill(), d.Fill(); !a.Settled() || a != b {
		t.Fatal("a fill is settled and comparable, as a mask must be")
	}
	if _, err := NewPath("M1 1 L x"); err == nil {
		t.Fatal("bad path data read")
	}
}

func TestAStrokeIsRoundAndWithinItsBounds(t *testing.T) {
	line := mustPath(t, "M0 0H10")
	s := line.Stroke(2)
	b := s.bounds()
	if b.Min.X >= -1 || b.Max.X <= 11 || b.Min.Y >= -1 || b.Max.Y <= 1 {
		t.Fatalf("the stroke's bounds %v do not hold its round caps", b)
	}
	// Drawn at 10 pixels to a unit, the line runs along the middle row.
	w, h := int(b.Size().W*10+0.5), int(b.Size().H*10+0.5)
	m := s.Coverage(w, h)
	mid := h / 2
	if m[mid*w+w/2] != 255 || m[0] != 0 {
		t.Fatalf("on the line %d, in the corner %d", m[mid*w+w/2], m[0])
	}
	if !(Stroke{Path: line, Width: 1}).Settled() {
		t.Fatal("a stroke is settled")
	}
}

const art = `<?xml version="1.0"?>
<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" viewBox="0 0 100 50" width="200px">
  <defs>
    <linearGradient id="stops"><stop offset="0" stop-color="#ff0000"/><stop offset="100%" style="stop-color:#0000ff;stop-opacity:0.5"/></linearGradient>
    <linearGradient id="sky" xlink:href="#stops" x1="0" y1="0" x2="0" y2="1"/>
  </defs>
  <rect x="0" y="0" width="100" height="50" fill="url(#sky)"/>
  <g fill="green" stroke="#000" stroke-width="2" transform="translate(10 5)" opacity="0.5">
    <circle cx="10" cy="10" r="5"/>
    <rect x="30" y="0" width="20" height="10" rx="3" style="fill:#abc;stroke:none"/>
  </g>
  <path d="M0 40h10" stroke="white" fill="none" transform="scale(2)"/>
  <ellipse cx="80" cy="25" rx="5" ry="3" display="none"/>
  <text x="0" y="0">left out</text>
</svg>`

func TestAnSVGIsReadIntoPartsWithTheirColoursGradientsAndTransforms(t *testing.T) {
	f, err := ParseSVG([]byte(art))
	if err != nil {
		t.Fatal(err)
	}
	if f.ViewBox != geom.Rc(0, 0, 100, 50) {
		t.Fatalf("view box %v", f.ViewBox)
	}
	if len(f.Parts) != 4 {
		t.Fatalf("%d parts, want the sky, the circle, the rounded rect and the line", len(f.Parts))
	}
	sky := f.Parts[0].FillGradient
	if sky == nil || sky.From != geom.Pt(0, 0) || sky.To != geom.Pt(0, 50) {
		t.Fatalf("the sky's gradient %+v, want down the rect, by its bounding box", sky)
	}
	if sky.Start != (color.NRGBA{R: 255, A: 255}) || sky.End != (color.NRGBA{B: 255, A: 128}) {
		t.Fatalf("the sky runs %v to %v, its stops linked from another gradient", sky.Start, sky.End)
	}
	circle := f.Parts[1]
	if b := circle.Path.Bounds(); b != geom.Rc(15, 10, 10, 10) {
		t.Fatalf("the circle, moved by its group, lies in %v", b)
	}
	if circle.Fill != (color.NRGBA{G: 128, A: 128}) || circle.Stroke != (color.NRGBA{A: 128}) || circle.Width != 2 {
		t.Fatalf("the circle inherits %+v", circle)
	}
	rect := f.Parts[2]
	if rect.Fill != (color.NRGBA{R: 0xaa, G: 0xbb, B: 0xcc, A: 128}) || rect.Stroke.A != 0 {
		t.Fatalf("the rect's style overrides its group: %+v", rect)
	}
	if b := rect.Path.Bounds(); b != geom.Rc(40, 5, 20, 10) {
		t.Fatalf("the rounded rect lies in %v", b)
	}
	line := f.Parts[3]
	if line.Fill.A != 0 || line.FillGradient != nil || line.Width != 2 || line.Path.Bounds() != geom.Rc(0, 80, 20, 0) {
		t.Fatalf("the line, scaled twice, is %+v in %v", line, line.Path.Bounds())
	}
}

func TestAFigurePaintsEachPartInPlace(t *testing.T) {
	f := mustSVG(t, art)
	var p paint.Painter
	p.Reset()
	f.Paint(&p, geom.Rc(0, 0, 200, 100))
	var masks []*paint.MaskOp
	for _, op := range p.Ops() {
		if m, ok := op.(*paint.MaskOp); ok {
			masks = append(masks, m)
		}
	}
	// The sky, the circle's fill and stroke, the rect, the line.
	if len(masks) != 5 {
		t.Fatalf("%d masks", len(masks))
	}
	if masks[0].Rect != geom.Rc(0, 0, 200, 100) || masks[0].Gradient == nil || masks[0].Gradient.To != geom.Pt(0, 100) {
		t.Fatalf("the sky drawn at %v with %+v", masks[0].Rect, masks[0].Gradient)
	}
	if masks[1].Rect != geom.Rc(30, 20, 20, 20) {
		t.Fatalf("the circle drawn at %v, want twice its place", masks[1].Rect)
	}
	if _, ok := masks[2].Shape.(Stroke); !ok {
		t.Fatalf("the circle's stroke is a %T", masks[2].Shape)
	}
	if got := Fit(geom.Rc(0, 0, 100, 50), geom.Rc(0, 0, 100, 100)); got != geom.Rc(0, 25, 100, 50) {
		t.Fatalf("fitted %v", got)
	}
}

func TestAnSVGItCannotReadSaysWhy(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`<html/>`, "not <svg>"},
		{`<svg><path d="M1 1 L x"/></svg>`, "path data"},
		{`<svg><rect width="1" height="1" fill="chartreuseish"/></svg>`, "colour"},
		{`<svg><g transform="spin(3)"/></svg>`, "transform"},
		{`<svg`, "svg"},
	} {
		_, err := ParseSVG([]byte(c.src))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want an error about %s", c.src, err, c.want)
		}
	}
}

// mustPath reads path data a test wrote.
func mustPath(t testing.TB, d string) *Path {
	t.Helper()
	p, err := NewPath(d)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// mustSVG reads an SVG file a test wrote.
func mustSVG(t testing.TB, src string) *Figure {
	t.Helper()
	f, err := ParseSVG([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return f
}
