package filemanager

import (
	"testing"

	"github.com/marrasen/gunim"
)

// At zoom 1.25 the pane is narrow, and every link under the path still
// lies within it.
func TestThePreviewLinksFitThePaneWhenZoomed(t *testing.T) {
	h := newHarness(t, "notes.txt")
	if err := h.w.Client().SetZoom(1.25); err != nil {
		t.Fatal(err)
	}
	h.until("the preview shows the folder's links", func() bool {
		return h.b.preview.cur != nil && len(h.b.preview.cur.links) > 0
	})
	h.frames(60)
	pane := h.bounds(func(b *browser) gunim.Node { return b.preview })
	for i, l := range h.b.preview.cur.links {
		r := h.bounds(func(b *browser) gunim.Node { return b.preview.cur.links[i] })
		if r.Min.X < pane.Min.X || r.Max.X > pane.Max.X || r.Min.Y < pane.Min.Y || r.Max.Y > pane.Max.Y {
			t.Errorf("the link %q lies at %v, outside the pane at %v", l.Text, r, pane)
		}
	}
}
