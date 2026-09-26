package gunim

import (
	"image/color"
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// swatch is a node that paints one red rectangle.
type swatch struct{ _ int }

func (swatch) Layout(c Constraints, _ Frame, _ Children) geom.Size { return geom.Sz(30, 20) }
func (swatch) Paint(p *paint.Painter, _ Frame, box geom.Size, _ Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(color.NRGBA{R: 0xff, A: 0xff}))
}

// What a node drew is kept, in its own space, and stays once the node
// has gone.
func TestANodesDrawingIsKeptPastItsTimeOnScreen(t *testing.T) {
	w, st, _ := newStage(t, paint.Translate(geom.Pt(5, 5)))
	n := &swatch{}
	d := w.ui.KeepDrawing(n)
	w.ui.Insert(st, n)
	run(w, 1)
	if d.Recording().Empty() || d.Size() != geom.Sz(30, 20) {
		t.Fatalf("drawn, the drawing is empty %v at %v", d.Recording().Empty(), d.Size())
	}
	w.ui.Remove(n)
	run(w, 30)
	var p paint.Painter
	p.Reset()
	p.Replay(d.Recording())
	r, ok := p.Ops()[0].(*paint.RRectOp)
	if !ok || r.Transform.Apply(r.Rect.Min) != (geom.Point{}) {
		t.Fatalf("gone, the drawing replays %#v", p.Ops())
	}
	w.ui.ForgetDrawing(n)
	if _, ok := w.ui.kept[n]; ok {
		t.Fatal("forgotten, the drawing is still kept")
	}
}
