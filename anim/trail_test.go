package anim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
)

func TestATrailGivesTheSpeedAsItWasLetGo(t *testing.T) {
	var tr Trail
	t0 := time.Unix(100, 0)
	// A slow start, then 600 across and 300 down a second for the last
	// tenth of a second: only the end counts.
	tr.Add(geom.Pt(0, 0), t0)
	tr.Add(geom.Pt(1, 0), t0.Add(200*time.Millisecond))
	for i := 1; i <= 6; i++ {
		tr.Add(geom.Pt(1+float32(i)*10, float32(i)*5), t0.Add(200*time.Millisecond+time.Duration(i)*1000*time.Millisecond/60))
	}
	end := t0.Add(300 * time.Millisecond)
	v := tr.Velocity(end)
	if v.X < 590 || v.X > 610 || v.Y < 295 || v.Y > 305 {
		t.Fatalf("the trail's velocity is %v, want about (600, 300)", v)
	}
	// Held still before letting go: no speed.
	if v := tr.Velocity(end.Add(200 * time.Millisecond)); v != (geom.Point{}) {
		t.Fatalf("a drag at rest for a fifth of a second gives %v", v)
	}
	tr.Reset()
	if v := tr.Velocity(end); v != (geom.Point{}) {
		t.Fatalf("a trail reset gives %v", v)
	}
}
