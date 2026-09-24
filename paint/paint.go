// Package paint records what a frame should look like.
//
// A [Painter] records. Nodes append commands to it during the paint
// pass and the driver replays the finished list on the GPU. Keeping the
// two apart lets painting run while the previous frame is still on the
// wire, and lets a node's paint code be tested on its own.
//
// Every shape here is one the GPU can draw from its signed distance
// field: a rounded rectangle covers panels, buttons, inputs, focus
// rings, dividers and shadows, at any scale, and stays a single shader
// as a widget animates to a fractional size.
package paint

import (
	"image/color"
	"math"
	"slices"

	"github.com/marrasen/gunim/geom"
)

// A Painter records the draw commands for one frame.
//
// The zero Painter is ready to use. The engine hands each frame's
// Painter to the tree and then to the driver, so a node should use it
// and let it go.
type Painter struct {
	ops []Op
	// bounds holds, for each op, the part of the window it can touch.
	// A layer's covers everything drawn inside it.
	bounds []geom.Rect
	// prev and prevBounds are the frame recorded before this one, and
	// hasPrev says there was one. blurs is set when a layer blurs, and
	// prevBlurs when one did in the frame before.
	prev       []Op
	prevBounds []geom.Rect
	hasPrev    bool
	blurs      bool
	prevBlurs  bool
	// open holds the index of each layer open, innermost last.
	open  []int
	stack []Transform
	cur   Transform
	// clip is the innermost clipping layer open, or nil.
	clip *Clip
	// ready is false in a zero Painter, whose cur has never been set;
	// at reads it as the identity until then.
	ready bool
	// popFn is pop as a func value, made once, so Push allocates
	// nothing.
	popFn func()
	// rrects and texts hold the ops themselves, in blocks, so a frame
	// allocates a block now and then, never an op at a time. The blocks
	// of the frame before this one are reused; the driver is done with
	// them by then.
	rrects, prevRRects slab[RRectOp]
	texts, prevTexts   slab[TextOp]
}

// slab hands out ops from blocks it keeps from frame to frame.
type slab[T any] struct {
	blocks [][]T
	// n is how many of the current block are taken, and at which
	// block is current.
	at, n int
}

const slabBlock = 256

// take returns a zeroed T from the slab.
func (s *slab[T]) take() *T {
	if s.at < len(s.blocks) && s.n == slabBlock {
		s.at, s.n = s.at+1, 0
	}
	if s.at == len(s.blocks) {
		s.blocks = append(s.blocks, make([]T, slabBlock))
	}
	v := &s.blocks[s.at][s.n]
	s.n++
	var zero T
	*v = zero
	return v
}

// reset makes every block available again.
func (s *slab[T]) reset() { s.at, s.n = 0, 0 }

// Everything is the damage that covers the whole window.
var Everything = geom.Rect{Min: geom.Pt(-1e9, -1e9), Max: geom.Pt(1e9, 1e9)}

// Reset starts a new frame. The frame recorded so far becomes the one
// [Painter.Damage] compares against, and the buffers of the one before
// it are reused.
func (p *Painter) Reset() {
	p.hasPrev = p.ready
	p.prev, p.ops = p.ops, p.prev[:0]
	p.prevBounds, p.bounds = p.bounds, p.prevBounds[:0]
	p.prevBlurs, p.blurs = p.blurs, false
	p.prevRRects, p.rrects = p.rrects, p.prevRRects
	p.prevTexts, p.texts = p.texts, p.prevTexts
	p.rrects.reset()
	p.texts.reset()
	p.open = p.open[:0]
	p.stack = p.stack[:0]
	p.cur = Identity
	p.ready = true
	p.clip = nil
}

// Forget drops the frame Damage compares against, so the next frame's
// damage is [Everything]. Use it when the frame recorded before is not
// the one on screen.
func (p *Painter) Forget() {
	p.hasPrev = false
	p.prev = p.prev[:0]
	p.prevBounds = p.prevBounds[:0]
}

// Ops returns the recorded commands in draw order.
func (p *Painter) Ops() []Op { return p.ops }

// Damage returns the part of the window where this frame differs from
// the one recorded before it: the old and new bounds of every op that
// changed, came or went. A driver redraws just that part, so a button
// easing into its hover colour costs the button and not the window.
//
// It is [Everything] for the first frame, and for a frame that blurs
// or followed one that did, since a blur spreads a change past its
// bounds. Ops are matched in order, so a node added early in the frame
// damages everything painted after it, which costs time and never
// correctness.
func (p *Painter) Damage() geom.Rect {
	if !p.hasPrev || p.blurs || p.prevBlurs {
		return Everything
	}
	var d geom.Rect
	n := min(len(p.ops), len(p.prev))
	for i := range n {
		if p.bounds[i] != p.prevBounds[i] || !sameOp(p.ops[i], p.prev[i]) {
			d = d.Union(p.bounds[i]).Union(p.prevBounds[i])
		}
	}
	for _, b := range p.bounds[n:] {
		d = d.Union(b)
	}
	for _, b := range p.prevBounds[n:] {
		d = d.Union(b)
	}
	return d
}

