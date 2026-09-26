package gunim

import (
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// A Drawing is what a node drew the last time it was painted, kept to
// be drawn again somewhere else: a pane shown small in an overview of
// every pane, including the ones that have left the screen, which show
// as they were last seen. [UI.KeepDrawing] makes one.
type Drawing struct {
	rec  paint.Recording
	size geom.Size
}

// Recording is what the node drew, in its own space, with its top left
// at the origin, for [paint.Painter.Replay]. It is empty until the node
// has been painted.
func (d *Drawing) Recording() *paint.Recording { return &d.rec }

// Size is the node's size as it was drawn.
func (d *Drawing) Size() geom.Size { return d.size }

// KeepDrawing keeps what n draws each time it is painted, from the next
// frame on, until [UI.ForgetDrawing]. It returns the same Drawing for n
// each time it is asked. Keeping costs a copy of the node's commands
// each frame it paints.
func (u *UI) KeepDrawing(n Node) *Drawing {
	if d, ok := u.kept[n]; ok {
		return d
	}
	if u.kept == nil {
		u.kept = map[Node]*Drawing{}
	}
	d := &Drawing{}
	u.kept[n] = d
	return d
}

// ForgetDrawing stops keeping what n draws, and lets go of what it drew.
func (u *UI) ForgetDrawing(n Node) { delete(u.kept, n) }
