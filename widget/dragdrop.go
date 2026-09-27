package widget

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// Draggable lets its child be dragged, carrying Data, to a
// [DropTarget] in this window or another of the application's.
//
// A press and a move of a few pixels starts the drag. A picture made
// by Ghost rides under the pointer, and the child dims while it is
// away, coming back when the drag ends. A press let go without moving
// is a click, which sends OnClick.
type Draggable struct {
	anim.Group
	Data any
	// Ghost makes what is carried under the pointer; nil carries
	// nothing.
	Ghost func() gunim.Node
	// OnClick is the intent a click sends; nil sends none.
	OnClick gunim.Intent

	child    gunim.Node
	press    geom.Point
	held     bool
	dragging bool
	away     *anim.Float
}

// NewDraggable returns child, draggable with data.
func NewDraggable(child gunim.Node, data any) *Draggable {
	d := &Draggable{Data: data, child: child, away: anim.NewFloat(0)}
	d.Add(d.away)
	return d
}

// Children implements [gunim.Composite].
func (d *Draggable) Children() []gunim.Node { return []gunim.Node{d.child} }

// Handle implements [gunim.Handler].
func (d *Draggable) Handle(e input.Event, u *gunim.UI) bool {
	th := u.Theme()
	switch e := e.(type) {
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		d.held, d.press = true, e.Pos
	case input.PointerMove:
		if !d.held || d.dragging {
			return false
		}
		if dx, dy := e.Pos.X-d.press.X, e.Pos.Y-d.press.Y; dx*dx+dy*dy < pickUp*pickUp {
			return true
		}
		d.dragging = true
		var ghost gunim.Node
		if d.Ghost != nil {
			ghost = d.Ghost()
		}
		u.StartDrag(d, d.Data, ghost, d.press)
		d.away.Animate(1, Quick.Get(th))
	case input.PointerUp:
		if d.held && !d.dragging && d.OnClick != nil {
			u.Send(d, d.OnClick)
		}
		d.held = false
	case input.DragEnd:
		d.held, d.dragging = false, false
		d.away.Animate(0, Settle.Get(th))
	default:
		return false
	}
	return true
}

// Layout implements [gunim.Node].
func (d *Draggable) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	k := kids.At(0)
	s := k.Layout(c)
	k.Place(geom.Point{})
	return s
}

// Paint implements [gunim.Node].
func (d *Draggable) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	if t := d.away.Value(); t > 0.001 {
		defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: 1 - 0.65*min(t, 1)})()
	}
	kids.At(0).Paint(p)
}

// DropTarget takes drops on its child: values dragged from a
// [Draggable] in the application, and files dropped from another
// program, such as a file manager. While something it takes is
// dragged over it, a ring glows around it.
type DropTarget struct {
	anim.Group
	// Accept says whether the target takes a drop of data, from inside
	// the application, or of paths, from another program. Nil takes
	// everything.
	Accept func(data any, paths []string) bool
	// OnDrop turns a drop into an intent for the application.
	OnDrop func(d input.Drop) gunim.Intent
	// Hint, when set, says what a drop would do, as the drag moves over the
	// target, for the picture the drag carries to show, such as a [DropHint].
	Hint func(e input.DragOver) any

	child gunim.Node
	glow  *anim.Float
}

// NewDropTarget returns child, taking drops.
func NewDropTarget(child gunim.Node) *DropTarget {
	t := &DropTarget{child: child, glow: anim.NewFloat(0)}
	t.Add(t.glow)
	return t
}

// Children implements [gunim.Composite].
func (t *DropTarget) Children() []gunim.Node { return []gunim.Node{t.child} }

func (t *DropTarget) takes(data any, paths []string) bool {
	return t.Accept == nil || t.Accept(data, paths)
}

// Handle implements [gunim.Handler].
func (t *DropTarget) Handle(e input.Event, u *gunim.UI) bool {
	th := u.Theme()
	switch e := e.(type) {
	case input.DragOver:
		if !t.takes(e.Data, nil) {
			return false
		}
		if t.Hint != nil {
			u.AnswerDrag(t.Hint(e))
		}
		t.glow.Animate(1, Quick.Get(th))
	case input.DragLeave:
		t.glow.Animate(0, Settle.Get(th))
	case input.Drop:
		if !t.takes(e.Data, e.Paths) {
			return false
		}
		t.glow.Animate(0, Settle.Get(th))
		if t.OnDrop != nil {
			u.Send(t, t.OnDrop(e))
		}
	default:
		return false
	}
	return true
}

// Layout implements [gunim.Node].
func (t *DropTarget) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	k := kids.At(0)
	s := k.Layout(c)
	k.Place(geom.Point{})
	return s
}

// Paint implements [gunim.Node].
func (t *DropTarget) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
	if g := t.glow.Value(); g > 0.01 {
		c := Accent.Get(f.Theme)
		c.A = uint8(float32(c.A) * min(g, 1))
		p.RRectStroke(geom.Rect{Max: box.Point()}.Inset(geom.Uniform(1)), CardRadius.Get(f.Theme),
			paint.Fill{}, paint.Stroke{Width: 2, Color: c})
	}
}
