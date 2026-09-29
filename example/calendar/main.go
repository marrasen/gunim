// Command calendar is a calendar built on gunim, with a simulated back end: a few months of events around today in
// four calendars, colleagues who send invitations, and events that repeat.
//
//	CGO_ENABLED=0 go run ./example/calendar
package main

import (
	"context"
	"errors"
	"flag"
	"image/png"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/widget"
)

func main() {
	seed := flag.Uint64("seed", 1, "the seed for the simulated events")
	runFor := flag.Duration("for", 0, "quit after this long; zero runs until the window closes")
	shot := flag.String("shot", "", "write the window to this PNG file after -after, and quit")
	after := flag.Duration("after", 2*time.Second, "how long -shot waits")
	do := flag.String("do", "", "a script of steps separated by semicolons to run at the start; see runScript")
	flag.Parse()
	var script []string
	if *do != "" {
		script = strings.Split(*do, ";")
	}
	if err := run(*seed, script, *runFor, *shot, *after); err != nil {
		log.Fatal(err)
	}
}

func run(seed uint64, script []string, runFor time.Duration, shot string, after time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, runFor)
		defer cancel()
	}
	return gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{Title: "Calendar", Size: geom.Sz(1280, 800), Root: widget.NewSurface()})
		if err != nil {
			return err
		}
		registerViews(w)
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
		app := newApp(ctx, c, seed, time.Now())
		app.runScript(script)
		return app.serve()
	})
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
