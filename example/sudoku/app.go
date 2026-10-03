package main

import (
	"context"
	"time"

	"github.com/marrasen/gunim"
)

// gameTopic is what the game view watches.
const gameTopic = "game"

// serve plays the game: it keeps the game's state and hears what the
// window sends.
func serve(ctx context.Context, c gunim.Client, level int) error {
	round := 1
	g := newGame(level, time.Now)
	g.Round = round
	if err := c.Mount(gunim.Root, "game", "game", g.Game, gameTopic); err != nil {
		return err
	}
	_ = c.Focus("game")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-c.Intents():
			if !ok {
				return c.Err()
			}
			switch in := ev.Intent.(type) {
			case Place:
				g.place(in)
			case Erase:
				g.erase(in.Cell)
			case Undo:
				g.undoMove()
			case Hint:
				g.hint(in.Cell)
			case Start:
				round++
				g = newGame(max(in.Level, 1), time.Now)
				g.Round = round
			default:
				continue
			}
			_ = c.Publish(gameTopic, g.Game)
		}
	}
}
