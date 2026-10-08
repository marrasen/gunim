package compass

import (
	"math"
	"testing"
)

// mat is a 3 by 3 matrix, row by row.
type mat [3][3]float64

func (a mat) mul(b mat) mat {
	var m mat
	for i := range 3 {
		for j := range 3 {
			for k := range 3 {
				m[i][j] += a[i][k] * b[k][j]
			}
		}
	}
	return m
}

func rad(deg float64) float64 { return deg * math.Pi / 180 }

// yaw turns a device clockwise by deg, seen from above, about the world's up.
func yaw(deg float64) mat {
	s, c := math.Sincos(rad(deg))
	return mat{{c, s, 0}, {-s, c, 0}, {0, 0, 1}}
}

// pitch tips a device's top edge up by deg, about its own x.
func pitch(deg float64) mat {
	s, c := math.Sincos(rad(deg))
	return mat{{1, 0, 0}, {0, c, -s}, {0, s, c}}
}

// roll lowers a device's right edge by deg, about its own y.
func roll(deg float64) mat {
	s, c := math.Sincos(rad(deg))
	return mat{{c, 0, s}, {0, 1, 0}, {-s, 0, c}}
}

// spin turns a device anticlockwise by deg in the plane of its screen, about its own z, as seen by the viewer.
func spin(deg float64) mat {
	s, c := math.Sincos(rad(deg))
	return mat{{c, -s, 0}, {s, c, 0}, {0, 0, 1}}
}

// lying returns the rotation matrix, as Android gives it, of a device that starts flat, screen up and its top edge
// north, and is turned by each of ms in turn, about its own axes as they lie by then.
func lying(ms ...mat) [9]float32 {
	m := mat{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}}
	for _, x := range ms {
		m = m.mul(x)
	}
	var r [9]float32
	for i := range 3 {
		for j := range 3 {
			r[3*i+j] = float32(m[i][j])
		}
	}
	return r
}

// near reports whether two headings are within tol degrees, round the circle.
func near(got, want, tol float64) bool {
	d := math.Mod(math.Abs(got-want), 360)
	return min(d, 360-d) <= tol
}

func TestAFlatDeviceFacesWhereItsTopEdgePoints(t *testing.T) {
	for _, deg := range []float64{0, 30, 90, 135, 180, 270, 359.5} {
		got, ok := Heading(lying(yaw(deg)), Rotation0)
		if !ok || !near(float64(got), deg, 1e-3) {
			t.Errorf("flat, its top edge at %v°, the heading is %v, %v; want %v", deg, got, ok, deg)
		}
	}
}

func TestATurnedScreenFacesWhereItsTopPoints(t *testing.T) {
	// The device's natural top edge points at 20°. Turned a quarter anticlockwise, the screen's top is the device's
	// right edge, a quarter clockwise round from it.
	for rot, want := range map[Rotation]float64{Rotation0: 20, Rotation90: 110, Rotation180: 200, Rotation270: 290} {
		got, ok := Heading(lying(yaw(20)), rot)
		if !ok || !near(float64(got), want, 1e-3) {
			t.Errorf("at rotation %d the heading is %v, %v; want %v", rot, got, ok, want)
		}
	}
}

func TestTippingTheDeviceUpKeepsItsHeading(t *testing.T) {
	// From flat to upright before the viewer, its back looking out, and a little past.
	for _, tip := range []float64{0, 20, 45, 70, 89, 90, 100} {
		got, ok := Heading(lying(yaw(250), pitch(tip)), Rotation0)
		if !ok || !near(float64(got), 250, 1e-3) {
			t.Errorf("tipped up %v°, the heading is %v, %v; want 250", tip, got, ok)
		}
	}
}

func TestRollingTheDeviceALittleKeepsItsHeading(t *testing.T) {
	// Flat, a right edge low turns the screen but not the way the top edge points.
	for _, r := range []float64{-30, -10, 10, 30} {
		got, ok := Heading(lying(yaw(60), roll(r)), Rotation0)
		if !ok || !near(float64(got), 60, 1e-3) {
			t.Errorf("flat and rolled %v°, the heading is %v, %v; want 60", r, got, ok)
		}
	}
	// Upright, a hand holding it crooked leaves its back looking the same way.
	for _, s := range []float64{-60, -45, -25, -10, 10, 25, 45, 60} {
		got, ok := Heading(lying(yaw(60), pitch(90), spin(s)), Rotation0)
		if !ok || !near(float64(got), 60, 1e-3) {
			t.Errorf("upright and crooked %v°, the heading is %v, %v; want 60", s, got, ok)
		}
	}
}

func TestADeviceOnItsSideFacesWhereItsTopEdgePoints(t *testing.T) {
	// Flat with its top edge east, then rolled onto its right edge: that edge points down, and the top edge still
	// points east.
	got, ok := Heading(lying(yaw(90), roll(90)), Rotation0)
	if !ok || !near(float64(got), 90, 1e-3) {
		t.Fatalf("on its side, the heading is %v, %v; want 90", got, ok)
	}
}

func TestTurningOntoItsSideMovesTheHeadingSmoothly(t *testing.T) {
	// Upright, its back looking at 60°, turned in quarter-degree steps until it lies on its side, its top edge
	// at 330°. The heading swings from the back to the top edge, with no jump between two readings.
	last := 60.0
	for s := 0.0; s <= 90; s += 0.25 {
		got, ok := Heading(lying(yaw(60), pitch(90), spin(s)), Rotation0)
		if !ok {
			t.Fatalf("crooked %v°, there is no heading", s)
		}
		if !near(float64(got), last, 2) {
			t.Fatalf("from crooked %v° to %v°, the heading jumps from %v to %v", s-0.25, s, last, got)
		}
		last = float64(got)
	}
	if !near(last, 330, 1e-3) {
		t.Fatalf("on its side, the heading is %v; want 330", last)
	}
}

func TestTheHeadingStaysBelow360(t *testing.T) {
	for _, deg := range []float64{-1e-9, 359.9999999, 360, 720} {
		got, ok := Heading(lying(yaw(deg)), Rotation0)
		if !ok || got < 0 || got >= 360 {
			t.Errorf("at %v° the heading is %v, %v; want it in [0, 360)", deg, got, ok)
		}
	}
}

func TestNoRotationHasNoHeading(t *testing.T) {
	if got, ok := Heading([9]float32{}, Rotation0); ok {
		t.Fatalf("an empty matrix gives the heading %v, want none", got)
	}
}
