package widget

import (
	"fmt"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/theme"
)

// flexNoGap is no room between the children.
var flexNoGap = theme.Length("test.flex.nogap", 0)

// mustFollow fails when b, where a child starts, is not at a, where what
// it follows has it.
func mustFollow(t *testing.T, when string, a, b float32) {
	t.Helper()
	if abs32(b-a) > 0.02 {
		t.Fatalf("%s: what follows starts at %v, and what it follows ends at %v", when, b, a)
	}
}

// What lies below a fold stays right under it on every frame it opens and
// shuts in, also turned round on the way.
func TestAColumnsChildStaysUnderAFoldOnEveryFrame(t *testing.T) {
	in, below, last := newSpot(50, 200), newSpot(50, 20), newSpot(50, 20)
	f := NewFold(in, false)
	w, run := stage(t, &frame{child: Column(newSpot(50, 30), f, below, last), size: geom.Sz(400, 0)})
	gap := Gap.Default()
	check := func(when string, frames int) {
		t.Helper()
		was := f.box.H
		moved := false
		for i := range frames {
			run(1)
			mustFollow(t, fmt.Sprintf("%s, frame %d", when, i), 30+gap+f.box.H+gap, below.at.Y)
			mustFollow(t, fmt.Sprintf("%s, frame %d", when, i), below.at.Y+20+gap, last.at.Y)
			moved = moved || f.box.H != was
		}
		if !moved {
			t.Fatalf("%s: the fold stood still", when)
		}
	}
	do(t, w, func(u *gunim.UI) { f.SetOpen(true, u) })
	check("opening", 5)
	do(t, w, func(u *gunim.UI) { f.SetOpen(false, u) })
	check("turned round", 4)
	do(t, w, func(u *gunim.UI) { f.SetOpen(true, u) })
	check("turned again", 90)
	if f.box.H != 200 {
		t.Fatalf("the fold opened to %v", f.box.H)
	}
	do(t, w, func(u *gunim.UI) { f.SetOpen(false, u) })
	check("shutting", 90)
}

// One fold opening as another shuts, as a dialog turns from one page to the
// next, never lie over each other, through the boxes round them too.
func TestFoldsThatTakeTurnsNeverOverlap(t *testing.T) {
	a, b := NewFold(newSpot(50, 240), true), NewFold(newSpot(50, 90), false)
	inner := Column(a, b)
	inner.Gap = flexNoGap
	below := newSpot(50, 20)
	w, run := stage(t, &frame{child: Column(NewCard(NewPad(inner)), below), size: geom.Sz(400, 0)})
	run(2)
	off := below.at.Y - a.box.H
	u := stageUI(t, w, run)
	for turn := range 3 {
		do(t, w, func(u *gunim.UI) { a.SetOpen(turn%2 == 1, u); b.SetOpen(turn%2 == 0, u) })
		for i := range 8 + 60*(turn%2) {
			run(1)
			when := fmt.Sprintf("turn %d, frame %d", turn, i)
			mustFollow(t, when, off+a.box.H+b.box.H, below.at.Y)
			if ra, ok := u.Bounds(a); ok && a.box.H > 0.5 {
				if rb, ok := u.Bounds(b); ok {
					mustFollow(t, when, ra.Max.Y, rb.Min.Y)
				}
			}
		}
	}
}

// What lies below a list moves with it on every frame a row comes and goes
// in, its first and last one too.
func TestAColumnsChildStaysUnderAListOnEveryFrame(t *testing.T) {
	l := NewList()
	below := newSpot(50, 20)
	w, run := stage(t, &frame{child: Column(l, below), size: geom.Sz(400, 0)})
	set := func(keys ...string) {
		do(t, w, func(u *gunim.UI) {
			Sync(l, u, keys, func(k string) Key { return Key(k) }, func(string) *spot { return newSpot(50, 60) }, nil)
		})
	}
	gap := Gap.Default()
	for _, keys := range [][]string{{"a"}, {"a", "b", "c"}, {"c"}, {"d", "c"}, {}} {
		set(keys...)
		for i := range 60 {
			run(1)
			// A row's last step takes it out of the tree, at under half a pixel.
			if want := max(l.height, 0) + gap; abs32(below.at.Y-want) > 0.5 {
				t.Fatalf("rows %v, frame %d: what follows starts at %v, want %v", keys, i, below.at.Y, want)
			}
		}
	}
}

