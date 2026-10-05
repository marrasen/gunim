//go:build linux || windows || darwin

package render

import (
	"image/color"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

func TestASceneDrawsLitMeshesInDepth(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	sphere := paint.NewSphere(24, 48, red)
	box := paint.NewBox(geom.V3(0.6, 0.6, 0.6), green)
	scene := paint.Scene{
		Camera: paint.Camera{Eye: geom.V3(0, 0, 5), At: geom.V3(0, 0, 0)},
		Light:  paint.Light{Direction: geom.V3(0, 0, -1)},
		Items: []paint.SceneItem{
			{Mesh: sphere},
			// The box sits in front of the sphere's right side, though it
			// is drawn first.
			{Mesh: box, Model: geom.Move3(geom.V3(0.6, 0, 1.2))},
		},
	}
	scene.Items[0], scene.Items[1] = scene.Items[1], scene.Items[0]
	pix := drawn(r, func(p *paint.Painter) { p.Scene(geom.Rc(200, 100, 400, 400), scene) })
	// The sphere's middle faces the light head on: red, lit.
	if got := pixelAt(pix, 370, 300); got[0] < 0xe0 || got[1] > 0x20 {
		t.Errorf("the sphere's middle is %v, want lit red", got)
	}
	// Its edge turns away from the light, and is darker.
	if mid, edge := pixelAt(pix, 370, 300), pixelAt(pix, 400, 190); edge[0] >= mid[0] {
		t.Errorf("the sphere's edge is %v and its middle %v, want the edge darker", edge, mid)
	}
	// The box hides the sphere where it stands in front of it.
	at := geom.Perspective(paint.DefaultFOV, 1, 0.1, 100).Mul(geom.LookAt(geom.V3(0, 0, 5), geom.V3(0, 0, 0), geom.V3(0, 1, 0))).Apply(geom.V3(0.6, 0, 1.5))
	bx, by := int(400+at.X*200), int(300-at.Y*200)
	if got := pixelAt(pix, bx, by); got[1] < 0xa0 || got[0] > 0x20 {
		t.Errorf("at %d, %d, before the sphere, the box is %v, want green", bx, by, got)
	}
	// Outside the sphere the view is clear, showing the black behind.
	if got := pixelAt(pix, 205, 105); got != px(black) {
		t.Errorf("in the view's corner the canvas is %v, want the black behind", got)
	}
	if got := pixelAt(pix, 150, 300); got != px(black) {
		t.Errorf("outside the view the canvas is %v, want it untouched", got)
	}
}

func TestAMeshIsUploadedOnceAndLetGo(t *testing.T) {
	r, done := hiddenGL(t)
	defer done()
	m := paint.NewSphere(4, 8, color.NRGBA{A: 0xff})
	s := paint.Scene{Camera: paint.Camera{Eye: geom.V3(0, 0, 3)}, Items: []paint.SceneItem{{Mesh: m}}}
	drawn(r, func(p *paint.Painter) { p.Scene(geom.Rc(0, 0, 100, 100), s) })
	mb := r.scenes.meshes[m]
	drawn(r, func(p *paint.Painter) { p.Scene(geom.Rc(0, 0, 100, 100), s) })
	if mb == nil || r.scenes.meshes[m] != mb {
		t.Fatal("the mesh was uploaded again for the second frame")
	}
	mb.used = mb.used.Add(-2 * meshIdle)
	r.evictMeshes()
	if _, ok := r.scenes.meshes[m]; ok {
		t.Error("a mesh left undrawn stayed on the GPU")
	}
}
