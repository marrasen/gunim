package main

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// stage mounts the game view in an offscreen window of size showing g,
// and returns the window, its root, and a way to step frames.
func stage(t *testing.T, size geom.Size, g Game) (*gunim.Window, *gameRoot, func(int)) {
	t.Helper()
	var root *gameRoot
	w := gunim.NewOffscreen(size, nil)
	gunim.RegisterView(w, "game",
		func(Game) *gameRoot { root = newGameRoot(nil); return root },
		func(r *gameRoot, s Game, u *gunim.UI) { r.show(s, u) })
	if err := w.Client().Mount(gunim.Root, "game", "game", g, gameTopic); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(2)
	return w, root, run
}

func tapAt(w *gunim.Window, run func(int), p geom.Point) {
	w.Input(input.PointerMove{Pos: p})
	w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary})
	run(1)
}

func intents(w *gunim.Window) []gunim.Intent {
	var out []gunim.Intent
	for len(w.Client().Intents()) > 0 {
		out = append(out, (<-w.Client().Intents()).Intent)
	}
	return out
}

func TestATapOnACellThenACandyPlacesIt(t *testing.T) {
	for _, size := range []geom.Size{geom.Sz(460, 860), geom.Sz(1100, 720)} {
		g, _ := newTestGame()
		g.Round = 1
		w, root, run := stage(t, size, g.Game)
		c := emptyCell(g)
		tapAt(w, run, root.cellCenter(c))
		if root.selected != c {
			t.Fatalf("%v: a tap on cell %d selected %d", size, c, root.selected)
		}
		tapAt(w, run, root.trayCenter(4))
		got := intents(w)
		if len(got) != 1 || got[0] != (Place{Cell: c, Digit: 4}) {
			t.Fatalf("%v: a tap on the 4 sent %v, want Place in cell %d", size, got, c)
		}
	}
}

func TestACandyPickedFirstGoesInEachCellTapped(t *testing.T) {
	g, _ := newTestGame()
	g.Round = 1
	w, root, run := stage(t, geom.Sz(460, 860), g.Game)
	tapAt(w, run, root.trayCenter(7))
	if root.armed != 7 {
		t.Fatalf("a tap on the 7 with no cell armed %d", root.armed)
	}
	var empty []int
	for c := range 81 {
		if g.Cells[c] == 0 && len(empty) < 2 {
			empty = append(empty, c)
		}
	}
	for _, c := range empty {
		tapAt(w, run, root.cellCenter(c))
	}
	got := intents(w)
	if len(got) != 2 || got[0] != (Place{Cell: empty[0], Digit: 7}) || got[1] != (Place{Cell: empty[1], Digit: 7}) {
		t.Fatalf("two cells tapped with the 7 armed sent %v", got)
	}
}

// TestEveryEventPlaysThroughToRest plays a game's events from awkward
// states, a frame at a time, and checks the window comes to rest but
// for the sky.
func TestEveryEventPlaysThroughToRest(t *testing.T) {
	g, clk := newTestGame()
	g.Round = 1
	w, root, run := stage(t, geom.Sz(460, 860), g.Game)
	publish := func() {
		if err := w.Client().Publish(gameTopic, g.Game); err != nil {
			t.Fatal(err)
		}
		run(1)
	}
	// A wrong candy, then the first row filled a frame apart, then a
	// hint and an undo, while the effects of each still play.
	row := []int{}
	for c := range 9 {
		if g.Cells[c] == 0 {
			row = append(row, c)
		}
	}
	g.place(Place{Cell: row[0], Digit: g.puzzle.Solution[row[0]]%9 + 1})
	publish()
	for _, c := range row {
		g.place(Place{Cell: c, Digit: g.puzzle.Solution[c]})
		clk.t = clk.t.Add(time.Second)
		publish()
	}
	g.hint(-1)
	publish()
	g.undoMove()
	publish()
	for range 400 {
		run(1)
	}
	if len(root.fx.parts) != 0 || len(root.fx.texts) != 0 {
		t.Fatalf("seven seconds on, %d particles and %d words still show", len(root.fx.parts), len(root.fx.texts))
	}
	if root.board.Step(time.Second/60) && root.selected < 0 {
		t.Fatal("seven seconds on, the board still moves")
	}
	if root.header.lives != 2 {
		t.Fatalf("the header shows %d lives after one wrong candy", root.header.lives)
	}
}

func TestWinningShowsTheCardWithItsStars(t *testing.T) {
	g, clk := newTestGame()
	g.Round = 1
	w, root, run := stage(t, geom.Sz(460, 860), g.Game)
	for c := range 81 {
		if g.Cells[c] == 0 {
			g.place(Place{Cell: c, Digit: g.puzzle.Solution[c]})
			clk.t = clk.t.Add(time.Second)
		}
	}
	if err := w.Client().Publish(gameTopic, g.Game); err != nil {
		t.Fatal(err)
	}
	run(180)
	if !root.card.shown || !root.card.win {
		t.Fatal("a won game shows no card")
	}
	for i := range 3 {
		if v := root.card.starsIn[i].Value(); v < 0.99 {
			t.Fatalf("three seconds after winning, star %d is %v in", i+1, v)
		}
	}
	// Its button starts the next level.
	b := root.card.button()
	tapAt(w, run, geom.Pt(b.Min.X+b.Size().W/2, b.Min.Y+b.Size().H/2))
	got := intents(w)
	if len(got) != 1 || got[0] != (Start{Level: 2}) {
		t.Fatalf("the card's button sent %v, want Start 2", got)
	}
}
