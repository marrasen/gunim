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

// apply carries out the intents the view sent, as the app would, and
// shows the game they leave.
func apply(t *testing.T, w *gunim.Window, run func(int), g *game) []gunim.Intent {
	t.Helper()
	got := intents(w)
	for _, in := range got {
		if p, ok := in.(Place); ok {
			g.place(p)
		}
	}
	if err := w.Client().Publish(gameTopic, g.Game); err != nil {
		t.Fatal(err)
	}
	run(1)
	return got
}

// wrongFor returns a digit cell c does not take.
func wrongFor(g *game, c int) int8 { return g.puzzle.Solution[c]%9 + 1 }

func TestARightCandyLetsItsCellGo(t *testing.T) {
	g, _ := newTestGame()
	g.Round = 1
	w, root, run := stage(t, geom.Sz(460, 860), g.Game)
	c := emptyCell(g)
	tapAt(w, run, root.cellCenter(c))
	tapAt(w, run, root.trayCenter(g.puzzle.Solution[c]))
	apply(t, w, run, g)
	if root.selected != -1 {
		t.Fatalf("cell %d filled, %d is still selected", c, root.selected)
	}
	// The next candy only cheers: no cell takes it.
	tapAt(w, run, root.trayCenter(g.puzzle.Solution[c]))
	if got := intents(w); len(got) != 0 {
		t.Fatalf("a candy tapped with no cell selected sent %v", got)
	}
}

func TestACandyWithNoCellSelectedPlacesNothing(t *testing.T) {
	for _, size := range []geom.Size{geom.Sz(460, 860), geom.Sz(1100, 720)} {
		g, _ := newTestGame()
		g.Round = 1
		w, root, run := stage(t, size, g.Game)
		for d := int8(1); d <= 9; d++ {
			tapAt(w, run, root.trayCenter(d))
			tapAt(w, run, root.trayCenter(d))
		}
		if got := intents(w); len(got) != 0 {
			t.Fatalf("%v: candies tapped with no cell selected sent %v", size, got)
		}
		if root.selected != -1 {
			t.Fatalf("%v: candies tapped selected cell %d", size, root.selected)
		}
		// A cell tapped after selects it, and takes nothing.
		c := emptyCell(g)
		tapAt(w, run, root.cellCenter(c))
		if got := intents(w); len(got) != 0 || root.selected != c {
			t.Fatalf("%v: a tap on cell %d sent %v and selected %d", size, c, got, root.selected)
		}
	}
}

func TestASecondTapOnACellLetsItGo(t *testing.T) {
	g, _ := newTestGame()
	g.Round = 1
	w, root, run := stage(t, geom.Sz(460, 860), g.Game)
	c := emptyCell(g)
	tapAt(w, run, root.cellCenter(c))
	tapAt(w, run, root.cellCenter(c))
	if root.selected != -1 {
		t.Fatalf("cell %d tapped twice is still selected", c)
	}
	tapAt(w, run, root.trayCenter(4))
	if got := intents(w); len(got) != 0 {
		t.Fatalf("the 4, after cell %d was let go, sent %v", c, got)
	}
}

func TestATapOnAFullCellSelectsNothing(t *testing.T) {
	g, _ := newTestGame()
	g.Round = 1
	w, root, run := stage(t, geom.Sz(460, 860), g.Game)
	// The candies land from running in first.
	run(300)
	full := -1
	for c := range 81 {
		if g.Cells[c] != 0 {
			full = c
			break
		}
	}
	empty := emptyCell(g)
	tapAt(w, run, root.cellCenter(empty))
	tapAt(w, run, root.cellCenter(full))
	if root.selected != -1 {
		t.Fatalf("a tap on full cell %d left %d selected", full, root.selected)
	}
	if !root.board.cells[full].cheering {
		t.Fatalf("a tap on full cell %d set no candy cheering", full)
	}
	tapAt(w, run, root.trayCenter(4))
	if got := intents(w); len(got) != 0 {
		t.Fatalf("the 4, after a full cell was tapped, sent %v", got)
	}
}

