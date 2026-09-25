package gunim

import (
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// A drag held near the edge of a scroll view scrolls it, so a row can
// be dragged to a place in a list longer than the view, or a picture
// dropped into a list at a place out of sight.
//
// Each frame a drag is held, the engine asks the scroll views around
// it to scroll, by how near the pointer is to their edges. The content
// then moves under a pointer that stands still, so the engine replays
// the pointer where it is: the node holding the drag hears it move, and
// a drag from [UI.StartDrag] lands on whatever is now under it.

// A DragHolder is a node that drags something while it holds the
// pointer, such as a list whose rows the pointer drags into order.
// While DragHeld reports true, scroll views around it scroll when the
// pointer nears their edges, and it hears the pointer move, where it
// stands, as the content moves under it.
type DragHolder interface {
	Node
	DragHeld() bool
}

// An EdgeScroller is a node that scrolls when a drag nears its edges,
// as a scroll view does. Each frame a drag is held, the engine calls
// EdgeScroll on the EdgeScrollers around it, innermost first, until
// one scrolls. p is the pointer, in the node's space, and dt the time
// the frame covers. It returns how far its content moved, in its own
// space, and zero when it stays put.
type EdgeScroller interface {
	Node
	EdgeScroll(p geom.Point, dt time.Duration, u *UI) geom.Point
}

// edgeScroll scrolls what a held drag is near the edge of, for a frame
// of dt, and replays the pointer when the content moved. It reports
// whether anything scrolled.
func (u *UI) edgeScroll(dt time.Duration) bool {
	var from *state
	var at geom.Point
	switch {
	case u.capture != nil && u.drag == nil && holdsDrag(u.capture):
		from, at = u.capture, u.pointer
	case u.dragOver:
		from, at = u.hit(u.root, u.dragOverAt), u.dragOverAt
	default:
		return false
	}
	for s := from; s != nil; s = s.parent {
		e, ok := s.node.(EdgeScroller)
		if !ok {
			continue
		}
		var moved geom.Point
		u.on(s, func() { moved = e.EdgeScroll(u.local(s, at), dt, u) })
		if moved == (geom.Point{}) {
			continue
		}
		// The nodes still stand where the last frame drew them, so the
		// point under the pointer now is where the content was that far
		// back.
		t := s.toWindow
		back := at.Sub(t.Apply(moved).Sub(t.Apply(geom.Point{})))
		if from == u.capture {
			c := u.capture
			u.deliver(c, input.PointerMove{Pos: u.local(c, back), Time: u.now})
		} else {
			u.dragHover(back, u.dragOverData)
		}
		u.invalid = true
		return true
	}
	return false
}

func holdsDrag(s *state) bool {
	h, ok := s.node.(DragHolder)
	return ok && h.DragHeld()
}
