//go:build !js

package text

import (
	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/shaping"

	"github.com/marrasen/gunim/paint"
)

func platformFace(rune) *font.Face { return nil }

// browserFont is the browser's own fonts, which only a program in a browser has; see browser_js.go.
type browserFont struct{}

func (*browserFont) isColor(uint32) bool { return false }

func (*browserFont) rasterize(uint32, float32, float32, bool) Mask { return Mask{} }

func platformCell(*Face, []rune, bool, float32, float32, float32, float32) (g paint.Glyph, ok bool) {
	return paint.Glyph{}, false
}

func (*browserFont) reach(uint32) float32 { return 0 }

func (*browserFont) keepGlyph(uint32) {}

func platformLacks(*Face, []rune) bool { return false }

func platformWarm() bool { return false }

func platformGlyph(*Face, rune, float32) (g paint.Glyph, advance float32, ok bool) {
	return paint.Glyph{}, 0, false
}

func platformRuns(_ *Face, inputs []shaping.Input) []shaping.Input { return inputs }

func platformShape(shaping.Input, float32) (out shaping.Output, ok bool) {
	return shaping.Output{}, false
}
