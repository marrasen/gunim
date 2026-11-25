package text

import (
	"slices"
	"testing"

	"golang.org/x/image/font/gofont/gomono"

	"github.com/marrasen/gunim/paint"
)

// monoWithMarks returns Go Mono with Inter's marks to fall back to, and no system fonts, until the test ends.
func monoWithMarks(t *testing.T) *Face {
	t.Helper()
	UseSystemFonts(false)
	t.Cleanup(func() { UseSystemFonts(true) })
	f, err := Parse(gomono.TTF)
	if err != nil {
		t.Fatal(err)
	}
	return f.Fallback(Inter(false))
}

// A mark no font has is left out, and those beside it that a font has are drawn.
func TestOnlyAMarkNoFontHasIsLeftOut(t *testing.T) {
	f := monoWithMarks(t)
	cell := func(s string) []paint.Glyph { return f.Cell(s, 14, 8, 13, 4) }
	want := cell("x̂")
	if len(want) != 2 {
		t.Fatalf("x with a circumflex is %+v", want)
	}
	for _, s := range []string{"x̂̅", "x̅̂", "x̅̂̐"} {
		if got := cell(s); !slices.Equal(got, want) {
			t.Errorf("%+q is %+v, want %+v", s, got, want)
		}
	}
	if got := cell("x̅̐"); !slices.Equal(got, cell("x")) {
		t.Errorf("x with two marks no font has is %+v", got)
	}
	// One composed with its letter, and one no font has.
	if got := cell("é̅"); !slices.Equal(got, cell("é")) {
		t.Errorf("e with an accent and a mark no font has is %+v", got)
	}
}

// A mark is put over its letter where it comes after it, and before it, as in text that reads from the right.
func TestAMarkGoesOverItsLetterFromEitherSide(t *testing.T) {
	f := monoWithMarks(t)
	run := f.Shape("x̂", 14)
	letter, mark := run.Glyphs[0], run.Glyphs[1]
	if letter.Face == mark.Face {
		t.Fatal("the letter and its mark are of one face")
	}
	middle := func(g paint.Glyph) float32 {
		face, _ := Lookup(g.Face)
		m := face.Rasterize(g.ID, 14, 0, Raster{})
		return g.At.X + float32(m.Offset.X) + float32(m.W)/2
	}
	for _, at := range []float32{0, 30} {
		letter.At.X, mark.At.X = at, at
		for _, glyphs := range [][]paint.Glyph{{letter, mark}, {mark, letter}} {
			f.overLetter(glyphs, 'x', 14)
			for _, g := range glyphs {
				if g.Face == letter.Face && g != letter {
					t.Errorf("the letter moved from %v to %v", letter.At, g.At)
				}
				if g.Face == mark.Face && (middle(g) < at+run.Advance/2-1 || middle(g) > at+run.Advance/2+1) {
					t.Errorf("the middle of the mark of %+v is at %v, want %v", glyphs, middle(g), at+run.Advance/2)
				}
			}
		}
	}
}

// A stamp tells once of a change to the fonts.
func TestAStampTellsOfTheFontsChanging(t *testing.T) {
	var s Stamp
	s.Moved()
	if s.Moved() {
		t.Error("with nothing changed it says so")
	}
	GoMono(true, false).Fallback()
	if !s.Moved() || s.Moved() {
		t.Error("it does not tell once of a face's new fallbacks")
	}
	was := Generation()
	UseSystemFonts(false)
	defer UseSystemFonts(true)
	if Generation() == was || !s.Moved() {
		t.Error("it does not tell of the system's fonts turned off")
	}
}
