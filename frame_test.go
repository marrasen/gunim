package gunim

import (
	"image/color"
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
		title  string
		full   bool
		bell   bool
		border driver.Border
	}
	var full bool
	RegisterView(w, "frame", func(ask) *frameNode { return &frameNode{} }, func(_ *frameNode, a ask, u *UI) {
		u.SetTitle(a.title)
		u.SetFullScreen(a.full)
		u.SetBorder(a.border)
		if a.bell {
			u.RequestAttention()
		}
		full = u.FullScreen()
	})
	c := w.Client()
	if err := c.Mount(Root, "frame", "frame", ask{}, "frame"); err != nil {
		t.Fatal(err)
	}
	border := driver.Border{Color: color.NRGBA{R: 0x30, G: 0x60, B: 0x90, A: 0xff}, Inactive: color.NRGBA{R: 0x80, A: 0xff}}
	if err := c.Publish("frame", ask{title: "gunimterm — build", full: true, bell: true, border: border}); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	off := w.Offscreen()
	var _ driver.Titler = off
	if off.Title() != "gunimterm — build" || !off.FullScreen() || !full || off.Attention() < 1 {
		t.Fatalf("the frame says %q, full %v (told %v), attention %d", off.Title(), off.FullScreen(), full, off.Attention())
	}
	if off.Border() != border {
		t.Fatalf("the border is %+v, want %+v", off.Border(), border)
	}
}
