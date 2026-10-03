package widget

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// textHost is a text widget the selection handles belong to.
type textHost interface {
	gunim.Node
	// caretRect returns where a caret before rune i would stand, and
	// hostIndex the rune a press at p is before, both in the widget's
	// space.
	caretRect(i int) geom.Rect
	hostIndex(p geom.Point, u *gunim.UI) int
}

// The size of a selection handle, whose tip is the middle of its top
// edge.
const (
	handleW = 24
	handleH = 30
)

// selHandle is one of the two handles a finger drags the ends of a
// selection by, as on a phone: a drop hanging below the text at the
// selection's start or end. Each sits in a popup of its own, so it can
// hang past the bottom of the field.
type selHandle struct {
	e    *editor
	host textHost
	// end is 0 for the start's handle, and 1 for the end's.
	end int
	// dragging is set while a finger drags the handle, and grab is where
	// it holds it, from the tip.
	dragging bool
	grab     geom.Point
}

// tip is where the handle points, in its own space.
func (*selHandle) tip() geom.Point { return geom.Pt(handleW/2, 0) }

// Layout implements [gunim.Node].
func (h *selHandle) Layout(gunim.Constraints, gunim.Frame, gunim.Children) geom.Size {
	return geom.Sz(handleW, handleH)
}

// Paint implements [gunim.Node]: a stem down from the tip to a round
// grip, in the accent.
func (h *selHandle) Paint(p *paint.Painter, f gunim.Frame, _ geom.Size, _ gunim.Children) {
	c := paint.Solid(Accent.Get(f.Theme))
	const r = 10
	cy := float32(handleH - r)
	p.RRect(geom.Rc(handleW/2-1.5, 0, 3, cy), 1.5, c)
	p.RRect(geom.Rc(handleW/2-r, cy-r, 2*r, 2*r), r, c)
}

// DragsTouch implements [gunim.TouchDragger]: a finger drags a handle.
func (h *selHandle) DragsTouch() bool { return h.dragging }

// Handle implements [gunim.Handler]. A drag moves the handle's end of
// the selection to the text under its tip, and the edit menu comes back
// as it lets go.
func (h *selHandle) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		h.e.closeMenu()
		h.dragging, h.grab = true, e.Pos.Sub(h.tip())
	case input.PointerMove:
		if !h.dragging {
			return false
		}
		at, ok := u.Convert(e.Pos.Sub(h.grab), h, h.host)
		if !ok {
			return true
		}
		// The tip hangs from the bottom of the line: the text it means
		// is half a line up.
		line := h.host.caretRect(0).Size().H
		h.e.moveEnd(h.end, h.host.hostIndex(geom.Pt(at.X, at.Y-line/2), u))
		u.Invalidate()
	case input.PointerUp:
		if !h.dragging {
			return false
		}
		h.dragging = false
		r := h.e.handleAnchor(h.host, h.end)
		h.e.openMenu(h.host, geom.Pt(r.Min.X+handleW/2, r.Max.Y), u)
	default:
		return false
	}
	return true
}

// showHandles opens the handles at the ends of the selection, for a
// selection a finger made, unless it is empty.
func (e *editor) showHandles(host textHost, u *gunim.UI) {
	e.closeHandles()
	if start, end := e.Selection(); start == end {
		return
	}
	for k := range e.handles {
		h := &selHandle{e: e, host: host, end: k}
		e.handleAt[k] = e.handleAnchor(host, k)
		e.handles[k] = u.OpenPopup(host, h, gunim.PopupOptions{Anchor: e.handleAt[k], Over: true})
	}
}

// placeHandles keeps the handles at the ends of the selection, as the
// text scrolls or a handle drags its end. It runs as host lays out.
func (e *editor) placeHandles(host textHost) {
	for k, p := range e.handles {
		if p == nil {
			continue
		}
		if at := e.handleAnchor(host, k); at != e.handleAt[k] {
			e.handleAt[k] = at
			p.Move(at)
		}
	}
}

// closeHandles takes the handles away, if they show.
func (e *editor) closeHandles() {
	for k, p := range e.handles {
		if p != nil {
			p.Close()
			e.handles[k] = nil
		}
	}
}

// handleAnchor returns where handle k hangs, in host's space: its tip at
// the bottom of the caret before the selection's start, or its end.
func (e *editor) handleAnchor(host textHost, k int) geom.Rect {
	start, end := e.Selection()
	i := start
	if k == 1 {
		i = end
	}
	r := host.caretRect(i)
	tip := geom.Pt(r.Min.X, r.Max.Y)
	min := tip.Sub(geom.Pt(handleW/2, 0))
	return geom.Rect{Min: min, Max: min.Add(geom.Pt(handleW, handleH))}
}

// moveEnd moves end k of the selection to rune i, keeping the other
// where it is and at least a rune between them.
func (e *editor) moveEnd(k, i int) {
	start, end := e.Selection()
	if k == 0 {
		e.anchor, e.caret = end, max(0, min(i, end-1))
	} else {
		e.anchor, e.caret = start, min(len(e.text), max(i, start+1))
	}
	e.hinted, e.goal = false, false
}
