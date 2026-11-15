//go:build linux || windows || darwin

package render

import (
	"image/color"
	"math"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// under returns an area of n points under the line from a to b, down to
// the base at y, in c at the line and foot at the base.
func under(a, b geom.Point, n int, y float32, c, foot color.NRGBA) []paint.AreaPoint {
	pts := make([]paint.AreaPoint, n)
	for i := range pts {
		t := float32(i) / float32(n-1)
		x := a.X + (b.X-a.X)*t
		pts[i] = paint.AreaPoint{At: geom.Pt(x, a.Y+(b.Y-a.Y)*t), Base: geom.Pt(x, y), Color: c, BaseColor: foot}
	}
	return pts
}

func TestAnAreaCoversWhatLiesBetweenItsLineAndItsBaseAndNoMore(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	pix := drawn(r, func(p *paint.Painter) {
		p.Area(under(geom.Pt(100, 100), geom.Pt(300, 100), 20, 140, white, white))
	})
	for _, c := range []struct {
		x, y int
		want [4]byte
		what string
	}{
		{200, 100, px(white), "its top row"},
		{200, 139, px(white), "its bottom row"},
		{200, 99, px(black), "the row above it"},
		{200, 140, px(black), "the row below it"},
		{100, 120, px(white), "its first column"},
		{299, 120, px(white), "its last column"},
		{99, 120, px(black), "the column before it"},
		{300, 120, px(black), "the column after it"},
	} {
		if got := pixelAt(pix, c.x, c.y); !near(got, c.want, 2) {
			t.Errorf("%s, at %d,%d, is %v, want %v", c.what, c.x, c.y, got, c.want)
		}
	}
}

func TestAnAreasEdgesAndEndsAreSmooth(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	// Its line and its base lie halfway across a row of pixels, and its
	// ends a quarter of a pixel into a column.
	pix := drawn(r, func(p *paint.Painter) {
		p.Area(under(geom.Pt(100.75, 100.5), geom.Pt(300.25, 100.5), 2, 140.5, white, white))
	})
	half, quarter := [4]byte{0x80, 0x80, 0x80, 0xff}, [4]byte{0x40, 0x40, 0x40, 0xff}
	for _, c := range []struct {
		x, y int
		want [4]byte
		what string
	}{
		{200, 100, half, "the pixel its line halves"},
		{200, 140, half, "the pixel its base halves"},
		{200, 99, px(black), "the row above its line"},
		{200, 141, px(black), "the row below its base"},
		{99, 120, px(black), "the column before its start"},
		{100, 120, quarter, "the column its start cuts a quarter into"},
		{101, 120, px(white), "the column after its start"},
		{299, 120, px(white), "the column before its end"},
		{300, 120, quarter, "the column its end cuts a quarter into"},
		{301, 120, px(black), "the column after its end"},
	} {
		if got := pixelAt(pix, c.x, c.y); !near(got, c.want, 3) {
			t.Errorf("%s, at %d,%d, is %v, want %v", c.what, c.x, c.y, got, c.want)
		}
	}
}

func TestAnAreasLineIsSmoothHoweverSteepItRuns(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	// The line climbs 60 pixels in 3, as a graph's does where its value
	// jumps: at the middle of row 140 it crosses x 101.475.
	pix := drawn(r, func(p *paint.Painter) {
		p.Area([]paint.AreaPoint{
			{At: geom.Pt(90, 170), Base: geom.Pt(90, 180), Color: white, BaseColor: white},
			{At: geom.Pt(100, 170), Base: geom.Pt(100, 180), Color: white, BaseColor: white},
			{At: geom.Pt(103, 110), Base: geom.Pt(103, 180), Color: white, BaseColor: white},
			{At: geom.Pt(120, 110), Base: geom.Pt(120, 180), Color: white, BaseColor: white},
		})
	})
	if got := pixelAt(pix, 100, 140); !near(got, px(black), 2) {
		t.Errorf("the pixel left of the climb is %v, want it dark", got)
	}
	if got := pixelAt(pix, 101, 140); got[0] < 0x60 || got[0] > 0xa8 {
		t.Errorf("the pixel the climb crosses is %v, want about half lit", got)
	}
	if got := pixelAt(pix, 102, 140); !near(got, px(white), 6) {
		t.Errorf("the pixel right of the climb is %v, want it lit", got)
	}
	// Above the low stretch and beside the climb nothing shows.
	for y := 100; y < 169; y++ {
		for x := 85; x < 100; x++ {
			if got := pixelAt(pix, x, y); !near(got, px(black), 2) {
				t.Fatalf("at %d,%d, outside the area, is %v, want it dark", x, y, got)
			}
		}
	}
}

func TestAnAreasColourChangesAcrossItAndAlongIt(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	pix := drawn(r, func(p *paint.Painter) {
		// Red at the line to blue at the base, ten pixels below.
		p.Area(under(geom.Pt(100, 100), geom.Pt(300, 100), 20, 110, red, blue))
		// Red at its start to blue at its end.
		p.Area([]paint.AreaPoint{
			{At: geom.Pt(100, 200), Base: geom.Pt(100, 240), Color: red, BaseColor: red},
			{At: geom.Pt(300, 200), Base: geom.Pt(300, 240), Color: blue, BaseColor: blue},
		})
	})
	for _, c := range []struct {
		x, y int
		want [4]byte
		what string
	}{
		// The middle of a row is half a pixel into it.
		{200, 100, [4]byte{0xf2, 0, 0x0d, 0xff}, "the row at its line"},
		{200, 105, [4]byte{0x73, 0, 0x8c, 0xff}, "the row past its middle"},
		{200, 109, [4]byte{0x0d, 0, 0xf2, 0xff}, "the row at its base"},
		{200, 220, [4]byte{0x7f, 0, 0x80, 0xff}, "halfway along it"},
		{150, 220, [4]byte{0xbf, 0, 0x40, 0xff}, "a quarter of the way along it"},
	} {
		if got := pixelAt(pix, c.x, c.y); !near(got, c.want, 4) {
			t.Errorf("%s, at %d,%d, is %v, want %v", c.what, c.x, c.y, got, c.want)
		}
	}
}

func TestAThinAreaFadesRatherThanBreakingUp(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	pix := drawn(r, func(p *paint.Painter) {
		p.Area(under(geom.Pt(100, 100.3), geom.Pt(300, 100.3), 20, 100.7, white, white))
	})
	for x := 110; x < 290; x += 7 {
		if got := pixelAt(pix, x, 100); !near(got, [4]byte{0x66, 0x66, 0x66, 0xff}, 4) {
			t.Fatalf("an area 0.4 of a pixel high is %v at %d, want two fifths lit", got, x)
		}
	}
}

func TestAnAreaWhoseLineMeetsItsBaseEndsThere(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	// A hill, from its base up and back down to it, and flat on it
	// after.
	pix := drawn(r, func(p *paint.Painter) {
		p.Area([]paint.AreaPoint{
			{At: geom.Pt(100, 140), Base: geom.Pt(100, 140), Color: white, BaseColor: white},
			{At: geom.Pt(200, 100), Base: geom.Pt(200, 140), Color: white, BaseColor: white},
			{At: geom.Pt(300, 140), Base: geom.Pt(300, 140), Color: white, BaseColor: white},
			{At: geom.Pt(400, 140), Base: geom.Pt(400, 140), Color: white, BaseColor: white},
		})
	})
	for _, c := range []struct {
		x, y int
		want [4]byte
		what string
	}{
		{200, 105, px(white), "under the hill's top"},
		{200, 139, px(white), "the hill's bottom row"},
		{120, 138, px(white), "under the hill's side"},
		{120, 125, px(black), "over the hill's side"},
		{200, 141, px(black), "under the base"},
		{350, 139, px(black), "over the flat stretch"},
		{350, 140, px(black), "under the flat stretch"},
		{97, 139, px(black), "before the hill"},
	} {
		if got := pixelAt(pix, c.x, c.y); !near(got, c.want, 3) {
			t.Errorf("%s, at %d,%d, is %v, want %v", c.what, c.x, c.y, got, c.want)
		}
	}
}

// hills returns a see-through area of n points under a wave, down to a
// base at y 200.
func hills(n int) []paint.AreaPoint {
	c := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x80}
	pts := make([]paint.AreaPoint, n)
	for i := range pts {
		x := 100.3 + 200.4*float32(i)/float32(n-1)
		y := 150 + 30*float32(math.Sin(float64(x)/9)) + 15*float32(math.Sin(float64(x)/2.3))
		pts[i] = paint.AreaPoint{At: geom.Pt(x, y), Base: geom.Pt(x, 200.4), Color: c, BaseColor: c}
	}
	return pts
}

