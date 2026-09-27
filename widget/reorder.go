package widget

import (
	"slices"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// reorder is a row the pointer holds in a [List].
type reorder struct {
	// key is the row. pressed is set from the press, and active once
	// the pointer has moved far enough to pick the row up.
	key     Key
	pressed bool
	active  bool
	// from is where the press was, grab how far below the row's top,
	// and y where the row's top is now, all in the list's space.
	from, grab, y float32
}

// pickUp is how far the pointer moves before it picks a row up, so a
// click stays a click.
const pickUp = 4

// Handle implements [gunim.Handler]. With Reorder set, a press on a row
// that nothing inside the row takes, and a move of a few pixels, picks
// the row up: it lifts, follows the pointer, and the other rows spring
// aside to open a gap where it would land. Letting go drops it into the
// gap and sends Reorder the new order. With OnClick set, a press let go
// on the row it began on without picking it up is a click.
func (l *List) Handle(e input.Event, u *gunim.UI) bool {
	if l.Reorder == nil && l.OnClick == nil {
		return false
	}
	th := u.Theme()
	switch e := e.(type) {
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		k, top, ok := l.rowAt(e.Pos.Y)
		if !ok {
			return false
		}
		l.drag = reorder{key: k, pressed: true, from: e.Pos.Y, grab: e.Pos.Y - top, y: top}
	case input.PointerMove:
		if !l.drag.pressed {
			return false
		}
		if l.Reorder != nil && !l.drag.active && abs32(e.Pos.Y-l.drag.from) >= pickUp {
			l.drag.active = true
			l.lift.Animate(1, Quick.Get(th))
		}
		if l.drag.active {
			l.drag.y = e.Pos.Y - l.drag.grab
		}
	case input.PointerUp:
		if !l.drag.pressed {
			return false
		}
		if k, _, ok := l.rowAt(e.Pos.Y); ok && !l.drag.active && l.OnClick != nil && k == l.drag.key {
			if v := l.OnClick(k); v != nil {
				u.Send(l, v)
			}
		}
		l.drop(u)
	default:
		return false
	}
	u.Invalidate()
	return true
}

// DragHeld implements [gunim.DragHolder]: a row held up by the pointer
// scrolls the views around the list when it nears their edges.
func (l *List) DragHeld() bool { return l.drag.active }

// drop ends a drag, into the gap, and tells the application of the new
// order. The row springs into its place from where it was let go.
func (l *List) drop(u *gunim.UI) {
	if l.drag.active {
		next := l.dropOrder()
		if !slices.Equal(next, l.order) {
			l.order = next
			u.Send(l, l.Reorder(slices.Clone(next)))
		}
		l.lift.Animate(0, Settle.Get(u.Theme()))
	}
	l.drag.pressed, l.drag.active = false, false
}

// rowAt returns the row at y in the list's space, and its top.
func (l *List) rowAt(y float32) (Key, float32, bool) {
	for k, s := range l.slots {
		if y >= s[0] && y < s[0]+s[1] {
			if r, ok := l.rows[k]; ok && r.presence != gunim.Exiting {
				return k, s[0], true
			}
		}
	}
	return "", 0, false
}

// dropOrder returns the order a drop now would leave: the dragged row
// taken out and put back where its middle falls among the others.
func (l *List) dropOrder() []Key {
	k := l.drag.key
	h := l.slots[k][1]
	middle := l.drag.y + h/2
	out := make([]Key, 0, len(l.order))
	placed := false
	y := float32(0)
	for _, o := range l.order {
		if o == k {
			continue
		}
		oh := l.slots[o][1]
		if !placed && middle < y+oh/2 {
			out = append(out, k)
			placed = true
		}
		out = append(out, o)
		y += oh + l.spacing
	}
	if !placed {
		out = append(out, k)
	}
	return out
}

// paintLifted draws a lifted row: a little larger, over a shadow that
// deepens as it rises.
func (l *List) paintLifted(p *paint.Painter, f gunim.Frame, _ geom.Size, kid gunim.Child) {
	t := min(max(l.lift.Value(), 0), 1)
	r := l.rows[l.drag.key]
	box := geom.Rc(0, r.y.Value(), kid.Size().W, kid.Size().H)
	defer p.Push(paint.Scale(1+0.02*t, box.Center()))()
	shadow := DialogShadow.Get(f.Theme)
	shadow.A = uint8(float32(shadow.A) * t)
	p.ShadowRRect(box, RowRadius.Get(f.Theme), paint.Fill{}, paint.Shadow{
		Offset: geom.Pt(0, 6*t), Blur: 18 * t, Color: shadow,
	})
	kid.Paint(p)
}
