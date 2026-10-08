package main

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/paint"
)

// Every mask the example draws is at most 128 units across: 256 pixels on a screen of two pixels to a unit, the
// most a driver keeps a settled mask at, so none is stretched.
func TestEveryMaskFitsTheDriversMostOnADoubleDensityScreen(t *testing.T) {
	a, err := newArt()
	if err != nil {
		t.Fatal(err)
	}
	var p paint.Painter
	p.Reset()
	a.Paint(&p, gunim.Frame{}, windowSize, gunim.Children{})
	masks := 0
	for _, op := range p.Ops() {
		m, ok := op.(*paint.MaskOp)
		if !ok {
			continue
		}
		masks++
		if s := m.Rect.Size(); s.W > 128 || s.H > 128 {
			t.Errorf("a %T is drawn %v by %v", m.Shape, s.W, s.H)
		}
	}
	if masks == 0 {
		t.Fatal("drew no masks")
	}
}
