package widget

import (
	"slices"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// A Key identifies an item across updates.
//
// Keys are what turn a fresh slice of data into animation. With them,
// [Sync] can tell an item that arrived from one that moved, so a new
// row grows into place, a departed row collapses while its neighbours
// close the gap, and everything else springs to its new position.
// Reach for whatever your data already uses: a database ID, a path, a
// job name.
type Key string

// List lays rows out in a column and animates every change to the set.
//
// It is the piece that makes published state worth animating. Hand it a
// slice with [Sync] and it works out the difference against what is on
// screen, rather than rebuilding and letting the result pop.
//
// Its look and motion come from the theme: [ListSpacing] between rows,
// [Quick] to carry rows in and to their new places, and [Settle] to
// carry them out, gentler because something on its way out should get
// out of the way.
type List struct {
	anim.Group

	// Reorder, when set, lets the pointer drag rows into a new order,
	// and turns the order a drop leaves into an intent for the
	// application. See [List.Handle].
	Reorder func(keys []Key) gunim.Intent
	// OnClick, when set, turns a click on a row that nothing inside the
	// row takes into an intent, so a row can be both clicked and dragged.
	OnClick func(key Key) gunim.Intent

	rows   map[Key]*row
	order  []Key
	height float32

	// slots holds each row's place from the last layout, for finding
	// the row under the pointer.
	slots map[Key][2]float32
	// drag is the row the pointer is dragging, and lift carries it up
	// off the list and back down.
	drag reorder
	lift *anim.Float
	// spacing is the gap between rows at the last layout.
	spacing float32
}

// NewList returns an empty list.
func NewList() *List {
	l := &List{rows: map[Key]*row{}, slots: map[Key][2]float32{}, lift: anim.NewFloat(0)}
	l.Add(l.lift)
	return l
}

// Len returns how many rows are on screen, counting those animating
// out.
func (l *List) Len() int { return len(l.order) }

// Height returns the content height from the last layout, including
// rows part way through collapsing. A scroll container reads it, and it
// moves smoothly while the list changes because the rows it measures
// are themselves springing.
func (l *List) Height() float32 { return l.height }

// Keys returns the rows in display order, including any animating out.
func (l *List) Keys() []Key { return slices.Clone(l.order) }

// Row returns the node rendering key.
func (l *List) Row(key Key) (gunim.Node, bool) {
	r, ok := l.rows[key]
	if !ok {
		return nil, false
	}
	return r.child, true
}

// RowOf returns the node rendering key, typed. Use it from a patch
// handler to reach one row and retarget a value on it.
//
//	if b, ok := widget.RowOf[*widget.Button](l, key); ok { ... }
func RowOf[N gunim.Node](l *List, key Key) (N, bool) {
	var zero N
	n, ok := l.Row(key)
	if !ok {
		return zero, false
	}
	typed, ok := n.(N)
	return typed, ok
}

// Sync reconciles the list against items.
//
// key gives each item its identity, build makes a node for an item
// arriving for the first time, and update refreshes one that is already
// on screen. update gets the UI, so it can animate with the theme's
// motion. It may be nil for rows that render from the node they were
// built with.
//
//	widget.Sync(l, u, s.Jobs,
//	    func(j Job) widget.Key { return widget.Key(j.ID) },
//	    newJobRow,
//	    (*jobRow).Set)
//
// An item that comes back while its row is still animating out revives
// that row, so a list that flickers between two states stays smooth
// instead of restarting.
func Sync[T any, N gunim.Node](l *List, u *gunim.UI, items []T,
	key func(T) Key, build func(T) N, update func(N, T, *gunim.UI),
) {
	staying := make(map[Key]bool, len(items))
	next := make([]Key, 0, len(items))

	for _, item := range items {
		k := key(item)
		if staying[k] {
			continue // a duplicate key: the first one wins
		}
		staying[k] = true
		next = append(next, k)

		if r, ok := l.rows[k]; ok {
			// Inserting a row that is on its way out reverses it, and
			// the springs keep the velocity they had.
			u.Insert(l, r)
			if update != nil {
				if typed, ok := r.child.(N); ok {
					update(typed, item, u)
				}
			}
			continue
		}

		r := newRow(k, build(item))
		l.rows[k] = r
		u.Insert(l, r)
	}

	leaving := make(map[Key]bool)
	for _, k := range l.order {
		if staying[k] {
			continue
		}
		leaving[k] = true
		if r, ok := l.rows[k]; ok {
			u.Remove(r)
		}
	}

	l.order = mergeOrder(l.order, next, leaving)
}

// mergeOrder returns next with the keys that are leaving kept in the
// places they held, so a row collapses where it sat rather than jumping
// to the end first.
func mergeOrder(prev, next []Key, leaving map[Key]bool) []Key {
	out := make([]Key, 0, len(prev)+len(next))
	added := make(map[Key]bool, len(prev)+len(next))
	add := func(k Key) {
		if !added[k] {
			out = append(out, k)
			added[k] = true
		}
	}

	i := 0
	for _, k := range prev {
		if leaving[k] {
			add(k)
			continue
		}
		// k stays, so emit every new key up to and including it.
		for i < len(next) {
			n := next[i]
			i++
			add(n)
			if n == k {
				break
			}
		}
	}
	for ; i < len(next); i++ {
		add(next[i])
	}
	return out
}

// Layout implements [gunim.Node].
func (l *List) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	spacing := ListSpacing.Get(f.Theme)
	l.spacing = spacing
	move := Quick.Get(f.Theme)
	// The engine owns what exists, so reap anything it has already
	// taken out of the tree.
	live := make(map[Key]gunim.Child, kids.Len())
	for kid := range kids.All {
		if r, ok := kid.Node().(*row); ok {
			live[r.key] = kid
		}
	}
	l.order = slices.DeleteFunc(l.order, func(k Key) bool {
		if _, ok := live[k]; ok {
			return false
		}
		delete(l.rows, k)
		return true
	})

	width := c.Max.W
	y := float32(0)
	order := l.order
	if l.drag.active {
		order = l.dropOrder()
	}
	clear(l.slots)
	for _, k := range order {
		kid, ok := live[k]
		if !ok {
			continue
		}
		r := l.rows[k]

		// Presence comes from the engine, so a row learns it is leaving
		// without having to track it.
		r.presence = kid.Presence()

		size := kid.Layout(gunim.Constraints{
			Min: geom.Sz(width, 0),
			Max: geom.Sz(width, c.Max.H),
		})

		// The target is where the row belongs; the spring says where it
		// is right now. That gap is the movement animation. A row being
		// dragged goes where the pointer has it.
		if l.drag.active && k == l.drag.key {
			r.y.Jump(l.drag.y)
		} else {
			r.y.Animate(y, move)
		}
		kid.Place(geom.Pt(0, r.y.Value()))
		l.slots[k] = [2]float32{y, size.H}

		y += size.H + closing(spacing, size.H, r)
	}
	if y > 0 {
		y -= spacing
	}
	l.height = y
	return c.Constrain(geom.Sz(width, y))
}

