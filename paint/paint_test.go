package paint

import (
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
