package glfw

import "testing"

func TestTheAccentSettingReads(t *testing.T) {
	c, on, err := accentBorder()
	if err != nil {
		t.Fatal(err)
	}
	if on && c.A != 0xff {
		t.Fatalf("the accent colour %v is not opaque", c)
	}
}

func TestARoundedShadowFallsAwayAtItsCornersAlone(t *testing.T) {
	c := roundedShade{x0: 100, y0: 100, x1: 900, y1: 700, r: 39, sigma: 34}
	if cut := c.cut(500, 60); cut != 0 {
		t.Errorf("above the middle of the top edge, the corners cut %v, want 0", cut)
	}
	corner := c.cut(105, 105)
	if corner <= 0 || corner >= 1 {
		t.Errorf("just inside the corner, the corners cut %v, want a part of the shadow", corner)
	}
	if far := c.cut(80, 80); far <= 0 || far >= corner {
		t.Errorf("beyond the corner the corners cut %v, want less than the %v just inside it", far, corner)
	}
	if square := (roundedShade{x0: 100, y0: 100, x1: 900, y1: 700, sigma: 34}).cut(105, 105); square != 0 {
		t.Errorf("square corners cut %v, want 0", square)
	}
}

func TestASmallWindowsShadowStripsDoNotOverlap(t *testing.T) {
	s := &drawnShadow{corner: 26, scale: 1, active: true}
	if err := s.draw(60, 40, 144, nil); err != nil {
		t.Fatal(err)
	}
	st := s.strips()
	if top, bottom := st[0], st[1]; top.y+top.h > bottom.y || st[2].h < 0 || st[3].h < 0 {
		t.Fatalf("the strips of a 60 by 40 window with round corners are %+v", st)
	}
}
