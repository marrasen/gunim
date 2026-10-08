package gunim

import "testing"

func TestADragStartsToScrollAZoneFromTheEdge(t *testing.T) {
	// A view 400 long: the zone is 48 at each end.
	if v := EdgeSpeed(200, 0, 400); v != 0 {
		t.Fatalf("a drag in the middle scrolls at %v", v)
	}
	if v := EdgeSpeed(400-EdgeZone-1, 0, 400); v != 0 {
		t.Fatalf("a drag just outside the zone scrolls at %v", v)
	}
	if v := EdgeSpeed(400-EdgeZone+1, 0, 400); v < edgeSlow {
		t.Fatalf("a drag just inside the zone scrolls at %v, want at least %v", v, edgeSlow)
	}
	if v := EdgeSpeed(EdgeZone-1, 0, 400); v > -edgeSlow {
		t.Fatalf("a drag just inside the top's zone scrolls at %v, want up at least %v", v, edgeSlow)
	}
}

func TestADragScrollsFasterNearerAndPastTheEdge(t *testing.T) {
	last := float32(0)
	for p := float32(400 - EdgeZone + 1); p <= 400+3*EdgeZone; p += 4 {
		v := EdgeSpeed(p, 0, 400)
		if v < last {
			t.Fatalf("at %v the scroll slows, from %v to %v", p, last, v)
		}
		last = v
	}
	if v := EdgeSpeed(400, 0, 400); v != edgeFast {
		t.Fatalf("at the edge the scroll is %v, want %v", v, edgeFast)
	}
	if v := EdgeSpeed(400+2*EdgeZone, 0, 400); v != edgeFastest {
		t.Fatalf("two zones past the edge the scroll is %v, want %v", v, edgeFastest)
	}
	if v := EdgeSpeed(-1000, 0, 400); v != -edgeFastest {
		t.Fatalf("far above the top the scroll is %v, want %v", v, -edgeFastest)
	}
}

func TestASmallViewHasAQuarterAtEachEnd(t *testing.T) {
	// A view 100 long below a header 20 tall: the zone is 20.
	if v := EdgeSpeed(99, 20, 100); v <= 0 {
		t.Fatalf("a drag at the bottom does not scroll down: %v", v)
	}
	if v := EdgeSpeed(79, 20, 100); v != 0 {
		t.Fatalf("a drag a quarter up from the bottom scrolls at %v", v)
	}
	if v := EdgeSpeed(60, 20, 20); v != 0 {
		t.Fatalf("a view with no room scrolls at %v", v)
	}
}
