package desktop

import (
	"testing"

	"github.com/marrasen/gunim/geom"
)

func TestPopupAt(t *testing.T) {
	area := geom.Rc(0, 0, 1920, 1080)
	tests := []struct {
		name   string
		anchor geom.Rect
		w, h   float32
		x, y   float32
	}{
		{"below, at the left edge", geom.Rc(100, 100, 120, 36), 200, 300, 100, 136},
		{"above where the bottom runs out", geom.Rc(100, 900, 120, 36), 200, 300, 100, 600},
		{"below, slid up, when above has less room", geom.Rc(100, 150, 120, 720), 200, 300, 100, 780},
		{"slid in from the right edge", geom.Rc(1850, 100, 60, 36), 200, 300, 1720, 136},
		{"slid in from the left edge", geom.Rc(-50, 100, 60, 36), 200, 300, 0, 136},
	}
	for _, tt := range tests {
		x, y := popupAt(tt.anchor, tt.w, tt.h, area)
		if x != tt.x || y != tt.y {
			t.Errorf("%s: at (%v, %v), want (%v, %v)", tt.name, x, y, tt.x, tt.y)
		}
	}
}
