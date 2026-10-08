// Command compass shows a compass needle that points to magnetic north as the phone turns, and the heading under
// it: a [gunim.HeadingWatcher] at its smallest. Where the device has no compass, as a desktop, it says so.
//
//	go run ./tools/gunimapk -run -name Compass ./example/compass
package main

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"os/signal"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/shape"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/widget"
)

// The needle on a 24-unit grid: its north half and its south half.
const (
	northPath = "M12 1L15.5 12L8.5 12Z"
	southPath = "M12 23L15.5 12L8.5 12Z"
)

var needleBox = geom.Rc(0, 0, 24, 24)

var (
	dial  = color.NRGBA{R: 0xf4, G: 0xee, B: 0xe0, A: 0xff}
	rim   = color.NRGBA{R: 0x6b, G: 0x5a, B: 0x48, A: 0xff}
	red   = color.NRGBA{R: 0xd0, G: 0x3c, B: 0x30, A: 0xff}
	slate = color.NRGBA{R: 0x7a, G: 0x84, B: 0x90, A: 0xff}
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	n, err := shape.NewPath(northPath)
	if err != nil {
		return err
	}
	s, err := shape.NewPath(southPath)
	if err != nil {
		return err
	}
	err = gunim.Main(ctx, func(a *gunim.App) error {
		w, werr := a.NewWindow(gunim.WindowOptions{Title: "gunim compass", Size: geom.Sz(360, 480), Root: widget.NewSurface()})
		if werr != nil {
			return werr
		}
		gunim.RegisterView(w, "rose", func(struct{}) *rose { return newRose(n, s) }, nil)
		c := w.Client()
		if merr := c.Mount(gunim.Root, "rose", "rose", struct{}{}); merr != nil {
			return merr
		}
		for range c.Intents() {
		}
		return c.Err()
	})
	if errors.Is(err, driver.ErrNoDriver) {
		log.Print("gunim has no driver for this operating system yet")
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// rose is a compass needle over a dial, with the heading in a label under it.
type rose struct {
	north, south *shape.Path
	label        *widget.Label
	// heading is the last reading, and heard says there has been one.
	heading input.Heading
	heard   bool
}

// newRose returns a rose with the needle's two halves.
func newRose(north, south *shape.Path) *rose {
	l := widget.NewLabel("")
	l.Align = text.AlignCenter
	return &rose{north: north, south: south, label: l}
}

// Children implements [gunim.Composite]: the label.
func (r *rose) Children() []gunim.Node { return []gunim.Node{r.label} }

// WatchesHeading implements [gunim.HeadingWatcher]: the rose watches for as long as it is shown.
func (r *rose) WatchesHeading() bool { return true }

// Handle implements [gunim.Handler]: each reading turns the needle.
func (r *rose) Handle(e input.Event, _ *gunim.UI) bool {
	h, ok := e.(input.Heading)
	if !ok {
		return false
	}
	r.heading, r.heard = h, true
	return true
}

// Layout implements [gunim.Node]: the label sits under the dial, and says what there is to say.
func (r *rose) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	r.label.Text = r.say(f.UI())
	box := c.Max
	for kid := range kids.All {
		kid.Layout(gunim.Tight(geom.Sz(box.W-32, 60)))
		kid.Place(geom.Pt(16, r.dial(box).Max.Y+24))
	}
	return box
}

// say returns what the label says.
func (r *rose) say(u *gunim.UI) string {
	switch {
	case !u.HasCompass():
		return "This device has no compass."
	case !r.heard:
		return "Waiting for the compass…"
	case r.heading.Accuracy <= input.HeadingLow:
		return fmt.Sprintf("%d°: unsure. Move the phone in a figure eight.", degrees(r.heading))
	default:
		return fmt.Sprintf("%d°", degrees(r.heading))
	}
}

// degrees returns the heading in whole degrees, from 0 to 359: a heading a hair below 360 shows as 0, as north.
func degrees(h input.Heading) int {
	return int(math.Round(float64(h.Degrees))) % 360
}

// dial returns where the dial is drawn in box.
func (r *rose) dial(box geom.Size) geom.Rect {
	d := min(box.W, box.H) - 64
	return geom.Rc((box.W-d)/2, 32, d, d)
}

// Paint implements [gunim.Node].
func (r *rose) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	d := r.dial(box)
	p.RRect(d, d.Size().W/2, paint.Solid(rim))
	p.RRect(d.Inset(geom.Uniform(6)), d.Size().W/2-6, paint.Solid(dial))
	func() {
		// The phone faces Degrees clockwise from north, so north lies as far anticlockwise of its top.
		defer p.Push(paint.Rotate(-r.heading.Degrees*math.Pi/180, d.Center()))()
		at := d.Inset(geom.Uniform(24))
		p.Mask(r.north.Fill(), r.north.Fill().In(needleBox, at), red)
		p.Mask(r.south.Fill(), r.south.Fill().In(needleBox, at), slate)
	}()
	for kid := range kids.All {
		kid.Paint(p)
	}
}
