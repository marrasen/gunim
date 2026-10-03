package main

import (
	"math/bits"
	"time"
)

// The vocabulary the two halves share.
type (
	// Game is what the window shows of the game being played.
	Game struct {
		// Round counts the games played, so the window can tell a new
		// one from the one going on.
		Round      int
		Level      int
		Difficulty Difficulty
		// Cells holds the digits on the board, Givens which of them the
		// puzzle started with, and Notes the digits pencilled in each
		// empty cell, as bits 1 to 9.
		Cells  Grid
		Givens [81]bool
		Notes  [81]uint16
		// Lives are the hearts left; a wrong candy breaks one.
		Lives int
		Score int
		// Combo counts the right candies placed each soon after the
		// last; it multiplies their score.
		Combo int
		// Hints counts the hints taken, and Mistakes the wrong candies.
		Hints, Mistakes int
		Won, Lost       bool
		// Stars is the game's stars, once won: three for no mistakes.
		Stars int
		// Events are the newest things to happen, oldest first, for the
		// window to play each once.
		Events []Event
	}

	// An Event is something that happened, numbered so the window plays
	// each once.
	Event struct {
		ID    int
		Kind  EventKind
		Cell  int
		Digit int8
		// Unit is the row, column or box a Done event finished: 0 to 8
		// rows, 9 to 17 columns, 18 to 26 boxes.
		Unit  int
		Combo int
		// Points is the score the event made.
		Points int
	}
	// EventKind says what an event was.
	EventKind int

	// Place puts digit in cell, or pencils it in as a note with Note.
	Place struct {
		Cell  int
		Digit int8
		Note  bool
	}
	// Erase empties a cell's notes.
	Erase struct{ Cell int }
	// Undo takes back the last candy or note.
	Undo struct{}
	// Hint reveals a cell: Cell, when it is empty, or the easiest
	// other.
	Hint struct{ Cell int }
	// Start starts a level, from the start.
	Start struct{ Level int }
)

// The kinds of event.
const (
	// Placed is a right candy put in its cell.
	Placed EventKind = iota + 1
	// Wrong is a candy that did not belong where it was put.
	Wrong
	// Done is a row, column or box finished.
	Done
	// DigitDone is the ninth of a digit placed: the tray has no more.
	DigitDone
	// Hinted is a cell revealed by a hint.
	Hinted
	// Won is the board finished.
	WonEvent
	// Lost is the last heart broken.
	LostEvent
)

// startLives is the hearts a game starts with.
const startLives = 3

// comboWindow is how soon after the last right candy the next must come
// to keep the combo going.
const comboWindow = 4 * time.Second

// game is a game being played, reached from the application half alone.
type game struct {
	Game
	puzzle Puzzle
	// undo holds the board and notes before each move.
	undo   []snapshot
	ids    int
	lastAt time.Time
	now    func() time.Time
}

type snapshot struct {
	cells Grid
	notes [81]uint16
}

// maxEvents is how many of the newest events the state keeps.
const maxEvents = 24

// newGame starts level.
func newGame(level int, now func() time.Time) *game {
	d := difficultyOf(level)
	p := generate(uint64(level)*0x2545f491+7, d)
	g := &game{puzzle: p, now: now}
	g.Level, g.Difficulty, g.Lives = level, p.Difficulty, startLives
	g.Cells = p.Givens
	for c := range 81 {
		g.Givens[c] = p.Givens[c] != 0
	}
	return g
}

// difficultyOf is how hard level is: the first levels are easy, and
// each difficulty lasts longer than the one before.
func difficultyOf(level int) Difficulty {
	switch {
	case level <= 6:
		return Easy
	case level <= 18:
		return Medium
	case level <= 36:
		return Hard
	}
	return Expert
}

func (g *game) event(e Event) {
	g.ids++
	e.ID = g.ids
	g.Events = append(g.Events, e)
	if len(g.Events) > maxEvents {
		g.Events = append(g.Events[:0:0], g.Events[len(g.Events)-maxEvents:]...)
	}
}