// Paint implements [gunim.Node]. A row that is lifted, being dragged or
// settling from a drop, is drawn last, over the rest, with a shadow.
func (l *List) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	var lifted gunim.Child
	up := false
	for kid := range kids.All {
		if r, ok := kid.Node().(*row); ok && r.key == l.drag.key && l.lift.Value() > 0.001 {
			lifted, up = kid, true
			continue
		}
		kid.Paint(p)
	}
	if up {
		l.paintLifted(p, f, box, lifted)
	}
}

// row wraps one item so the list can animate its arrival, its departure
// and the space it takes up.
//
// It exists because a departing row has to hold its place while it
// collapses. The engine keeps a node until its [gunim.Transitioner]
// reports settled, and row is what reports that, whatever the item
// inside it happens to be.
type row struct {
	anim.Group

	key   Key
	child gunim.Node

	// y is the row's top edge, springing toward the position the list
	// works out for it.
	y *anim.Float
	// height carries the row open on arrival and closed on departure,
	// which is what makes neighbours slide up to fill the gap.
	height *anim.Float
	// fade and slide carry the contents.
	fade  *anim.Float
	slide *anim.Float

	presence gunim.Presence
	// inner is the item's own height at the last layout.
	inner float32
	// instant is set for a row a virtual list builds or drops as it
	// scrolls, which appears and goes at once: the change is in what
	// is in view, where nothing arrived or left.
	instant bool
	// laid is set by the row's first layout.
	laid bool
}

