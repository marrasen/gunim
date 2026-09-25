package widget

import (
	"slices"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// VirtualList shows a long list of items and scrolls through it,
// building only the rows in view and a little beyond. A list of a
// hundred thousand items costs about what the few dozen on screen do.
//
// Rows that scroll into view appear at once, and rows that scroll out
// are dropped. Changes to the items animate as a [List]'s do: a new
// item in view grows into place, a removed one collapses while its
// neighbours close the gap, and moved ones spring to their new places.
//
// A row's height is known once it has been laid out. Until then the
// list assumes Estimate. When a row above the view turns out taller or
// shorter than assumed, the list moves its offset by the difference,
// so what is on screen stays still.
//
// The list scrolls itself, as a [Scroll] does: the wheel and the keys
// glide it with a spring, and a thin bar shows while it moves.
type VirtualList struct {
	scrolling

	// Estimate is the height assumed for a row not yet laid out. Zero
	// means 40.
	Estimate float32
	// Spacing is the room between rows. It defaults to the theme's
	// [ListSpacing]; a table's rows sit against each other.
	Spacing theme.Token[float32]

	build func(Key) gunim.Node

	// order is the keys in display order, including rows leaving;
	// gone marks those leaving.
	order []Key
	gone  map[Key]bool
	// heights holds each row's height from the last time it was laid
	// out, spacing left out.
	heights map[Key]float32
	// live holds the rows in the tree.
	live map[Key]*row
	// animate marks the keys that arrived through SetKeys, whose rows
	// grow in when built.
	animate map[Key]bool

	// entry holds each row's height and the spacing after it, in
	// order, and tops sums them, so finding the row at an offset and
	// the offset of a row are quick however long the list. dirty says
	// the order changed and both need building again.
	entry []float32
	tops  fenwick
	dirty bool
	gap   float32
}

// anchor returns the first row at or below the top of the view, and
// where it sits in the content, when the list is scrolled from its top.
func (l *VirtualList) anchor() (Key, float32, bool) {
	offset := l.offset.Value()
	if offset <= 0 {
		return "", 0, false
	}
	var best Key
	at, found := float32(0), false
	for k, r := range l.live {
		if l.gone[k] {
			continue
		}
		if y := r.y.Target(); y >= offset && (!found || y < at) {
			best, at, found = k, y, true
		}
	}
	return best, at, found
}

// entryOf is the room a row of height h takes: its height and the
// spacing after it. The spacing closes with a row closing, r, so the
// row takes no room once it is shut.
func (l *VirtualList) entryOf(h float32, r *row) float32 {
	return h + closing(l.gap, h, r)
}

// closing returns the spacing after a row of height h: all of it,
// shrinking with a row that is opening or closing, r, in proportion to
// how open it is. r may be nil.
func closing(spacing, h float32, r *row) float32 {
	if h <= 0 {
		return 0
	}
	if r != nil && r.inner > 0 && h < r.inner {
		return spacing * h / r.inner
	}
	return spacing
}

// index builds entry and tops from the order and the known heights.
func (l *VirtualList) index() {
	l.entry = l.entry[:0]
	for _, k := range l.order {
		h := l.height(k)
		r, ok := l.live[k]
		if ok && l.gone[k] {
			h = r.height.Value()
		}
		l.entry = append(l.entry, l.entryOf(h, r))
	}
	l.tops.reset(l.entry)
	l.dirty = false
}

// fenwick sums a list of numbers by prefix, and finds where a running
// sum passes a value, each in time logarithmic in the list's length.
type fenwick struct{ t []float64 }

func (f *fenwick) reset(vals []float32) {
	f.t = f.t[:0]
	for _, v := range vals {
		f.t = append(f.t, float64(v))
	}
	for i := range f.t {
		if j := i | (i + 1); j < len(f.t) {
			f.t[j] += f.t[i]
		}
	}
}

// add adds d to value i.
func (f *fenwick) add(i int, d float64) {
	for ; i < len(f.t); i |= i + 1 {
		f.t[i] += d
	}
}

// sum returns the sum of the first n values.
func (f *fenwick) sum(n int) float64 {
	s := 0.0
	for i := n - 1; i >= 0; i = (i & (i + 1)) - 1 {
		s += f.t[i]
	}
	return s
}

// find returns the first i whose running sum through i passes y: the
// row that holds offset y. Past the end it returns the length.
func (f *fenwick) find(y float64) int {
	pos := -1
	step := 1
	for step*2 <= len(f.t) {
		step *= 2
	}
	for ; step > 0; step /= 2 {
		if next := pos + step; next < len(f.t) && f.t[next] <= y {
			pos = next
			y -= f.t[next]
		}
	}
	return pos + 1
}

// overscan is how far past the view, above and below, the list keeps
// rows built, so a short scroll finds them ready.
const overscan = 300

// NewVirtualList returns an empty list that builds a row's node with
// build when the row comes into view.
func NewVirtualList(build func(Key) gunim.Node) *VirtualList {
	l := &VirtualList{
		scrolling: newScrolling(),
		Spacing:   ListSpacing,
		build:     build,
		gone:      map[Key]bool{},
		heights:   map[Key]float32{},
		live:      map[Key]*row{},
		animate:   map[Key]bool{},
	}
	return l
}

// Len returns how many items the list holds, leaving rows left out.
func (l *VirtualList) Len() int { return len(l.order) - len(l.gone) }

// Built returns how many rows are built right now.
func (l *VirtualList) Built() int { return len(l.live) }

// SetKeys makes keys the list's items, in order. Call it from a view's
// update function. Rows already built stay built and spring to their
// new places; a new item in view grows in, and a removed one in view
// collapses. Changes out of view take effect at once.
func (l *VirtualList) SetKeys(keys []Key, u *gunim.UI) {
	staying := make(map[Key]bool, len(keys))
	next := make([]Key, 0, len(keys))
	for _, k := range keys {
		if staying[k] {
			continue // a duplicate key: the first one wins
		}
		staying[k] = true
		next = append(next, k)
	}
	first := len(l.order) == 0
	leaving := map[Key]bool{}
	for _, k := range l.order {
		if staying[k] {
			if r, ok := l.live[k]; ok && l.gone[k] {
				// Back while collapsing: it opens again.
				u.Insert(l, r)
				delete(l.gone, k)
			}
			continue
		}
		if r, ok := l.live[k]; ok {
			leaving[k] = true
			l.gone[k] = true
			// A removed item collapses, even in a row that appeared at
			// once.
			r.instant = false
			u.Remove(r)
			continue
		}
		delete(l.heights, k)
	}
	known := make(map[Key]bool, len(l.order))
	for _, k := range l.order {
		known[k] = true
	}
	for _, k := range next {
		if !known[k] && !first {
			l.animate[k] = true
		}
	}
	l.order = mergeOrder(l.order, next, leaving)
	l.dirty = true
	u.Invalidate()
}

// ScrollToKey glides the list so key's row is at the top of the view,
// or as near as the list's end allows.
func (l *VirtualList) ScrollToKey(key Key, u *gunim.UI) {
	spacing := l.Spacing.Get(u.Theme())
	y := float32(0)
	for _, k := range l.order {
		if k == key {
			l.ScrollTo(y, Quick.Get(u.Theme()))
			return
		}
		y += l.height(k) + spacing
	}
}

func (l *VirtualList) height(k Key) float32 {
	if h, ok := l.heights[k]; ok {
		return h
	}
	if l.Estimate > 0 {
		return l.Estimate
	}
	return 40
}

// Handle implements [gunim.Handler].
func (l *VirtualList) Handle(e input.Event, u *gunim.UI) bool { return l.handle(e, u) }

// Reveal implements [gunim.Revealer]: it scrolls just far enough to
// bring r, in the list's own space, into view.
func (l *VirtualList) Reveal(r geom.Rect, u *gunim.UI) { l.reveal(r, u) }

// EdgeScroll implements [gunim.EdgeScroller]: a drag held near the top
// or the bottom scrolls the list.
func (l *VirtualList) EdgeScroll(p geom.Point, dt time.Duration, u *gunim.UI) geom.Point {
	return l.edgeScroll(p, dt, u)
}

// Layout implements [gunim.Node].
//
// It walks the keys once, adding up heights, and builds a row for each
// key in view that has none, drops the rows that have scrolled away,
// and lays out and places the rest.
func (l *VirtualList) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	own := c.Max
	width := own.W
	spacing := l.Spacing.Get(f.Theme)
	move := Quick.Get(f.Theme)
	l.th, l.viewport = f.Theme, own.H

	// Rows the engine has taken out are gone for good.
	children := make(map[*row]gunim.Child, kids.Len())
	for kid := range kids.All {
		if r, ok := kid.Node().(*row); ok {
			children[r] = kid
		}
	}
	for k, r := range l.live {
		if _, ok := children[r]; !ok {
			delete(l.live, k)
			if l.gone[k] {
				delete(l.gone, k)
				delete(l.heights, k)
				l.order = deleteKey(l.order, k)
				l.dirty = true
			}
		}
	}
	if l.dirty || spacing != l.gap {
		// Items that come or go above the view shift what is in it.
		// Hold the first row in view where it is, unless the list is at
		// its top, where a new item should push in where it shows.
		anchor, at, ok := l.anchor()
		l.gap = spacing
		l.index()
		if ok {
			if i := slices.Index(l.order, anchor); i >= 0 {
				if d := float32(l.tops.sum(i)) - at; d != 0 {
					l.shift(d)
					// The rows move by as much, so none moves on screen.
					for _, r := range l.live {
						anim.Shift(r.y, d)
					}
				}
			}
		}
	}

	offset := l.offset.Value()
	top, bottom := offset-overscan, offset+own.H+overscan
	keep := make(map[Key]bool, len(l.live))
	i := l.tops.find(float64(max(top, 0)))
	// A row closed to nothing just above takes no room, and may be one
	// growing in: lay it out too.
	for i > 0 && l.entry[i-1] == 0 {
		i--
	}
	y := float32(l.tops.sum(i))
	// carry is how far the rows after one above the view that changed
	// height have moved, along with the offset, this frame.
	var carry float32
	for ; i < len(l.order) && y <= bottom; i++ {
		k := l.order[i]
		r, built := l.live[k]
		if built && carry != 0 {
			anim.Shift(r.y, carry)
		}
		if !built {
			if l.gone[k] {
				y += l.entry[i]
				continue
			}
			r = newRow(k, l.build(k))
			r.instant = !l.animate[k]
			if r.instant {
				r.fade.Jump(1)
				r.slide.Jump(1)
			}
			delete(l.animate, k)
			r.y.Jump(y)
			l.live[k] = r
			children[r] = kids.Build(r)
		}
		keep[k] = true
		kid := children[r]
		r.presence = kid.Presence()
		size := kid.Layout(gunim.Constraints{Min: geom.Sz(width, 0), Max: geom.Sz(width, 0)})
		if !l.gone[k] {
			l.heights[k] = r.inner
		}
		if e := l.entryOf(size.H, r); e != l.entry[i] {
			d := e - l.entry[i]
			// Above the view means starting above it: a row at its top
			// edge that grows is one growing into view.
			above := y < offset && y+l.entry[i] <= offset
			l.entry[i] = e
			l.tops.add(i, float64(d))
			if above {
				// A row above the view changed height: move the offset
				// with it, so what is on screen stays still.
				offset += d
				l.shift(d)
				carry += d
			}
		}
		r.y.Animate(y, move)
		kid.Place(geom.Pt(0, r.y.Value()-offset))
		y += l.entry[i]
	}
	// Rows that have scrolled away go at once.
	for k, r := range l.live {
		if !keep[k] && !l.gone[k] {
			r.instant = true
			kids.Drop(r)
			delete(l.live, k)
		}
	}
	l.fit(max(0, float32(l.tops.sum(len(l.order)))-spacing), own.H, f.Theme)
	return own
}

// Paint implements [gunim.Node].
func (l *VirtualList) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	func() {
		defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: 1, Clip: true})()
		offset := l.offset.Value()
		for kid := range kids.All {
			r, ok := kid.Node().(*row)
			if !ok {
				continue
			}
			// Only what reaches the view is drawn; the rows built
			// beyond it wait there for a scroll.
			top := r.y.Value() - offset
			if top > box.H || top+kid.Size().H < 0 {
				continue
			}
			kid.Paint(p)
		}
	}()
	l.paintBar(p, f, box)
}

func deleteKey(keys []Key, k Key) []Key {
	for i, o := range keys {
		if o == k {
			return append(keys[:i], keys[i+1:]...)
		}
	}
	return keys
}
