package icon

import (
	"math"
	"testing"
	"time"
)

// A width that is not a number is redrawn every frame, where kept it would never be found again.
func TestAStrokeOfNoNumberWideIsNotKept(t *testing.T) {
	s := Stroke{Icon: X, Width: float32(math.NaN()), Progress: 1}
	if s.Settled() {
		t.Fatal("a stroke NaN wide says it is settled")
	}
}

// A curve with huge control points is drawn in bounded time and memory.
func TestAHugeCurveIsFlattenedInBoundedSteps(t *testing.T) {
	start := time.Now()
	for _, c := range []string{"M0 0C1e18 0 0 1e18 1 1", "M0 0C1e30 0 0 1e30 1 1"} {
		(&Icon{Path: c}).Stroke().Coverage(24, 24)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("two huge curves took %v", d)
	}
}
