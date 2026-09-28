package desktop

import "testing"

func TestADrawnShadowWindowLeavesAnEdgeAsWideAsItsBorder(t *testing.T) {
	w := &Window{shadow: true, content: 1.5, edge: 2}
	if r, e := w.cornerRadius(); r != 12 || e != 2 {
		t.Fatalf("with the border 2 px wide, the corner is %v and the edge %v, want 12 and 2", r, e)
	}
	w.edge = 3.5
	if r, e := w.cornerRadius(); r != 12 || e != 3.5 {
		t.Fatalf("with the border 3.5 px wide, the corner is %v and the edge %v, want 12 and 3.5", r, e)
	}
	w.shadow = false
	if r, e := w.cornerRadius(); r != 0 || e != 0 {
		t.Fatalf("with the system's shadow, the corner is %v and the edge %v, want 0 and 0", r, e)
	}
}
