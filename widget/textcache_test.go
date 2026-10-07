package widget

import "testing"

func TestCutRunLeavesNothingWhereEvenTheEllipsisIsTooWide(t *testing.T) {
	face, size := faceIn(Font, nil), TextSize.Default()
	run, ell := face.Shape("Hello", size), face.Shape("…", size)
	if got := cutRun(run, ell, ell.Advance/2); len(got.Glyphs) != 0 || got.Advance != 0 {
		t.Fatalf("in half an ellipsis's room the cut text is %d glyphs, %v wide", len(got.Glyphs), got.Advance)
	}
	if got := cutRun(run, ell, ell.Advance); len(got.Glyphs) != len(ell.Glyphs) || got.Advance != ell.Advance {
		t.Fatalf("in an ellipsis's room the cut text is %d glyphs, %v wide; want the ellipsis alone", len(got.Glyphs),
			got.Advance)
	}
}
