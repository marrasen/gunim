package main

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// gameTopic is what a test's game view watches.
const gameTopic = "game"

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
	if len(got) != 1 || got[0] != (ShowMap{}) {
		t.Fatalf("the card's button sent %v, want the map", got)
	}
}

func TestACandyPickedEarlierLetsGoOnceACellIsFilledTheOtherWay(t *testing.T) {
	g, _ := newTestGame()
	g.Round = 1
	w, root, run := stage(t, geom.Sz(460, 860), g.Game)
	var empty []int
	for c := range 81 {
		if g.Cells[c] == 0 && len(empty) < 2 {
			empty = append(empty, c)
		}
	}
	// The 4 left picked from earlier, as a cell is selected and the 7
	// tapped: the 7 goes in that cell.
	root.armed, root.selected = 4, empty[0]
	tapAt(w, run, root.trayCenter(7))
	if got := intents(w); len(got) != 1 || got[0] != (Place{Cell: empty[0], Digit: 7}) {
		t.Fatalf("cell then 7 sent %v", got)
	}
	// Tapping the next cell only selects it: the 4 is no longer picked.
	tapAt(w, run, root.cellCenter(empty[1]))
	if got := intents(w); len(got) != 0 {
		t.Fatalf("a tap on the next cell, after placing cell first, sent %v; want it only selected", got)
	}
	if root.selected != empty[1] || root.armed != 0 {
		t.Fatalf("selected %d, armed %d; want the cell selected and nothing picked", root.selected, root.armed)
	}
}

func TestAPickedCandyLetsGoWhenItsDigitIsUsedUp(t *testing.T) {
	g, _ := newTestGame()
	g.Round = 1
	w, root, run := stage(t, geom.Sz(460, 860), g.Game)
	tapAt(w, run, root.trayCenter(5))
	if root.armed != 5 {
		t.Fatalf("armed %d", root.armed)
	}
	for c := range 81 {
		if g.Cells[c] == 0 && g.puzzle.Solution[c] == 5 {
			g.place(Place{Cell: c, Digit: 5})
		}
	}
	if err := w.Client().Publish(gameTopic, g.Game); err != nil {
		t.Fatal(err)
	}
	run(1)
	if root.armed != 0 {
		t.Fatalf("the 5s all placed, the 5 is still picked")
	}
}

func TestTheGameKeepsClearOfAPhonesBars(t *testing.T) {
	g, _ := newTestGame()
	g.Round = 1
	w, root, run := stage(t, geom.Sz(412, 915), g.Game)
	w.Offscreen().SetSafeArea(geom.Insets{Top: 50, Bottom: 24})
	run(2)
	var head, tools geom.Rect
	gunim.RegisterPatch(w, "game", func(r *gameRoot, _ struct{}, u *gunim.UI) {
		head, _ = u.Bounds(r.header)
		tools, _ = u.Bounds(r.tools)
	})
	if err := w.Client().Patch("game", struct{}{}); err != nil {
		t.Fatal(err)
	}
	run(1)
	if head.Min.Y < 50 {
		t.Fatalf("the header starts at %v, under the status bar", head.Min.Y)
	}
	if tools.Max.Y > 915-24 {
		t.Fatalf("the tools end at %v, under the navigation bar", tools.Max.Y)
	}
	_ = root
}