// sameOp reports whether two ops draw the same thing.
func sameOp(a, b Op) bool {
	switch a := a.(type) {
	case *RRectOp:
		b, ok := b.(*RRectOp)
		if !ok {
			return false
		}
		ga, gb := a.Fill.Gradient, b.Fill.Gradient
		if (ga == nil) != (gb == nil) || (ga != nil && *ga != *gb) {
			return false
		}
		a2, b2 := *a, *b
		a2.Fill.Gradient, b2.Fill.Gradient = nil, nil
		return a2 == b2
	case *TextOp:
		b, ok := b.(*TextOp)
		return ok && a.Size == b.Size && a.Color == b.Color && a.Transform == b.Transform &&
			slices.Equal(a.Glyphs, b.Glyphs)
	case *ImageOp:
		b, ok := b.(*ImageOp)
		return ok && *a == *b
	case *LayerOp:
		b, ok := b.(*LayerOp)
		return ok && *a == *b
	case *LayerEndOp:
		_, ok := b.(*LayerEndOp)
		return ok
	}
	return false
}

// Transform is an affine transform, stored as the two rows of a 2x3
// matrix. Scale and translate cover almost everything a widget does;
// rotation is there for spinners and the odd flourish.
type Transform struct {
	A, B, C float32 // x' = A*x + B*y + C
	D, E, F float32 // y' = D*x + E*y + F
}

// Identity is the transform that leaves a point where it is.
var Identity = Transform{1, 0, 0, 0, 1, 0}

// Translate moves by p.
func Translate(p geom.Point) Transform { return Transform{1, 0, p.X, 0, 1, p.Y} }

// Scale scales by s about the point at.
//
// A centre is what you want nine times out of ten: a dialog growing
// into place should swell from its middle and stay where it is.
func Scale(s float32, at geom.Point) Transform {
	return Transform{s, 0, at.X * (1 - s), 0, s, at.Y * (1 - s)}
}

// Rotate turns by rad radians about the point at, clockwise on screen.
func Rotate(rad float32, at geom.Point) Transform {
	s, c := float32(math.Sin(float64(rad))), float32(math.Cos(float64(rad)))
	return Transform{
		c, -s, at.X - c*at.X + s*at.Y,
		s, c, at.Y - s*at.X - c*at.Y,
	}
}

// Mul returns t applied after u.
func (t Transform) Mul(u Transform) Transform {
	return Transform{
		A: t.A*u.A + t.B*u.D,
		B: t.A*u.B + t.B*u.E,
		C: t.A*u.C + t.B*u.F + t.C,
		D: t.D*u.A + t.E*u.D,
		E: t.D*u.B + t.E*u.E,
		F: t.D*u.C + t.E*u.F + t.F,
	}
}

// Invert returns the transform that undoes t. It reports false when t
// has no inverse, as when it scales to zero and folds the plane onto a
// line.
func (t Transform) Invert() (Transform, bool) {
	det := t.A*t.E - t.B*t.D
	if det == 0 {
		return Transform{}, false
	}
	a, b := t.E/det, -t.B/det
	d, e := -t.D/det, t.A/det
	return Transform{
		A: a, B: b, C: -(a*t.C + b*t.F),
		D: d, E: e, F: -(d*t.C + e*t.F),
	}, true
}

// Apply maps a point through t.
func (t Transform) Apply(p geom.Point) geom.Point {
	return geom.Point{X: t.A*p.X + t.B*p.Y + t.C, Y: t.D*p.X + t.E*p.Y + t.F}
}

// at returns the transform in force. A zero Painter starts from the
// identity.
func (p *Painter) at() Transform {
	if !p.ready {
		return Identity
	}
	return p.cur
}

// Transform returns the transform in force, which maps the current
// drawing space to window space.
func (p *Painter) Transform() Transform { return p.at() }

// Push applies t on top of the current transform and returns the
// function that pops it, so a node can write:
//
//	defer p.Push(paint.Translate(origin))()
func (p *Painter) Push(t Transform) func() {
	p.stack = append(p.stack, p.at())
	p.cur = p.at().Mul(t)
	p.ready = true
	if p.popFn == nil {
		p.popFn = p.pop
	}
	return p.popFn
}

