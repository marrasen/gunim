package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// openOn selects name among the icons and opens the viewer on it with Space.
func (h *harness) openOn(name string) {
	h.t.Helper()
	h.do(Command{Name: CmdViewIcons})
	h.until("the icons settle", func() bool { return h.icons().on && h.icons().in.Value() == 1 })
	h.pick(name)
	h.w.Input(key(input.KeySpace, 0))
	h.until("the viewer opens", func() bool { return h.a.viewer.open })
}

func TestTheViewerOpensStepsAndCloses(t *testing.T) {
	h := newPicHarness(t, []string{"a.png", "b.png", "c.png"}, "notes.txt")
	h.openOn("b.png")
	if v := h.a.viewer.view; v.Index != 2 || v.Count != 3 || v.Name != "b.png" {
		t.Fatalf("the viewer shows %s, %d of %d; want b.png, 2 of 3", v.Name, v.Index, v.Count)
	}
	h.until("the picture is decoded to fit", func() bool { return h.a.viewer.view.Image != nil })
	if v := h.a.viewer.view; v.W != 60 || v.H != 40 || v.Loading {
		t.Fatalf("the viewer has the picture %d by %d, loading %v", v.W, v.H, v.Loading)
	}
	if _, ok := h.focused().(*viewer); !ok {
		t.Fatalf("the keyboard is on %T, want the viewer", h.focused())
	}
	h.w.Input(key(input.KeyRight, 0))
	h.until("Right goes to the next picture", func() bool {
		return h.a.viewer.view.Name == "c.png" && h.a.nav.cursor == "c.png"
	})
	h.frames(30)
	h.w.Input(key(input.KeyRight, 0))
	h.frames(5)
	if h.a.viewer.view.Name != "c.png" {
		t.Fatal("Right at the last picture went past it")
	}
	h.w.Input(key(input.KeyLeft, 0))
	h.until("Left goes back", func() bool { return h.a.viewer.view.Name == "b.png" })
	h.w.Input(key(input.KeyEscape, 0))
	h.until("Escape closes the viewer", func() bool { return !h.a.viewer.open })
	h.until("the keyboard is back on the icons", func() bool { return h.focused() == gunim.Node(h.icons().grid) })
	if !h.a.nav.sel["b.png"] {
		t.Fatal("the picture shown last is not selected")
	}
}

func TestTheViewerFliesFromTheTile(t *testing.T) {
	h := newHarness(t)
	if err := writeImage(filepath.Join(h.dir, "big.png"), picture(900, 600, rings)); err != nil {
		t.Fatal(err)
	}
	h.do(Command{Name: CmdRefresh})
	h.until("the picture is listed", func() bool { return len(h.shown()) == 1 })
	h.do(Command{Name: CmdViewIcons})
	h.until("the thumbnail arrives", func() bool { return h.icons().thumbs["big.png"].img != nil })
	thumb := h.icons().thumbs["big.png"].img
	tiles := h.bounds(func(b *browser) gunim.Node { return b.listing.cur.icons.grid })
	h.openOn("big.png")
	// drawn is where the picture, as its thumbnail or decoded, is drawn largest in the last frame.
	drawn := func() geom.Rect {
		var r geom.Rect
		for _, op := range h.w.Offscreen().Ops() {
			pic := h.a.viewer.view.Image
			if im, ok := op.(*paint.ImageOp); ok && (im.Image == thumb || pic != nil && im.Image == pic) && im.Opacity > 0 {
				m := im.Transform
				if d := (geom.Rect{Min: m.Apply(im.Rect.Min), Max: m.Apply(im.Rect.Max)}); d.Size().W > r.Size().W {
					r = d
				}
			}
		}
		return r
	}
	first := drawn()
	h.frames(90)
	last := drawn()
	if !tiles.Contains(first.Center()) || first.Size().W >= last.Size().W || last.Size().W < 400 {
		t.Fatalf("the picture is drawn at %v as it opens and %v once open; want it to grow out of the tiles in %v",
			first, last, tiles)
	}
	// A click beside the picture closes the viewer.
	h.click(last.Max.Add(geom.Pt(8, 8)))
	h.until("the viewer closes", func() bool { return !h.a.viewer.open })
}

func TestAPictureThatCannotBeReadSaysWhyInTheViewer(t *testing.T) {
	h := newPicHarness(t, []string{"good.png"}, "broken.png")
	h.openOn("broken.png")
	h.until("the viewer says why", func() bool { return h.a.viewer.view.Err != "" })
	if !strings.Contains(h.a.viewer.view.Err, "broken.png") || h.a.viewer.view.Image != nil {
		t.Fatalf("the viewer shows %+v", h.a.viewer.view)
	}
	h.frames(2)
	v, ok := h.focused().(*viewer)
	if !ok || v.info.Text != h.a.viewer.view.Err || !v.pic.broken {
		t.Fatal("the viewer does not show the error")
	}
}

func TestTheWheelZoomsAboutThePointer(t *testing.T) {
	v := newViewer()
	v.box, v.scale = geom.Sz(1000, 800), 1
	v.state = Viewing{W: 4000, H: 2000}
	v.fit.Jump(v.fitRect())
	// where returns where the point p of the screen lies in the picture, from 0 to 1, as the zoom is heading.
	where := func(p geom.Point) geom.Point {
		fit := v.fit.Target()
		z := v.zoom.Target()
		c := fit.Center().Add(v.pan.Target())
		w, h := fit.Size().W*z, fit.Size().H*z
		return geom.Pt((p.X-c.X+w/2)/w, (p.Y-c.Y+h/2)/h)
	}
	p := geom.Pt(700, 300)
	before := where(p)
	v.zoomAt(2.5, p)
	after := where(p)
	if d := after.Sub(before); d.X*d.X+d.Y*d.Y > 1e-6 {
		t.Fatalf("zooming about %v moved the point under it from %v to %v", p, before, after)
	}
	if v.zoom.Target() != 2.5 {
		t.Fatalf("the zoom is heading for %v, want 2.5", v.zoom.Target())
	}
	v.zoomAt(0.5, p)
	if v.zoom.Target() != 1 || v.pan.Target() != (geom.Point{}) {
		t.Fatalf("zooming out past the fit left zoom %v and pan %v", v.zoom.Target(), v.pan.Target())
	}
}
