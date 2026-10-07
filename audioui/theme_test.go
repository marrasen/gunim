package audioui

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// A light theme colours the audio pieces' text and guides, through widget's Ink alone.
func TestALightThemeColoursTheAudioPieces(t *testing.T) {
	light := theme.NewLive(widget.Light())
	ink := widget.Ink.Get(light)
	if ink == widget.Ink.Default() {
		t.Fatal("the light theme leaves widget.Ink at its dark default")
	}
	gain := float32(0)
	f := NewFader(func() float32 { return gain }, nil)
	var p paint.Painter
	f.Paint(&p, gunim.Frame{Scale: 1, Theme: light}, geom.Sz(24, 200), gunim.Children{})
	for _, op := range p.Ops() {
		if r, ok := op.(*paint.RRectOp); ok {
			c := r.Fill.Solid
			if c.R == ink.R && c.G == ink.G && c.B == ink.B && c.A > 0 {
				return
			}
		}
	}
	t.Fatalf("under the light theme the fader drew nothing in its ink %v", ink)
}
