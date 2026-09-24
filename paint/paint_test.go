package paint

import (
	"image/color"
	"math"
	"testing"

	"github.com/marrasen/gunim/geom"
)

func TestInvertUndoesTheTransform(t *testing.T) {
	c, s := float32(math.Cos(0.7)), float32(math.Sin(0.7))
	tr := Translate(geom.Pt(30, -12)).Mul(Transform{A: 2 * c, B: -2 * s, D: 2 * s, E: 2 * c}).Mul(Scale(1.5, geom.Pt(4, 9)))
	inv, ok := tr.Invert()
	if !ok {
		t.Fatal("an invertible transform reported no inverse")
	}
	for _, p := range []geom.Point{{}, geom.Pt(10, 20), geom.Pt(-7, 3.5)} {
		back := inv.Apply(tr.Apply(p))
		if d := back.Sub(p); d.X*d.X+d.Y*d.Y > 1e-6 {
			t.Fatalf("%v came back as %v", p, back)
		}
	}
}

func TestInvertOfAZeroScaleReportsNone(t *testing.T) {
	if _, ok := Scale(0, geom.Pt(5, 5)).Invert(); ok {
		t.Fatal("a transform onto a single point reported an inverse")
	}
}

func TestClipContainsOnlyWhatEveryClipShows(t *testing.T) {
	var p Painter
	if !p.Clip().Contains(geom.Pt(-1000, 1000)) {
		t.Fatal("with nothing clipping, a point was clipped")
	}
	closeOuter := p.Layer(LayerOpts{Bounds: geom.Rc(0, 0, 100, 100), Clip: true})
	closeInner := p.Layer(LayerOpts{Bounds: geom.Rc(50, 0, 100, 100), Clip: true, Radius: 10})
	c := p.Clip()
	cases := []struct {
		p    geom.Point
		want bool
	}{
		{geom.Pt(75, 50), true},   // inside both
		{geom.Pt(25, 50), false},  // inside the outer only
		{geom.Pt(125, 50), false}, // inside the inner only
		{geom.Pt(51, 1), false},   // in the inner's rounded corner
	}
	for _, tc := range cases {
		if got := c.Contains(tc.p); got != tc.want {
			t.Errorf("Contains(%v) = %v, want %v", tc.p, got, tc.want)
		}
	}
	closeInner()
	if !p.Clip().Contains(geom.Pt(25, 50)) {
		t.Fatal("closing the inner layer left its clip in force")
	}
	closeOuter()
	if p.Clip() != nil {
		t.Fatal("closing every layer left a clip in force")
	}
	// A clip survives its frame, for input against what was shown.
	if !c.Contains(geom.Pt(75, 50)) {
		t.Fatal("a clip changed after its layers closed")
	}
}

func TestZeroPainterStartsAtTheIdentity(t *testing.T) {
	var p Painter
	p.RRect(geom.Rc(10, 20, 30, 40), 0, Fill{})
	op, ok := p.Ops()[0].(*RRectOp)
	if !ok || op.Transform != Identity {
		t.Fatalf("a zero Painter recorded %+v, want the identity transform", p.Ops()[0])
	}
	if b := p.bounds[0]; b != geom.Rc(8.5, 18.5, 33, 43) {
		t.Fatalf("bounds %v, want the rectangle drawn and a pixel and a half", b)
	}
}

// frame records one frame of three buttons, the middle one in colour c,
// and one layer around them all.
func frame(p *Painter, c color.NRGBA) {
	p.Reset()
	defer p.Layer(LayerOpts{Bounds: geom.Rc(0, 0, 400, 100), Opacity: 1, Clip: true})()
	p.RRect(geom.Rc(10, 10, 80, 30), 4, Solid(color.NRGBA{A: 255}))
	p.RRect(geom.Rc(110, 10, 80, 30), 4, Solid(c))
	p.RRect(geom.Rc(210, 10, 80, 30), 4, Solid(color.NRGBA{A: 255}))
}

func TestDamageIsWhatChanged(t *testing.T) {
	var p Painter
	frame(&p, color.NRGBA{R: 10, A: 255})
	if p.Damage() != Everything {
		t.Fatal("the first frame's damage is less than everything")
	}
	frame(&p, color.NRGBA{R: 10, A: 255})
	if d := p.Damage(); !d.Empty() {
		t.Fatalf("an unchanged frame damaged %v", d)
	}
	frame(&p, color.NRGBA{R: 20, A: 255})
	if d := p.Damage(); d != geom.Rc(108.5, 8.5, 83, 33) {
		t.Fatalf("a recoloured button damaged %v, want just the button", d)
	}
}

func TestDamageOfAChangedLayerCoversItsContents(t *testing.T) {
	var p Painter
	draw := func(opacity float32) {
		p.Reset()
		defer p.Layer(LayerOpts{Bounds: geom.Rc(0, 0, 50, 50), Opacity: opacity})()
		p.RRect(geom.Rc(100, 100, 20, 20), 0, Solid(color.NRGBA{A: 255}))
	}
	draw(1)
	draw(0.5)
	if d := p.Damage(); !d.Contains(geom.Pt(110, 110)) || !d.Contains(geom.Pt(25, 25)) {
		t.Fatalf("a layer fading damaged %v, want its bounds and all it holds", d)
	}
}

func TestBlurDamagesEverything(t *testing.T) {
	var p Painter
	draw := func() {
		p.Reset()
		defer p.Layer(LayerOpts{Bounds: geom.Rc(0, 0, 50, 50), Opacity: 1, Backdrop: 8})()
	}
	draw()
	draw()
	if p.Damage() != Everything {
		t.Fatal("a frame that blurs damaged less than everything")
	}
}
