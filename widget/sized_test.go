package widget

import (
	"testing"

	"github.com/marrasen/gunim/geom"
)

func TestSizedKeepsItsChildWithinTheParent(t *testing.T) {
	s := newSpot(10, 10)
	sized := NewSized(s, 300, 400)
	_, run := stage(t, &frame{child: sized, size: geom.Sz(200, 100)})
	run(1)
	if s.box != geom.Sz(200, 100) {
		t.Fatalf("a Sized of 300 by 400 in 200 by 100 laid its child out at %v, want 200 by 100", s.box)
	}
	// Within loose limits, as a column gives, the width shrinks to fit.
	loose := newSpot(10, 10)
	_, run = stage(t, &frame{child: Column(NewSized(loose, 300, 0)), size: geom.Sz(200, 100)})
	run(1)
	if loose.box.W != 200 {
		t.Fatalf("a Sized 300 wide in a 200 px column laid its child out %v wide, want 200", loose.box.W)
	}
}
