package main

import (
	"context"
	"log"
	"time"

	"github.com/marrasen/gunim"
)

// The world the window shows: the map, or a game, and the progress.
type (
	World struct {
		// Map says the map shows, rather than the game.
		Map bool
		// Stars holds the stars won on each level, from level 1, and
		// Unlocked the highest level open.
		Stars    []int
		Unlocked int
		// Unlock is the latest level opened by a win, numbered so the
		// map plays its opening once.
		Unlock Unlock
		Game   Game
	}
	// Unlock is a level opened.
	Unlock struct{ ID, Level int }

	// ShowMap goes to the map.
	ShowMap struct{}
)

// worldTopic is what the world view watches.
const worldTopic = "world"

// app is the application half: the progress, and the game played.
type app struct {
	World
	g     *game
	prog  progress
	dir   string
	round int
}

// serve keeps the world and hears what the window sends. A level of
// zero opens on the map.
func serve(ctx context.Context, c gunim.Client, level int, dir string) error {
	a := &app{dir: dir, prog: loadProgress(dir)}
	a.Map = true
	if level > 0 {
		a.prog.Unlocked = max(a.prog.Unlocked, min(level, levelCount))
		a.start(level)
	}
	a.refresh()
	if err := c.Mount(gunim.Root, "world", "world", a.World, worldTopic); err != nil {
		return err
	}
	_ = c.Focus("world")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-c.Intents():
			if !ok {
				return c.Err()
			}
			if !a.handle(ev.Intent) {
				continue
			}
			a.refresh()
			a.save()
			_ = c.Publish(worldTopic, a.World)
		}
	}
}

// handle carries out an intent, and reports whether it was one.
func (a *app) handle(in gunim.Intent) bool {
	switch in := in.(type) {
	case Start:
		a.start(in.Level)
	case ShowMap:
		a.Map = true
	case Place, Erase, Undo, Hint:
		if a.g == nil || a.Map {
			return false
		}
		won := a.g.Won
		switch in := in.(type) {
		case Place:
			a.g.place(in)
		case Erase:
			a.g.erase(in.Cell)
		case Undo:
			a.g.undoMove()
		case Hint:
			a.g.hint(in.Cell)
		}
		if a.g.Won && !won {
			a.won()
		}
	default:
		return false
	}
	return true
}

// start plays level, picking up where it was left if it was.
func (a *app) start(level int) {
	level = max(1, min(level, levelCount))
	if level > a.prog.Unlocked {
		return
	}
	a.round++
	a.g = newGame(level, time.Now)
	a.g.Round = a.round
	if cur := a.prog.Current; cur != nil && cur.Level == level {
		a.g.restore(cur)
	}
	a.Map = false
}

// won records the stars of the game just won, and opens the next level.
func (a *app) won() {
	l := a.g.Level
	a.prog.Stars[l-1] = max(a.prog.Stars[l-1], a.g.Stars)
	if l == a.prog.Unlocked && l < levelCount {
		a.prog.Unlocked = l + 1
		a.Unlock = Unlock{ID: a.Unlock.ID + 1, Level: l + 1}
	}
}

// refresh copies the progress and the game into what the window sees.
func (a *app) refresh() {
	a.Stars = append(a.Stars[:0:0], a.prog.Stars...)
	a.Unlocked = a.prog.Unlocked
	if a.g != nil {
		a.Game = a.g.Game
	}
}

// save keeps the progress, and the game while it goes on.
func (a *app) save() {
	a.prog.Current = nil
	if a.g != nil && !a.g.over() {
		a.prog.Current = a.g.saved()
	}
	if err := a.prog.save(a.dir); err != nil {
		log.Printf("sudoku: %v", err)
	}
}