// A fold that comes and goes takes what follows along. The gap it brings and
// takes away springs, so nothing jumps and nothing lies over the fold.
func TestAFoldThatComesTakesWhatFollowsAlong(t *testing.T) {
	first, below := newSpot(50, 30), newSpot(50, 20)
	col := Column(first, below)
	w, run := stage(t, &frame{child: col, size: geom.Sz(400, 0)})
	f := NewFold(newSpot(50, 120), false)
	f.Comes = true
	gap := Gap.Default()
	do(t, w, func(u *gunim.UI) { u.InsertAt(col, 1, f) })
	was := below.at.Y
	step := func(when string, frames int) {
		t.Helper()
		for i := range frames {
			h := f.box.H
			run(1)
			top := 30 + gap
			if below.at.Y < top+f.box.H-0.02 {
				t.Fatalf("%s, frame %d: what follows starts at %v, over a fold that ends at %v", when, i, below.at.Y, top+f.box.H)
			}
			// No more than the fold moved, and the gap's spring.
			if d := abs32(below.at.Y - was); d > abs32(f.box.H-h)+4 {
				t.Fatalf("%s, frame %d: what follows jumped %v as the fold moved %v", when, i, d, f.box.H-h)
			}
			was = below.at.Y
		}
	}
	step("coming", 90)
	at(t, below, geom.Pt(0, 30+gap+120+gap))
	do(t, w, func(u *gunim.UI) { u.Remove(f) })
	step("going", 4)
	for range 120 {
		run(1)
		if d := abs32(below.at.Y - was); d > 25 {
			t.Fatalf("as the fold went, what follows jumped %v", d)
		}
		was = below.at.Y
	}
	at(t, below, geom.Pt(0, 30+gap))
}

// A child that comes at once while a fold moves has the others glide to make
// room for it, on top of the way they move with the fold.
func TestAnInsertionGlidesWhileAFoldMoves(t *testing.T) {
	f := NewFold(newSpot(50, 200), false)
	below, come := newSpot(50, 20), newSpot(50, 40)
	inner := Column(f)
	col := Column(inner, below)
	w, run := stage(t, &frame{child: col, size: geom.Sz(400, 0)})
	do(t, w, func(u *gunim.UI) { f.SetOpen(true, u) })
	run(3)
	was, h := below.at.Y, f.box.H
	// Inside the box that holds the fold, so one size changes both ways.
	do(t, w, func(u *gunim.UI) { u.InsertAt(inner, 0, come) })
	for i := range 90 {
		run(1)
		if d := (below.at.Y - was) - (f.box.H - h); d > 16 || d < -4 {
			t.Fatalf("frame %d: as a 48 px child came, what follows moved %v apart from the fold", i, d)
		}
		was, h = below.at.Y, f.box.H
	}
	gap := Gap.Default()
	at(t, below, geom.Pt(0, 40+gap+200+gap))
}

// A fold's content that grows at once has what follows glide, as a fold at
// rest only passes it on.
func TestAFoldsContentThatGrowsAtOnceHasTheOthersGlide(t *testing.T) {
	in, below := newSpot(50, 40), newSpot(50, 20)
	_, run := stage(t, &frame{child: Column(NewFold(in, true), below), size: geom.Sz(400, 0)})
	was := below.at.Y
	in.size.H = 140
	run(1)
	if d := below.at.Y - was; d > 40 {
		t.Fatalf("as the fold's content grew 100 at once, what follows jumped %v", d)
	}
	run(120)
	at(t, below, geom.Pt(0, 140+Gap.Default()))
}

