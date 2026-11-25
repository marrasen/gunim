//go:build js && wasm

package web_test

import (
	"image/color"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver/web"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

// aside shows its one child, or keeps a drawing of it and shows it no more.
type aside struct {
	word  *word
	drawn *gunim.Drawing
	gone  bool
}

func (a *aside) Children() []gunim.Node { return []gunim.Node{a.word} }

func (a *aside) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	a.drawn = f.UI().KeepDrawing(a.word)
	kids.At(0).Layout(c)
	return c.Max
}

func (a *aside) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	if !a.gone {
		kids.At(0).Paint(p)
	}
}

// word draws a run.
type word struct{ run text.Run }

func (w *word) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(w.run.Box())
}

func (w *word) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, _ gunim.Children) {
	w.run.Paint(p, geom.Point{}, color.NRGBA{A: 255})
}

// A drawing kept of a node that has left the screen, which draws it no more, outlives the table too.
func TestAKeptDrawingOutlivesTheTable(t *testing.T) {
	a := &aside{word: &word{run: text.Default().Shape("سلام", 40)}}
	w := gunimtest.New(t, geom.Sz(200, 100), nil)
	w.PaintWhenLooked(false)
	gunim.RegisterView(w, "aside", func(struct{}) gunim.Node { return a }, nil)
	if err := w.Client().Mount(gunim.Root, "aside", "aside", nil); err != nil {
		t.Fatal(err)
	}
	frame := func() { w.Frame(time.Second / 60) }
	frame()
	frame()
	frame()
	glyphs := a.drawn.Recording().Glyphs()
	if len(glyphs) != 1 {
		t.Fatalf("the drawing is of %d glyphs", len(glyphs))
	}
	a.gone = true
	web.TurnTable(t, frame)
	if face, _ := text.Lookup(glyphs[0].Face); face.Reach(glyphs[0].ID) == 0 {
		t.Error("the drawing's glyph is gone")
	}
}
