package paint

import (
	"image/color"
	"math"
	"slices"
	"testing"

	"github.com/marrasen/gunim/geom"
)

// swell returns an area of n points under a wave, shifted along by dx,
// down to a base at y 100, as an animated area is sampled afresh each
// frame into one slice.
func swell(pts []AreaPoint, dx float32) []AreaPoint {
	for i := range pts {
		x := float32(i) * 4
		pts[i] = AreaPoint{
			At:        geom.Pt(x, 50+20*float32(math.Sin(float64(x+dx)/30))),
			Base:      geom.Pt(x, 100),
			Color:     color.NRGBA{R: uint8(i), G: 0x80, B: 0xff, A: 0x80},
			BaseColor: color.NRGBA{R: uint8(i), G: 0x80, B: 0xff},
		}
	}
	return pts
}

// areaAt returns the painter's i-th op, which must be an area.
func areaAt(t *testing.T, p *Painter, i int) *AreaOp {
	t.Helper()
	a, ok := p.Ops()[i].(*AreaOp)
	if !ok {
		t.Fatalf("op %d is a %T, want an area", i, p.Ops()[i])
	}
	return a
}

func TestAnAreaOfFewerThanTwoPointsRecordsNothing(t *testing.T) {
	var p Painter
	p.Reset()
	p.Area(nil)
	p.Area([]AreaPoint{{At: geom.Pt(1, 1), Base: geom.Pt(1, 9), Color: color.NRGBA{A: 0xff}}})
	if n := len(p.Ops()); n != 0 {
		t.Fatalf("areas of no point and of one recorded %d commands, want none", n)
	}
}

func TestAnAreaIsOneCommandHoweverManyItsPoints(t *testing.T) {
	var p Painter
	p.Reset()
	p.Area(swell(make([]AreaPoint, 400), 0))
	if n := len(p.Ops()); n != 1 {
		t.Fatalf("an area of 400 points recorded %d commands, want one", n)
	}
}

func TestAnAreaKeepsItsPointsAsTheyWereWhenRecorded(t *testing.T) {
	var p Painter
	p.Reset()
	pts := swell(make([]AreaPoint, 50), 0)
	want := slices.Clone(pts)
	end := p.Blend(BlendAdd)
	p.Area(pts)
	end()
	swell(pts, 10)
	got := areaAt(t, &p, 0)
	if !slices.Equal(got.Points, want) {
		t.Fatal("the op's points changed with the caller's slice")
	}
	if got.Blend != BlendAdd {
		t.Errorf("the area has blend %v, want the BlendAdd in force", got.Blend)
	}
	// An append to one area's points does not reach the next area's.
	p.Area(want)
	_ = append(got.Points, AreaPoint{At: geom.Pt(99, 99)})
	if next := areaAt(t, &p, 1).Points; !slices.Equal(next, want) {
		t.Fatal("an append to the first area's points reached the second's")
	}
}

func TestAnAreasBoundsHoldItsLineAndItsBase(t *testing.T) {
	var p Painter
	p.Reset()
	defer p.Push(Translate(geom.Pt(100, 0)))()
	p.Area([]AreaPoint{
		{At: geom.Pt(10, 20), Base: geom.Pt(12, 90)},
		{At: geom.Pt(50, 5), Base: geom.Pt(48, 80)},
	})
	// Grown by the pixel and a half every op is grown by.
	want := geom.Rect{Min: geom.Pt(100+10-1.5, 5-1.5), Max: geom.Pt(100+50+1.5, 90+1.5)}
	if got := p.bounds[0]; got != want {
		t.Fatalf("the area's bounds are %v, want %v", got, want)
	}
}

func TestAnAreaThatChangesDamagesAndOneThatStaysDoesNot(t *testing.T) {
	var p Painter
	pts := swell(make([]AreaPoint, 100), 0)
	draw := func() {
		p.Reset()
		p.Area(pts)
	}
	draw()
	draw()
	if d := p.Damage(); !d.Empty() {
		t.Fatalf("an area drawn the same again damaged %v", d)
	}
	// Two frames on, the buffer the first frame copied into is reused;
	// the frame compared against must still hold its own points.
	draw()
	swell(pts, 5)
	draw()
	if d := p.Damage(); d.Empty() {
		t.Fatal("an area that moved damaged nothing")
	}
	draw()
	if d := p.Damage(); !d.Empty() {
		t.Fatalf("the moved area drawn the same again damaged %v", d)
	}
}

func TestAnAreaDrawnEveryFrameAllocatesNothing(t *testing.T) {
	var p Painter
	left, right := make([]AreaPoint, 200), make([]AreaPoint, 200)
	dx := float32(0)
	frame := func() {
		p.Reset()
		dx++
		p.Area(swell(left, dx))
		p.Area(swell(right, -dx))
		p.Damage()
	}
	// The first frames grow the painter's buffers to what a frame takes.
	for range 3 {
		frame()
	}
	if n := testing.AllocsPerRun(50, frame); n != 0 {
		t.Fatalf("a frame of two areas allocated %v times, want none", n)
	}
}

func TestAnAreaKeptInARecordingSurvivesLaterFrames(t *testing.T) {
	var p Painter
	pts := swell(make([]AreaPoint, 60), 0)
	want := slices.Clone(pts)
	p.Reset()
	pop := p.Push(Translate(geom.Pt(50, 0)))
	mark := p.Mark()
	p.Area(pts)
	var r Recording
	p.Keep(mark, &r)
	pop()
	for i := range 4 {
		p.Reset()
		p.Area(swell(pts, float32(i+1)))
	}
	for range 3 {
		p.Reset()
		p.Replay(&r)
		if len(p.Ops()) != 1 {
			t.Fatalf("replayed %d commands, want the area", len(p.Ops()))
		}
		got := areaAt(t, &p, 0)
		if !slices.Equal(got.Points, want) {
			t.Fatal("the kept area came back changed")
		}
		// It lies where it was drawn in the recording's space, now the
		// window's.
		if at := got.Transform.Apply(got.Points[0].At); at != want[0].At {
			t.Fatalf("the replayed area starts at %v, want %v", at, want[0].At)
		}
	}
}

func TestAnAreaCarriedOnKeepsItsPoints(t *testing.T) {
	var p Painter
	pts := swell(make([]AreaPoint, 80), 0)
	want := slices.Clone(pts)
	p.Reset()
	mark := p.Mark()
	p.Area(pts)
	run := p.RunFrom(mark)
	for i := range 3 {
		p.Reset()
		mark := p.Mark()
		if !p.Again(run) {
			t.Fatalf("frame %d: the area from the frame before was refused", i)
		}
		run = p.RunFrom(mark)
		// Another area, drawn anew, fills the buffers the carried one
		// was copied from.
		p.Area(swell(pts, float32(i+1)))
		if d := p.Damage(); i > 0 && d.Empty() {
			t.Fatalf("frame %d: the area drawn anew moved and damaged nothing", i)
		}
		if got := areaAt(t, &p, 0); !slices.Equal(got.Points, want) {
			t.Fatalf("frame %d: the carried area's points changed", i)
		}
	}
}
