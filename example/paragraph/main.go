// Command paragraph shows paragraph layout: a column of text whose width
// springs between narrow and wide, re-wrapping every frame, with
// English, Hebrew and Arabic in one paragraph and a right-to-left
// paragraph after it.
//
// Hebrew and Arabic need fonts that have them. It looks for Noto Sans
// Hebrew and Noto Sans Arabic where Debian and Ubuntu install them, and
// shows boxes in their place when they are missing.
//
//	CGO_ENABLED=0 go run ./example/paragraph
package main

import (
	"context"
	"flag"
	"image/color"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

const sample = "Paragraphs wrap where Unicode allows, and a word such as שלום or مرحبا sits in its place inside English text, read in its own direction.\n\n" +
	"שלום עולם! פסקה זו מתחילה בעברית, ולכן היא נקראת מימין לשמאל ומיושרת לימין."

var fallbacks = []string{
	"/usr/share/fonts/truetype/noto/NotoSansHebrew-Regular.ttf",
	"/usr/share/fonts/truetype/noto/NotoSansArabic-Regular.ttf",
}

func main() {
	runFor := flag.Duration("for", 0, "quit after this long; zero runs until the window closes")
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

	face := text.Default()
	var extra []*text.Face
	for _, path := range fallbacks {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("no %s; its script will show as boxes", path)
			continue
		}
		f, err := text.Parse(data)
		if err != nil {
			return err
		}
		extra = append(extra, f)
	}
	face.Fallback(extra...)

	return gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{
			Title: "gunim paragraph",
			Size:  geom.Sz(760, 520),
			Root:  newColumn(face),
		})
		if err != nil {
			return err
		}
		for ev := range w.Client().Intents() {
			_ = ev
		}
		return w.Client().Err()
	})
}

// column lays the sample out in a box whose width springs between a
// narrow and a wide target, so the text re-wraps as it moves.
type column struct {
	anim.Group
	face  *text.Face
	width *anim.Float
	wide  bool
}

func newColumn(face *text.Face) *column {
	c := &column{face: face, width: anim.NewFloat(0)}
	c.Add(c.width)
	return c
}

// Step implements [gunim.Animator]. Each time the width settles, it
// sets off for the other target after a pause.
func (c *column) Step(dt time.Duration) bool {
	if !c.Group.Step(dt) {
		c.wide = !c.wide
		target := float32(0)
		if c.wide {
			target = 1
		}
		c.width.Animate(target, anim.Tween{Duration: 2 * time.Second, Ease: anim.EaseInOut})
	}
	return true
}

// Layout implements [gunim.Node].
func (c *column) Layout(cs gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return cs.Max
}

var (
	background = color.NRGBA{R: 0x16, G: 0x18, B: 0x1e, A: 0xff}
	panel      = color.NRGBA{R: 0x22, G: 0x26, B: 0x30, A: 0xff}
	ink        = color.NRGBA{R: 0xec, G: 0xef, B: 0xf4, A: 0xff}
)

// Paint implements [gunim.Node].
func (c *column) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(background))

	const pad = 20
	width := 220 + (box.W-2*pad-2*pad-220)*c.width.Value()
	para := c.face.Layout(sample, text.Style{Size: 17, LineHeight: 1.15}, width)
	frame := geom.Rc(pad, pad, width+2*pad, para.Size.H+2*pad)
	p.RRect(frame, 12, paint.Solid(panel))
	para.Paint(p, geom.Pt(2*pad, 2*pad), ink)
}
