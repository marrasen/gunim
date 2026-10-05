package geom

import (
	"math"
	"testing"
)

func close3(a, b Vec3) bool { return a.Sub(b).Len() < 1e-4 }

func TestTurnsMoveTheAxesAsTheySay(t *testing.T) {
	q := float32(math.Pi / 2)
	cases := []struct {
		m        Mat4
		in, want Vec3
	}{
		{TurnX(q), V3(0, 1, 0), V3(0, 0, 1)},
		{TurnY(q), V3(0, 0, 1), V3(1, 0, 0)},
		{TurnZ(q), V3(1, 0, 0), V3(0, 1, 0)},
		{Move3(V3(1, 2, 3)).Mul(Scale3(V3(2, 2, 2))), V3(1, 1, 1), V3(3, 4, 5)},
	}
	for _, c := range cases {
		if got := c.m.Apply(c.in); !close3(got, c.want) {
			t.Errorf("%v went to %v, want %v", c.in, got, c.want)
		}
	}
}

func TestACameraSeesWhatItLooksAtInTheMiddle(t *testing.T) {
	view := LookAt(V3(3, 2, 5), V3(1, 0, -1), V3(0, 1, 0))
	proj := Perspective(math.Pi/3, 1.5, 0.1, 100)
	if got := proj.Mul(view).Apply(V3(1, 0, -1)); math.Abs(float64(got.X)) > 1e-4 || math.Abs(float64(got.Y)) > 1e-4 {
		t.Errorf("the point looked at shows at %v, want the middle of the view", got)
	}
	// A point nearer the camera lies nearer the front of the depth range.
	nearZ := proj.Mul(view).Apply(V3(2.5, 1.5, 3.5)).Z
	farZ := proj.Mul(view).Apply(V3(-1, -2, -7)).Z
	if !(nearZ < farZ) || nearZ < -1 || farZ > 1 {
		t.Errorf("depths %v near and %v far, want near below far, within -1 to 1", nearZ, farZ)
	}
}

func TestANormalStaysUprightOnAStretchedSurface(t *testing.T) {
	// A slope at 45 degrees, stretched twice as wide, leans less, and its
	// normal leans more upright to match.
	m := Scale3(V3(2, 1, 1))
	n := m.NormalMatrix()
	normal := V3(-1, 1, 0).Unit()
	got := V3(n[0]*normal.X+n[3]*normal.Y+n[6]*normal.Z, n[1]*normal.X+n[4]*normal.Y+n[7]*normal.Z,
		n[2]*normal.X+n[5]*normal.Y+n[8]*normal.Z).Unit()
	// The stretched slope runs along (2, 1, 0).
	if d := got.Dot(V3(2, 1, 0).Unit()); math.Abs(float64(d)) > 1e-4 {
		t.Errorf("the normal %v leans %v along the stretched slope, want it at right angles", got, d)
	}
	// A turn keeps a normal as it turns the surface.
	tn := TurnZ(0.7).NormalMatrix()
	up := V3(tn[3], tn[4], tn[5])
	if want := TurnZ(0.7).Apply(V3(0, 1, 0)); !close3(up, want) {
		t.Errorf("a turned surface's normal is %v, want %v", up, want)
	}
}
