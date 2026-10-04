package main

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// winLevel plays a's game through to its win, with no mistakes.
func winLevel(a *app) {
	for c := range 81 {
		if a.g.Cells[c] == 0 {
			a.handle(Place{Cell: c, Digit: a.g.puzzle.Solution[c]})
		}
	}
}

func TestWinningALevelOpensTheNextAndKeepsTheStars(t *testing.T) {
	dir := t.TempDir()
	a := &app{dir: dir, prog: loadProgress(dir)}
	a.handle(Start{Level: 2})
	if a.g != nil {
		t.Fatal("a locked level started")
	}
	a.handle(Start{Level: 1})
	winLevel(a)
	if a.prog.Unlocked != 2 || a.prog.Stars[0] != 3 || a.Unlock != (Unlock{ID: 1, Level: 2}) {
		t.Fatalf("won level 1: unlocked %d, stars %d, unlock %+v", a.prog.Unlocked, a.prog.Stars[0], a.Unlock)
	}
	a.save()
	b := loadProgress(dir)
	if b.Unlocked != 2 || b.Stars[0] != 3 || b.Current != nil {
		t.Fatalf("read back: unlocked %d, stars %d, current %v", b.Unlocked, b.Stars[0], b.Current)
	}
}

func TestAGameLeftHalfPlayedPicksUpWhereItWas(t *testing.T) {
	dir := t.TempDir()
	a := &app{dir: dir, prog: loadProgress(dir)}
	a.handle(Start{Level: 1})
	c := emptyCell(a.g)
	a.handle(Place{Cell: c, Digit: a.g.puzzle.Solution[c]})
	a.handle(Place{Cell: emptyCell(a.g), Digit: a.g.puzzle.Solution[emptyCell(a.g)]%9 + 1})
	a.save()
	b := &app{dir: dir, prog: loadProgress(dir)}
	b.handle(Start{Level: 1})
	if b.g.Cells[c] == 0 || b.g.Lives != 2 || b.g.Score != a.g.Score {
		t.Fatalf("picked up: cell %d holds %d, lives %d, score %d; want the candy, 2 lives, score %d",
			c, b.g.Cells[c], b.g.Lives, b.g.Score, a.g.Score)
	}
}

// stageWorld mounts the world view in an offscreen window of size.
func stageWorld(t *testing.T, size geom.Size, s World) (*gunim.Window, *worldRoot, func(int)) {
	t.Helper()
	var root *worldRoot
	w := gunim.NewOffscreen(size, nil)
	gunim.RegisterView(w, "world",
		func(World) *worldRoot { root = newWorldRoot(nil); return root },
		func(r *worldRoot, s World, u *gunim.UI) { r.show(s, u) })
	if err := w.Client().Mount(gunim.Root, "world", "world", s, worldTopic); err != nil {
		t.Fatal(err)
	}
	// As serve does, the keys go to the world.
	_ = w.Client().Focus("world")
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(2)
	return w, root, run
}

func TestATapOnAnOpenLevelPlaysItAndALockedOneDoesNot(t *testing.T) {
	dir := t.TempDir()
	a := &app{dir: dir, prog: loadProgress(dir)}
	a.prog.Unlocked = 3
	a.Map = true
	a.refresh()
	w, root, run := stageWorld(t, geom.Sz(460, 860), a.World)
	tapAt(w, run, root.m.nodeOnScreen(4))
	tapAt(w, run, root.m.nodeOnScreen(3))
	got := intents(w)
	if len(got) != 1 || got[0] != (Start{Level: 3}) {
		t.Fatalf("taps on locked 4 and open 3 sent %v, want Start 3", got)
	}
}

