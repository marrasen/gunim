package widget

import (
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Axis is the direction a [Flex] lays its children along.
type Axis uint8

const (
	// Horizontal lays children out left to right, as a row.
	Horizontal Axis = iota
	// Vertical lays children out top to bottom, as a column.
	Vertical
)

// Justify places children along a [Flex]'s main axis when they leave
// space over.
type Justify uint8

const (
	// JustifyStart packs children against the start.
	JustifyStart Justify = iota
	// JustifyCenter packs them in the middle.
	JustifyCenter
	// JustifyEnd packs them against the end.
	JustifyEnd
	// JustifyBetween spreads the space over between them.
	JustifyBetween
)

// Cross places each child across a [Flex]'s main axis.
type Cross uint8

const (
	// CrossStart puts a child against the start of the cross axis: the
	// top of a row, the left of a column.
	CrossStart Cross = iota
	// CrossCenter centres it.
	CrossCenter
	// CrossEnd puts it against the end.
	CrossEnd
	// CrossStretch makes it fill the cross axis.
	CrossStretch
)

// Flex lays its children out in a row or a column.
//
// A child can grow to share the space over, in proportion to the weight
// [Flex.Grow] gives it. When the children want more room than a
// bounded Flex has, those that do not grow shrink in proportion to their
// natural size, as in CSS flexbox, a label no narrower than its widest
// word while the others can give room: each is laid out again within
// its share, so a label wraps and a button shortens its label. A growing
// child then gets a sliver. A child that will not shrink, such as one
// with a fixed size, ends at the Flex's far edge, so every child stays
// inside, where a click can reach it. When a child arrives or leaves, the others
// spring to their new places with the theme's [Reflow] motion; when the
// Flex itself changes size, as a window does while it is resized, they
// move with it at once, so nothing trails behind the edge being dragged.
type Flex struct {
	Axis    Axis
	Justify Justify
	Cross   Cross
	// Gap is the space between children. It defaults to the theme's
	// [Gap]; set it to a token of your own to change it.
	Gap theme.Token[float32]

	kids []gunim.Node
	grow map[gunim.Node]float32
	// at springs each child to its place; last is the size the Flex
	// was last given, so a change of it moves children at once.
	at   map[gunim.Node]*anim.Point
	last gunim.Constraints
	laid bool
}

// Row returns a Flex that lays kids out left to right. Kids too wide
// for the row together shrink in proportion to their width, each down
// to its [Shrinker.MinWidth] where it has one.
func Row(kids ...gunim.Node) *Flex { return newFlex(Horizontal, kids) }

// Column returns a Flex that lays kids out top to bottom. Kids too tall
// for the column together shrink in proportion to their height. A
// [Shrinker] counts in a row alone: a column shrinks its kids by height.
func Column(kids ...gunim.Node) *Flex { return newFlex(Vertical, kids) }

func newFlex(axis Axis, kids []gunim.Node) *Flex {
	return &Flex{
		Axis: axis,
		Gap:  Gap,
		kids: kids,
		grow: map[gunim.Node]float32{},
		at:   map[gunim.Node]*anim.Point{},
	}
}

// Grow makes n take a share of the space the other children leave, in
// proportion to weight, and returns the Flex. A child that grows is laid
// out at exactly its share.
func (f *Flex) Grow(n gunim.Node, weight float32) *Flex {
	f.grow[n] = weight
	return f
}

// Children implements [gunim.Composite], so a Flex arrives with the
// children it was built with. More can be added with UI.Insert.
func (f *Flex) Children() []gunim.Node { return f.kids }

// Step implements [gunim.Animator].
func (f *Flex) Step(dt time.Duration) bool {
	moving := false
	for _, p := range f.at {
		if p.Step(dt) {
			moving = true
		}
	}
	return moving
}

// main and cross split a size or point along the Flex's axes.
func (f *Flex) main(s geom.Size) float32 {
	if f.Axis == Horizontal {
		return s.W
	}
	return s.H
}

func (f *Flex) cross(s geom.Size) float32 {
	if f.Axis == Horizontal {
		return s.H
	}
	return s.W
}

// size builds a size, or a point, from a main and a cross extent.
func (f *Flex) size(m, c float32) geom.Size {
	if f.Axis == Horizontal {
		return geom.Sz(m, c)
	}
	return geom.Sz(c, m)
}

// squeezed is the least room a child is given on a bounded main axis, so
// it sees a bound to fit, however little space is left: a main-axis
// maximum of 0 means unbounded.
const squeezed = 1.0 / 64

// Layout implements [gunim.Node].
//
// A zero maximum on the main axis means unbounded, as inside a scroll
// view: children then get their natural size, and growing children get
// nothing extra.
func (f *Flex) Layout(c gunim.Constraints, fr gunim.Frame, kids gunim.Children) geom.Size {
	gap := f.Gap.Get(fr.Theme)
	maxMain, maxCross := f.main(c.Max), f.cross(c.Max)
	n := kids.Len()

	// Measure the children that keep their natural size, and total the
	// weight of those that grow.
	sizes := make([]geom.Size, n)
	var used, natural, weight float32
	for i := range n {
		kid := kids.At(i)
		if w := f.grow[kid.Node()]; w > 0 && maxMain > 0 {
			weight += w
			continue
		}
		sizes[i] = kid.Layout(f.childConstraints(0, maxCross))
		natural += f.main(sizes[i])
	}
	gaps := float32(0)
	if n > 1 {
		gaps = gap * float32(n-1)
	}
	used = natural + gaps

	// On a bounded main axis, children that do not fit shrink in
	// proportion to their natural size, and are laid out again within
	// their share: a label wraps, a button shortens its label.
	if maxMain > 0 && used > maxMain && natural > 0 {
		rooms := f.shrink(kids, sizes, fr, max(0, maxMain-gaps))
		used = gaps
		for i := range n {
			kid := kids.At(i)
			if f.grow[kid.Node()] > 0 {
				continue
			}
			sizes[i] = kid.Layout(f.within(max(squeezed, rooms[i]), maxCross))
			used += f.main(sizes[i])
		}
	}

	// Share out what is left among the children that grow.
	free := max(0, maxMain-used)
	for i := range n {
		kid := kids.At(i)
		if w := f.grow[kid.Node()]; w > 0 && maxMain > 0 {
			share := max(squeezed, free*w/weight)
			sizes[i] = kid.Layout(f.childConstraints(share, maxCross))
			used += f.main(sizes[i])
		}
	}

	// The Flex fills a bounded main axis when it has space to hand out
	// or to spread; otherwise it hugs its children.
	var crossSize float32
	for _, s := range sizes {
		crossSize = max(crossSize, f.cross(s))
	}
	if f.Cross == CrossStretch && maxCross > 0 {
		crossSize = maxCross
	}
	mainSize := used
	if maxMain > 0 && (weight > 0 || f.Justify != JustifyStart) {
		mainSize = maxMain
	}
	own := c.Constrain(f.size(mainSize, crossSize))

	pos, step := f.justify(f.main(own)-used, n)
	f.place(c, fr, kids, sizes, own, pos, gap+step)
	f.last, f.laid = c, true
	return own
}

// Shrinker is a node that knows the least width it takes in a row
// before it looks broken, such as a [Label]'s widest word. A row short
// of room squeezes it no narrower than MinWidth, where the others can
// give room, and lays it out again at its share. An app's node says how
// narrow it can go by implementing it.
type Shrinker interface {
	// MinWidth returns the least width the node takes, in logical
	// pixels, in the theme of f.
	MinWidth(f gunim.Frame) float32
}

var _ Shrinker = (*Label)(nil)

// shrink shares room among the children that do not grow, given their
// natural sizes, as CSS flexbox does: each shrinks in proportion to its
// size, down to its least width, and those at their least leave the rest
// to shrink further. Where the least widths alone are too wide, every
// child shrinks in proportion to its size.
func (f *Flex) shrink(kids gunim.Children, sizes []geom.Size, fr gunim.Frame, room float32) []float32 {
	n := len(sizes)
	rooms := make([]float32, n)
	least := make([]float32, n)
	var natural, floor float32
	for i := range n {
		if f.grow[kids.At(i).Node()] > 0 {
			continue
		}
		natural += f.main(sizes[i])
		if s, ok := kids.At(i).Node().(Shrinker); ok && f.Axis == Horizontal {
			least[i] = min(s.MinWidth(fr), f.main(sizes[i]))
		}
		floor += least[i]
	}
	if floor >= room {
		for i := range n {
			rooms[i] = f.main(sizes[i]) * room / natural
		}
		return rooms
	}
	frozen := make([]bool, n)
	for {
		// Share what the children at their least leave among the rest.
		left, flexible := room, float32(0)
		for i := range n {
			if frozen[i] {
				left -= rooms[i]
			} else {
				flexible += f.main(sizes[i])
			}
		}
		froze := false
		for i := range n {
			if frozen[i] {
				continue
			}
			rooms[i] = 0
			if flexible > 0 {
				rooms[i] = f.main(sizes[i]) * left / flexible
			}
			if rooms[i] < least[i] {
				rooms[i], frozen[i], froze = least[i], true, true
			}
		}
		if !froze {
			return rooms
		}
	}
}

// within are the constraints for a child that does not grow, squeezed to
// at most main long.
func (f *Flex) within(main, maxCross float32) gunim.Constraints {
	cs := f.childConstraints(0, maxCross)
	cs.Max = f.size(main, f.cross(cs.Max))
	return cs
}

// childConstraints are what a child is laid out within: exactly main
// long when main is set, and within the cross extent.
func (f *Flex) childConstraints(main, maxCross float32) gunim.Constraints {
	var cs gunim.Constraints
	cs.Max = f.size(main, maxCross)
	if main > 0 {
		cs.Min = f.size(main, 0)
	}
	if f.Cross == CrossStretch && maxCross > 0 {
		cs.Min = f.size(f.main(cs.Min), maxCross)
	}
	return cs
}

// justify returns where the first child starts along the main axis,
// and the extra space between each pair, given the space left over.
func (f *Flex) justify(spare float32, n int) (start, between float32) {
	if spare <= 0 {
		return 0, 0
	}
	switch f.Justify {
	case JustifyCenter:
		return spare / 2, 0
	case JustifyEnd:
		return spare, 0
	case JustifyBetween:
		if n > 1 {
			return 0, spare / float32(n-1)
		}
	case JustifyStart:
	}
	return 0, 0
}

// place positions each child, springing it there when the Flex kept its
// size and a child moved, and jumping when the Flex itself was resized.
func (f *Flex) place(c gunim.Constraints, fr gunim.Frame, kids gunim.Children, sizes []geom.Size, own geom.Size, pos, step float32) {
	resized := !f.laid || c != f.last
	motion := Reflow.Get(fr.Theme)
	seen := make(map[gunim.Node]bool, kids.Len())
	for i := range kids.Len() {
		kid := kids.At(i)
		s := sizes[i]
		var cross float32
		switch f.Cross {
		case CrossCenter:
			cross = (f.cross(own) - f.cross(s)) / 2
		case CrossEnd:
			cross = f.cross(own) - f.cross(s)
		case CrossStart, CrossStretch:
		}
		// A child that would end past the Flex's edge, where it can be
		// seen and never clicked, comes back to end at the edge.
		at := pos
		if end := f.main(own) - f.main(s); at > end {
			at = max(0, end)
		}
		target := f.size(at, cross).Point()

		node := kid.Node()
		seen[node] = true
		p, ok := f.at[node]
		switch {
		case !ok:
			p = anim.NewPoint(target)
			f.at[node] = p
		case resized:
			p.Jump(target)
		default:
			p.Animate(target, motion)
		}
		kid.Place(p.Value())
		pos += f.main(s) + step
	}
	// Forget children that have left.
	for node := range f.at {
		if !seen[node] {
			delete(f.at, node)
		}
	}
}

// Paint implements [gunim.Node].
func (f *Flex) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for kid := range kids.All {
		kid.Paint(p)
	}
}

// Spacer is an empty node, for a [Flex] to grow into the space between
// children.
//
//	sp := widget.NewSpacer()
//	row := widget.Row(title, sp, button).Grow(sp, 1)
type Spacer struct {
	// Size is the space it takes when it does not grow.
	Size geom.Size
}

// NewSpacer returns a spacer of no size of its own.
func NewSpacer() *Spacer { return &Spacer{} }

// Layout implements [gunim.Node].
func (s *Spacer) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(s.Size)
}

// Paint implements [gunim.Node].
func (s *Spacer) Paint(*paint.Painter, gunim.Frame, geom.Size, gunim.Children) {}
