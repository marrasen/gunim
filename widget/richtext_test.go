package widget

import (
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

type openedLink struct{ URL string }

func TestRichTextWrapsAndFollowsItsLinks(t *testing.T) {
	r := NewRichText(
		RichSpan{Text: "Read the "},
		RichSpan{Text: "manual", OnClick: Sends(openedLink{"https://example.com"})},
		RichSpan{Text: " before you report a fault in the particle counter, please.", Face: BoldFont},
	)
	w, run := stage(t, &frame{child: r, size: geom.Sz(200, 300)})
	if len(r.laid.Lines) < 2 {
		t.Fatalf("at 200 wide the text took %d lines, want it wrapped", len(r.laid.Lines))
	}
	var at geom.Point
	for _, pc := range r.laid.Lines[0].Pieces {
		if pc.Span == 1 {
			at = geom.Pt(pc.At.X+pc.Run.Advance/2, pc.At.Y+pc.Run.Height()/2)
		}
	}
	w.Input(input.PointerMove{Pos: at})
	run(1)
	if r.hover != 1 {
		t.Fatalf("with the pointer on the link, hover is %d", r.hover)
	}
	click(w, at.X, at.Y)
	run(1)
	if got := sent(w); len(got) != 1 || got[0] != (openedLink{"https://example.com"}) {
		t.Fatalf("a click on the link sent %v", got)
	}
	click(w, 2, at.Y)
	run(1)
	if got := sent(w); len(got) != 0 {
		t.Fatalf("a click on plain text sent %v", got)
	}
}
