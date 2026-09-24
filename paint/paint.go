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

	"github.com/marrasen/gunim/geom"
)

// A Painter records the draw commands for one frame.
//
// The zero Painter is ready to use. The engine hands each frame's
// Painter to the tree and then to the driver, so a node should use it
// and let it go.
type Painter struct {
	ops    []Op
	stack  []Transform
	cur    Transform
	damage geom.Rect
}

// Reset clears the painter so its buffers can be reused next frame.
func (p *Painter) Reset() {
	p.ops = p.ops[:0]
	p.stack = p.stack[:0]
	p.cur = Identity
	p.damage = geom.Rect{}
}

// Ops returns the recorded commands in draw order.
func (p *Painter) Ops() []Op { return p.ops }

// Damage returns the union of everything painted this frame. A driver
// that can do partial presentation uses it to avoid redrawing the whole
// window when only a button is pulsing.
func (p *Painter) Damage() geom.Rect { return p.damage }

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

// Apply maps a point through t.
func (t Transform) Apply(p geom.Point) geom.Point {
	return geom.Point{X: t.A*p.X + t.B*p.Y + t.C, Y: t.D*p.X + t.E*p.Y + t.F}
}

// Push applies t on top of the current transform and returns the
// function that pops it, so a node can write:
//
//	defer p.Push(paint.Translate(origin))()
func (p *Painter) Push(t Transform) func() {
	p.stack = append(p.stack, p.cur)
	p.cur = p.cur.Mul(t)
	return p.pop
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
	// Blur, in pixels, applied to the layer's own contents.
	Blur float32
	// Backdrop, in pixels, blurs whatever is already behind the layer.
	// This is the frosted glass behind a modal, and it is why layers
	// are a first-class idea here: it needs the frame so far as a
	// texture.
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
	p.record(&LayerOp{Opts: o, Transform: p.cur}, o.Bounds)
	return func() { p.ops = append(p.ops, &LayerEndOp{}) }
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
	Glyphs    []Glyph
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

func (*RRectOp) isOp()    {}
func (*TextOp) isOp()     {}
func (*LayerOp) isOp()    {}
func (*LayerEndOp) isOp() {}

// LayerOp opens an offscreen group; LayerEndOp composites it.
type LayerOp struct {
	Opts      LayerOpts
	Transform Transform
}

// LayerEndOp closes the most recently opened layer.
type LayerEndOp struct{}

// RRect records a rounded rectangle.
func (p *Painter) RRect(r geom.Rect, radius float32, f Fill) {
	p.record(&RRectOp{Rect: r, Radius: radius, Fill: f, Transform: p.cur}, r)
}

// RRectStroke records a rounded rectangle with an outline.
func (p *Painter) RRectStroke(r geom.Rect, radius float32, f Fill, s Stroke) {
	p.record(&RRectOp{Rect: r, Radius: radius, Fill: f, Stroke: s, Transform: p.cur}, r)
}

// ShadowRRect records a rounded rectangle with a drop shadow.
func (p *Painter) ShadowRRect(r geom.Rect, radius float32, f Fill, sh Shadow) {
	grown := geom.Rect{
		Min: geom.Pt(r.Min.X-sh.Blur-sh.Spread+sh.Offset.X, r.Min.Y-sh.Blur-sh.Spread+sh.Offset.Y),
		Max: geom.Pt(r.Max.X+sh.Blur+sh.Spread+sh.Offset.X, r.Max.Y+sh.Blur+sh.Spread+sh.Offset.Y),
	}
	p.record(&RRectOp{Rect: r, Radius: radius, Fill: f, Shadow: sh, Transform: p.cur}, grown)
}

// Text records a shaped run.
func (p *Painter) Text(g []Glyph, c color.NRGBA, bounds geom.Rect) {
	p.record(&TextOp{Glyphs: g, Color: c, Transform: p.cur}, bounds)
}

func (p *Painter) record(op Op, bounds geom.Rect) {
	p.ops = append(p.ops, op)
	// Transform the corners so damage is tracked in window space,
	// whatever space the node happened to be painting in.
	a, b := p.cur.Apply(bounds.Min), p.cur.Apply(bounds.Max)
	p.damage = p.damage.Union(geom.Rect{Min: a, Max: b}.Normalized())
}
