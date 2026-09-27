package paint

import (
	"image/color"
	"testing"

	"github.com/marrasen/gunim/geom"
)

// cells draws n small squares, as a grid of cells draws.
func cells(p *Painter, n int, c color.NRGBA) {
	for i := range n {
		p.RRect(geom.Rc(float32(i*10), 0, 10, 10), 0, Solid(c))
	}
}

// A run drawn again in the next frame is the same commands, and the
// frame's damage is only what changed around it.
func TestARunCarriedOnIsTheSameAndDamagesNothing(t *testing.T) {
	var p Painter
	red, blue := color.NRGBA{R: 0xff, A: 0xff}, color.NRGBA{B: 0xff, A: 0xff}
	p.Reset()
	mark := p.Mark()
	cells(&p, 50, red)
	run := p.RunFrom(mark)
	p.RRect(geom.Rc(0, 100, 10, 10), 0, Solid(red))
	was := append([]Op(nil), p.Ops()...)

	p.Reset()
	if !p.Again(run) {
		t.Fatal("the run from the frame before was refused")
	}
	p.RRect(geom.Rc(0, 100, 10, 10), 0, Solid(blue))
	if len(p.Ops()) != len(was) {
		t.Fatalf("the frame holds %d commands, want %d", len(p.Ops()), len(was))
	}
	for i := range 50 {
		if !sameOp(p.Ops()[i], was[i]) {
			t.Fatalf("command %d changed in carrying", i)
		}
	}
	if d := p.Damage(); d.Min.Y < 98 {
		t.Fatalf("the damage is %v, want the one square that changed", d)
	}
}

// A run from two frames back, or from another painter, or holding a
// layer, is refused, and nothing is recorded.
func TestARunFromElsewhereIsRefused(t *testing.T) {
	var p, other Painter
	red := color.NRGBA{R: 0xff, A: 0xff}
	p.Reset()
	old := p.RunFrom(p.Mark())
	cells(&p, 5, red)
	old = p.RunFrom(old.from)
	p.Reset()
	cells(&p, 5, red)
	p.Reset()
	if p.Again(old) {
		t.Fatal("a run from two frames back was taken")
	}
	other.Reset()
	cells(&other, 5, red)
	theirs := other.RunFrom(0)
	other.Reset()
	if p.Again(theirs) {
		t.Fatal("another painter's run was taken")
	}
	p.Reset()
	mark := p.Mark()
	func() {
		defer p.Layer(LayerOpts{Bounds: geom.Rc(0, 0, 50, 50), Opacity: 1})()
		cells(&p, 3, red)
	}()
	layered := p.RunFrom(mark)
	p.Reset()
	if p.Again(layered) || len(p.Ops()) != 0 {
		t.Fatalf("a run holding a layer was taken, leaving %d commands", len(p.Ops()))
	}
}
