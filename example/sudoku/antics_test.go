package main

import (
	"time"

	"testing"

	"github.com/marrasen/gunim/geom"
)

func TestANewLevelsCandiesRunInAndLeapToTheirCells(t *testing.T) {
	g, _ := newTestGame()
	g.Round = 1
	_, root, run := stage(t, geom.Sz(460, 860), g.Game)
	b := root.board
	s := root.cell()
	var last [81]geom.Point
	var seen, was [81]bool
	for f := range 360 {
		for c := range 81 {
			k := &b.cells[c]
			if was[c] && !k.entering {
				// It landed: last seen at its cell.
				if d := dist(last[c], b.cellMid(c)); d > s*0.5 {
					t.Fatalf("frame %d: cell %d's candy landed %v from its cell", f, c, d)
				}
			}
			was[c] = k.entering
			if !k.entering || k.enterAt < 0 {
				continue
			}
			at, _, _, _ := b.enterPose(c, k.enterAt)
			if !seen[c] {
				if x := at.X + root.boardAt.X; x > 0 && x < 460 {
					t.Fatalf("frame %d: cell %d's candy first shows at x %v, inside the window", f, c, x)
				}
			} else if d := dist(last[c], at); d > s*0.5 {
				t.Fatalf("frame %d: cell %d's candy jumped %v in a frame", f, c, d)
			}
			last[c], seen[c] = at, true
		}
		run(1)
	}
	for c := range 81 {
		if g.Cells[c] == 0 {
			continue
		}
		k := &b.cells[c]
		if !seen[c] || k.entering {
			t.Fatalf("six seconds in, cell %d's candy: seen %v, still coming %v", c, seen[c], k.entering)
		}
		if v := k.pop.Value(); v < 0.99 || v > 1.01 {
			t.Fatalf("six seconds in, cell %d's candy still wobbles at %v", c, v)
		}
	}
}

func TestPickingACandyCheersItsDigitAndLightsWhatItRulesOut(t *testing.T) {
	g, _ := newTestGame()
	g.Round = 1
	w, root, run := stage(t, geom.Sz(460, 860), g.Game)
	run(300)
	b := root.board
	tapAt(w, run, root.trayCenter(5))
	var fives []int
	for c := range 81 {
		if g.Cells[c] == 5 {
			fives = append(fives, c)
		}
	}
	if len(fives) == 0 {
		t.Fatal("the test's puzzle holds no 5")
	}
	var lit [81]float32
	var hops, grows [81]float32
	cheered := map[int]bool{}
	for f := range 75 {
		var light [81]float32
		b.cheerLight(&light)
		for c, a := range light {
			lit[c] = max(lit[c], a)
		}
		for _, c := range fives {
			k := &b.cells[c]
			if !k.cheering {
				continue
			}
			cheered[c] = true
			hop, grow, _, _ := cheerPose(k.cheerAt)
			if d := max(abs32(hop-hops[c]), abs32(grow-grows[c])); d > 0.08 {
				t.Fatalf("frame %d: cell %d's candy hopped or grew by %v of a cell in a frame", f, c, d)
			}
			hops[c], grows[c] = hop, grow
		}
		run(1)
	}
	for _, c := range fives {
		if !cheered[c] {
			t.Fatalf("the 5 in cell %d never cheered", c)
		}
		if b.cells[c].cheering {
			t.Fatalf("a second and a quarter on, the 5 in cell %d still cheers", c)
		}
		for i := range 9 {
			for _, o := range []int{c/9*9 + i, i*9 + c%9} {
				if lit[o] < 0.5 {
					t.Fatalf("cell %d, in line with the 5 in cell %d, lit only %v", o, c, lit[o])
				}
			}
		}
	}
	for o := range 81 {
		inLine := false
		for _, c := range fives {
			inLine = inLine || o/9 == c/9 || o%9 == c%9
		}
		if !inLine && lit[o] > 0.05 {
			t.Fatalf("cell %d, in line with no 5, lit %v", o, lit[o])
		}
	}
}

// winStaged stages a game and wins it, and returns the root and a way
// to step frames.
func winStaged(t *testing.T) (*gameRoot, func(int)) {
	t.Helper()
	g, clk := newTestGame()
	g.Round = 1
	w, root, run := stage(t, geom.Sz(460, 860), g.Game)
	run(300)
	for c := range 81 {
		if g.Cells[c] == 0 {
			g.place(Place{Cell: c, Digit: g.puzzle.Solution[c]})
			clk.t = clk.t.Add(time.Second)
		}
	}
	if err := w.Client().Publish(gameTopic, g.Game); err != nil {
		t.Fatal(err)
	}
	run(1)
	return root, run
}

func eatenCount(b *board) int {
	n := 0
	for c := range b.cells {
		if b.cells[c].eaten {
			n++
		}
	}
	return n
}

