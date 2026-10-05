package paint

import (
	"image/color"
	"math"
	"testing"

	"github.com/marrasen/gunim/geom"
)

func TestAProjectionTakesPointsBackWhereTheyCameFrom(t *testing.T) {
	var p Painter
	p.Reset()
	defer p.Push(Translate(geom.Pt(40, 30)))()
	endOuter := p.Layer(LayerOpts{Bounds: geom.Rc(0, 0, 400, 300), Tilt: Tilt{X: 0.3, Distance: 700}})
	defer endOuter()
	endInner := p.Layer(LayerOpts{Bounds: geom.Rc(100, 50, 200, 100), Tilt: Tilt{Y: -0.8}})
	defer endInner()
	pr := p.Projection()
	for _, flat := range []geom.Point{{X: 140, Y: 80}, {X: 330, Y: 190}, {X: 200, Y: 120}} {
		back, ok := pr.Unapply(pr.Apply(flat))
		if !ok || math.Abs(float64(back.X-flat.X)) > 1e-2 || math.Abs(float64(back.Y-flat.Y)) > 1e-2 {
			t.Errorf("%v showed at %v and came back as %v, %v", flat, pr.Apply(flat), back, ok)
		}
	}
	// The middle of a tilted layer stays where it was.
	mid := geom.Pt(240, 130)
	if got := pr.outer.Apply(mid); math.Abs(float64(got.X-mid.X)) > 1e-3 {
		t.Errorf("the outer layer's middle, tipped back about its level line, moved across to %v", got)
	}
}

func TestATiltIsDamageEverywhere(t *testing.T) {
	var p Painter
	p.Reset()
	p.RRect(geom.Rc(0, 0, 10, 10), 0, Solid(color.NRGBA{A: 0xff}))
	p.Reset()
	end := p.Layer(LayerOpts{Bounds: geom.Rc(0, 0, 10, 10), Tilt: Tilt{Y: 0.1}})
	p.RRect(geom.Rc(0, 0, 10, 10), 0, Solid(color.NRGBA{A: 0xff}))
	end()
	if d := p.Damage(); d != Everything {
		t.Errorf("a frame that tilts damages %v, want everything", d)
	}
}

func TestAClipInATiltedLayerHoldsWhereItShows(t *testing.T) {
	var p Painter
	p.Reset()
	end := p.Layer(LayerOpts{Bounds: geom.Rc(0, 0, 200, 100), Clip: true, Tilt: Tilt{Y: 1.1, Distance: 400}})
	defer end()
	h := Tilt{Y: 1.1, Distance: 400}.Homography(geom.Pt(100, 50))
	in, _ := h.Apply(geom.Pt(190, 50))
	if !p.Clip().Contains(in) {
		t.Errorf("%v, where the clip's 190, 50 shows, is clipped", in)
	}
	if p.Clip().Contains(geom.Pt(195, 50)) {
		t.Error("195, 50, where the clip lay flat and shows no more, takes input")
	}
}
