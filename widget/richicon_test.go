package widget

import (
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/text"
)

// iconPiece returns the laid-out piece of r's span i that is its icon.
func iconPiece(t *testing.T, r *RichText, i int) (line int, pc text.Piece) {
	t.Helper()
	for n, l := range r.laid.Lines {
		for _, pc := range l.Pieces {
			if at, isIcon, ok := r.span(pc); ok && isIcon && at == i {
				return n, pc
			}
		}
	}
	t.Fatalf("span %d's icon was not laid out", i)
	return 0, text.Piece{}
}

func TestRichTextDrawsAnIconInlineAsTallAsItsText(t *testing.T) {
	r := NewRichText(
		RichSpan{Text: "Saved "},
		RichSpan{Icon: icon.Check, Ink: ToastSuccessInk},
		RichSpan{Text: " to "},
		RichSpan{Icon: icon.Folder, Text: "Documents"},
	)
	w, _ := stage(t, &frame{child: r, size: geom.Sz(600, 100)})
	ms := maskOps(w.Offscreen())
	if len(ms) != 2 || strokeOf(t, ms[0]).Icon != icon.Check || strokeOf(t, ms[1]).Icon != icon.Folder {
		t.Fatalf("the text drew %d icons, want a check and a folder", len(ms))
	}
	size := TextSize.Default()
	if s := ms[0].Rect.Size(); s != geom.Sz(size, size) {
		t.Errorf("the icon is %v, want as tall as the text, %v", s, size)
	}
	if ms[0].Color != ToastSuccessInk.Default() || ms[1].Color != Ink.Default() {
		t.Errorf("the icons are %v and %v, want each in its span's colour", ms[0].Color, ms[1].Color)
	}
	_, check := iconPiece(t, r, 1)
	if check.Run.Advance != size {
		t.Errorf("an icon alone takes %v, want its size, %v", check.Run.Advance, size)
	}
	_, folder := iconPiece(t, r, 3)
	if folder.Run.Advance != size+IconGap.Default() {
		t.Errorf("an icon before text takes %v, want its size and a gap", folder.Run.Advance)
	}
	if got := r.Access().Name; got != "Saved  to Documents" {
		t.Errorf("the text reads as %q", got)
	}
}

func TestAnInlineIconWrapsAsAWord(t *testing.T) {
	r := NewRichText(RichSpan{Text: "Open the file "}, RichSpan{Icon: icon.File, Text: "report.pdf"})
	// Wide enough for the first words and the icon, too narrow for the icon's word as well.
	face := faceIn(Font, nil)
	width := face.Shape("Open the file ", TextSize.Default()).Advance + TextSize.Default() + 10
	stage(t, &frame{child: r, size: geom.Sz(width, 100)})
	line, pc := iconPiece(t, r, 1)
	if line != 1 || pc.At.X != 0 {
		t.Fatalf("the icon is on line %d at %v, want at the start of the second line with its word", line, pc.At.X)
	}
}

func TestALinkedIconIsPartOfTheLink(t *testing.T) {
	r := NewRichText(RichSpan{Text: "See the "}, RichSpan{Icon: icon.ExternalLink, Text: "manual", On: openedLink{"m"}})
	w, run := stage(t, &frame{child: r, size: geom.Sz(600, 100)})
	_, pc := iconPiece(t, r, 1)
	at := geom.Pt(pc.At.X+pc.Run.Size/2, pc.At.Y+pc.Run.Height()/2)
	if r.linkAt(at) != 1 {
		t.Fatalf("the icon is not part of the link")
	}
	click(w, at.X, at.Y)
	run(1)
	if got := sent(w); len(got) != 1 || got[0] != (openedLink{"m"}) {
		t.Fatalf("a click on the linked icon sent %v", got)
	}
	ms := maskOps(w.Offscreen())
	if len(ms) != 1 || ms[0].Color != LinkInk.Default() {
		t.Fatalf("the linked icon drew %d masks, want one in the link's colour", len(ms))
	}
}
