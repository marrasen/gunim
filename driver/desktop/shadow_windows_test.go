package desktop

import (
	"testing"

	"github.com/marrasen/gunim/driver"
)

func TestADrawnShadowWindowLeavesAnEdgeOnlyForABorder(t *testing.T) {
	w := &Window{shadow: true, content: 1.5}
	if r, e := w.cornerRadius(); r != 12 || e != 2 {
		t.Fatalf("with the system's border, the corner is %v and the edge %v, want 12 and 2", r, e)
	}
	w.border = driver.Border{None: true}
	if r, e := w.cornerRadius(); r != 12 || e != 0 {
		t.Fatalf("with no border, the corner is %v and the edge %v, want 12 and 0", r, e)
	}
	w.shadow = false
	if r, e := w.cornerRadius(); r != 0 || e != 0 {
		t.Fatalf("with the system's shadow, the corner is %v and the edge %v, want 0 and 0", r, e)
	}
}
