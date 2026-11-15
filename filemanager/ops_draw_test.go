package filemanager

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// kinds counts the commands p holds: its areas, its ribbons and all of
// them, and the points of its first area.
func kinds(p *paint.Painter) (areas, ribbons, all, points int) {
	for _, op := range p.Ops() {
		switch op := op.(type) {
		case *paint.AreaOp:
			if areas == 0 {
				points = len(op.Points)
			}
			areas++
		case *paint.RibbonOp:
			ribbons++
		}
	}
	return areas, ribbons, len(p.Ops()), points
}

func TestTheSpeedGraphIsOneAreaAndOneRibbonWhateverTheScale(t *testing.T) {
	g := newOpGraph(&glide{shown: 0.8})
	for i := range 200 {
		g.samples = append(g.samples, speedAt{done: 0.8 * float32(i) / 199, rate: float64(1 + i%9)})
	}
	g.top.Jump(12)
	g.speed = 5
	box := geom.Sz(360, 40)
	draw := func(scale float32) (areas, ribbons, all, points int) {
		var p paint.Painter
		p.Reset()
		g.Paint(&p, gunim.Frame{Scale: scale}, box, gunim.Children{})
		return kinds(&p)
	}
	areas, ribbons, all, points := draw(1)
	if areas != 1 || ribbons != 1 {
		t.Fatalf("the graph draws %d areas and %d ribbons, want its fill as one area and its line as one ribbon", areas, ribbons)
	}
	// A point for each 2 px of the 288 px done, not a shape for each
	// column.
	if points < 140 || points > 150 {
		t.Errorf("the fill has %d points, want about half of 288", points)
	}
	// The track, the layer that cuts to it and its end, the fill, the
	// line, the line at now, and a dash for each 8 px across.
	if all > 6+46 {
		t.Errorf("the graph draws %d commands, want some 50", all)
	}
	if _, _, big, _ := draw(3); big != all {
		t.Errorf("at three device pixels a pixel the graph draws %d commands, and at one %d, want as many", big, all)
	}
}

func TestTheRingIsAShapeAndARibbon(t *testing.T) {
	for _, c := range []struct {
		what         string
		done         glide
		all, ribbons int
	}{
		{"with nothing done", glide{}, 1, 0},
		// The track, what is done, and its two round ends.
		{"two fifths done", glide{shown: 0.4}, 4, 1},
		{"all done", glide{shown: 1}, 1, 0},
		{"counting", glide{unknown: true}, 4, 1},
	} {
		r := &opRing{done: &c.done}
		var p paint.Painter
		p.Reset()
		r.Paint(&p, gunim.Frame{Scale: 1}, geom.Sz(18, 18), gunim.Children{})
		if _, ribbons, all, _ := kinds(&p); all != c.all || ribbons != c.ribbons {
			t.Errorf("%s the ring draws %d commands, %d of them ribbons, want %d and %d", c.what, all, ribbons, c.all, c.ribbons)
		}
	}
}
