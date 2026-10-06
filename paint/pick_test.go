package paint

import (
	"image/color"
	"math"
	"testing"

	"github.com/marrasen/gunim/geom"
)

// pickScene is a sphere of radius 1 at the origin and a small box in
// front of its right side, seen from 5 along +Z, drawn into 400 by 400
// at 100, 100.
func pickScene() (Scene, geom.Rect, func(geom.Vec3) geom.Point) {
	s := Scene{
		Camera: Camera{Eye: geom.V3(0, 0, 5)},
		Items: []SceneItem{
			{Mesh: NewSphere(24, 48, color.NRGBA{A: 0xff})},
			{Mesh: NewBox(geom.V3(0.4, 0.4, 0.4), color.NRGBA{A: 0xff}), Model: geom.Move3(geom.V3(0.6, 0, 1.5))},
		},
	}
	r := geom.Rc(100, 100, 400, 400)
	m := s.Camera.Matrix(1)
	// shows returns where a point of the world shows in r.
	shows := func(v geom.Vec3) geom.Point {
		p := m.Apply(v)
		return geom.Pt(r.Min.X+(p.X+1)/2*400, r.Min.Y+(1-p.Y)/2*400)
	}
	return s, r, shows
}

func TestATapPicksTheNearestItemUnderIt(t *testing.T) {
	s, r, shows := pickScene()
	hit, ok := s.Pick(r, shows(geom.V3(-0.3, 0.2, 0)))
	if !ok || hit.Item != 0 {
		t.Fatalf("a tap on the sphere's left picked %+v, %v, want the sphere", hit, ok)
	}
	if d := hit.Point.Len(); math.Abs(float64(d-1)) > 0.02 {
		t.Errorf("the tap met the sphere at %v, %v from its middle, want on its surface", hit.Point, d)
	}
	if hit.Point.Z <= 0 || hit.Normal.Z <= 0 {
		t.Errorf("the tap met the sphere at %v facing %v, want its near side, facing the eye", hit.Point, hit.Normal)
	}
	// The box stands in front of the sphere's right side.
	hit, ok = s.Pick(r, shows(geom.V3(0.6, 0, 1.7)))
	if !ok || hit.Item != 1 {
		t.Errorf("a tap on the box, before the sphere, picked %+v, %v, want the box", hit, ok)
	}
	if _, ok := s.Pick(r, geom.Pt(110, 110)); ok {
		t.Error("a tap on the empty corner of the view picked an item")
	}
}

func TestATapFollowsAnItemsPlacement(t *testing.T) {
	s, r, shows := pickScene()
	// The sphere, squashed and moved left, leaves where it was.
	s.Items[0].Model = geom.Move3(geom.V3(-1.2, 0, 0)).Mul(geom.Scale3(geom.V3(0.5, 1, 1)))
	if hit, ok := s.Pick(r, shows(geom.V3(-1.2, 0.5, 0.8))); !ok || hit.Item != 0 {
		t.Errorf("a tap on the moved sphere picked %+v, %v, want it", hit, ok)
	}
	if hit, ok := s.Pick(r, shows(geom.V3(0.1, -0.5, 0.5))); ok && hit.Item == 0 {
		t.Errorf("a tap where the sphere was before it moved picked it, at %v", hit.Point)
	}
}

func TestAnItemIsSeeThroughByItsTintOrItsColours(t *testing.T) {
	solid := NewSphere(4, 8, color.NRGBA{R: 0xff, A: 0xff})
	glass := NewSphere(4, 8, color.NRGBA{B: 0xff, A: 0x80})
	cases := []struct {
		it   SceneItem
		want bool
	}{
		{SceneItem{Mesh: solid}, false},
		{SceneItem{Mesh: solid, Tint: color.NRGBA{0xff, 0xff, 0xff, 0x40}}, true},
		{SceneItem{Mesh: glass}, true},
	}
	for i, c := range cases {
		if got := c.it.SeeThrough(); got != c.want {
			t.Errorf("case %d: see-through %v, want %v", i, got, c.want)
		}
	}
}
