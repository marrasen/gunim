// Command scene shows gunim's depth: a head turning in a 3D view, and a
// card below it that turns over in perspective.
//
//	CGO_ENABLED=0 go run ./example/scene
//
// The head is meshes, lit by a light from the upper left: a glossy
// ellipsoid, two eyes and a hat. It turns on its own; a drag turns it
// faster, and it eases back to its own pace. A tap on the hat gives it
// another colour, and a tap on the head puts a glass bubble round it,
// or takes it off: Scene.Pick finds what the tap is on, and the bubble
// is see-through. A tap on the card turns it over to its back, and
// another turns it back. -shot writes the window to a PNG after -after,
// and quits; -bubble starts with the bubble on.
package main

import (
	"context"
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
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

func main() {
	shot := flag.String("shot", "", "write the window to this PNG file after -after, and quit")
	after := flag.Duration("after", 1500*time.Millisecond, "how long -shot waits")
	bubble := flag.Bool("bubble", false, "start with the glass bubble round the head")
	flag.Parse()
	if err := run(*shot, *after, *bubble); err != nil {
		log.Fatal(err)
	}
}

func run(shot string, after time.Duration, bubble bool) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{Title: "gunim scene", Size: geom.Sz(420, 720), Root: widget.NewSurface()})
		if err != nil {
			return err
		}
		gunim.RegisterView(w, "stage", func(struct{}) *stage {
			s := newStage()
			s.bubbled = bubble
			return s
		}, nil)
		c := w.Client()
		if err := c.Mount(gunim.Root, "stage", "stage", struct{}{}); err != nil {
			return err
		}
		if shot != "" {
			go func() {
				time.Sleep(after)
				if err := writeShot(ctx, c, shot); err != nil {
					log.Print(err)
				}
				c.Close()
			}()
		}
		for range c.Intents() {
		}
		return nil
	})
	switch {
	case errors.Is(err, driver.ErrNoDriver):
		log.Print("gunim has no driver for this operating system yet")
		return nil
	case errors.Is(err, context.Canceled):
		return nil
	}
	return err
}

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

var (
	skin  = color.NRGBA{0xf1, 0xbf, 0x98, 0xff}
	eyes  = color.NRGBA{0x2b, 0x22, 0x30, 0xff}
	glass = color.NRGBA{0xc8, 0xe6, 0xff, 0x48}
	front = color.NRGBA{0x5b, 0x7c, 0xfa, 0xff}
	back  = color.NRGBA{0xf2, 0x9e, 0x4c, 0xff}
	white = color.NRGBA{0xff, 0xff, 0xff, 0xff}
)

// stage is the head's view above the card.
type stage struct {
	head, eye, brim, crown, bubble *paint.Mesh
	// hat is the hat's colour, from hats, and bubbled says the glass
	// bubble is round the head.
	hat     int
	bubbled bool
	// downAt is where the press in progress came down, to tell a tap
	// from a drag.
	downAt geom.Point
	// turn is the head's angle, and spin how fast it turns, in radians a
	// second, easing back to idle after a drag.
	turn, spin float32
	dragAt     geom.Point
	dragging   bool
	// flip is the card's turn, easing towards flipTo, a half turn for its
	// back.
	flip, flipTo float32
	size         geom.Size
}

const idle = 0.6

// hats are the colours a tap on the hat goes through.
var hats = []color.NRGBA{{0xd8, 0x3b, 0x5c, 0xff}, {0x3b, 0x8f, 0xd8, 0xff}, {0x4c, 0xb8, 0x6a, 0xff}, {0xf2, 0xb1, 0x34, 0xff}}

// The items of the scene, by their index.
const (
	itemHead = iota
	itemLeftEye
	itemRightEye
	itemBrim
	itemCrown
	itemBubble
)

func newStage() *stage {
	return &stage{
		head:   paint.NewSphere(32, 64, skin),
		eye:    paint.NewSphere(12, 24, eyes),
		brim:   paint.NewBox(geom.V3(2.1, 0.1, 2.1), white),
		crown:  paint.NewBox(geom.V3(1.2, 0.7, 1.2), white),
		bubble: paint.NewSphere(32, 64, white),
		spin:   idle,
	}
}

func (s *stage) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	s.size = c.Max
	return c.Max
}

// view and card are where the head's view and the card lie.
func (s *stage) view() geom.Rect { return geom.Rc(20, 20, s.size.W-40, s.size.W-40) }

func (s *stage) card() geom.Rect {
	v := s.view()
	return geom.Rc(s.size.W/2-100, v.Max.Y+30, 200, 130)
}