func TestAnAreaOfManyShortStretchesShowsNoSeams(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	// Stretches of a third of a pixel: every pixel inside is laid over
	// once, so none is brighter or darker than half lit.
	pix := drawn(r, func(p *paint.Painter) { p.Area(hills(601)) })
	half := [4]byte{0x80, 0x80, 0x80, 0xff}
	for y := 100; y < 205; y++ {
		for x := 95; x < 306; x++ {
			got := pixelAt(pix, x, y)
			if got[0] > 0x82 {
				t.Fatalf("at %d,%d the area is %v, brighter than half lit: two stretches overlap", x, y, got)
			}
			if y >= 197 && y < 200 && x > 101 && x < 300 && !near(got, half, 2) {
				t.Fatalf("at %d,%d, inside the area, is %v, want half lit", x, y, got)
			}
		}
	}
	// A straight one draws the same in short stretches as in long.
	short := drawn(r, func(p *paint.Painter) {
		p.Area(under(geom.Pt(100.3, 100.2), geom.Pt(300.6, 120.7), 601, 160.4, white, white))
	})
	long := drawn(r, func(p *paint.Painter) {
		p.Area(under(geom.Pt(100.3, 100.2), geom.Pt(300.6, 120.7), 46, 160.4, white, white))
	})
	for y := 85; y < 170; y++ {
		for x := 95; x < 306; x++ {
			if a, b := pixelAt(short, x, y), pixelAt(long, x, y); !near(a, b, 2) {
				t.Fatalf("at %d,%d the area of short stretches is %v and of long ones %v", x, y, a, b)
			}
		}
	}
}

