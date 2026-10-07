package widget

import (
	"slices"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
)

// A click while the rows spring to a new order lands on the row drawn under the pointer, on every frame of the
// move.
func TestAClickOnAListMovingHitsTheRowDrawnThere(t *testing.T) {
	w, l, run := newReorderList(t)
	l.OnClick = func(k Key) gunim.Intent { return rowClicked{k} }
	back := keys(5)
	slices.Reverse(back)
	if err := w.Client().Update("l", shownItems{back}); err != nil {
		t.Fatal(err)
	}
	for frame := range 40 {
		run(1)
		for _, y := range []float32{20, 100, 180} {
			click(w, 100, y)
			got := sent(w)
			if len(got) == 0 {
				// A gap between rows: none is drawn there.
				for k, r := range l.rows {
					if at := r.y.Value(); y >= at && y < at+40 {
						t.Fatalf("frame %d: a click at %v hit nothing, with row %s drawn at %v", frame, y, k, at)
					}
				}
				continue
			}
			c, ok := got[0].(rowClicked)
			if !ok {
				t.Fatalf("frame %d: a click sent %v", frame, got)
			}
			k := c.Key
			if at := l.rows[k].y.Value(); y < at || y >= at+40 {
				t.Fatalf("frame %d: a click at %v hit row %s, drawn at %v", frame, y, k, at)
			}
		}
	}
	// The first frame of the move, row 0 is still drawn at the top.
	if err := w.Client().Update("l", shownItems{keys(5)}); err != nil {
		t.Fatal(err)
	}
	run(1)
	click(w, 100, 20)
	if got := sent(w); len(got) != 1 || got[0] != (rowClicked{"4"}) {
		t.Fatalf("a click at the top as the order turns back sent %v, want row 4, still drawn there", got)
	}
}

// While a row leaves a table, the rows below it close up; RowAt and RowRect follow them as drawn, on every frame.
func TestATableFindsTheRowDrawnWhileOneLeaves(t *testing.T) {
	w, tbl, _, run := newTableStage(t, 10)
	u := viewUI(t, w, "t", run)
	tbl.SetKeys(slices.Delete(slices.Clone(tbl.keys), 2, 3), u)
	for frame := range 40 {
		run(1)
		for y := tbl.headH + 1; y < tbl.headH+8*tbl.rowH; y += 7 {
			p := geom.Pt(50, y)
			k, ok := tbl.RowAt(p)
			for key, r := range tbl.list.live {
				b, drawn := u.Bounds(r)
				if !drawn || tbl.list.gone[key] {
					continue
				}
				if b.Min.Y+0.5 < y && y < b.Max.Y-0.5 && (!ok || k != key) {
					t.Fatalf("frame %d: at y %v RowAt gave %q, %v, with row %s drawn there, %v", frame, y, k, ok, key, b)
				}
				if rr, _ := tbl.RowRect(key); abs32(rr.Min.Y-b.Min.Y) > 0.5 {
					t.Fatalf("frame %d: RowRect puts row %s at %v, drawn at %v", frame, key, rr.Min.Y, b.Min.Y)
				}
			}
		}
	}
}

// While the tiles spring to a new size and new places, TileAt and TileRect follow them as drawn, on every frame.
func TestATileGridFindsTheTileDrawnWhileTheTilesSpring(t *testing.T) {
	g, w, run := tileStage(t, 30)
	u := stageUI(t, w, run)
	g.Size = geom.Sz(110, 80)
	u.Invalidate()
	for frame := range 40 {
		run(1)
		for i, tc := range g.live {
			b, drawn := u.Bounds(tc)
			if !drawn || b.Size().W < 2 {
				continue
			}
			// Where tiles cross on their way, any tile drawn there will do.
			got := g.TileAt(b.Center())
			if hit, ok := g.live[got]; got != i && (!ok || !boundsHold(u, hit, b.Center())) {
				t.Fatalf("frame %d: TileAt at the middle of tile %d, drawn at %v, gave %d", frame, i, b, got)
			}
			if r := g.TileRect(i); abs32(r.Min.X-b.Min.X) > 0.5 || abs32(r.Min.Y-b.Min.Y) > 0.5 {
				t.Fatalf("frame %d: TileRect puts tile %d at %v, drawn at %v", frame, i, r, b)
			}
		}
	}
}

// boundsHold reports whether n was last drawn over p.
func boundsHold(u *gunim.UI, n gunim.Node, p geom.Point) bool {
	b, ok := u.Bounds(n)
	return ok && b.Contains(p)
}

// A list scrolled to a key while a row above it leaves goes to where the key's row is drawn.
func TestScrollToKeyCountsALeavingRowByTheRoomItTakes(t *testing.T) {
	w, l, run := newVirtual(t, keys(100), 40)
	u := viewUI(t, w, "v", run)
	if err := w.Client().Update("v", shownKeys{slices.Delete(keys(100), 1, 2)}); err != nil {
		t.Fatal(err)
	}
	run(3)
	l.ScrollToKey("30", u)
	run(180)
	r, ok := l.live["30"]
	if !ok {
		t.Fatal("row 30 is not built after scrolling to it")
	}
	if d := r.y.Value() - l.offset.Value(); abs32(d) > 0.5 {
		t.Fatalf("scrolled to row 30, it is drawn %v from the top", d)
	}
}