func (p *Painter) pop() {
	n := len(p.stack) - 1
	p.cur = p.stack[n]
	p.stack = p.stack[:n]
}

// LayerOpts describes an offscreen group.
type LayerOpts struct {
	// Bounds is the area the layer covers.
	Bounds geom.Rect
	// Opacity multiplies the whole group at once, so overlapping shapes
	// inside it stay opaque to one another as the group fades.
	Opacity float32
	// Blur blurs the layer's own contents. It is the standard deviation
	// of a Gaussian in logical pixels, as in CSS's blur().
	Blur float32
	// Backdrop blurs whatever is already behind the layer, within its
	// bounds, rounded by Radius when the layer clips. It is a standard
	// deviation like Blur. This is the frosted glass behind a modal,
	// and it is why layers are a first-class idea here: it needs the
	// frame so far as a texture.
	Backdrop float32
	// Clip confines drawing to Bounds, rounded by Radius.
	Clip   bool
	Radius float32
}

// Layer opens an offscreen group and returns the function that closes
// and composites it:
//
//	defer p.Layer(paint.LayerOpts{Opacity: t, Backdrop: 12 * t})()
func (p *Painter) Layer(o LayerOpts) func() {
	if o.Blur > 0 || o.Backdrop > 0 {
		p.blurs = true
	}
	p.record(&LayerOp{Opts: o, Transform: p.at()}, o.Bounds)
	at := len(p.ops) - 1
	p.open = append(p.open, at)
	outer := p.clip
	if o.Clip {
		c := &Clip{outer: outer, rect: o.Bounds, radius: o.Radius}
		c.inv, c.ok = p.at().Invert()
		p.clip = c
	}
	return func() {
		if i := slices.Index(p.open, at); i >= 0 {
			p.open = slices.Delete(p.open, i, i+1)
		}
		// The end covers what the layer does, since compositing it
		// draws there.
		p.ops = append(p.ops, &LayerEndOp{})
		p.bounds = append(p.bounds, p.bounds[at])
		p.clip = outer
	}
}

// Clip returns the clipping in force: every open layer that clips,
// innermost first. It is nil when nothing clips.
func (p *Painter) Clip() *Clip { return p.clip }

// A Clip is the area a clipping layer lets drawing through, together
// with the clips around it. It stays valid after the frame that made it,
// which lets input be tested against what the frame showed.
type Clip struct {
	outer  *Clip
	inv    Transform
	ok     bool
	rect   geom.Rect
	radius float32
}

// Contains reports whether p, a point in window space, lies inside c and
// every clip around it. A nil Clip contains every point.
func (c *Clip) Contains(p geom.Point) bool {
	for ; c != nil; c = c.outer {
		if !c.ok || !insideRounded(c.inv.Apply(p), c.rect, c.radius) {
			return false
		}
	}
	return true
}

// insideRounded reports whether p lies inside r with its corners
// rounded by radius.
func insideRounded(p geom.Point, r geom.Rect, radius float32) bool {
	if !r.Contains(p) {
		return false
	}
	radius = min(radius, (r.Max.X-r.Min.X)/2, (r.Max.Y-r.Min.Y)/2)
	if radius <= 0 {
		return true
	}
	// Distance into the corner square, from the centre of its rounding.
	dx := max(r.Min.X+radius-p.X, p.X-(r.Max.X-radius), 0)
	dy := max(r.Min.Y+radius-p.Y, p.Y-(r.Max.Y-radius), 0)
	return dx*dx+dy*dy <= radius*radius
}

// Fill describes how a shape is coloured. Exactly one of Solid or
// Gradient applies; a zero Gradient means Solid.
type Fill struct {
	Solid    color.NRGBA
	Gradient *Gradient
}

// Solid is shorthand for a flat fill.
func Solid(c color.NRGBA) Fill { return Fill{Solid: c} }

// Gradient is a linear gradient between two points.
type Gradient struct {
	From, To   geom.Point
	Start, End color.NRGBA
}

// Stroke describes an outline.
type Stroke struct {
	Width float32
	Color color.NRGBA
}

// RRectOp draws a rounded rectangle, optionally stroked, optionally
// with a drop shadow. One command covers most of a widget set.
type RRectOp struct {
	Rect      geom.Rect
	Radius    float32
	Fill      Fill
	Stroke    Stroke
	Shadow    Shadow
	Transform Transform
}

// Shadow is a soft drop shadow behind a shape. A zero Shadow is
// skipped.
type Shadow struct {
	Offset geom.Point
	Blur   float32
	Spread float32
	Color  color.NRGBA
}

