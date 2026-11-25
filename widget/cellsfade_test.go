package widget

import (
	"image/color"
	"slices"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Two palettes: a ground, what is written on it, and a ground of some cells.
var (
	nightGround, nightInk, nightMark = color.NRGBA{0x10, 0x12, 0x18, 0xff}, color.NRGBA{0xe0, 0xe0, 0xe8, 0xff}, color.NRGBA{0x80, 0x20, 0x30, 0xff}
	dayGround, dayInk, dayMark       = color.NRGBA{0xfa, 0xf8, 0xf0, 0xff}, color.NRGBA{0x20, 0x22, 0x28, 0xff}, color.NRGBA{0xff, 0xc0, 0xa0, 0xff}
)

// fadeStage shows a grid wearing a theme of its ground, in the night's colours, with a function that runs fn on the UI
// goroutine and a frame.
func fadeStage(t *testing.T) (w *gunim.Window, g *CellGrid, themed *Themed, run func(int), on func(func(u *gunim.UI))) {
	t.Helper()
	g = NewCellGrid()
	g.Size = 14
	g.Resize(8, 2)
	themed = NewThemed(g, Dark().With(theme.Set(Background, nightGround)))
	dress(g, nightGround, nightInk, nightMark)
	w = gunimtest.New(t, geom.Sz(200, 100), nil)
	w.PaintWhenLooked(false)
	gunim.RegisterView(w, "cells", func(struct{}) gunim.Node { return themed }, nil)
	if err := w.Client().Mount(gunim.Root, "cells", "cells", nil, "cells"); err != nil {
		t.Fatal(err)
	}
	run = func(n int) {
		for range n {
			w.Frame(16_666_667)
		}
	}
	gunim.RegisterPatch(w, "cells", func(_ gunim.Node, fn func(u *gunim.UI), u *gunim.UI) { fn(u) })
	on = func(fn func(u *gunim.UI)) {
		t.Helper()
		if err := w.Client().Patch("cells", fn); err != nil {
			t.Fatal(err)
		}
		run(1)
	}
	run(40)
	return w, g, themed, run, on
}

// dress sets g's rows in a palette: a cell on the ground said outright, one on a ground of its own, one with the
// grid's own ground, underlined, and one that had a ground in the other palette alone.
func dress(g *CellGrid, ground, ink, mark color.NRGBA) {
	row := []Cell{{Rune: 'a', FG: ink, BG: ground}, {Rune: 'b', FG: ink, BG: mark}, {Rune: 'c', FG: ink, Style: CellUnderline}, {Rune: 'd', FG: ink}}
	if ground == nightGround {
		row[3].BG = mark
	}
	g.SetRow(0, row)
}

// nearTint reports whether two colours, as they cover what is under them, differ by at most d on a channel.
func nearTint(a, b color.NRGBA, d int) bool {
	off := func(x, y uint8) bool {
		cx, cy := int(x)*int(a.A)/255, int(y)*int(b.A)/255
		return cx-cy > d || cy-cx > d
	}
	return !off(a.R, b.R) && !off(a.G, b.G) && !off(a.B, b.B) && int(a.A)-int(b.A) <= d && int(b.A)-int(a.A) <= d
}

// TestAGridsColoursGlideWithTheTheme: on every frame, in step with it, and land on what they were set to.
func TestAGridsColoursGlideWithTheTheme(t *testing.T) {
	w, g, themed, run, on := fadeStage(t)
	g.SetCursor(Cursor{Col: 6, Row: 0, Visible: true, Shape: CursorBar, Color: nightMark})
	run(40)
	was, _ := cellsRow(w.Offscreen().Ops(), 0)
	before := slices.Clone(was.Cells)
	on(func(u *gunim.UI) {
		themed.Use(Dark().With(theme.Set(Background, dayGround)))
		dress(g, dayGround, dayInk, dayMark)
		g.SetCursor(Cursor{Col: 6, Row: 0, Visible: true, Shape: CursorBar, Color: dayMark})
		g.FadeColors(u)
	})
	last, cursor, alpha := before, nightMark, uint8(255)
	for frame := 0; ; frame++ {
		ops := w.Offscreen().Ops()
		now, ok := cellsRow(ops, 0)
		if !ok {
			t.Fatal("the row is not drawn")
		}
		// The ground said outright is the ground round it on every frame, to the eye.
		ground := ops[slices.IndexFunc(ops, func(op paint.Op) bool { _, is := op.(*paint.RRectOp); return is })].(*paint.RRectOp).Fill.Solid
		if !nearTint(now.Cells[0].BG, ground, 3) {
			t.Fatalf("frame %d: a cell of the ground is %v on the ground %v", frame, now.Cells[0].BG, ground)
		}
		for x := range 4 {
			c, l := now.Cells[x], last[x]
			if !nearTint(c.BG, l.BG, 36) || !nearTint(c.FG, l.FG, 36) {
				t.Fatalf("frame %d: cell %d went from %v on %v to %v on %v", frame, x, l.FG, l.BG, c.FG, c.BG)
			}
			if !c.Text && c.Pattern == 0 {
				t.Fatalf("frame %d: cell %d shows no letter", frame, x)
			}
		}
		// The ground of its own fades out as that, clear of the grid's.
		if c := now.Cells[3].BG; c.A > alpha || c.A > 0 && !nearTint(color.NRGBA{c.R, c.G, c.B, 255}, nightMark, 0) {
			t.Fatalf("frame %d: the ground that goes is %v, after %d", frame, c, alpha)
		} else {
			alpha = c.A
		}
		cur := drawnOps(ops)[len(drawnOps(ops))-1].(*paint.RRectOp).Fill.Solid
		if !nearTint(cur, cursor, 36) {
			t.Fatalf("frame %d: the cursor went from %v to %v", frame, cursor, cur)
		}
		last, cursor = now.Cells, cur
		if !g.fade.Active() && frame > 2 {
			break
		}
		if frame > 200 {
			t.Fatal("the fade does not end")
		}
		run(1)
	}
	run(2)
	now, _ := cellsRow(w.Offscreen().Ops(), 0)
	for x, want := range []cellTint{{dayGround, dayInk}, {dayMark, dayInk}, {color.NRGBA{}, dayInk}, {color.NRGBA{}, dayInk}} {
		if got := (cellTint{now.Cells[x].BG, now.Cells[x].FG}); got != want {
			t.Errorf("cell %d came to %v, want %v", x, got, want)
		}
	}
	if g.from != nil || !w.Still() {
		t.Errorf("at the end the grid keeps %d rows, and is still: %v", len(g.from), w.Still())
	}
}

// TestAGridsFadeGoesOnFromWhereItIs: where the palette changes back on the way, and where the screen scrolls.
func TestAGridsFadeGoesOnFromWhereItIs(t *testing.T) {
	w, g, themed, run, on := fadeStage(t)
	shown := func() []paint.CellPaint {
		op, ok := cellsRow(w.Offscreen().Ops(), 0)
		if !ok {
			t.Fatal("the row is not drawn")
		}
		return op.Cells
	}
	on(func(u *gunim.UI) {
		// Before the rows, as after them.
		g.FadeColors(u)
		themed.Use(Dark().With(theme.Set(Background, dayGround)))
		dress(g, dayGround, dayInk, dayMark)
	})
	run(8)
	mid := slices.Clone(shown())
	if nearTint(mid[1].BG, nightMark, 8) || nearTint(mid[1].BG, dayMark, 8) {
		t.Fatalf("part way, the ground is %v", mid[1].BG)
	}
	// Another letter comes at once, in the colour the place shows.
	g.SetRow(0, []Cell{{Rune: 'z', FG: dayInk, BG: dayGround}, {Rune: 'y', FG: dayInk, BG: dayMark}})
	run(1)
	if now := shown(); now[1].Glyph.ID == mid[1].Glyph.ID || !nearTint(now[1].BG, mid[1].BG, 36) {
		t.Fatalf("scrolled part way, the cell went from %+v to %+v", mid[1], now[1])
	}
	mid = slices.Clone(shown())
	on(func(u *gunim.UI) {
		themed.Use(Dark().With(theme.Set(Background, nightGround)))
		dress(g, nightGround, nightInk, nightMark)
		g.FadeColors(u)
	})
	for frame := range 80 {
		now := shown()
		for x := range 2 {
			if !nearTint(now[x].BG, mid[x].BG, 36) || !nearTint(now[x].FG, mid[x].FG, 36) {
				t.Fatalf("back, frame %d: cell %d went from %v on %v to %v on %v", frame, x, mid[x].FG, mid[x].BG, now[x].FG, now[x].BG)
			}
		}
		mid = slices.Clone(now)
		run(1)
	}
	if got := shown()[1].BG; got != nightMark {
		t.Errorf("it came back to %v", got)
	}
}

// TestAGridFadesNothingItHasNotDrawn: the first frame shows the colours set, and so does one with animation off, or a
// grid of another size.
func TestAGridFadesNothingItHasNotDrawn(t *testing.T) {
	g := NewCellGrid()
	g.Resize(8, 2)
	dress(g, nightGround, nightInk, nightMark)
	var nilUI *gunim.UI
	g.FadeColors(nilUI)
	if g.from != nil || g.fade.Active() {
		t.Error("a grid not drawn yet fades")
	}
	w, g2, _, run, on := fadeStage(t)
	anim.SetScale(0)
	t.Cleanup(func() { anim.SetScale(1) })
	on(func(u *gunim.UI) {
		dress(g2, dayGround, dayInk, dayMark)
		g2.FadeColors(u)
	})
	if op, _ := cellsRow(w.Offscreen().Ops(), 0); op.Cells[1].BG != dayMark {
		t.Errorf("with animation off the ground is %v", op.Cells[1].BG)
	}
	anim.SetScale(1)
	on(func(u *gunim.UI) {
		g2.FadeColors(u)
		g2.Resize(10, 3)
		dress(g2, nightGround, nightInk, nightMark)
	})
	run(3)
	if op, _ := cellsRow(w.Offscreen().Ops(), 0); len(op.Cells) != 10 || op.Cells[1].BG != nightMark {
		t.Errorf("resized, the ground is %v", op.Cells[1].BG)
	}
}