// A wrong candy leaves its cell selected for another try. Letting the
// cell go then means no candy tapped after goes anywhere: the heart
// lost was the only one.
func TestAfterAWrongCandyNothingGoesInUnasked(t *testing.T) {
	g, _ := newTestGame()
	g.Round = 1
	w, root, run := stage(t, geom.Sz(460, 860), g.Game)
	c := emptyCell(g)
	tapAt(w, run, root.cellCenter(c))
	tapAt(w, run, root.trayCenter(wrongFor(g, c)))
	if got := apply(t, w, run, g); len(got) != 1 {
		t.Fatalf("one candy tapped sent %v", got)
	}
	if g.Lives != startLives-1 || root.selected != c {
		t.Fatalf("after a wrong candy: %d lives, cell %d selected; want %d and %d", g.Lives, root.selected, startLives-1, c)
	}
	tapAt(w, run, root.cellCenter(c))
	for d := int8(1); d <= 9; d++ {
		tapAt(w, run, root.trayCenter(d))
	}
	if got := apply(t, w, run, g); len(got) != 0 {
		t.Fatalf("candies tapped after the cell was let go sent %v", got)
	}
	if g.Lives != startLives-1 {
		t.Fatalf("%d lives; want %d", g.Lives, startLives-1)
	}
}

// The ring pops in on the cell tapped, slides to the next, and fades
// as the cell lets go, frame by frame, and the board comes to rest.
func TestTheSelectionRingAnimates(t *testing.T) {
	g, _ := newTestGame()
	g.Round = 1
	w, root, run := stage(t, geom.Sz(460, 860), g.Game)
	run(300)
	b := root.board
	var empty []int
	for c := range 81 {
		if g.Cells[c] == 0 && (len(empty) == 0 || empty[0]%9 != c%9) {
			empty = append(empty, c)
		}
	}
	a, z := empty[0], empty[1]

	tapAt(w, run, root.cellCenter(a))
	if v := b.ringIn.Value(); v <= 0 || v >= 0.9 {
		t.Fatalf("a frame after the tap, the ring is %v in; want it on its way", v)
	}
	if b.ringX.Value() != float32(a%9) || b.ringY.Value() != float32(a/9) {
		t.Fatalf("the ring pops in at %v,%v, not on cell %d", b.ringX.Value(), b.ringY.Value(), a)
	}
	for range 60 {
		run(1)
	}
	if v := b.ringIn.Value(); v < 0.98 || v > 1.02 {
		t.Fatalf("a second on, the ring is %v in", v)
	}

	// To the next cell it slides, through the columns between.
	tapAt(w, run, root.cellCenter(z))
	from, to := float32(a%9), float32(z%9)
	between := false
	for range 60 {
		x := b.ringX.Value()
		if x != from && x != to {
			between = true
		}
		if b.ringIn.Value() < 0.5 {
			t.Fatalf("sliding, the ring faded to %v", b.ringIn.Value())
		}
		run(1)
	}
	if !between {
		t.Fatal("the ring jumped from cell to cell")
	}
	if b.ringX.Value() != to || b.ringY.Value() != float32(z/9) {
		t.Fatalf("a second on, the ring is at %v,%v, not on cell %d", b.ringX.Value(), b.ringY.Value(), z)
	}

	// Let go, it fades, never growing back, and the board rests.
	tapAt(w, run, root.cellCenter(z))
	last := b.ringIn.Value()
	if last >= 1 {
		t.Fatalf("a frame after the cell let go, the ring is %v in", last)
	}
	for range 60 {
		run(1)
		v := b.ringIn.Value()
		if v > last+1e-4 {
			t.Fatalf("fading, the ring grew from %v to %v", last, v)
		}
		last = v
	}
	if last > 0.01 {
		t.Fatalf("a second after letting go, the ring is %v in", last)
	}
	if b.Step(time.Second / 60) {
		t.Fatal("a second after letting go, the board still moves")
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
