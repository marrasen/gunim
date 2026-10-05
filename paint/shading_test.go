package paint

import (
	"image/color"
	"testing"

	"github.com/marrasen/gunim/geom"
)

func TestAnEllipseClipTakesInputWithinItAlone(t *testing.T) {
	var p Painter
	end := p.Layer(LayerOpts{Bounds: geom.Rc(0, 0, 200, 100), Clip: true, Ellipse: true})
	defer end()
	c := p.Clip()
	for _, in := range []geom.Point{{X: 100, Y: 50}, {X: 2, Y: 50}, {X: 100, Y: 2}} {
		if !c.Contains(in) {
			t.Errorf("%v, inside the ellipse, is clipped", in)
		}
	}
	for _, out := range []geom.Point{{X: 5, Y: 5}, {X: 195, Y: 95}, {X: 30, Y: 10}} {
		if c.Contains(out) {
			t.Errorf("%v, in the box's corner past the ellipse, takes input", out)
		}
	}
}

func TestAGradientsStopsAreDamage(t *testing.T) {
	stops := func(c color.NRGBA) Fill {
		return Fill{Gradient: &Gradient{To: geom.Pt(10, 0), Stops: []Stop{{At: 0.5, Color: c}}}}
	}
	var p Painter
	p.Reset()
	p.RRect(geom.Rc(0, 0, 10, 10), 0, stops(color.NRGBA{R: 1}))
	p.Reset()
	p.RRect(geom.Rc(0, 0, 10, 10), 0, stops(color.NRGBA{R: 1}))
	if d := p.Damage(); !d.Empty() {
		t.Errorf("the same stops in a new gradient damage %v, want nothing", d)
	}
	p.Reset()
	p.RRect(geom.Rc(0, 0, 10, 10), 0, stops(color.NRGBA{R: 2}))
	if d := p.Damage(); d.Empty() {
		t.Error("a stop of another colour damages nothing")
	}
	p.Reset()
	p.MaskFill(square{}, geom.Rc(0, 0, 10, 10), stops(color.NRGBA{R: 2}))
	p.Reset()
	p.MaskFill(square{}, geom.Rc(0, 0, 10, 10), stops(color.NRGBA{R: 3}))
	if d := p.Damage(); d.Empty() {
		t.Error("a mask's gradient of another colour damages nothing")
	}
}

func TestADrawnRRectCoversItsShadowAndStroke(t *testing.T) {
	var p Painter
	p.Reset()
	defer p.Push(Translate(geom.Pt(100, 0)))()
	p.DrawRRect(RRectOp{Rect: geom.Rc(0, 0, 10, 10), Stroke: Stroke{Width: 4, Color: color.NRGBA{A: 1}},
		Shadow: Shadow{Offset: geom.Pt(0, 20), Blur: 5, Color: color.NRGBA{A: 1}},
		Inset:  [2]Shadow{{Blur: 3, Color: color.NRGBA{A: 1}}}})
	op, ok := p.Ops()[0].(*RRectOp)
	if !ok || op.Transform != Translate(geom.Pt(100, 0)) || op.Inset[0].Blur != 3 {
		t.Fatalf("recorded %+v, want the op in the transform in force", p.Ops()[0])
	}
	d := p.Damage()
	if d.Min.X > 98 || d.Min.Y > -2 || d.Max.X < 112 || d.Max.Y < 35 {
		t.Errorf("the op covers %v, want its stroke and its shadow, below it", d)
	}
}