func TestAnAreaKeepsToItsClipsTransformAndLayer(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	pts := under(geom.Pt(0, 45), geom.Pt(400, 45), 30, 55, white, white)
	// high runs near the top of a layer, through its corners.
	high := under(geom.Pt(0, 5), geom.Pt(400, 5), 30, 15, white, white)
	pix := drawn(r, func(p *paint.Painter) {
		// Cut to an upright rectangle, in place.
		end := p.Layer(paint.LayerOpts{Bounds: geom.Rc(100, 0, 100, 200), Opacity: 1, Clip: true})
		p.Area(pts)
		end()
		// Moved down and cut to a rounded rectangle, offscreen.
		pop := p.Push(paint.Translate(geom.Pt(0, 100)))
		end = p.Layer(paint.LayerOpts{Bounds: geom.Rc(100, 0, 100, 100), Opacity: 1, Clip: true, Radius: 50})
		p.Area(high)
		end()
		pop()
		// Cut to an ellipse, in place.
		pop = p.Push(paint.Translate(geom.Pt(0, 200)))
		end = p.Layer(paint.LayerOpts{Bounds: geom.Rc(100, 0, 100, 100), Opacity: 1, Clip: true, Ellipse: true})
		p.Area(high)
		end()
		pop()
		// At half opacity, scaled to twice the size about its middle.
		pop = p.Push(paint.Scale(2, geom.Pt(200, 350)))
		end = p.Layer(paint.LayerOpts{Bounds: geom.Rc(0, 300, 400, 100), Opacity: 0.5})
		pop2 := p.Push(paint.Translate(geom.Pt(0, 300)))
		p.Area(pts)
		pop2()
		end()
		pop()
	})
	for _, c := range []struct {
		x, y int
		want [4]byte
		what string
	}{
		{150, 50, px(white), "inside the rectangle"},
		{99, 50, px(black), "left of the rectangle"},
		{200, 50, px(black), "right of the rectangle"},
		{150, 110, px(white), "inside the rounded rectangle"},
		{105, 110, px(black), "in the rounded rectangle's cut corner"},
		{150, 210, px(white), "inside the ellipse"},
		{105, 210, px(black), "outside the ellipse"},
		{200, 341, [4]byte{0x80, 0x80, 0x80, 0xff}, "the area at half opacity, twice as high"},
		{200, 339, px(black), "past the area twice as high"},
	} {
		if got := pixelAt(pix, c.x, c.y); !near(got, c.want, 3) {
			t.Errorf("%s, at %d,%d, is %v, want %v", c.what, c.x, c.y, got, c.want)
		}
	}
}