func newRow(key Key, child gunim.Node) *row {
	r := &row{
		key:    key,
		child:  child,
		y:      anim.NewFloat(0),
		height: anim.NewFloat(0),
		fade:   anim.NewFloat(0),
		slide:  anim.NewFloat(0),
	}
	r.Add(r.y, r.height, r.fade, r.slide)
	return r
}

// Children implements [gunim.Composite], so inserting a row brings the
// item with it.
func (r *row) Children() []gunim.Node { return []gunim.Node{r.child} }

// Transition implements [gunim.Transitioner].
//
// The row reports settled once its contents and its height have both
// come to rest, which is what holds its place in the tree long enough
// for the gap to close.
func (r *row) Transition(p gunim.Presence, f gunim.Frame) bool {
	if r.instant {
		switch p {
		case gunim.Entering:
			r.fade.Jump(1)
			r.slide.Jump(1)
		case gunim.Exiting, gunim.Present:
		}
		return true
	}
	switch p {
	case gunim.Entering:
		r.fade.Animate(1, Quick.Get(f.Theme))
		r.slide.Animate(1, Quick.Get(f.Theme))
	case gunim.Exiting:
		r.fade.Animate(0, Settle.Get(f.Theme))
		r.slide.Animate(0, Settle.Get(f.Theme))
	case gunim.Present:
		// Settled, with the arrival already behind it.
	}
	return !r.fade.Active() && !r.height.Active()
}

// Layout implements [gunim.Node].
func (r *row) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	inner := kid.Layout(gunim.Constraints{
		Min: geom.Sz(c.Max.W, 0),
		Max: geom.Sz(c.Max.W, c.Max.H),
	})
	kid.Place(geom.Point{})

	// Open toward the item's own height, and closed while leaving. The
	// list reads the result as this row's height, so the rows below it
	// move as it goes.
	r.inner = inner.H
	target := inner.H
	spring := Quick.Get(f.Theme)
	if r.presence == gunim.Exiting {
		target, spring = 0, Settle.Get(f.Theme)
	}
	if r.instant && !r.laid {
		r.height.Jump(target)
	}
	r.laid = true
	r.height.Animate(target, spring)

	return geom.Sz(c.Max.W, r.height.Value())
}

// Paint implements [gunim.Node].
func (r *row) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	t := r.fade.Value()
	if t <= 0 || box.H <= 0 {
		return
	}
	// At rest, the row draws its item as it is. The layer is for the
	// fade and the clip while it opens and closes, and each one ends a
	// batch of drawing.
	if t >= 1 && r.slide.Value() >= 1 && box.H >= r.inner {
		kids.At(0).Paint(p)
		return
	}
	// Clipping to the animated height keeps the item its own size while
	// the row closes over it, so the contents slide out of view instead
	// of squashing.
	defer p.Layer(paint.LayerOpts{
		Bounds:  geom.Rect{Max: box.Point()},
		Opacity: t,
		Clip:    true,
		Radius:  RowRadius.Get(f.Theme),
	})()
	defer p.Push(paint.Translate(geom.Pt(-16*(1-r.slide.Value()), 0)))()
	kids.At(0).Paint(p)
}
