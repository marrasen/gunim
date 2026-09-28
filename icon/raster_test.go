package icon

import (
	"bytes"
	"math"
	"testing"
)

func TestPathDataParses(t *testing.T) {
	for _, d := range []string{
		"M2 12h20",
		"M10 20a1 1 0 0 0 .553.895l2 1A1 1 0 0 0 14 21v-7",
		"M4 4a2 2 0 012 2",
		"M1-2.5.5e1 3zm1 1c1 1 2 2 3 3s1 1 2 2q1 0 2 2t2 2",
		"M12 17h.01",
	} {
		if err := (&Icon{Path: d}).Check(); err != nil {
			t.Errorf("%q: %v", d, err)
		}
	}
	for _, d := range []string{"12 3", "M1", "M1 1 a1 1 0 2 0 3 3", "M1 1 L x"} {
		if err := (&Icon{Path: d}).Check(); err == nil {
			t.Errorf("%q parsed, want an error", d)
		}
	}
	subs, _ := parse("M2 12h20M5 5v2")
	if len(subs) != 2 || subs[0][0].p[0] != (pt{2, 12}) || subs[0][0].p[3] != (pt{22, 12}) || subs[1][0].p[3] != (pt{5, 7}) {
		t.Fatalf("parsed %v", subs)
	}
}

func TestAnArcFollowsItsCircle(t *testing.T) {
	subs, err := parse("M2 12a10 10 0 1 0 20 0a10 10 0 1 0-20 0")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range flatten(subs, 1, pt{}, false, nil) {
		for _, p := range l.pts {
			if r := length(p.sub(pt{12, 12})); math.Abs(float64(r-10)) > 0.01 {
				t.Fatalf("a point on the circle lies %v from its centre, want 10", r)
			}
		}
	}
}

// cov returns the coverage at x, y of an icon drawn w pixels square.
func cov(m []byte, w, x, y int) byte { return m[y*w+x] }

func TestALineCoversItsPixelsWithRoundCaps(t *testing.T) {
	line := &Icon{Path: "M4 12h16"}
	m := line.Stroke().Coverage(24, 24)
	for x := 4; x < 20; x++ {
		if cov(m, 24, x, 11) != 255 || cov(m, 24, x, 12) != 255 {
			t.Fatalf("pixel %d on the line is %d and %d, want both rows covered", x, cov(m, 24, x, 11), cov(m, 24, x, 12))
		}
		if cov(m, 24, x, 10) != 0 || cov(m, 24, x, 13) != 0 {
			t.Fatalf("pixel %d beside the line is %d and %d, want nothing", x, cov(m, 24, x, 10), cov(m, 24, x, 13))
		}
	}
	// At 48 pixels the line runs from x 8 to 40 on y 24, 4 wide, and its caps are half circles of radius 2.
	m = line.Stroke().Coverage(48, 48)
	if c := cov(m, 48, 7, 23); c != 255 {
		t.Errorf("the cap next to the end is %d, want covered", c)
	}
	if c := cov(m, 48, 6, 22); c == 0 || c > 128 {
		t.Errorf("the cap's outer corner is %d, want partly covered as a round cap leaves it", c)
	}
	if c := cov(m, 48, 5, 24); c != 0 {
		t.Errorf("past the cap is %d, want nothing", c)
	}
}

func TestADiagonalIsAntialiased(t *testing.T) {
	m := (&Icon{Path: "M4 4l16 13"}).Stroke().Coverage(24, 24)
	partial := 0
	for _, c := range m {
		if c > 0 && c < 255 {
			partial++
		}
	}
	if partial < 20 {
		t.Fatalf("%d pixels are partly covered, want the diagonal's edges softened", partial)
	}
}

func TestACircleStaysRound(t *testing.T) {
	const n = 96
	m := (&Icon{Path: "M2 12a10 10 0 1 0 20 0a10 10 0 1 0-20 0"}).Stroke().Coverage(n, n)
	// The coverage-weighted radius in each of 16 directions.
	var sum, weight [16]float64
	for y := range n {
		for x := range n {
			c := float64(m[y*n+x])
			if c == 0 {
				continue
			}
			dx, dy := float64(x)+0.5-n/2, float64(y)+0.5-n/2
			bin := int((math.Atan2(dy, dx) + math.Pi) / (2 * math.Pi) * 16)
			bin = min(bin, 15)
			sum[bin] += c * math.Hypot(dx, dy)
			weight[bin] += c
		}
	}
	for i := range sum {
		if r := sum[i] / weight[i]; math.Abs(r-40) > 0.2 {
			t.Errorf("direction %d has radius %.2f, want 40", i, r)
		}
	}
}

func TestProgressLimitsTheStrokedLength(t *testing.T) {
	line := &Icon{Path: "M4 12h16"}
	for _, c := range (Stroke{Icon: line, Width: 2}).Coverage(48, 48) {
		if c != 0 {
			t.Fatal("a stroke at progress 0 drew something")
		}
	}
	full, whole := (Stroke{Icon: line, Width: 2, Progress: 1}).Coverage(48, 48), line.Stroke().Coverage(48, 48)
	if !bytes.Equal(full, whole) {
		t.Fatal("a stroke at progress 1 differs from the settled one")
	}
	half := (Stroke{Icon: line, Width: 2, Progress: 0.5}).Coverage(48, 48)
	if cov(half, 48, 12, 24) != 255 || cov(half, 48, 34, 24) != 0 {
		t.Fatalf("half drawn, the line covers %d near its start and %d near its end, want the start alone",
			cov(half, 48, 12, 24), cov(half, 48, 34, 24))
	}
	if (Stroke{Icon: line, Width: 2, Progress: 0.5}).Settled() || !line.Stroke().Settled() {
		t.Fatal("a stroke part drawn reads as settled, or a whole one as not")
	}
}

func TestAFilledDotIsSolid(t *testing.T) {
	dot := &Icon{Fill: "M12 6a6 6 0 1 0 0 12a6 6 0 1 0 0-12z"}
	m := (Stroke{Icon: dot, Width: 0.5, Progress: 1}).Coverage(24, 24)
	if cov(m, 24, 12, 12) != 255 || cov(m, 24, 9, 12) != 255 {
		t.Fatalf("the middle of a filled circle is %d and %d, want covered", cov(m, 24, 12, 12), cov(m, 24, 9, 12))
	}
	if c := cov(m, 24, 12, 3); c != 0 {
		t.Fatalf("outside the filled circle is %d", c)
	}
}