func TestAnAreaAddsItsLightUnderBlendAdd(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	pix := drawn(r, func(p *paint.Painter) {
		p.RRect(geom.Rc(100, 80, 200, 40), 0, paint.Solid(ember))
		defer p.Blend(paint.BlendAdd)()
		p.Area(under(geom.Pt(100, 95), geom.Pt(300, 95), 2, 105, ember, ember))
	})
	if got := pixelAt(pix, 200, 100); !near(got, [4]byte{0xc0, 0x60, 0x20, 0xff}, 2) {
		t.Errorf("the added area over its own colour is %v, want it twice as bright", got)
	}
}

func TestAnAreaDrawsInTheBatchOfTheShapesAroundIt(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	pts := under(geom.Pt(100, 100), geom.Pt(300, 200), 100, 260, blue, red)
	frame := func(area bool) []paint.Op {
		var p paint.Painter
		p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(black))
		p.RRect(geom.Rc(10, 10, 50, 50), 4, paint.Solid(red))
		if area {
			p.Area(pts)
		}
		p.RRect(geom.Rc(10, 10, 50, 50), 4, paint.Solid(red))
		return p.Ops()
	}
	w, h := int(benchSize.W), int(benchSize.H)
	cost := func(ops []paint.Op) (draws int, allocs float64) {
		r.canvasOK, r.direct = false, false
		r.Draw(ops, paint.Everything, w, h, 1)
		r.draws = 0
		r.Draw(ops, paint.Everything, w, h, 1)
		draws = r.draws
		allocs = testing.AllocsPerRun(10, func() { r.Draw(ops, paint.Everything, w, h, 1) })
		return draws, allocs
	}
	plainDraws, plainAllocs := cost(frame(false))
	draws, allocs := cost(frame(true))
	if draws != plainDraws {
		t.Errorf("a frame of shapes and an area took %d draw calls, want the %d of the shapes alone", draws, plainDraws)
	}
	// The race detector allocates of its own.
	if !raceOn && allocs > plainAllocs {
		t.Errorf("a frame with an area allocated %v times, want the %v of one without", allocs, plainAllocs)
	}
}

func TestAnAreaMovingRedrawsInPartAsItWouldInFull(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	w, h := int(benchSize.W), int(benchSize.H)
	var p paint.Painter
	record := func(y float32) {
		p.Reset()
		p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(black))
		p.RRect(geom.Rc(500, 400, 100, 100), 8, paint.Solid(red))
		p.Area([]paint.AreaPoint{
			{At: geom.Pt(100, 120), Base: geom.Pt(100, 180), Color: red, BaseColor: blue},
			{At: geom.Pt(150, y), Base: geom.Pt(150, 180), Color: blue, BaseColor: white},
			{At: geom.Pt(200, 120), Base: geom.Pt(200, 180), Color: white, BaseColor: red},
		})
	}
	// The first frame goes straight to the window, and the second
	// fills the canvas, so the third is the first to redraw in part.
	for _, y := range []float32{140, 140, 100} {
		record(y)
		r.Draw(p.Ops(), p.Damage(), w, h, 1)
	}
	if s := r.Redrawn.Size(); s.W > 160 || s.H > 120 {
		t.Fatalf("an area's line rising redrew %v, want about the area", r.Redrawn)
	}
	partial := canvas(r)
	r.canvasOK = false
	r.Draw(p.Ops(), p.Damage(), w, h, 1)
	full := canvas(r)
	for i := range full {
		if d := int(full[i]) - int(partial[i]); d > 1 || d < -1 {
			px := i / 4
			t.Fatalf("pixel (%d, %d) is %d redrawn in part and %d in full", px%w, h-1-px/w, partial[i], full[i])
		}
	}
}

func TestAnAreaAHairHighShowsNothingAboveItsLine(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	// A tall start, then a line a hair over its base, as a graph's is
	// where its value is next to nothing: too thin to say which way is
	// up, it takes that of the tall start.
	pix := drawn(r, func(p *paint.Painter) {
		pts := []paint.AreaPoint{{At: geom.Pt(100, 100), Base: geom.Pt(100, 140), Color: white, BaseColor: white}}
		for x := float32(110); x <= 300; x += 10 {
			pts = append(pts, paint.AreaPoint{At: geom.Pt(x, 140-1e-5*x), Base: geom.Pt(x, 140), Color: white, BaseColor: white})
		}
		p.Area(pts)
	})
	for y := 100; y < 160; y++ {
		for x := 112; x < 305; x++ {
			if got := pixelAt(pix, x, y); !near(got, px(black), 2) {
				t.Fatalf("at %d,%d, by a line a hair over its base, is %v, want it dark", x, y, got)
			}
		}
	}
}
