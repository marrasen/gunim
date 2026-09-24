// Command twowindows is the first milestone: two windows, each drawing
// an animated rounded rectangle at its own monitor's refresh rate.
//
// With two monitors attached, each window opens on its own. Every
// second it prints how many frames each window drew, which should match
// each monitor's rate.
//
//	CGO_ENABLED=0 go run ./example/twowindows
//	CGO_ENABLED=0 go run ./example/twowindows -for 5s
package main

import (
	"context"
	"flag"
	"fmt"
	"image/color"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

func main() {
	runFor := flag.Duration("for", 0, "quit after this long; zero runs until the windows close")
	flag.Parse()
	if err := run(*runFor); err != nil {
		log.Fatal(err)
	}
}
func run(runFor time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, runFor)
		defer cancel()
	}

	return gunim.Main(ctx, func(a *gunim.App) error {
		monitors := a.Monitors()
		for _, m := range monitors {
			fmt.Printf("monitor %q: %v at %.0f Hz, scale %.2f\n", m.Name, m.Bounds.Size(), m.RefreshRate, m.Scale)
		}
		hues := []color.NRGBA{
			{R: 0x5e, G: 0x9c, B: 0xff, A: 0xff},
			{R: 0xff, G: 0x8a, B: 0x5e, A: 0xff},
		}
		var windows []*gunim.Window
		for i, hue := range hues {
			o := gunim.WindowOptions{
				Title: fmt.Sprintf("gunim %d", i+1),
				Size:  geom.Sz(640, 360),
				Root:  newBouncer(fmt.Sprintf("Window %d", i+1), hue),
			}
			switch {
			case len(monitors) > 1:
				o.Monitor = &monitors[i%len(monitors)]
			case i > 0:
				// One monitor: put the second window beside the first.
				o.Parent, o.Anchor = windows[0], geom.Pt(o.Size.W+40, 0)
			}
			w, err := a.NewWindow(o)
			if err != nil {
				return err
			}
			windows = append(windows, w)
		}
		report(ctx, windows)
		return nil
	})
}

// report prints each window's frame rate once a second until ctx ends
// or every window has closed.
func report(ctx context.Context, windows []*gunim.Window) {
	var wg sync.WaitGroup
	closed := make(chan struct{})
	for _, w := range windows {
		// Draining a window's intents is how to learn that it has
		// closed; these windows send none worth reading.
		wg.Go(func() {
			for ev := range w.Client().Intents() {
				_ = ev
			}
		})
	}
	go func() { wg.Wait(); close(closed) }()

	last := make([]int64, len(windows))
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-closed:
			return
		case <-tick.C:
		}
		var line strings.Builder
		for i, w := range windows {
			n := w.Stats().Frames
			fmt.Fprintf(&line, "  window %d: %3d fps (monitor %.0f Hz)", i+1, n-last[i], w.RefreshRate())
			last[i] = n
		}
		fmt.Println(line.String())
	}
}

// bouncer swings a rounded rectangle from side to side for as long as
// the window is open.
type bouncer struct {
	anim.Group
	title text.Run
	hue   color.NRGBA
	x     *anim.Float
	right bool
}

func newBouncer(title string, hue color.NRGBA) *bouncer {
	b := &bouncer{title: text.Default().Shape(title, 22), hue: hue, x: anim.NewFloat(0)}
	b.Add(b.x)
	return b
}

// Step implements [gunim.Animator]. Each time the spring settles it
// sets off for the other side, so the window keeps drawing.
func (b *bouncer) Step(dt time.Duration) bool {
	if !b.Group.Step(dt) {
		b.right = !b.right
		target := float32(0)
		if b.right {
			target = 1
		}
		b.x.Animate(target, anim.Bouncy)
	}
	return true
}

// Layout implements [gunim.Node].
func (b *bouncer) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}

// Paint implements [gunim.Node].
func (b *bouncer) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(color.NRGBA{R: 0x16, G: 0x18, B: 0x1e, A: 0xff}))
	b.title.Paint(p, geom.Pt(24, 18), color.NRGBA{R: 0xec, G: 0xef, B: 0xf4, A: 0xff})

	const w, h = 160, 110
	// Bouncy overshoots by about 11%, so the swing covers the middle 80%
	// of the track and the overshoot stays inside the margins.
	x := 40 + (box.W-w-80)*(0.1+0.8*b.x.Value())
	y := (box.H - h) / 2
	r := geom.Rc(x, y, w, h)
	p.ShadowRRect(r, 18, paint.Fill{}, paint.Shadow{
		Offset: geom.Pt(0, 10),
		Blur:   24,
		Color:  color.NRGBA{A: 0x90},
	})
	light := b.hue
	light.R, light.G, light.B = lighten(light.R), lighten(light.G), lighten(light.B)
	p.RRectStroke(r, 18, paint.Fill{Gradient: &paint.Gradient{
		From: r.Min, To: geom.Pt(r.Min.X, r.Max.Y), Start: light, End: b.hue,
	}}, paint.Stroke{Width: 1.5, Color: color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x40}})

	// A layer with a rounded clip and half opacity, so the example
	// exercises the offscreen path too.
	dot := geom.Rc(x+w/2-14, y+h/2-14, 28, 28)
	defer p.Layer(paint.LayerOpts{Bounds: dot, Opacity: 0.6, Clip: true, Radius: 14})()
	p.RRect(dot, 0, paint.Solid(color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}))
}

func lighten(c uint8) uint8 { return c + (0xff-c)/3 }
