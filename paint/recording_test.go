package paint

import (
	"image/color"
	"testing"

	"github.com/marrasen/gunim/geom"
)

// What a node drew at one place is drawn again at another, scaled, the
// commands moved with it and a layer kept whole.
func TestARecordingIsDrawnAgainElsewhere(t *testing.T) {
	var p Painter
	p.Reset()
	red := color.NRGBA{R: 0xff, A: 0xff}
	pop := p.Push(Translate(geom.Pt(100, 50)))
	mark := p.Mark()
	func() {
		defer p.Layer(LayerOpts{Bounds: geom.Rc(0, 0, 40, 20), Opacity: 0.5})()
		p.RRect(geom.Rc(10, 5, 20, 10), 2, Solid(red))
		p.Text([]Glyph{{ID: 7, At: geom.Pt(1, 2)}}, 12, red, geom.Rc(0, 0, 10, 12))
	}()
	var r Recording
	p.Keep(mark, &r)
	pop()

	var q Painter
	q.Reset()
	defer q.Push(Translate(geom.Pt(300, 0)))()
	defer q.Push(Scale(0.5, geom.Point{}))()
	q.Replay(&r)
	ops := q.Ops()
	if len(ops) != 4 {
		t.Fatalf("replayed %d commands, want the layer, the rectangle, the text and the layer's end", len(ops))
	}
	if _, ok := ops[0].(*LayerOp); !ok {
		t.Fatalf("the first command is a %T", ops[0])
	}
	rr, ok := ops[1].(*RRectOp)
	if !ok || rr.Fill.Solid != red {
		t.Fatalf("the second command is %#v", ops[1])
	}
	// Its corner, at (10, 5) in the node's space, lands at 300 + 5.
	if at := rr.Transform.Apply(rr.Rect.Min); at != geom.Pt(305, 2.5) {
		t.Fatalf("the rectangle starts at %v, want (305, 2.5)", at)
	}
	if tx, ok := ops[2].(*TextOp); !ok || len(tx.Glyphs) != 1 || tx.Glyphs[0].ID != 7 {
		t.Fatalf("the third command is %#v", ops[2])
	}
	if _, ok := ops[3].(*LayerEndOp); !ok {
		t.Fatalf("the last command is a %T", ops[3])
	}
	// Kept again, the recording's buffers are reused and it holds the new
	// stretch alone.
	p.Reset()
	mark = p.Mark()
	p.RRect(geom.Rc(0, 0, 1, 1), 0, Solid(red))
	p.Keep(mark, &r)
	if len(r.ops) != 1 || len(r.glyphs) != 0 {
		t.Fatalf("kept again, the recording holds %d commands and %d glyphs", len(r.ops), len(r.glyphs))
	}
}
