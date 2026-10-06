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

func TestAnInverseUndoesTheMatrix(t *testing.T) {
	m := Perspective(1, 1.3, 0.1, 50).Mul(LookAt(V3(1, 2, 6), V3(0, 0, 0), V3(0, 1, 0))).Mul(TurnY(0.4)).Mul(Scale3(V3(1, 2, 0.5)))
	inv, ok := m.Invert()
	if !ok {
		t.Fatal("an invertible matrix reported no inverse")
	}
	for _, p := range []Vec3{{}, V3(0.3, -0.2, 0.5), V3(-1, 1, 1)} {
		if back := inv.Apply(m.Apply(p)); !close3(back, p) {
			t.Errorf("%v came back as %v", p, back)
		}
	}
	if _, ok := Scale3(V3(1, 0, 1)).Invert(); ok {
		t.Error("a matrix that folds space flat reported an inverse")
	}
}

// A camera looking straight down or up along +Y sees as one just off
// that line, in front, does: nothing collapses, and the picture's up is
// -Z looking down, +Z looking up.
func TestLookAtAlongUp(t *testing.T) {
	for _, c := range []struct {
		eye  Vec3
		upIs Vec3
	}{
		{V3(0, 5, 0), V3(0, 0, -1)},
		{V3(0, -5, 0), V3(0, 0, 1)},
	} {
		view := LookAt(c.eye, V3(0, 0, 0), V3(0, 1, 0))
		near := LookAt(c.eye.Add(V3(0, 0, 1e-3)), V3(0, 0, 0), V3(0, 1, 0))
		for i := range view {
			if math.IsNaN(float64(view[i])) || math.Abs(float64(view[i]-near[i])) > 1e-3 {
				t.Fatalf("from %v the view is %v; just off the line it is %v", c.eye, view, near)
			}
		}
		// The picture's up, in the scene, is the view's second row.
		if got := V3(view[1], view[5], view[9]); got.Sub(c.upIs).Len() > 1e-5 {
			t.Errorf("from %v the picture's up is %v, want %v", c.eye, got, c.upIs)
		}
		// The point looked at lies straight ahead, 5 away.
		if at := view.Apply(V3(0, 0, 0)); at.Sub(V3(0, 0, -5)).Len() > 1e-5 {
			t.Errorf("from %v the point looked at is at %v in the camera's space", c.eye, at)
		}
	}
}
