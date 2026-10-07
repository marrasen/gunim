package widget

import (
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// blank is a tile that draws nothing.
type blank struct{ _ int }

func (*blank) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size { return c.Max }

func (*blank) Paint(*paint.Painter, gunim.Frame, geom.Size, gunim.Children) {}

type tilesPicked struct {
	Sel    [][2]int
	Cursor int
}

type tileOpened struct{ I int }

type tilesInView struct{ First, Count int }

// tileStage puts a grid of n tiles 80 by 60 in a frame 400 by 300: four columns, 88 apart from x 28, and rows 68
// apart from y 14.
func tileStage(t *testing.T, n int) (g *TileGrid, w *gunim.Window, run func(int)) {
	t.Helper()
	g = NewTileGrid(geom.Sz(80, 60))
	g.Tile = func(int) gunim.Node { return &blank{} }
	g.OnSelect = func(sel [][2]int, cursor int) gunim.Intent { return tilesPicked{sel, cursor} }
	g.OnActivate = func(i int) gunim.Intent { return tileOpened{i} }
	g.OnView = func(first, count int) gunim.Intent { return tilesInView{first, count} }
	g.n = n
	w, run = stage(t, &frame{child: g, size: geom.Sz(400, 300)})
	run(2)
	return g, w, run
}

// tileAt returns the middle of tile i on screen, with the grid at the top.
func tileAt(i int) geom.Point {
	return geom.Pt(28+float32(i%4)*88+40, 14+float32(i/4)*68+30)
}

func press(w *gunim.Window, p geom.Point, clicks int, mods input.Mods) {
	w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: clicks, Mods: mods, Time: time.Now()})
	w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary, Mods: mods, Time: time.Now()})
}

func lastPicked(t *testing.T, w *gunim.Window) tilesPicked {
	t.Helper()
	var last tilesPicked
	found := false
	for _, v := range sent(w) {
		if p, ok := v.(tilesPicked); ok {
			last, found = p, true
		}
	}
	if !found {
		t.Fatal("no change of the tiles selected was sent")
	}
	return last
}

func TestTileGridBuildsOnlyTheTilesInView(t *testing.T) {
	g, w, run := tileStage(t, 100000)
	if g.Columns() != 4 {
		t.Fatalf("the grid has %d columns, want 4", g.Columns())
	}
	if n := g.Built(); n == 0 || n > 40 {
		t.Fatalf("the grid built %d tiles of 100,000", n)
	}
	var view tilesInView
	for _, v := range sent(w) {
		if tv, ok := v.(tilesInView); ok {
			view = tv
		}
	}
	if view.First != 0 || view.Count != g.Built() {
		t.Fatalf("the grid reported %+v in view, with %d built", view, g.Built())
	}
	press(w, tileAt(0), 1, 0)
	w.Input(input.KeyPress{Key: input.KeyEnd, Time: time.Now()})
	run(90)
	if _, c := g.Selected(); c != 99999 {
		t.Fatalf("End put the keyboard on %d, want the last tile", c)
	}
	if _, ok := g.live[99999]; !ok || g.Built() > 40 {
		t.Fatalf("after End the last tile is not built, with %d built", g.Built())
	}
}

func TestTileGridArrowsMoveInTwoDirections(t *testing.T) {
	g, w, run := tileStage(t, 30)
	press(w, tileAt(5), 1, 0)
	run(1)
	for _, c := range []struct {
		key  input.Key
		want int
	}{
		{input.KeyRight, 6}, {input.KeyDown, 10}, {input.KeyLeft, 9}, {input.KeyUp, 5},
		{input.KeyPageDown, 21}, {input.KeyPageDown, 29}, {input.KeyHome, 0}, {input.KeyUp, 0}, {input.KeyEnd, 29},
	} {
		w.Input(input.KeyPress{Key: c.key, Time: time.Now()})
		run(1)
		if _, cur := g.Selected(); cur != c.want {
			t.Fatalf("after %v the keyboard is on %d, want %d", c.key, cur, c.want)
		}
	}
	w.Input(input.KeyPress{Key: input.KeyUp, Mods: input.ModShift, Time: time.Now()})
	run(1)
	if got := lastPicked(t, w); !slices.Equal(got.Sel, [][2]int{{25, 30}}) || got.Cursor != 25 {
		t.Fatalf("Shift+Up from 29 selected %+v, want 25 to 29", got)
	}
	w.Input(input.KeyPress{Key: input.KeyA, Mods: input.ModControl, Time: time.Now()})
	run(1)
	if got := lastPicked(t, w); !slices.Equal(got.Sel, [][2]int{{0, 30}}) {
		t.Fatalf("Ctrl+A selected %v", got.Sel)
	}
	w.Input(input.KeyPress{Key: input.KeyEnter, Time: time.Now()})
	run(1)
	if !slices.Contains(sent(w), gunim.Intent(tileOpened{25})) {
		t.Fatal("Enter did not open the tile the keyboard is on")
	}
}