// Along a row, a fold moves what its place says it must on the frame: in a
// row that fills its width, a growing child gives the room, and what lies
// past it stays still.
func TestARowFollowsASidewaysFoldHoweverItIsJustified(t *testing.T) {
	gap := Gap.Default()
	fold := func() *Fold {
		f := NewFold(newSpot(100, 20), false)
		f.Axis = Horizontal
		return f
	}
	for _, c := range []struct {
		name    string
		justify Justify
		grow    bool
		// want is where the row's last child, 30 wide, starts in 400 px with the fold w wide.
		want func(w float32) float32
	}{
		{"start", JustifyStart, false, func(w float32) float32 { return w + gap }},
		{"center", JustifyCenter, false, func(w float32) float32 { return (400-w-gap-30)/2 + w + gap }},
		{"end", JustifyEnd, false, func(float32) float32 { return 370 }},
		{"between", JustifyBetween, false, func(float32) float32 { return 370 }},
		{"growing", JustifyStart, true, func(float32) float32 { return 370 }},
	} {
		f, last, grower := fold(), newSpot(30, 20), newSpot(0, 20)
		row := Row(f, last)
		if c.grow {
			row = Row(f, grower, last).Grow(grower, 1)
		}
		row.Justify = c.justify
		w, run := stage(t, &frame{child: row, size: geom.Sz(400, 100)})
		do(t, w, func(u *gunim.UI) { f.SetOpen(true, u) })
		for i := range 60 {
			run(1)
			when := fmt.Sprintf("%s, frame %d", c.name, i)
			mustFollow(t, when, c.want(f.box.W), last.at.X)
			if c.grow {
				mustFollow(t, when, f.box.W+gap, grower.at.X)
			}
		}
		if f.box.W != 100 {
			t.Fatalf("%s: the fold opened to %v", c.name, f.box.W)
		}
	}
}

// A row as tall as a fold in it keeps its other children centred on the
// fold as it moves.
func TestARowKeepsItsChildrenCentredOnAFoldThatMoves(t *testing.T) {
	f, s := NewFold(newSpot(50, 100), false), newSpot(30, 20)
	row := Row(f, s)
	row.Cross = CrossCenter
	w, run := stage(t, &frame{child: Column(row), size: geom.Sz(400, 0)})
	do(t, w, func(u *gunim.UI) { f.SetOpen(true, u) })
	for i := range 60 {
		run(1)
		mustFollow(t, fmt.Sprintf("frame %d", i), (max(f.box.H, 20)-20)/2, s.at.Y)
	}
}

// In a column too short for its children, what a fold takes as it opens the
// others give, with nothing lying over anything on the way.
func TestAnOverfullColumnFollowsAFold(t *testing.T) {
	a, b := newSpot(50, 60), newSpot(50, 60)
	f := NewFold(newSpot(50, 100), false)
	w, run := stage(t, &frame{child: Column(a, f, b), size: geom.Sz(400, 120)})
	gap := Gap.Default()
	for _, open := range []bool{true, false} {
		do(t, w, func(u *gunim.UI) { f.SetOpen(open, u) })
		for i := range 60 {
			run(1)
			if want := a.box.H + gap + f.box.H + gap; abs32(b.at.Y-want) > 0.5 {
				t.Fatalf("open %v, frame %d: what follows starts at %v, want %v", open, i, b.at.Y, want)
			}
		}
	}
}

// A text area that grows with its text takes what follows along.
func TestAColumnsChildStaysUnderATextAreaThatGrows(t *testing.T) {
	a := NewTextArea()
	a.Rows, a.MaxRows = 1, 6
	below := newSpot(50, 20)
	w, run := stage(t, &frame{child: Column(a, below), size: geom.Sz(400, 0)})
	u := stageUI(t, w, run)
	do(t, w, func(u *gunim.UI) { a.SetText("one\ntwo\nthree\nfour", u) })
	first := below.at.Y
	for i := range 60 {
		run(1)
		r, _ := u.Bounds(a)
		mustFollow(t, fmt.Sprintf("frame %d", i), r.Max.Y+Gap.Default(), below.at.Y)
	}
	if below.at.Y <= first {
		t.Fatal("the area did not grow")
	}
}

// With animation slowed, what follows stays as close; with it off, the fold
// and what follows are there on the next frame.
func TestAColumnFollowsAFoldAtAnyScale(t *testing.T) {
	t.Cleanup(func() { anim.SetScale(1) })
	f, below := NewFold(newSpot(50, 200), false), newSpot(50, 20)
	w, run := stage(t, &frame{child: Column(f, below), size: geom.Sz(400, 0)})
	gap := Gap.Default()
	anim.SetScale(3)
	do(t, w, func(u *gunim.UI) { f.SetOpen(true, u) })
	for i := range 30 {
		run(1)
		mustFollow(t, fmt.Sprintf("slow, frame %d", i), f.box.H+gap, below.at.Y)
	}
	// Turned off on the way, both land.
	anim.SetScale(0)
	run(1)
	at(t, below, geom.Pt(0, 200+gap))
	do(t, w, func(u *gunim.UI) { f.SetOpen(false, u) })
	run(1)
	at(t, below, geom.Pt(0, gap))
}