func TestTheMapZoomsIntoTheGameAndBackOut(t *testing.T) {
	dir := t.TempDir()
	a := &app{dir: dir, prog: loadProgress(dir)}
	a.Map = true
	a.refresh()
	w, root, run := stageWorld(t, geom.Sz(460, 860), a.World)
	publish := func() {
		a.refresh()
		if err := w.Client().Publish(worldTopic, a.World); err != nil {
			t.Fatal(err)
		}
	}
	a.handle(Start{Level: 1})
	publish()
	run(6)
	if v := root.onMap.Value(); v <= 0.05 || v >= 0.95 {
		t.Fatalf("a tenth of a second into the zoom, the map is %v shown", v)
	}
	run(90)
	if root.onMap.Value() > 0.01 {
		t.Fatal("a second and a half on, the map still shows")
	}
	// The game takes the taps now, and the map none.
	c := emptyCell(a.g)
	tapAt(w, run, root.game.cellCenter(c))
	if root.game.selected != c {
		t.Fatalf("a tap on cell %d in the game selected %d", c, root.game.selected)
	}
	// Winning, the card's Continue goes back to the map, where level 2
	// opens and the heart hops to it.
	winLevel(a)
	publish()
	run(240)
	b := root.game.card.button()
	tapAt(w, run, geom.Pt(b.Min.X+b.Size().W/2, b.Min.Y+b.Size().H/2))
	for _, in := range intents(w) {
		a.handle(in)
	}
	publish()
	run(180)
	if root.onMap.Value() < 0.99 {
		t.Fatal("three seconds after Continue, the map does not show")
	}
	if v := root.m.token.Value(); v != 2 {
		t.Fatalf("three seconds after Continue, the heart is at level %v, want it hopped to 2", v)
	}
	if root.m.open.Value() < 0.99 {
		t.Fatal("level 2 has not opened")
	}
}

func TestEscapeInAGameGoesToTheMap(t *testing.T) {
	dir := t.TempDir()
	a := &app{dir: dir, prog: loadProgress(dir)}
	a.handle(Start{Level: 1})
	a.refresh()
	w, _, run := stageWorld(t, geom.Sz(460, 860), a.World)
	w.Input(input.KeyPress{Key: input.KeyEscape})
	run(1)
	if got := intents(w); len(got) != 1 || got[0] != (ShowMap{}) {
		t.Fatalf("Escape in a game sent %v, want the map", got)
	}
}

func TestTheSoundButtonStepsThroughItsSettingsAndKeepsThem(t *testing.T) {
	dir := t.TempDir()
	a := &app{dir: dir, prog: loadProgress(dir)}
	a.handle(Start{Level: 1})
	a.refresh()
	w, root, run := stageWorld(t, geom.Sz(460, 860), a.World)
	run(60)
	var button geom.Rect
	gunim.RegisterPatch(w, "world", func(r *worldRoot, _ struct{}, u *gunim.UI) {
		b, _ := u.Bounds(r.game.tools)
		button = r.game.tools.slot(4).Add(b.Min)
	})
	if err := w.Client().Patch("world", struct{}{}); err != nil {
		t.Fatal(err)
	}
	run(1)
	at := geom.Pt(button.Min.X+button.Size().W/2, button.Min.Y+20)
	for _, want := range []Sound{SoundMusic, SoundEffects, SoundOff, SoundAll} {
		tapAt(w, run, at)
		if root.game.sfx.mode != want {
			t.Fatalf("a tap on Sound set %v, want %v", root.game.sfx.mode, want)
		}
		for _, in := range intents(w) {
			a.handle(in)
		}
	}
	a.save()
	if p := loadProgress(dir); p.Sound != SoundAll {
		t.Fatalf("kept %v, want the last set, %v", p.Sound, SoundAll)
	}
}

func TestAHiddenWindowStopsTheSound(t *testing.T) {
	dir := t.TempDir()
	a := &app{dir: dir, prog: loadProgress(dir)}
	a.Map = true
	a.refresh()
	w, root, run := stageWorld(t, geom.Sz(460, 860), a.World)
	w.Input(driver.WindowShown{Shown: false})
	run(1)
	if !root.game.sfx.away {
		t.Fatal("hidden, the sound plays on")
	}
	w.Input(driver.WindowShown{Shown: true})
	run(1)
	if root.game.sfx.away {
		t.Fatal("shown again, the sound stays off")
	}
}
