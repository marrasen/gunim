package widget

import (
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// dragTiles is a grid of 20 draggable tiles, with a drop target right of it that takes them.
func dragTiles(t *testing.T) (g *TileGrid, w *gunim.Window, run func(int), drops *[]any) {
	t.Helper()
	g = NewTileGrid(geom.Sz(60, 60))
	g.Tile = func(int) gunim.Node { return &block{h: 60} }
	g.DragTiles = func(sel [][2]int, _ geom.Point) (any, gunim.Node, geom.Point) {
		return rowsDragged{sel}, NewDragGhost(&block{h: 20}, geom.Pt(4, 4)), geom.Pt(4, 4)
	}
	var got []any
	target := NewDropTarget(&block{h: 300})
	target.OnDrop = func(e input.Drop, u *gunim.UI) gunim.Intent { got = append(got, e.Data); return nil }
	w, run = stage(t, &halves{left: g, right: target})
	g.n = 20
	run(10)
	return g, w, run, &got
}

func TestDraggingASelectedTileDragsTheWholeSelection(t *testing.T) {
	g, w, run, drops := dragTiles(t)
	g.runs, g.cursor = [][2]int{{1, 2}, {3, 5}}, 3
	at := g.TileRect(3).Center()
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	w.Input(input.PointerMove{Pos: at.Add(geom.Pt(10, 0)), Time: time.Now()})
	run(5)
	if !g.lift.dragging {
		t.Fatal("a press on a selected tile and a move did not start a drag")
	}
	w.Input(input.PointerMove{Pos: geom.Pt(600, 100), Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(600, 100), Button: input.ButtonPrimary, Time: time.Now()})
	run(60)
	if len(*drops) != 1 || !slices.Equal(dragged((*drops)[0]), [][2]int{{1, 2}, {3, 5}}) {
		t.Fatalf("the target got %v, want the three tiles", *drops)
	}
	if sel, _ := g.Selected(); !slices.Equal(sel, [][2]int{{1, 2}, {3, 5}}) || g.lift.armed {
		t.Fatalf("after the drop %v is selected and the press is held %v", sel, g.lift.armed)
	}
}

func TestAClickOnASelectedTileWithoutAMoveSelectsItAlone(t *testing.T) {
	g, w, run, drops := dragTiles(t)
	g.runs, g.cursor = [][2]int{{1, 2}, {3, 5}}, 3
	at := g.TileRect(3).Center()
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary, Time: time.Now()})
	run(5)
	if sel, _ := g.Selected(); !slices.Equal(sel, [][2]int{{3, 4}}) || len(*drops) != 0 {
		t.Fatalf("a click on a selected tile left %v selected, and dropped %v", sel, *drops)
	}
}