func TestTileGridClicksSelectAsAListDoes(t *testing.T) {
	_, w, run := tileStage(t, 30)
	press(w, tileAt(2), 1, 0)
	press(w, tileAt(9), 1, input.ModShift)
	run(1)
	if got := lastPicked(t, w).Sel; !slices.Equal(got, [][2]int{{2, 10}}) {
		t.Fatalf("a Shift+click on 9 from 2 selected %v", got)
	}
	press(w, tileAt(5), 1, input.ModControl)
	run(1)
	if got := lastPicked(t, w).Sel; !slices.Equal(got, [][2]int{{2, 5}, {6, 10}}) {
		t.Fatalf("a Ctrl+click on 5 left %v selected", got)
	}
	press(w, tileAt(7), 2, 0)
	run(1)
	if !slices.Contains(sent(w), gunim.Intent(tileOpened{7})) {
		t.Fatal("a double click did not open the tile")
	}
}

func TestTileGridBandSelectsWhatItTouches(t *testing.T) {
	g, w, run := tileStage(t, 10)
	// Below the last row, in empty space, to the middle of tile 1.
	from := geom.Pt(380, 250)
	w.Input(input.PointerDown{Pos: from, Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	for _, p := range []geom.Point{{X: 300, Y: 200}, {X: 200, Y: 120}, tileAt(1)} {
		w.Input(input.PointerMove{Pos: p, Time: time.Now()})
		run(1)
	}
	if g.bandIn.Value() <= 0 {
		t.Fatal("the band does not show while it is drawn")
	}
	w.Input(input.PointerUp{Pos: tileAt(1), Button: input.ButtonPrimary, Time: time.Now()})
	run(1)
	if got, want := lastPicked(t, w).Sel, [][2]int{{1, 4}, {5, 8}, {9, 10}}; !slices.Equal(got, want) {
		t.Fatalf("the band selected %v, want %v", got, want)
	}
	run(60)
	if g.bandIn.Value() > 0.01 {
		t.Fatal("the band stays once it is let go")
	}
	// A press on empty space without Ctrl clears the selection.
	press(w, from, 1, 0)
	run(1)
	if got := lastPicked(t, w); got.Sel != nil || got.Cursor != -1 {
		t.Fatalf("a press on empty space left %+v", got)
	}
}

func TestTileGridSpringsToANewTileSize(t *testing.T) {
	g, _, run := tileStage(t, 30)
	g.Size = geom.Sz(160, 120)
	run(4)
	if g.Columns() != 2 {
		t.Fatalf("with larger tiles the grid has %d columns, want 2", g.Columns())
	}
	cell := g.live[5]
	r := cell.rect()
	if r.Size().W <= 80 || r.Size().W >= 160 {
		t.Fatalf("four frames in, tile 5 is %v wide, want between 80 and 160", r.Size().W)
	}
	run(120)
	if got := cell.rect(); got.Size() != geom.Sz(160, 120) || got.Min != g.target(5).Min {
		t.Fatalf("tile 5 settled at %v, want %v", got, g.target(5))
	}
}

func TestTileGridArrivesFromWhereItIsTold(t *testing.T) {
	g, w, run := tileStage(t, 8)
	onTiles(t, w, func(u *gunim.UI) {
		g.Arrive(func(i int) (geom.Rect, bool) { return geom.Rc(0, float32(i)*20, 400, 20), true }, u)
	})
	run(1)
	if r := g.live[3].rect(); r != geom.Rc(0, 60, 400, 20) {
		t.Fatalf("tile 3 starts at %v, want its row", r)
	}
	run(120)
	if r := g.live[3].rect(); r != g.target(3) {
		t.Fatalf("tile 3 landed at %v, want %v", r, g.target(3))
	}
	onTiles(t, w, func(u *gunim.UI) {
		g.Depart(func(i int) (geom.Rect, bool) { return geom.Rc(0, float32(i)*20, 400, 20), true }, u)
	})
	run(2)
	if g.Departed() {
		t.Fatal("the tiles have gone at once")
	}
	run(120)
	if !g.Departed() || g.live[3].rect() != geom.Rc(0, 60, 400, 20) {
		t.Fatalf("after leaving tile 3 is at %v", g.live[3].rect())
	}
}

func TestTileGridTakesCtrlWithTheWheel(t *testing.T) {
	g, w, run := tileStage(t, 8)
	var got float32
	g.OnZoom = func(n float32, _ *gunim.UI) { got += n }
	w.Input(input.Scroll{Pos: tileAt(1), Notches: geom.Pt(0, 2), Mods: input.ModControl, Time: time.Now()})
	run(1)
	if got != 2 || !g.ZoomsWithWheel() {
		t.Fatalf("Ctrl with the wheel zoomed the tiles by %v notches, want 2", got)
	}
}

type tileAct struct{ do func(u *gunim.UI) }

// onTiles runs do on the window's goroutine, with its UI, through a patch.
func onTiles(t *testing.T, w *gunim.Window, do func(u *gunim.UI)) {
	t.Helper()
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, a tileAct, u *gunim.UI) { a.do(u) })
	if err := w.Client().Patch("stage", tileAct{do}); err != nil {
		t.Fatal(err)
	}
}

