// Command vectorart shows package shape: a fox drawn from an SVG file, as a vector editor saves one, large and
// small, and a leaf from a line of path data, filled and stroked, swaying by a transform so its masks are drawn
// only once.
//
//	CGO_ENABLED=0 go run ./example/vectorart
package main

import (
	"context"
	_ "embed"
	"errors"
	"flag"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"os/signal"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/shape"
)

//go:embed fox.svg
var foxSVG []byte

// fox is read once, when the program starts; drawing it costs a quad for each of its parts.
var fox = shape.MustSVG(foxSVG)

// leaf is an oak leaf on a 24-unit grid, its vein a line down the middle.
var (
	leaf = shape.MustPath("M12 1C14 3 16 3 15.5 5.5C18 5 19 7 17 9C20 9 21 11 18.5 13C21 14 20 16 17.5 16" +
		"C18 18.5 16 19 14 18C14 20 13 21 12.5 22L11.5 22C11 21 10 20 10 18C8 19 6 18.5 6.5 16C4 16 3 14 5.5 13" +
		"C3 11 4 9 7 9C5 7 6 5 8.5 5.5C8 3 10 3 12 1Z")
	vein    = shape.MustPath("M12 3V23")
	leafBox = geom.Rc(0, 0, 24, 24)
)

func main() {
	runFor := flag.Duration("for", 0, "quit after this long; zero runs until the window closes")
	shot := flag.String("shot", "", "write the window to this PNG file after -after, and quit")
	after := flag.Duration("after", time.Second, "how long -shot waits")
	flag.Parse()
	if err := run(*runFor, *shot, *after); err != nil {
		log.Fatal(err)
	}
}

func run(runFor time.Duration, shot string, after time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, runFor)
		defer cancel()
	}
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{Title: "gunim vector art", Size: geom.Sz(720, 420), Root: &art{}})
		if err != nil {
			return err
		}
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
		for range c.Intents() {
		}
		return c.Err()
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

// art draws the fox and the leaf, the leaf swaying.
type art struct{ t float64 }

// Step implements [gunim.Animator]: the leaf sways for ever.
func (a *art) Step(dt time.Duration) bool {
	a.t += dt.Seconds()
	return true
}

// Layout implements [gunim.Node].
func (a *art) Layout(cs gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size { return cs.Max }

var (
	meadow    = color.NRGBA{R: 0xe8, G: 0xf4, B: 0xdc, A: 0xff}
	leafGreen = color.NRGBA{R: 0x5c, G: 0xa0, B: 0x3c, A: 0xff}
	leafDark  = color.NRGBA{R: 0x2c, G: 0x6c, B: 0x24, A: 0xff}
)

// Paint implements [gunim.Node].
func (a *art) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(meadow))
	fox.Paint(p, shape.Fit(fox.ViewBox, geom.Rc(20, 20, 380, 380)))
	fox.Paint(p, shape.Fit(fox.ViewBox, geom.Rc(420, 300, 100, 100)))
	// The leaf turns about its stem by a transform: its masks stay the size they were drawn at.
	r := geom.Rc(470, 30, 220, 220)
	stem := geom.Pt(r.Center().X, r.Max.Y)
	defer p.Push(paint.Rotate(float32(0.15*math.Sin(a.t*1.6)), stem))()
	p.Mask(leaf.Fill(), leaf.Fill().In(leafBox, r), leafGreen)
	p.Mask(leaf.Stroke(0.5), leaf.Stroke(0.5).In(leafBox, r), leafDark)
	p.Mask(vein.Stroke(0.6), vein.Stroke(0.6).In(leafBox, r), leafDark)
}