func TestPacManEatsAWonBoard(t *testing.T) {
	root, run := winStaged(t)
	b := root.board
	s := root.cell()
	eaten, out := 0, false
	var last geom.Point
	for f := range 60 * 30 {
		run(1)
		n := eatenCount(b)
		if n < eaten || n > eaten+2 {
			t.Fatalf("frame %d: %d candies eaten after %d", f, n, eaten)
		}
		eaten = n
		if !b.pac.on || b.pac.wait > 0 {
			if out {
				break
			}
			continue
		}
		at, _ := pathAt(b.pacPath(), b.pac.dist)
		if out {
			if d := dist(last, at); d > pacSpeed*s/60*1.5 {
				t.Fatalf("frame %d: Pac-Man jumped %v in a frame", f, d)
			}
		} else if x := at.X + root.boardAt.X; x > 0 {
			t.Fatalf("Pac-Man first shows at x %v, inside the window", x)
		}
		last, out = at, true
	}
	if !out || b.pac.on {
		t.Fatalf("thirty seconds after the win: Pac-Man came out %v, is out still %v", out, b.pac.on)
	}
	if eaten != 81 {
		t.Fatalf("Pac-Man ate %d candies of 81", eaten)
	}
	if !root.card.shown {
		t.Fatal("the card went away as Pac-Man ate")
	}
}

func TestAHintsCandyRunsInAndLightsItsCellAsItLands(t *testing.T) {
	g, _ := newTestGame()
	g.Round = 1
	w, root, run := stage(t, geom.Sz(460, 860), g.Game)
	run(300)
	b := root.board
	s := root.cell()
	c := emptyCell(g)
	g.hint(c)
	if err := w.Client().Publish(gameTopic, g.Game); err != nil {
		t.Fatal(err)
	}
	k := &b.cells[c]
	var last geom.Point
	seen, landed := false, false
	for f := range 120 {
		run(1)
		if k.entering {
			at, _, _, _ := b.enterPose(c, k.enterAt)
			if !seen {
				if x := at.X + root.boardAt.X; x > 0 && x < 460 {
					t.Fatalf("the hint's candy first shows at x %v, inside the window", x)
				}
			} else if d := dist(last, at); d > s*0.5 {
				t.Fatalf("frame %d: the hint's candy jumped %v", f, d)
			}
			if gl := k.glow.Value(); gl > 0 {
				t.Fatalf("frame %d: the cell lights %v before the candy lands", f, gl)
			}
			last, seen = at, true
			continue
		}
		if !seen {
			t.Fatal("the hint's candy never ran in")
		}
		if !landed {
			landed = true
			if d := dist(last, b.cellMid(c)); d > s*0.5 {
				t.Fatalf("the hint's candy landed %v from its cell", d)
			}
			if k.glow.Value() < 0.9 {
				t.Fatalf("the hint's candy landed and the cell lights only %v", k.glow.Value())
			}
		}
	}
	if !landed {
		t.Fatal("two seconds after the hint, its candy has not landed")
	}
}

func TestContinueGoesToTheMapAndPacManStops(t *testing.T) {
	dir := t.TempDir()
	a := &app{dir: dir, prog: loadProgress(dir)}
	a.handle(Start{Level: 1})
	a.refresh()
	w, root, run := stageWorld(t, geom.Sz(460, 860), a.World)
	publish := func() {
		a.refresh()
		if err := w.Client().Publish(worldTopic, a.World); err != nil {
			t.Fatal(err)
		}
	}
	run(300)
	winLevel(a)
	publish()
	run(60 * 5)
	b := root.game.board
	eaten := eatenCount(b)
	if !b.pac.on || eaten == 0 {
		t.Fatalf("five seconds after the win, Pac-Man is out %v and has eaten %d", b.pac.on, eaten)
	}
	bt := root.game.card.button()
	tapAt(w, run, geom.Pt(bt.Min.X+bt.Size().W/2, bt.Min.Y+bt.Size().H/2))
	got := intents(w)
	if len(got) != 1 || got[0] != (ShowMap{}) {
		t.Fatalf("Continue sent %v, want the map", got)
	}
	a.handle(got[0])
	publish()
	run(60)
	if b.pac.on {
		t.Fatal("a second after Continue, Pac-Man is still out")
	}
	if n := eatenCount(b); n > eaten+1 {
		t.Fatalf("Pac-Man ate on to %d candies, from %d, as the map came up", n, eaten)
	}
}

func abs32(x float32) float32 { return max(x, -x) }

// cellMid returns the middle of cell c, in the board's space.
func (b *board) cellMid(c int) geom.Point {
	r := cellRect(c, b.size.W)
	return geom.Pt((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
}
