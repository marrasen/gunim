package paint

import (
	"image/color"
	"slices"
	"testing"

	"github.com/marrasen/gunim/geom"
)

// crowd returns n items of mesh, each placed apart, as a game's world
// of sweets and sprites is.
func crowd(mesh *Mesh, n int) []SceneItem {
	items := make([]SceneItem, n)
	for i := range items {
		items[i] = SceneItem{Mesh: mesh, Model: geom.Move3(geom.V3(float32(i%40), float32(i/40), 0)), Shine: 32}
	}
	return items
}

// moveCrowd moves every item along by dx, in place, as a game moves its
// world in the slice it keeps from frame to frame.
func moveCrowd(items []SceneItem, dx float32) {
	for i := range items {
		items[i].Model = geom.Move3(geom.V3(float32(i%40)+dx, float32(i/40), 0))
	}
}

// gameFrame returns a func that records a frame as a game does: a
// scene of 1500 items moving, and 300 masks and 300 shapes over it.
func gameFrame() (frame func(), items int) {
	var p Painter
	crowd := crowd(NewBox(geom.V3(1, 1, 1), color.NRGBA{R: 0xff, A: 0xff}), 1500)
	shade := &Gradient{To: geom.Pt(16, 0), Start: color.NRGBA{A: 0xff}, End: color.NRGBA{R: 0xff, A: 0xff}}
	var shape Shape = square{true}
	cam := Camera{Eye: geom.V3(20, 18, 40), At: geom.V3(20, 18, 0)}
	dx := float32(0)
	return func() {
		p.Reset()
		dx += 0.25
		moveCrowd(crowd, dx)
		p.Scene(geom.Rc(0, 0, 800, 600), Scene{Camera: cam, Items: crowd})
		for i := range 300 {
			r := geom.Rc(float32(i), 0, 16, 16)
			if i%2 == 0 {
				p.Mask(shape, r, color.NRGBA{G: 0xff, A: 0xff})
			} else {
				p.MaskFill(shape, r, Fill{Gradient: shade})
			}
			p.RRect(r, 4, Solid(color.NRGBA{B: 0xff, A: 0xff}))
		}
		p.Damage()
	}, len(crowd)
}

func TestASceneDrawnEveryFrameAllocatesNothing(t *testing.T) {
	frame, items := gameFrame()
	// The first frames grow the painter's buffers to what a frame takes.
	for range 3 {
		frame()
	}
	if n := testing.AllocsPerRun(50, frame); n != 0 {
		t.Fatalf("a frame of a scene of %d items and 300 masks allocated %v times, want none", items, n)
	}
}

func BenchmarkAFrameOfASceneAndMasks(b *testing.B) {
	frame, _ := gameFrame()
	b.ReportAllocs()
	for b.Loop() {
		frame()
	}
}

func TestASceneKeepsItsItemsAsTheyWereWhenRecorded(t *testing.T) {
	var p Painter
	p.Reset()
	items := crowd(NewBox(geom.V3(1, 1, 1), color.NRGBA{A: 0xff}), 3)
	want := slices.Clone(items)
	p.Scene(geom.Rc(0, 0, 100, 100), Scene{Items: items})
	moveCrowd(items, 5)
	got := p.Ops()[0].(*SceneOp).Scene.Items
	if !slices.Equal(got, want) {
		t.Fatalf("the op's items changed with the caller's slice: %v", got)
	}
	// An append to one scene's items does not reach the next scene's.
	p.Scene(geom.Rc(0, 0, 100, 100), Scene{Items: want})
	_ = append(got, SceneItem{Shine: 99})
	if next := p.Ops()[1].(*SceneOp).Scene.Items; !slices.Equal(next, want) {
		t.Fatalf("an append to the first scene's items reached the second's: %v", next)
	}
}

func TestASceneThatChangesDamagesAndOneThatStaysDoesNot(t *testing.T) {
	var p Painter
	items := crowd(NewBox(geom.V3(1, 1, 1), color.NRGBA{A: 0xff}), 50)
	r := geom.Rc(10, 10, 110, 110)
	draw := func() {
		p.Reset()
		p.Scene(r, Scene{Items: items})
	}
	draw()
	draw()
	if d := p.Damage(); !d.Empty() {
		t.Fatalf("a scene drawn the same again damaged %v", d)
	}
	// Two frames on, the buffer the first frame copied into is reused;
	// the frame compared against must still hold its own items.
	draw()
	moveCrowd(items, 1)
	draw()
	if d := p.Damage(); d.Empty() {
		t.Fatal("a scene whose items moved damaged nothing")
	}
	draw()
	if d := p.Damage(); !d.Empty() {
		t.Fatalf("the moved scene drawn the same again damaged %v", d)
	}
}
