package main

import (
	"testing"
	"time"
)

// clock is a test's time, moved by hand.
type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newTestGame() (*game, *clock) {
	c := &clock{t: time.Unix(1000, 0)}
	return newGame(1, c.now), c
}

// emptyCell returns an empty cell of g.
func emptyCell(g *game) int {
	for c := range 81 {
		if g.Cells[c] == 0 {
			return c
		}
	}
	return -1
}

func kinds(es []Event) []EventKind {
	out := make([]EventKind, 0, len(es))
	for _, e := range es {
		out = append(out, e.Kind)
	}
	return out
}

func TestARightCandyScoresAndQuickOnesCombo(t *testing.T) {
	g, clk := newTestGame()
	for i := 1; i <= 3; i++ {
		c := emptyCell(g)
		g.place(Place{Cell: c, Digit: g.puzzle.Solution[c]})
		if g.Combo != i {
			t.Fatalf("candy %d placed a second after the last: combo %d, want %d", i, g.Combo, i)
		}
		clk.t = clk.t.Add(time.Second)
	}
	if g.Score < 50+100+150 {
		t.Fatalf("score %d after a combo of three, want at least 300", g.Score)
	}
	clk.t = clk.t.Add(10 * time.Second)
	c := emptyCell(g)
	g.place(Place{Cell: c, Digit: g.puzzle.Solution[c]})
	if g.Combo != 1 {
		t.Fatalf("a candy ten seconds on: combo %d, want it started over", g.Combo)
	}
}

func TestAWrongCandyBreaksAHeartAndThreeLoseTheGame(t *testing.T) {
	g, _ := newTestGame()
	c := emptyCell(g)
	wrong := g.puzzle.Solution[c]%9 + 1
	for i := range 3 {
		g.place(Place{Cell: c, Digit: wrong})
		if g.Cells[c] != 0 {
			t.Fatal("a wrong candy stayed in its cell")
		}
		if g.Lives != 2-i {
			t.Fatalf("after %d wrong candies, %d lives", i+1, g.Lives)
		}
	}
	if !g.Lost || g.Events[len(g.Events)-1].Kind != LostEvent {
		t.Fatalf("three wrong candies: lost %v, events %v", g.Lost, kinds(g.Events))
	}
	g.place(Place{Cell: c, Digit: g.puzzle.Solution[c]})
	if g.Cells[c] != 0 {
		t.Fatal("a lost game took a candy")
	}
}

func TestFinishingTheBoardFinishesGroupsAndWins(t *testing.T) {
	g, clk := newTestGame()
	done := 0
	for c := range 81 {
		if g.Cells[c] == 0 {
			g.place(Place{Cell: c, Digit: g.puzzle.Solution[c]})
			clk.t = clk.t.Add(time.Second)
			for _, e := range g.Events {
				if e.Kind == Done && e.ID > done {
					done = e.ID
				}
			}
		}
	}
	if !g.Won || g.Stars != 3 {
		t.Fatalf("a board filled with no mistakes: won %v, %d stars", g.Won, g.Stars)
	}
	last := g.Events[len(g.Events)-1]
	if last.Kind != WonEvent {
		t.Fatalf("the last event is %v, want the win", last.Kind)
	}
}

func TestNotesClearAsTheirDigitIsPlacedAndUndoBringsThemBack(t *testing.T) {
	g, _ := newTestGame()
	c := emptyCell(g)
	d := g.puzzle.Solution[c]
	// A note of d in a peer of c, that c's d rules out.
	p := -1
	for _, q := range peers[c] {
		if g.Cells[q] == 0 {
			p = q
			break
		}
	}
	g.place(Place{Cell: p, Digit: d, Note: true})
	if g.Notes[p] != 1<<d {
		t.Fatalf("notes %b after pencilling %d", g.Notes[p], d)
	}
	g.place(Place{Cell: c, Digit: d})
	if g.Notes[p] != 0 {
		t.Fatal("placing a digit left it pencilled in a peer")
	}
	g.undoMove()
	if g.Cells[c] != 0 || g.Notes[p] != 1<<d {
		t.Fatal("undo did not bring back the cell and the note")
	}
}

func TestAHintFillsTheCellAskedOrAnEasyOne(t *testing.T) {
	g, _ := newTestGame()
	c := emptyCell(g)
	g.hint(c)
	if g.Cells[c] != g.puzzle.Solution[c] || g.Hints != 1 {
		t.Fatalf("a hint on cell %d left %d, hints %d", c, g.Cells[c], g.Hints)
	}
	g.hint(-1)
	if g.Hints != 2 {
		t.Fatal("a hint with no cell did nothing")
	}
}
