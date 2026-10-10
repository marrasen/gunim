package gunim

import (
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// fingerEvent takes a finger beyond the first from the driver, in a
// window that takes them: its FingerDown goes to the node under it, or
// the nearest around it that takes it, and the rest of that finger to
// the same node, wherever it goes meanwhile, as a pointer's press does.
func (u *UI) fingerEvent(root *state, e input.Finger) {
	if e.Phase == input.FingerDown {
		took := u.dispatchAt(root, e.Pos, func(local geom.Point) input.Event {
			return input.Finger{ID: e.ID, Pos: local, Phase: e.Phase, Time: e.Time}
		})
		if took != nil {
			if u.fingers == nil {
				u.fingers = map[int]*state{}
			}
			u.fingers[e.ID] = took
		}
		return
	}
	s := u.fingers[e.ID]
	if e.Phase == input.FingerUp {
		delete(u.fingers, e.ID)
	}
	if s == nil {
		return
	}
	pos := e.Pos
	if pos != input.Away {
		pos = u.local(s, pos)
	}
	u.deliver(s, input.Finger{ID: e.ID, Pos: pos, Phase: e.Phase, Time: e.Time})
}

// loseFingers lets the fingers that went to s, or a node inside it,
// which is leaving the tree, go nowhere more.
func (u *UI) loseFingers(s *state) {
	for id, to := range u.fingers {
		if to.within(s) {
			delete(u.fingers, id)
		}
	}
}