func (g *game) save() {
	g.undo = append(g.undo, snapshot{g.Cells, g.Notes})
	if len(g.undo) > 200 {
		g.undo = g.undo[1:]
	}
}

// over says the game takes no more moves.
func (g *game) over() bool { return g.Won || g.Lost }

// place carries out a Place.
func (g *game) place(in Place) {
	c := in.Cell
	if g.over() || c < 0 || c >= 81 || in.Digit < 1 || in.Digit > 9 || g.Cells[c] != 0 {
		return
	}
	if in.Note {
		g.save()
		g.Notes[c] ^= 1 << in.Digit
		return
	}
	if g.puzzle.Solution[c] != in.Digit {
		g.Mistakes++
		g.Lives--
		g.Combo = 0
		g.event(Event{Kind: Wrong, Cell: c, Digit: in.Digit})
		if g.Lives <= 0 {
			g.Lost = true
			g.event(Event{Kind: LostEvent})
		}
		return
	}
	g.save()
	now := g.now()
	if !g.lastAt.IsZero() && now.Sub(g.lastAt) <= comboWindow {
		g.Combo++
	} else {
		g.Combo = 1
	}
	g.lastAt = now
	points := 50 * g.Combo
	g.Score += points
	g.put(c, in.Digit, Event{Kind: Placed, Cell: c, Digit: in.Digit, Combo: g.Combo, Points: points})
}

// put sets digit in cell, clears it from its peers' notes, and tells of
// it with e, and of the groups and digits it finishes.
func (g *game) put(c int, d int8, e Event) {
	g.Cells[c] = d
	g.Notes[c] = 0
	for _, p := range peers[c] {
		g.Notes[p] &^= 1 << d
	}
	g.event(e)
	for _, u := range unitsOf[c] {
		if g.full(u) {
			g.Score += 200
			g.event(Event{Kind: Done, Cell: c, Unit: u, Points: 200})
		}
	}
	if g.count(d) == 9 {
		g.event(Event{Kind: DigitDone, Digit: d})
	}
	if g.Cells == g.puzzle.Solution {
		g.Won = true
		g.Stars = g.stars()
		g.event(Event{Kind: WonEvent})
	}
}

// full reports whether unit u is filled.
func (g *game) full(u int) bool {
	for _, c := range units[u] {
		if g.Cells[c] == 0 {
			return false
		}
	}
	return true
}

// count returns how many of digit d the board holds.
func (g *game) count(d int8) int {
	n := 0
	for _, v := range g.Cells {
		if v == d {
			n++
		}
	}
	return n
}

// stars rates a won game: three with no mistakes and a hint at most,
// two with one mistake or a few hints, one otherwise.
func (g *game) stars() int {
	switch {
	case g.Mistakes == 0 && g.Hints <= 1:
		return 3
	case g.Mistakes <= 1 && g.Hints <= 3:
		return 2
	}
	return 1
}

// erase clears a cell's notes.
func (g *game) erase(c int) {
	if g.over() || c < 0 || c >= 81 || g.Notes[c] == 0 {
		return
	}
	g.save()
	g.Notes[c] = 0
}

// undoMove takes back the last move.
func (g *game) undoMove() {
	if g.over() || len(g.undo) == 0 {
		return
	}
	s := g.undo[len(g.undo)-1]
	g.undo = g.undo[:len(g.undo)-1]
	g.Cells, g.Notes = s.cells, s.notes
	g.Combo = 0
}

// hint reveals cell c, when it is empty, or else the empty cell with
// fewest candidates.
func (g *game) hint(c int) {
	if g.over() {
		return
	}
	if c < 0 || c >= 81 || g.Cells[c] != 0 {
		c = -1
		fewest := 10
		for i := range 81 {
			if g.Cells[i] != 0 {
				continue
			}
			if n := bits.OnesCount16(g.Cells.candidates(i)); n < fewest {
				c, fewest = i, n
			}
		}
		if c < 0 {
			return
		}
	}
	g.save()
	g.Hints++
	g.Combo = 0
	d := g.puzzle.Solution[c]
	g.put(c, d, Event{Kind: Hinted, Cell: c, Digit: d})
}
