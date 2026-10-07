package vecpath

import (
	"math"
	"testing"
)

func TestPathDataParsesIntoItsSubpaths(t *testing.T) {
	subs, err := Parse("M2 12h20M5 5v2")
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != 2 || subs[0][0].P[0] != (Pt{2, 12}) || subs[0][0].P[3] != (Pt{22, 12}) || subs[1][0].P[3] != (Pt{5, 7}) {
		t.Fatalf("parsed %v", subs)
	}
}

func TestAnArcFollowsItsCircle(t *testing.T) {
	subs, err := Parse("M2 12a10 10 0 1 0 20 0a10 10 0 1 0-20 0")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range Flatten(subs, func(p Pt) Pt { return p }, false, nil) {
		for _, p := range l.Pts {
			if r := Length(p.Sub(Pt{12, 12})); math.Abs(float64(r-10)) > 0.01 {
				t.Fatalf("a point on the circle lies %v from its centre, want 10", r)
			}
		}
	}
}

func TestATransformMovesEveryPointAndThenComposes(t *testing.T) {
	subs, _ := Parse("M1 2C3 4 5 6 7 8")
	move := Affine{A: 1, D: 1, E: 10}
	twice := Affine{A: 2, D: 2}
	got := Transform(subs, move.Then(twice))
	if got[0][0].P[0] != (Pt{22, 4}) || got[0][0].P[3] != (Pt{34, 16}) {
		t.Fatalf("moved then doubled: %v", got)
	}
	if subs[0][0].P[0] != (Pt{1, 2}) {
		t.Fatal("Transform changed its input")
	}
	lo, hi, ok := Bounds(subs)
	if !ok || lo != (Pt{1, 2}) || hi != (Pt{7, 8}) {
		t.Fatalf("bounds %v %v %v", lo, hi, ok)
	}
}

// square returns the closed square from x0, y0 to x1, y1, run clockwise or not.
func square(x0, y0, x1, y1 float32, clockwise bool) Polyline {
	pts := []Pt{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
	if !clockwise {
		pts = []Pt{{x0, y0}, {x0, y1}, {x1, y1}, {x1, y0}}
	}
	return Polyline{Pts: pts, Fill: true}
}

func TestAFillCoversItsInsideAndSmoothsItsEdges(t *testing.T) {
	m := Fill([]Polyline{square(2, 2, 7.5, 8, true)}, 10, 10, false)
	if m[5*10+4] != 255 || m[0] != 0 || m[5*10+9] != 0 {
		t.Fatalf("inside %d, corner %d, right of it %d", m[5*10+4], m[0], m[5*10+9])
	}
	// The right edge runs through the middle of column 7.
	if c := m[5*10+7]; c < 120 || c > 135 {
		t.Fatalf("the half-covered pixel is %d, want about 128", c)
	}
}

func TestAHoleStaysClearByEitherRule(t *testing.T) {
	outer := square(0, 0, 10, 10, true)
	// Run the other way, a hole by the nonzero rule; by even-odd, a hole either way.
	for _, c := range []struct {
		inner   Polyline
		evenOdd bool
	}{{square(3, 3, 7, 7, false), false}, {square(3, 3, 7, 7, true), true}, {square(3, 3, 7, 7, false), true}} {
		m := Fill([]Polyline{outer, c.inner}, 10, 10, c.evenOdd)
		if m[5*10+5] != 0 || m[1*10+1] != 255 {
			t.Fatalf("even-odd %v: the hole is %d, the ring %d", c.evenOdd, m[5*10+5], m[1*10+1])
		}
	}
	// Run the same way, by the nonzero rule, the inner square is no hole.
	if m := Fill([]Polyline{outer, square(3, 3, 7, 7, true)}, 10, 10, false); m[5*10+5] != 255 {
		t.Fatal("a square inside another, run the same way, is filled by the nonzero rule")
	}
}
