// Command calculator is a calculator with a graph, drawn in a window of
// its own making, to show what gunim's animation does.
//
// The window has no system title bar: its own holds the switch between
// the calculator and the graph, the title, which moves the window, and
// the window's buttons. It grows in as it opens.
//
// On the calculator, keys squash and spring back with a ripple from
// where they were pressed, typed characters roll into the display, a
// sum with no answer shakes it, and a sum worked out flies in an arc to
// the tape, whose rows spring down to make room.
//
// A sum with no answer also sends a red echo out from the window's
// edges, onto the desktop around it, like a sonar's ping; a curve kept
// on the graph sends a green one.
//
// On the graph, curves draw themselves on; the one being typed morphs
// as each key goes in, and flares as it is kept. The grid thickens and
// thins as the wheel zooms about the pointer; a drag pans and a flick
// coasts to a stop; a double click springs home. A dot traces the curve
// under the pointer, on a spring. Switching, the graph grows out of the
// display as the keypad slides away.
//
//	CGO_ENABLED=0 go run ./example/calculator
//
// Keys typed at the keyboard work too: digits and signs, s c t r l for
// sin cos tan √ ln, p for π, Enter for =, Backspace, Escape to clear,
// and Tab to switch. -type types keys itself, one at a time, once the
// window has opened: -type "1÷0=" shows the red echo.
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
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
)

func main() {
	runFor := flag.Duration("for", 0, "quit after this long; zero runs until the window closes")
	keys := flag.String("keys", "", "keys to press as the window opens, such as \"12×3=\" or \"sin(x)=\"")
	typed := flag.String("type", "", "keys to type one at a time once the window has opened, to watch them go in")
	graph := flag.Bool("graph", false, "open on the graph")
	shot := flag.String("shot", "", "write the window to this PNG file after -after, and quit")
	after := flag.Duration("after", 1500*time.Millisecond, "how long -shot waits")
	iconTo := flag.String("icon", "", "write the icon, 256 pixels square, to this PNG file, and quit")
	flag.Parse()
	if *iconTo != "" {
		if err := writeIcon(*iconTo); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := run(*runFor, *keys, *typed, *graph, *shot, *after); err != nil {
		log.Fatal(err)
	}
}

func run(runFor time.Duration, keys, typed string, graph bool, shot string, after time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, runFor)
		defer cancel()
	}
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{
			Title: "Calculator",
			Size:  geom.Sz(980, 660),
			Icons: icons(),
		})
		if err != nil {
			return fmt.Errorf("calculator: %w", err)
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
		return serve(ctx, c, Calc{Graph: graph}, keys, typed)
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

// writeIcon writes the icon, at its largest, to a PNG file.
func writeIcon(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return errors.Join(png.Encode(f, drawIcon(256)), f.Close())
}
