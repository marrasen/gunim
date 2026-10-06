package gunim

import (
	"math"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// A PinchZoomer is a node that zooms something of its own with two fingers, smoothly, as a map does. A pinch that
// starts over it, or over a node inside it, while ZoomsWithPinch reports true goes to it as [input.Pinch], from start
// to end, wherever the fingers go meanwhile, with Pos and Delta in the node's own space. The window's zoom leaves such
// a pinch alone.
//
// A pinch over anything else is Ctrl with the wheel at the point between the fingers, a notch for each quarter the
// fingers spread or close, which zooms a [WheelZoomer] under them or the window where it zooms.
type PinchZoomer interface {
	Node
	ZoomsWithPinch() bool
}

// pinchStep is how much the fingers' spread grows for one notch of the wheel, for a pinch over no PinchZoomer: a
// notch of a wheel zooms by about a quarter.
const pinchStep = 1.25

// pinchGesture is a pinch going on in a window. to is the PinchZoomer it started over, which hears all of it, or nil
// when it started over none and goes as Ctrl with the wheel. lost says to left the tree before the pinch ended, and
// the rest of the pinch goes nowhere.
type pinchGesture struct {
	root *state
	to   *state
	lost bool
}

// pinchEvent takes a pinch from the driver. Its start chooses where the whole pinch goes: to the PinchZoomer under
// the fingers, or as Ctrl with the wheel when there is none.
func (u *UI) pinchEvent(root *state, e input.Pinch) {
	if e.Phase == input.PinchStart {
		if g := u.pinch; g != nil {
			// A pinch that never ended, ended now.
			u.pinchTo(g, input.Pinch{Pos: e.Pos, Scale: 1, Phase: input.PinchEnd, Time: e.Time})
		}
		u.pinch = &pinchGesture{root: root, to: u.pinchZoomerAt(root, e.Pos)}
	}
	g := u.pinch
	if g == nil {
		g = &pinchGesture{root: root}
	}
	if e.Phase == input.PinchEnd {
		u.pinch = nil
	}
	u.pinchTo(g, e)
}

// pinchTo hands e to where g goes.
func (u *UI) pinchTo(g *pinchGesture, e input.Pinch) {
	switch {
	case g.lost:
	case g.to != nil:
		pos := u.local(g.to, e.Pos)
		u.deliver(g.to, input.Pinch{Pos: pos, Delta: pos.Sub(u.local(g.to, e.Pos.Sub(e.Delta))), Scale: e.Scale, Phase: e.Phase, Time: e.Time})
	case e.Phase == input.PinchMove && e.Scale > 0 && e.Scale != 1:
		notches := float32(math.Log(float64(e.Scale)) / math.Log(pinchStep))
		u.handleRaw(g.root, input.Scroll{Pos: e.Pos, Notches: geom.Pt(0, notches), Mods: input.ModControl, Time: e.Time})
	}
}

// pinchZoomerAt returns the node at p, or the nearest around it, that zooms with a pinch now, or nil.
func (u *UI) pinchZoomerAt(root *state, p geom.Point) *state {
	for s := u.hit(root, p); s != nil; s = s.parent {
		if z, ok := s.node.(PinchZoomer); ok && z.ZoomsWithPinch() {
			return s
		}
	}
	return nil
}

// losePinch lets the pinch going on go nowhere more if it goes to s or a node inside it, which is leaving the tree.
func (u *UI) losePinch(s *state) {
	if g := u.pinch; g != nil && g.to != nil && g.to.within(s) {
		g.to, g.lost = nil, true
	}
}