func (s *stage) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, _ gunim.Children) {
	v := s.view()
	p.ShadowRRect(v, 28, paint.Fill{Gradient: &paint.Gradient{From: geom.Pt(v.Min.X+v.Size().W/2, v.Min.Y+v.Size().H*0.4),
		To: geom.Pt(v.Max.X, v.Max.Y), Radial: true, Start: color.NRGBA{0x3c, 0x42, 0x5e, 0xff}, End: color.NRGBA{0x1a, 0x1c, 0x28, 0xff}}},
		paint.Shadow{Offset: geom.Pt(0, 8), Blur: 18, Color: color.NRGBA{0, 0, 0, 0x70}})
	end := p.Layer(paint.LayerOpts{Bounds: v, Opacity: 1, Clip: true, Radius: 28})
	p.Scene(v, s.scene())
	end()

	// The card: two one-sided faces back to back, turning together.
	c := s.card()
	face := func(fill color.NRGBA, turn float32, words float32) {
		end := p.Layer(paint.LayerOpts{Bounds: c, Opacity: 1, Clip: true, Radius: 18,
			Tilt: paint.Tilt{Y: turn, Distance: 700, OneSided: true}})
		p.RRect(c, 0, paint.Solid(fill))
		p.RRect(geom.Rc(c.Min.X+20, c.Min.Y+22, 70*words, 14), 7, paint.Solid(color.NRGBA{0xff, 0xff, 0xff, 0xc8}))
		p.RRect(geom.Rc(c.Min.X+20, c.Min.Y+46, 110, 10), 5, paint.Solid(color.NRGBA{0xff, 0xff, 0xff, 0x70}))
		p.DrawRRect(paint.RRectOp{Rect: geom.Rc(c.Max.X-64, c.Max.Y-50, 44, 30), Radius: 15, Fill: paint.Solid(white),
			Inset: [2]paint.Shadow{{Offset: geom.Pt(0, -3), Blur: 4, Color: color.NRGBA{0, 0, 0, 0x50}}}})
		end()
	}
	face(front, s.flip, 2)
	face(back, s.flip-math.Pi, 1)
}

// scene is the head as it stands now.
func (s *stage) scene() paint.Scene {
	turn := geom.TurnY(s.turn)
	item := func(m *paint.Mesh, model geom.Mat4, shine float32) paint.SceneItem {
		return paint.SceneItem{Mesh: m, Model: model, Shine: shine}
	}
	hat := hats[s.hat]
	items := []paint.SceneItem{
		itemHead:     item(s.head, turn.Mul(geom.Scale3(geom.V3(0.9, 1.08, 0.95))), 24),
		itemLeftEye:  item(s.eye, turn.Mul(geom.Move3(geom.V3(-0.32, 0.18, 0.84))).Mul(geom.Scale3(geom.V3(0.12, 0.16, 0.08))), 96),
		itemRightEye: item(s.eye, turn.Mul(geom.Move3(geom.V3(0.32, 0.18, 0.84))).Mul(geom.Scale3(geom.V3(0.12, 0.16, 0.08))), 96),
		itemBrim:     {Mesh: s.brim, Model: turn.Mul(geom.Move3(geom.V3(0, 0.98, 0))).Mul(geom.TurnZ(-0.1)), Tint: hat, Shine: 8},
		itemCrown:    {Mesh: s.crown, Model: turn.Mul(geom.Move3(geom.V3(-0.04, 1.36, 0))).Mul(geom.TurnZ(-0.1)), Tint: hat, Shine: 8},
	}
	if s.bubbled {
		items = append(items, paint.SceneItem{Mesh: s.bubble, Model: geom.Move3(geom.V3(0, 0.3, 0)).Mul(geom.Scale3(geom.V3(1.6, 1.6, 1.6))),
			Tint: glass, Shine: 64})
	}
	return paint.Scene{Camera: paint.Camera{Eye: geom.V3(0, 0.6, 5.2), At: geom.V3(0, 0.25, 0)}, Items: items}
}

// tap carries out a tap at p in the head's view: on the hat, the next
// colour; on the head, or the bubble round it, the bubble on or off.
func (s *stage) tap(p geom.Point) {
	hit, ok := s.scene().Pick(s.view(), p)
	if !ok {
		return
	}
	switch hit.Item {
	case itemBrim, itemCrown:
		s.hat = (s.hat + 1) % len(hats)
	case itemHead, itemLeftEye, itemRightEye, itemBubble:
		s.bubbled = !s.bubbled
	}
}

func (s *stage) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		if s.card().Contains(e.Pos) {
			if s.flipTo == 0 {
				s.flipTo = math.Pi
			} else {
				s.flipTo = 0
			}
			u.Invalidate()
			return true
		}
		if s.view().Contains(e.Pos) {
			s.dragging, s.dragAt, s.downAt = true, e.Pos, e.Pos
			return true
		}
	case input.PointerMove:
		if s.dragging {
			s.spin = (e.Pos.X - s.dragAt.X) / 30
			s.turn += (e.Pos.X - s.dragAt.X) / 80
			s.dragAt = e.Pos
			u.Invalidate()
			return true
		}
	case input.PointerUp:
		if d := e.Pos.Sub(s.downAt); s.dragging && d.X*d.X+d.Y*d.Y < 8*8 {
			s.tap(e.Pos)
			u.Invalidate()
		}
		s.dragging = false
	}
	return false
}

// Step turns the head and the card by the time gone.
func (s *stage) Step(dt time.Duration) bool {
	t := float32(dt.Seconds())
	if !s.dragging {
		s.spin += (idle - s.spin) * min(1, t*2)
		s.turn += s.spin * t
	}
	s.flip += (s.flipTo - s.flip) * min(1, t*7)
	if math.Abs(float64(s.flipTo-s.flip)) < 1e-3 {
		s.flip = s.flipTo
	}
	return true
}
