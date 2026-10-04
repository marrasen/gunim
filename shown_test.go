package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// hearsShown records the window's hidden and shown events.
type hearsShown struct{ got []input.Event }

func (*hearsShown) Layout(c Constraints, _ Frame, _ Children) geom.Size { return c.Max }
func (*hearsShown) Paint(*paint.Painter, Frame, geom.Size, Children)    {}
func (*hearsShown) Focusable() bool                                     { return true }
func (h *hearsShown) Handle(e input.Event, _ *UI) bool {
	switch e.(type) {
	case input.WindowHidden, input.WindowShown:
		h.got = append(h.got, e)
		return true
	}
	return false
}

func TestAWindowHiddenAndShownTellsTheFocusedNode(t *testing.T) {
	h := &hearsShown{}
	w := NewOffscreen(geom.Sz(200, 200), h)
	w.Frame(time.Second / 60)
	w.Input(driver.WindowShown{Shown: false})
	w.Input(driver.WindowShown{Shown: true})
	w.Frame(time.Second / 60)
	if len(h.got) != 2 {
		t.Fatalf("heard %v, want hidden then shown", h.got)
	}
	if _, ok := h.got[0].(input.WindowHidden); !ok {
		t.Fatalf("first heard %T, want WindowHidden", h.got[0])
	}
	if _, ok := h.got[1].(input.WindowShown); !ok {
		t.Fatalf("then heard %T, want WindowShown", h.got[1])
	}
}
