package text

import (
	"testing"
)

func TestShapeLaysGlyphsAlongTheBaseline(t *testing.T) {
	run := Default().Shape("Hello", 16)
	if len(run.Glyphs) != 5 {
		t.Fatalf("got %d glyphs, want 5", len(run.Glyphs))
	}
	for i := 1; i < len(run.Glyphs); i++ {
		if run.Glyphs[i].At.X <= run.Glyphs[i-1].At.X {
			t.Fatalf("glyph %d at x=%v, left of glyph %d", i, run.Glyphs[i].At.X, i-1)
		}
	}
	if run.Advance <= run.Glyphs[4].At.X {
		t.Fatalf("Advance %v ends inside the last glyph", run.Advance)
	}
	if run.Ascent <= 0 || run.Descent <= 0 {
		t.Fatalf("Ascent %v, Descent %v: want both positive", run.Ascent, run.Descent)
	}
}

func TestShapeScalesWithSize(t *testing.T) {
	small := Default().Shape("gunim", 10).Advance
	large := Default().Shape("gunim", 20).Advance
	if d := large - 2*small; d < -0.5 || d > 0.5 {
		t.Fatalf("advance at 20 px is %v, want twice %v", large, small)
	}
}

func TestRasterizeCoversTheGlyph(t *testing.T) {
	f := Default()
	run := f.Shape("O", 32)
	m := f.Rasterize(run.Glyphs[0].ID, 32, 0, Raster{})
	if m.W == 0 || m.H == 0 {
		t.Fatal("empty mask for O")
	}
	// An O is inked around its edge and hollow in the middle.
	if m.Pix[(m.H/2)*m.W+m.W/2] != 0 {
		t.Fatal("the middle of the O is inked; its inner contour was lost")
	}
	var edge byte
	for x := range m.W {
		edge = max(edge, m.Pix[(m.H/2)*m.W+x])
	}
	if edge < 200 {
		t.Fatalf("strongest coverage across the O is %d, want a solid stroke", edge)
	}
	// The O sits above the baseline, so its mask starts above the origin.
	if m.Offset.Y >= 0 {
		t.Fatalf("mask offset %v, want it above the baseline", m.Offset)
	}
}

func TestRasterizeASpaceIsEmpty(t *testing.T) {
	f := Default()
	run := f.Shape(" ", 16)
	if m := f.Rasterize(run.Glyphs[0].ID, 16, 0, Raster{}); m.W != 0 {
		t.Fatalf("a space rasterized to %dx%d", m.W, m.H)
	}
}

func TestLookupFindsTheFace(t *testing.T) {
	f := Default()
	run := f.Shape("a", 12)
	got, ok := Lookup(run.Glyphs[0].Face)
	if !ok || got != f {
		t.Fatal("Lookup lost the face a glyph names")
	}
}
