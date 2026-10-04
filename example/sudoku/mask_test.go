package main

import (
	"math"
	"testing"
	"time"
)

func TestAShapesFieldDrawsItAsItsDistanceDoes(t *testing.T) {
	for shape := range 9 {
		const w = 96
		m := candyMask{shape: shape, grow: 0.05}
		got := m.Coverage(w, w)
		worst := 0.0
		for y := range w {
			for x := range w {
				px := (float64(x) + 0.5 - w/2) / (w / 2)
				py := (float64(y) + 0.5 - w/2) / (w / 2)
				d := shapeDistance(shape, px, py) - 0.05
				want := 255 * max(0, min(1, 0.5-d*w/2))
				worst = max(worst, math.Abs(float64(got[y*w+x])-want))
			}
		}
		// A few levels of 255 at the star's points, where the field rounds most.
		if worst > 16 {
			t.Errorf("shape %d: the field's mask differs from the shape's by %v of 255", shape, worst)
		}
	}
}

func TestMasksAreCheapOnceTheFieldIsMade(t *testing.T) {
	for shape := range 9 {
		fieldOf(shape)
	}
	start := time.Now()
	for shape := range 9 {
		for size := 20; size < 160; size += 10 {
			candyMask{shape: shape}.Coverage(size, size)
		}
	}
	if took := time.Since(start); took > 150*time.Millisecond {
		t.Errorf("126 masks took %v", took)
	}
}
