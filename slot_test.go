package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// shell is a view's root with a 40-high header over a box that takes the
// views mounted under it.
type shell struct {
	header, body *Box
}

func (s *shell) Children() []Node { return []Node{s.header, s.body} }

func (s *shell) Slot() Node { return s.body }

func (s *shell) Layout(c Constraints, _ Frame, kids Children) geom.Size {
	kids.At(0).Layout(Tight(geom.Sz(c.Max.W, 40)))
	kids.At(0).Place(geom.Point{})
	kids.At(1).Layout(Tight(geom.Sz(c.Max.W, c.Max.H-40)))
	kids.At(1).Place(geom.Pt(0, 40))
	return c.Max
}

func (s *shell) Paint(p *paint.Painter, _ Frame, _ geom.Size, kids Children) {
	for k := range kids.All {
		k.Paint(p)
	}
}

// marker remembers where it was drawn, in window space.
type marker struct{ at geom.Point }

func (m *marker) Layout(c Constraints, _ Frame, _ Children) geom.Size { return c.Max }

func (m *marker) Paint(p *paint.Painter, _ Frame, _ geom.Size, _ Children) {
	m.at = p.Transform().Apply(geom.Point{})
}

func TestAViewMountedUnderASlottedRootGoesInItsSlot(t *testing.T) {
	w := newTestWindow()
	sh := &shell{header: &Box{}, body: &Box{}}
	m := &marker{}
	RegisterView(w, "shell", func(struct{}) *shell { return sh }, nil)
	RegisterView(w, "page", func(struct{}) *marker { return m }, nil)
	c := w.Client()
	if err := c.Mount(Root, "shell", "shell", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Mount("shell", "page", "page", nil); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	w.Frame(time.Second / 60)
	if !inTree(w.ui, m) || m.at.Y != 40 {
		t.Fatalf("the page is drawn at %v, want below the 40-high header", m.at)
	}
}
