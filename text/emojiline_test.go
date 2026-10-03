package text

import (
	"testing"

	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"

	"github.com/marrasen/gunim/geom"
)

func TestAGlyphWithNoInkKeepsItsWidthUnlessItIsASpace(t *testing.T) {
	// A COLRv1 emoji has no outline to measure: the shaper gives it no
	// width, and the wrapper would take it at a line's end for a space.
	runes := []rune("a 🃏")
	out := shaping.Output{Glyphs: []shaping.Glyph{
		{ClusterIndex: 0, XAdvance: fixed.I(8), Width: fixed.I(7)},
		{ClusterIndex: 1, XAdvance: fixed.I(4)},
		{ClusterIndex: 2, XAdvance: fixed.I(20)},
	}}
	inkWidths(&out, runes)
	if got := out.Glyphs[0].Width; got != fixed.I(7) {
		t.Errorf("a glyph with ink got width %v, want its own 7", got)
	}
	if got := out.Glyphs[1].Width; got != 0 {
		t.Errorf("a space got width %v, want none, so it trims at a line's end", got)
	}
	if got := out.Glyphs[2].Width; got != fixed.I(20) {
		t.Errorf("the emoji got width %v, want its advance, 20", got)
	}
}

func TestASpaceAfterAnEmojiIsTheTextsSpace(t *testing.T) {
	// The segmenter leaves a space in the face before it; an emoji font
	// sets a space as wide as an emoji.
	em := emojiFace(t)
	UseSystemFonts(false)
	defer UseSystemFonts(true)
	base := Default()
	base.Fallback(em)
	defer base.Fallback()

	plain := base.Shape("a b", 16)
	spaceW := plain.CaretX(2) - plain.CaretX(1)
	r := base.Shape("😀 b", 16)
	if got := r.CaretX(2) - r.CaretX(1); abs(got-spaceW) > 0.01 {
		t.Fatalf("the space after the emoji is %.2f wide, want the text's space, %.2f", got, spaceW)
	}
}

func TestTheCaretGoesAfterAnEmojiEndingALine(t *testing.T) {
	em := emojiFace(t)
	UseSystemFonts(false)
	defer UseSystemFonts(true)
	base := Default()
	base.Fallback(em)
	defer base.Fallback()

	p := base.Layout("Hej 😀\nXx", Style{Size: 16}, 0)
	l := p.Lines[0]
	if l.Run.CaretX(5) <= l.Run.CaretX(4) {
		t.Fatalf("the caret after the emoji is at %.1f, before it at %.1f: the emoji lost its width",
			l.Run.CaretX(5), l.Run.CaretX(4))
	}
	if got := p.Index(geom.Pt(l.Run.Advance+4, 4)); got != 5 {
		t.Fatalf("a tap past the line's end put the caret at %d, want 5, after the emoji", got)
	}
}