func TestTileGridJumpsToATile(t *testing.T) {
	g, w, run := tileStage(t, 1000)
	onTiles(t, w, func(u *gunim.UI) { g.JumpToTile(401, u) })
	run(1)
	// Row 100 starts at 14 + 100*68; the view puts it at the top, less the padding.
	if got := g.Offset(); got != 6800 {
		t.Fatalf("the grid jumped to %v, want 6800", got)
	}
	if _, ok := g.live[401]; !ok {
		t.Fatal("tile 401 is not built after the jump")
	}
}

// A zoom moves the tiles under a still pointer, and the tile lit is the
// one under it at the new zoom, with no move of the pointer.
func TestTileGridHoverFollowsAZoom(t *testing.T) {
	g, w, run := tileStage(t, 30)
	at := tileAt(7)
	w.Input(input.PointerMove{Pos: at, Time: time.Now()})
	run(1)
	if g.hover != 7 {
		t.Fatalf("the pointer over tile 7 lit %d", g.hover)
	}
	if err := w.Client().SetZoom(1.5); err != nil {
		t.Fatal(err)
	}
	for range 30 {
		run(1)
		// The grid's own space is the window's, shrunk by the zoom.
		if want := g.at(at.Mul(1 / 1.5)); g.hover != want {
			t.Fatalf("at zoom 1.5 the pointer is over tile %d, and %d is lit", want, g.hover)
		}
	}
	if g.hover == 7 {
		t.Fatal("the zoom left tile 7 under the pointer, so the test tests nothing")
	}
}

func TestTileGridRebuildsItsTilesForANewSet(t *testing.T) {
	g, w, run := tileStage(t, 20)
	built := map[int]int{}
	g.Tile = func(i int) gunim.Node { built[i]++; return &blank{} }
	sent(w)
	do(t, w, func(u *gunim.UI) { g.Rebuild(u) })
	run(2)
	// Every tile in view is built again, once, and the view is told anew.
	if built[0] != 1 || built[5] != 1 {
		t.Fatalf("after Rebuild, tiles built %v; want each in view once more", built)
	}
	var told bool
	for _, in := range sent(w) {
		if _, ok := in.(tilesInView); ok {
			told = true
		}
	}
	if !told {
		t.Fatal("Rebuild did not tell OnView the tiles built anew")
	}
	run(2)
	if built[0] != 1 {
		t.Fatalf("tile 0 was built %d times; want once", built[0])
	}
}
