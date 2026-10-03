// Command sudoku is a sudoku of candies, to show how far gunim's
// animation goes: candies drop in and wobble like jelly, wrong ones
// shake and crumble, finished rows sweep with light, combos burst with
// stars, and a won board rains confetti.
//
//	go run ./example/sudoku
//
// It is made for a phone held upright, and lays itself out beside the
// board on a wide window. Tap a cell, then a candy; or tap a candy
// first, then each cell it goes in. Keys: the arrows move, 1 to 9
// place, Shift with a digit pencils a note, N switches notes on, Z
// undoes, H hints.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/speaker"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
)

func main() {
	level := flag.Int("level", 0, "a level to play at once, rather than open on the map")
	reset := flag.Bool("reset", false, "forget the levels won, and start over")
	saveIn := flag.String("progress", configDir(), "the folder the levels won are kept in")
	runFor := flag.Duration("for", 0, "quit after this long; zero runs until the window closes")
	shot := flag.String("shot", "", "write the window to this PNG file after -after, and quit")
	after := flag.Duration("after", 2*time.Second, "how long -shot waits")
	size := flag.String("size", "460x860", "the window's size, as 1100x760 for a wide one")
	mute := flag.Bool("mute", false, "start with the sound off")
	flag.Parse()
	var w, h float32
	if _, err := fmt.Sscanf(*size, "%gx%g", &w, &h); err != nil || w <= 0 || h <= 0 {
		log.Fatalf("sudoku: -size %q: want a width and a height, as 460x860", *size)
	}
	if *reset {
		if err := forget(*saveIn); err != nil {
			log.Fatal(err)
		}
	}
	if err := run(*level, *saveIn, *runFor, *shot, *after, geom.Sz(w, h), *mute); err != nil {
		log.Fatal(err)
	}
}

func run(level int, saveIn string, runFor time.Duration, shot string, after time.Duration, size geom.Size, mute bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, runFor)
		defer cancel()
	}
	mix := audio.NewMixer()
	if _, err := speaker.Open(mix, speaker.Options{Name: "Candy Sudoku"}); err != nil {
		log.Printf("sudoku: no sound: %v", err)
		mix = nil
	}
	s := newSFX(mix)
	if mute {
		s.setMuted(true)
	}
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{Title: "Candy Sudoku", Size: size})
		if err != nil {
			return fmt.Errorf("sudoku: %w", err)
		}
		registerViews(w, s)
		c := w.Client()
		if shot != "" {
			go func() {
				select {
				case <-time.After(after):
				case <-ctx.Done():
					return
				}
				if err := writeShot(ctx, c, shot); err != nil {
					log.Print(err)
				}
				c.Close()
			}()
		}
		return serve(ctx, c, level, saveIn)
	})
	if errors.Is(err, driver.ErrNoDriver) {
		log.Print("gunim has no driver for this operating system yet")
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// writeShot writes what the window shows to a PNG file.
func writeShot(ctx context.Context, c gunim.Client, path string) error {
	img, err := c.Shot(ctx)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return errors.Join(png.Encode(f, img), f.Close())
}
