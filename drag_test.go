package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// carrier starts a drag of its word when pressed and moved, and
// records how the drag ended.
type carrier struct {
	word  string
	ended []input.DragEnd
	ghost *recorder
}

func (c *carrier) Handle(e input.Event, u *UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		return true
	case input.PointerMove:
		c.ghost = &recorder{}
		u.StartDrag(c, c.word, &sized{recorder: c.ghost, size: geom.Sz(20, 10)}, geom.Pt(5, 5))
		return true
	case input.DragEnd:
		c.ended = append(c.ended, e)
		return true
	}
	return false
}

func (c *carrier) Layout(cs Constraints, _ Frame, _ Children) geom.Size { return cs.Max }
func (c *carrier) Paint(*paint.Painter, Frame, geom.Size, Children)     {}

// sized is a recorder of a fixed size.
type sized struct {
	*recorder
	size geom.Size
}

func (s *sized) Layout(Constraints, Frame, Children) geom.Size { return s.size }

// basket takes a drag of any string, and records what it heard.
type basket struct{ recorder }

func (b *basket) Handle(e input.Event, u *UI) bool {
	switch e := e.(type) {
	case input.DragOver:
		_, ok := e.Data.(string)
		b.events = append(b.events, e)
		return ok
	case input.Drop, input.DragLeave:
		b.events = append(b.events, e)
		return true
	}
	return false
}

// twoWindows returns two windows of one application side by side on a
// pretend screen, 800 wide each: a carrier in the first's left half,
// and a basket filling the second.
func twoWindows(t *testing.T) (a, b *Window, c *carrier, bk *basket) {
	t.Helper()
	app := &App{}
	a, b = newTestWindow(), newTestWindow()
	for i, w := range []*Window{a, b} {
		w.app = app
		app.windows.add(w)
		w.mustOffscreen(t).SetOrigin(geom.Pt(float32(i)*800, 0))
	}
	c = &carrier{word: "apple"}
	a.ui.Insert(a.ui.Root(), &stage{t: paint.Identity})
	a.ui.Insert(a.ui.root.kids[0].node, c)
	bk = &basket{}
	b.ui.Insert(b.ui.Root(), bk)
	run(a, 1)
	run(b, 1)
	return a, b, c, bk
}

func TestADragCarriesItsDataToAnotherWindow(t *testing.T) {
	a, b, c, bk := twoWindows(t)
	a.Input(input.PointerDown{Pos: geom.Pt(30, 30), Time: time.Now()})
	a.Input(input.PointerMove{Pos: geom.Pt(40, 30), Time: time.Now()})
	run(a, 1)
	if len(a.ui.popups) != 1 {
		t.Fatal("no picture follows the drag")
	}
	// Out of window a, over window b: 1000 on the screen is 200 in b.
	a.Input(input.PointerMove{Pos: geom.Pt(1000, 50), Time: time.Now()})
	run(b, 1)
	if !bk.got(input.DragOver{}) {
		t.Fatal("the basket in the other window heard no DragOver")
	}
	for _, e := range bk.events {
		if o, ok := e.(input.DragOver); ok && !near(o.Pos, geom.Pt(200, 50)) {
			t.Fatalf("DragOver at %v in the basket's space, want (200, 50)", o.Pos)
		}
	}
	a.Input(input.PointerUp{Pos: geom.Pt(1000, 50), Time: time.Now()})
	run(b, 1)
	run(a, 1)
	var drop input.Drop
	for _, e := range bk.events {
		if d, ok := e.(input.Drop); ok {
			drop = d
		}
	}
	if drop.Data != "apple" {
		t.Fatalf("the basket got a drop of %v, want apple", drop.Data)
	}
	if len(c.ended) != 1 || !c.ended[0].Taken {
		t.Fatalf("the carrier heard %v, want one DragEnd, taken", c.ended)
	}
	run(a, 120)
	if len(a.ui.popups) != 0 {
		t.Fatal("the picture under the pointer stayed after the drop")
	}
}

func TestADragLetGoOverNothingEndsUntaken(t *testing.T) {
	a, b, c, bk := twoWindows(t)
	a.Input(input.PointerDown{Pos: geom.Pt(30, 30), Time: time.Now()})
	a.Input(input.PointerMove{Pos: geom.Pt(1000, 50), Time: time.Now()})
	run(b, 1)
	// Off both windows: the basket hears it leave.
	a.Input(input.PointerMove{Pos: geom.Pt(3000, 50), Time: time.Now()})
	run(b, 1)
	if !bk.got(input.DragLeave{}) {
		t.Fatal("the basket heard no DragLeave")
	}
	a.Input(input.PointerUp{Pos: geom.Pt(3000, 50), Time: time.Now()})
	run(a, 1)
	if len(c.ended) != 1 || c.ended[0].Taken {
		t.Fatalf("the carrier heard %v, want one DragEnd, untaken", c.ended)
	}
}

func TestFilesDroppedOnAWindowReachTheNodeUnderThem(t *testing.T) {
	w := newTestWindow()
	bk := &basket{}
	w.ui.Insert(w.ui.Root(), bk)
	run(w, 1)
	w.Input(input.Drop{Pos: geom.Pt(10, 10), Paths: []string{"/tmp/a.png"}})
	var got input.Drop
	for _, e := range bk.events {
		if d, ok := e.(input.Drop); ok {
			got = d
		}
	}
	if len(got.Paths) != 1 || got.Paths[0] != "/tmp/a.png" {
		t.Fatalf("the basket got %v, want the dropped file", got)
	}
}
