package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// frameNode does to the window's frame whatever it is told to, on its
// first update.
type frameNode struct{ _ int }

func (*frameNode) Layout(c Constraints, _ Frame, _ Children) geom.Size { return c.Max }
func (*frameNode) Paint(*paint.Painter, Frame, geom.Size, Children)    {}

func TestTheFrameIsTheApplications(t *testing.T) {
	w := NewOffscreen(geom.Sz(200, 100), nil)
	type ask struct {
		title string
		full  bool
		bell  bool
	}
	var full bool
	RegisterView(w, "frame", func(ask) *frameNode { return &frameNode{} }, func(_ *frameNode, a ask, u *UI) {
		u.SetTitle(a.title)
		u.SetFullScreen(a.full)
		if a.bell {
			u.RequestAttention()
		}
		full = u.FullScreen()
	})
	c := w.Client()
	if err := c.Mount(Root, "frame", "frame", ask{}, "frame"); err != nil {
		t.Fatal(err)
	}
	if err := c.Publish("frame", ask{title: "gunimterm — build", full: true, bell: true}); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	off := w.Offscreen()
	var _ driver.Titler = off
	if off.Title() != "gunimterm — build" || !off.FullScreen() || !full || off.Attention() < 1 {
		t.Fatalf("the frame says %q, full %v (told %v), attention %d", off.Title(), off.FullScreen(), full, off.Attention())
	}
}
