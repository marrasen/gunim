package paint

import (
	"image/color"
	"testing"

	"github.com/marrasen/gunim/geom"
)

// damageOf paints two frames with the painter and returns the second's
// damage as rectangles.
func damageOf(first, second func(p *Painter)) []geom.Rect {
	var p Painter
	p.Reset()
	first(&p)
	p.Reset()
	second(&p)
	return p.DamageRects()
}

var (
	dark  = Solid(color.NRGBA{A: 0xff})
	light = Solid(color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
)

func TestDamageFarApartIsTwoRects(t *testing.T) {
	paint := func(top, bottom Fill) func(p *Painter) {
		return func(p *Painter) {
			p.RRect(geom.Rc(0, 0, 2000, 1000), 0, dark)
			// A playhead at the top, a meter at the bottom.
			p.RRect(geom.Rc(500, 0, 2, 100), 0, top)
			p.RRect(geom.Rc(1500, 900, 8, 80), 0, bottom)
		}
	}
	rs := damageOf(paint(dark, dark), paint(light, light))
	if len(rs) != 2 {
		t.Fatalf("damage %v, want two rects", rs)
	}
	var p Painter
	p.Reset()
	paint(dark, dark)(&p)
	p.Reset()
	paint(light, light)(&p)
	if d, want := p.Damage(), rs[0].Union(rs[1]); d != want {
		t.Fatalf("Damage is %v, want %v, the rectangle round both", d, want)
	}
}

func TestDamageNearTogetherIsOneRect(t *testing.T) {
	paint := func(f Fill) func(p *Painter) {
		return func(p *Painter) {
			// A meter's lights, a few pixels apart.
			for i := range 10 {
				p.RRect(geom.Rc(100, float32(10+i*6), 8, 4), 0, f)
			}
		}
	}
	if rs := damageOf(paint(dark), paint(light)); len(rs) != 1 {
		t.Fatalf("damage %v, want one rect", rs)
	}
}

func TestDamageOfManyScatteredChangesIsAFewRects(t *testing.T) {
	paint := func(f Fill) func(p *Painter) {
		return func(p *Painter) {
			for i := range 50 {
				p.RRect(geom.Rc(float32(i*97%1900), float32(i*53%900), 4, 4), 0, f)
			}
		}
	}
	if rs := damageOf(paint(dark), paint(light)); len(rs) > MaxDamageRects || len(rs) == 0 {
		t.Fatalf("damage is %d rects, want 1 to %d", len(rs), MaxDamageRects)
	}
}

func TestDamageThatCoversMostOfItsBoundsIsOneRect(t *testing.T) {
	paint := func(f Fill) func(p *Painter) {
		return func(p *Painter) {
			p.RRect(geom.Rc(0, 0, 100, 100), 0, f)
			p.RRect(geom.Rc(0, 140, 100, 100), 0, f)
		}
	}
	if rs := damageOf(paint(dark), paint(light)); len(rs) != 1 {
		t.Fatalf("damage %v, want one rect: the gap is small beside the two", rs)
	}
}

func TestDamageOfNothingChangedIsNone(t *testing.T) {
	paint := func(p *Painter) { p.RRect(geom.Rc(0, 0, 10, 10), 0, dark) }
	if rs := damageOf(paint, paint); len(rs) != 0 {
		t.Fatalf("damage %v, want none", rs)
	}
}

func TestDamageOfTheFirstFrameIsEverything(t *testing.T) {
	var p Painter
	p.RRect(geom.Rc(0, 0, 10, 10), 0, dark)
	if rs := p.DamageRects(); len(rs) != 1 || rs[0] != Everything {
		t.Fatalf("damage %v, want Everything", rs)
	}
}