// Resized as a fold opens, a column keeps what follows under it.
func TestAColumnResizedAsAFoldOpensKeepsWhatFollowsUnderIt(t *testing.T) {
	f, below := NewFold(newSpot(50, 200), false), newSpot(50, 20)
	col := Column(f, below)
	col.Justify = JustifyCenter
	fr := &frame{child: col, size: geom.Sz(400, 500)}
	w, run := stage(t, fr)
	gap := Gap.Default()
	do(t, w, func(u *gunim.UI) { f.SetOpen(true, u) })
	for i := range 40 {
		if i%3 == 0 {
			fr.size.H += 7
		}
		run(1)
		mustFollow(t, fmt.Sprintf("frame %d", i), (fr.size.H-f.box.H-gap-20)/2+f.box.H+gap, below.at.Y)
	}
}

// In a row too narrow for its children, only what a fold's motion moved is
// followed: a child that changes size at once has the others glide.
func TestAnOverfullRowGlidesForASizeChangedAtOnceAsAFoldMoves(t *testing.T) {
	f := NewFold(newSpot(30, 30), false)
	f.Axis = Horizontal
	a, b, last := newSpot(150, 30), newSpot(250, 30), newSpot(40, 30)
	w, run := stage(t, &frame{child: Row(f, a, b, last), size: geom.Sz(300, 40)})
	do(t, w, func(u *gunim.UI) { f.SetOpen(true, u) })
	run(2)
	was, starts := last.at.X, b.at.X
	a.size.W = 70
	for i := range 60 {
		run(1)
		// The fold's own part of it is 2 px on a frame, and the spring's at most some more.
		if d := abs32(last.at.X - was); d > 3 {
			t.Fatalf("frame %d: the last child jumped %v", i, d)
		}
		if d := abs32(b.at.X - starts); d > 14 {
			t.Fatalf("frame %d: what follows the child that shrank at once jumped %v", i, d)
		}
		was, starts = last.at.X, b.at.X
	}
}

// Squeezed in a row, a fold fills the room it has on every frame, so nothing
// jumps as the row comes to be too narrow.
func TestAFoldSqueezedInARowNeverJumps(t *testing.T) {
	mk := func() *Fold {
		f := NewFold(newSpot(200, 20), false)
		f.Axis = Horizontal
		return f
	}
	f, g, last := mk(), mk(), newSpot(60, 20)
	w, run := stage(t, &frame{child: Row(f, g, last), size: geom.Sz(300, 40)})
	gap := Gap.Default()
	for _, open := range []bool{true, false} {
		do(t, w, func(u *gunim.UI) { f.SetOpen(open, u) })
		run(4)
		do(t, w, func(u *gunim.UI) { g.SetOpen(open, u) })
		fw, gw := f.box.W, g.box.W
		for i := range 60 {
			run(1)
			mustFollow(t, fmt.Sprintf("open %v, frame %d", open, i), f.box.W+gap+g.box.W+gap, last.at.X)
			if d := max(abs32(f.box.W-fw), abs32(g.box.W-gw)); d > 30 {
				t.Fatalf("open %v, frame %d: a fold's width jumped %v", open, i, d)
			}
			fw, gw = f.box.W, g.box.W
		}
	}
}

// A growing child takes what a fold leaves from the frame the fold lets the
// column out of its squeeze.
func TestAColumnLeavingItsSqueezeFollowsAFold(t *testing.T) {
	a, grower := newSpot(50, 60), newSpot(50, 0)
	f := NewFold(newSpot(50, 300), true)
	w, run := stage(t, &frame{child: Column(a, grower, f).Grow(grower, 1), size: geom.Sz(400, 200)})
	gap := Gap.Default()
	for _, open := range []bool{false, true} {
		do(t, w, func(u *gunim.UI) { f.SetOpen(open, u) })
		for i := range 60 {
			run(1)
			when := fmt.Sprintf("open %v, frame %d", open, i)
			mustFollow(t, when, a.box.H+gap, grower.at.Y)
		}
	}
}