// TextOp draws a run of already-shaped text.
//
// Shaping happens outside paint, in the text package, because it
// depends on the font and the script, which outlive any one frame.
// Glyphs carry fractional positions, so a label animating across the
// screen stays steady as it crosses pixel boundaries.
type TextOp struct {
	Glyphs []Glyph
	// Size is the font size in logical pixels.
	Size      float32
	Color     color.NRGBA
	Transform Transform
}

// Glyph is one positioned glyph from a shaped run.
type Glyph struct {
	// ID identifies the glyph within its face, after shaping. Shaping
	// breaks the link with runes: one rune can become several glyphs,
	// and several runes can become one.
	ID uint32
	// At is the glyph origin, with subpixel precision.
	At geom.Point
	// Face indexes the font face the glyph was shaped with.
	Face uint32
}

// An Op is one recorded draw command.
type Op interface{ isOp() }

// LayerOp opens an offscreen group; LayerEndOp composites it.
type LayerOp struct {
	Opts      LayerOpts
	Transform Transform
}

// LayerEndOp closes the most recently opened layer.
type LayerEndOp struct{}

func (*RRectOp) isOp()    {}
func (*TextOp) isOp()     {}
func (*LayerOp) isOp()    {}
func (*LayerEndOp) isOp() {}

// rrect takes an RRectOp from the painter's blocks.
func (p *Painter) rrect(op RRectOp) *RRectOp {
	v := p.rrects.take()
	*v = op
	return v
}

// RRect records a rounded rectangle.
func (p *Painter) RRect(r geom.Rect, radius float32, f Fill) {
	p.record(p.rrect(RRectOp{Rect: r, Radius: radius, Fill: f, Transform: p.at()}), r)
}

// RRectStroke records a rounded rectangle with an outline, which is
// centred on the rectangle's edge.
func (p *Painter) RRectStroke(r geom.Rect, radius float32, f Fill, s Stroke) {
	half := s.Width / 2
	p.record(p.rrect(RRectOp{Rect: r, Radius: radius, Fill: f, Stroke: s, Transform: p.at()}),
		geom.Rect{Min: geom.Pt(r.Min.X-half, r.Min.Y-half), Max: geom.Pt(r.Max.X+half, r.Max.Y+half)})
}

// ShadowRRect records a rounded rectangle with a drop shadow.
func (p *Painter) ShadowRRect(r geom.Rect, radius float32, f Fill, sh Shadow) {
	grown := geom.Rect{
		Min: geom.Pt(r.Min.X-sh.Blur-sh.Spread+sh.Offset.X, r.Min.Y-sh.Blur-sh.Spread+sh.Offset.Y),
		Max: geom.Pt(r.Max.X+sh.Blur+sh.Spread+sh.Offset.X, r.Max.Y+sh.Blur+sh.Spread+sh.Offset.Y),
	}
	p.record(p.rrect(RRectOp{Rect: r, Radius: radius, Fill: f, Shadow: sh, Transform: p.at()}), grown.Union(r))
}

// Text records a shaped run at size logical pixels. bounds is the
// area the run covers, for damage tracking. The text package's Run.Paint
// is the usual way to call it.
func (p *Painter) Text(g []Glyph, size float32, c color.NRGBA, bounds geom.Rect) {
	op := p.texts.take()
	*op = TextOp{Glyphs: g, Size: size, Color: c, Transform: p.at()}
	p.record(op, bounds)
}

// record adds op, which draws within bounds in the current space. The
// bounds are kept in window space, whatever space the node happened to
// be painting in, grown by a pixel and a half for antialiasing.
func (p *Painter) record(op Op, bounds geom.Rect) {
	t := p.at()
	first := t.Apply(bounds.Min)
	w := geom.Rect{Min: first, Max: first}
	for _, c := range [...]geom.Point{{X: bounds.Max.X, Y: bounds.Min.Y}, bounds.Max, {X: bounds.Min.X, Y: bounds.Max.Y}} {
		q := t.Apply(c)
		w.Min = geom.Pt(min(w.Min.X, q.X), min(w.Min.Y, q.Y))
		w.Max = geom.Pt(max(w.Max.X, q.X), max(w.Max.Y, q.Y))
	}
	const aa = 1.5
	w = geom.Rect{Min: geom.Pt(w.Min.X-aa, w.Min.Y-aa), Max: geom.Pt(w.Max.X+aa, w.Max.Y+aa)}
	p.ops = append(p.ops, op)
	p.bounds = append(p.bounds, w)
	// Every layer open around the op draws where it does.
	for _, i := range p.open {
		p.bounds[i] = p.bounds[i].Union(w)
	}
}
