//go:build linux || windows || darwin

package render

import (
	"image"
	"image/color"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// ember is a dim orange, which two of added together make twice as
// bright, short of white in every channel.
var ember = color.NRGBA{R: 0x60, G: 0x30, B: 0x10, A: 0xff}

func TestAddedShapesSumTheirLightWhereTheyOverlap(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	t.Logf("dual-source blending: %v", r.dual)
	pix := drawn(r, func(p *paint.Painter) {
		p.RRect(geom.Rc(100, 100, 100, 50), 0, paint.Solid(ember))
		p.RRect(geom.Rc(150, 100, 100, 50), 0, paint.Solid(ember))
		defer p.Blend(paint.BlendAdd)()
		p.RRect(geom.Rc(100, 200, 100, 50), 0, paint.Solid(ember))
		p.RRect(geom.Rc(150, 200, 100, 50), 0, paint.Solid(ember))
	})
	twice := [4]byte{0xc0, 0x60, 0x20, 0xff}
	for _, c := range []struct {
		x, y int
		want [4]byte
		what string
	}{
		{125, 125, px(ember), "a shape drawn normally"},
		{175, 125, px(ember), "where two shapes drawn normally overlap"},
		{125, 225, px(ember), "an added shape on black"},
		{175, 225, twice, "where two added shapes overlap"},
	} {
		if got := pixelAt(pix, c.x, c.y); !near(got, c.want, 2) {
			t.Errorf("%s is %v, want %v", c.what, got, c.want)
		}
	}
}

func TestAnAddedMaskAndImageAddTheirLightToo(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := 0; i < len(src.Pix); i += 4 {
		copy(src.Pix[i:], []byte{ember.R, ember.G, ember.B, 0xff})
	}
	img := paint.NewImage(src)
	n := 0
	pix := drawn(r, func(p *paint.Painter) {
		defer p.Blend(paint.BlendAdd)()
		p.Mask(countedSquare{n: &n, settled: true}, geom.Rc(100, 100, 100, 50), ember)
		p.Mask(countedSquare{n: &n, settled: true}, geom.Rc(150, 100, 100, 50), ember)
		p.Image(img, geom.Rc(100, 200, 100, 50), paint.ImageOpts{Opacity: 1})
		p.Image(img, geom.Rc(150, 200, 100, 50), paint.ImageOpts{Opacity: 1})
	})
	twice := [4]byte{0xc0, 0x60, 0x20, 0xff}
	if got := pixelAt(pix, 125, 125); !near(got, px(ember), 2) {
		t.Errorf("an added mask on black is %v, want %v", got, px(ember))
	}
	if got := pixelAt(pix, 175, 125); !near(got, twice, 2) {
		t.Errorf("where two added masks overlap is %v, want %v", got, twice)
	}
	if got := pixelAt(pix, 125, 225); !near(got, px(ember), 2) {
		t.Errorf("an added image on black is %v, want %v", got, px(ember))
	}
	if got := pixelAt(pix, 175, 225); !near(got, twice, 2) {
		t.Errorf("where two added images overlap is %v, want %v", got, twice)
	}
}

func TestAddedOpsDrawInTheBatchOfTheOnesAroundThem(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	n := 0
	frame := func(add bool) *paint.Painter {
		var p paint.Painter
		p.RRect(geom.Rect{Max: benchSize.Point()}, 0, paint.Solid(black))
		for i := range 20 {
			end := func() {}
			if add && i%2 == 1 {
				end = p.Blend(paint.BlendAdd)
			}
			x := float32(i * 20)
			p.RRect(geom.Rc(x, 10, 15, 15), 4, paint.Solid(ember))
			p.Mask(countedSquare{n: &n, settled: true}, geom.Rc(x, 40, 15, 15), ember)
			end()
		}
		return &p
	}
	draws := func(p *paint.Painter) int {
		w, h := int(benchSize.W), int(benchSize.H)
		r.Draw(p.Ops(), paint.Everything, w, h, 1)
		r.draws = 0
		r.Draw(p.Ops(), paint.Everything, w, h, 1)
		return r.draws
	}
	normal, mixed := draws(frame(false)), draws(frame(true))
	if mixed != normal {
		t.Errorf("a frame adding every other op drew in %d calls, want the %d of one drawn normally", mixed, normal)
	}
}

func TestAnAddedShapeInAFadedLayerAddsItsShareOfLight(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	grey := color.NRGBA{R: 0x40, G: 0x40, B: 0x40, A: 0xff}
	light := color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xff}
	pix := drawn(r, func(p *paint.Painter) {
		p.RRect(geom.Rc(50, 50, 400, 400), 0, paint.Solid(grey))
		defer p.Layer(paint.LayerOpts{Bounds: geom.Rc(100, 100, 300, 300), Opacity: 0.5})()
		defer p.Blend(paint.BlendAdd)()
		p.RRect(geom.Rc(100, 100, 100, 100), 0, paint.Solid(light))
	})
	// Half of the light, added to the grey behind the layer.
	if got := pixelAt(pix, 150, 150); !near(got, [4]byte{0x80, 0x80, 0x80, 0xff}, 2) {
		t.Errorf("the added shape in a layer at half opacity is %v, want the grey and half the light", got)
	}
	if got := pixelAt(pix, 300, 300); !near(got, px(grey), 2) {
		t.Errorf("the rest of the layer is %v, want the grey behind it", got)
	}
}

func TestAnAddedShapeInAnEllipseIsCutToIt(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	pix := drawn(r, func(p *paint.Painter) {
		p.RRect(geom.Rc(100, 100, 200, 200), 0, paint.Solid(ember))
		defer p.Layer(paint.LayerOpts{Bounds: geom.Rc(100, 100, 200, 200), Opacity: 1, Clip: true, Ellipse: true})()
		defer p.Blend(paint.BlendAdd)()
		p.RRect(geom.Rc(100, 100, 200, 200), 0, paint.Solid(ember))
	})
	if got := pixelAt(pix, 200, 200); !near(got, [4]byte{0xc0, 0x60, 0x20, 0xff}, 2) {
		t.Errorf("inside the ellipse is %v, want the light added twice", got)
	}
	if got := pixelAt(pix, 103, 103); !near(got, px(ember), 2) {
		t.Errorf("in the corner outside the ellipse is %v, want the light added once", got)
	}
}
