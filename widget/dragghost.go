package widget

import (
	"image/color"
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// DropEffect is what a drop does, as the picture a drag carries shows it.
type DropEffect uint8

// The effects of a drop.
const (
	// DropRefused is a drop that does nothing.
	DropRefused DropEffect = iota
	DropMove
	DropCopy
	// DropLink makes a link to what is dragged, such as a shortcut or a
	// favourite.
	DropLink
)

// DropHint says what a drop would do. A drop target gives it to
// [gunim.UI.AnswerDrag], and a [DragGhost] shows it beside the pointer.
type DropHint struct {
	Text   string
	Effect DropEffect
}

// DragGhost tokens.
var (
	// DragGhostMargin is the room around the picture for it to trail the
	// pointer, tilt and cast its shadow in.
	DragGhostMargin = theme.Length("dragghost.margin", 44)
	// DragGhostTrail is the spring the picture follows the pointer on.
	DragGhostTrail = theme.Spring("dragghost.trail", anim.Spring{Response: 0.2, Damping: 0.62})
	// DropRefusedInk marks a drop that would do nothing.
	DropRefusedInk = theme.Color("drop.refused", color.NRGBA{R: 0xe5, G: 0x48, B: 0x4d, A: 0xff})
)

// DragGhost is a picture for a drag to carry, given to
// [gunim.UI.StartDrag]: its child on a card that trails the pointer on
// a spring and tilts as it swings, a badge on its corner, and what a
// drop would do beside the pointer.
//
// It pops up as the drag starts. Taken, it shrinks into the pointer;
// let go where a drop is refused, it shakes its head; let go over
// nothing, it sinks and fades.
type DragGhost struct {
	anim.Group
	// Badge is shown on the card's corner, such as a count; empty shows
	// none.
	Badge string
	// Stack is how many more cards to fan out behind the card, for a drag
	// of several things. Two at most show.
	Stack int

	child  gunim.Node
	grab   geom.Point
	margin float32
	trail  anim.Spring
	// lag is how far the card trails the pointer, and vel how fast that
	// changes, per axis.
	lag, vel [2]float64
	last     geom.Point
	moved    bool
	// hint is the answer showing, kept as it fades.
	hint     DropHint
	hintOn   *anim.Float
	hintW    *anim.Float
	in, out  *anim.Float
	ended    bool
	taken    bool
	size     geom.Size
	hintRun  shapedText
	badgeRun shapedText
	opaque   bool
}

// NewDragGhost returns a picture of child for a drag, held grab from the
// child's top left corner: the grab given to StartDrag.
func NewDragGhost(child gunim.Node, grab geom.Point) *DragGhost {
	g := &DragGhost{child: child, grab: grab, hintOn: anim.NewFloat(0), hintW: anim.NewFloat(0),
		in: anim.NewFloat(0), out: anim.NewFloat(0), trail: DragGhostTrail.Default()}
	g.Add(g.hintOn, g.hintW, g.in, g.out)
	return g
}

// Children implements [gunim.Composite].
func (g *DragGhost) Children() []gunim.Node { return []gunim.Node{g.child} }

// Hint returns the answer the ghost shows, and whether it shows one.
func (g *DragGhost) Hint() (DropHint, bool) { return g.hint, g.hintOn.Target() > 0 }

// PopupPadding implements [gunim.PopupPadder]: the room around the card.
func (g *DragGhost) PopupPadding() geom.Insets {
	return geom.Insets{Left: g.margin, Top: g.margin, Right: g.margin, Bottom: g.margin}
}

// maxTilt is the most the card tilts, in radians.
const maxTilt = 0.22

// Handle implements [gunim.Handler].
func (g *DragGhost) Handle(e input.Event, u *gunim.UI) bool {
	th := u.Theme()
	switch e := e.(type) {
	case input.DragMove:
		if g.moved {
			d := e.At.Sub(g.last)
			reach := float64(g.margin) * 0.8
			g.lag[0] = max(-reach, min(reach, g.lag[0]-float64(d.X)))
			g.lag[1] = max(-reach, min(reach, g.lag[1]-float64(d.Y)))
		}
		g.last, g.moved = e.At, true
	case input.DragAnswer:
		if h, ok := e.Answer.(DropHint); ok {
			if h != g.hint || g.hintOn.Target() == 0 {
				g.hint = h
				g.hintOn.Jump(min(g.hintOn.Value(), 0.4))
			}
			g.hintOn.Animate(1, Bounce.Get(th))
		} else {
			g.hintOn.Animate(0, Settle.Get(th))
		}
	case input.DragEnd:
		g.ended, g.taken = true, e.Taken
	default:
		return false
	}
	u.Invalidate()
	return true
}

// Step implements [gunim.Animator]: the card's spring back to the
// pointer, and the rest.
func (g *DragGhost) Step(dt time.Duration) bool {
	moving := g.Group.Step(dt)
	for i := range g.lag {
		if g.lag[i] == 0 && g.vel[i] == 0 {
			continue
		}
		g.lag[i], g.vel[i] = g.trail.Follow(g.lag[i], g.vel[i], 0, dt)
		if math.Abs(g.lag[i]) < 0.05 && math.Abs(g.vel[i]) < 1 {
			g.lag[i], g.vel[i] = 0, 0
		}
		moving = true
	}
	return moving
}

// refused reports whether the ghost was let go where a drop does nothing.
func (g *DragGhost) refused() bool {
	return g.ended && !g.taken && g.hintOn.Target() > 0 && g.hint.Effect == DropRefused
}

// Transition implements [gunim.Transitioner].
func (g *DragGhost) Transition(p gunim.Presence, f gunim.Frame) bool {
	switch p {
	case gunim.Entering:
		g.in.Animate(1, Bounce.Get(f.Theme))
		return !g.in.Active()
	case gunim.Exiting:
		switch {
		case g.refused():
			g.out.Animate(1, anim.Tween{Duration: 620 * time.Millisecond, Ease: anim.Linear})
		case g.taken:
			g.out.Animate(1, anim.Tween{Duration: 280 * time.Millisecond, Ease: anim.EaseInOut})
		default:
			g.out.Animate(1, anim.Tween{Duration: 320 * time.Millisecond})
		}
		return !g.out.Active()
	case gunim.Present:
	}
	return true
}

// hintSize is the height of the line beside the pointer, and hintGap the
// room between it and the card.
const (
	hintSize = 26
	hintGap  = 12
)

// Layout implements [gunim.Node].
func (g *DragGhost) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	th := f.Theme
	g.trail = DragGhostTrail.Get(th)
	g.opaque = !f.Transparent
	g.margin = DragGhostMargin.Get(th)
	if g.opaque {
		g.margin = 0
	}
	k := kids.At(0)
	g.size = k.Layout(gunim.Loose(geom.Sz(480, 480)))
	k.Place(geom.Pt(g.margin, g.margin))
	run := g.hintRun.shape(faceIn(Font, th), g.hint.Text, TextSize.Get(th)*0.9)
	pill := hintSize + run.Advance + 12
	if g.hintW.Value() == 0 {
		g.hintW.Jump(pill)
	}
	g.hintW.Animate(pill, Quick.Get(th))
	w := g.size.W + hintGap + pill + 4
	h := max(g.size.H, g.grab.Y+hintSize/2+4)
	return c.Constrain(geom.Sz(w+2*g.margin, h+2*g.margin))
}

// Paint implements [gunim.Node].
func (g *DragGhost) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	th := f.Theme
	in := min(max(g.in.Value(), 0), 1.2)
	out := min(max(g.out.Value(), 0), 1)
	pointer := geom.Pt(g.margin, g.margin).Add(g.grab)
	opacity := min(in, 1) * (1 - out)
	scale := 0.86 + 0.14*g.in.Value()
	var shift geom.Point
	spin := float32(0)
	switch {
	case g.refused():
		// A shake of the head, then a fade.
		shift.X = float32(12 * math.Sin(float64(out)*5*math.Pi) * float64(1-out))
		opacity = min(in, 1) * min(1, 2*(1-out))
	case g.taken:
		scale *= 1 - 0.88*out
		spin = 0.35 * out
	case g.ended:
		scale *= 1 - 0.08*out
		shift.Y = 18 * out
	}
	if opacity <= 0.001 {
		return
	}
	if g.opaque {
		p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(CardFill.Get(th)))
	}
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: opacity})()
	g.paintCard(p, th, pointer, scale, shift, spin, kids)
	g.paintHint(p, th, pointer, out)
}

// paintCard draws the card, the cards behind it and the badge, trailing
// the pointer and tilted.
func (g *DragGhost) paintCard(p *paint.Painter, th *theme.Live, pointer geom.Point, scale float32, shift geom.Point,
	spin float32, kids gunim.Children) {
	lag := geom.Pt(float32(g.lag[0]), float32(g.lag[1]))
	tilt := max(-maxTilt, min(maxTilt, lag.X*0.006+float32(g.vel[0])*0.0004)) + spin
	fan := min(float32(math.Abs(g.vel[0])+math.Abs(g.vel[1]))*0.00025, 0.12)
	defer p.Push(paint.Translate(lag.Add(shift)))()
	defer p.Push(paint.Rotate(tilt, pointer))()
	defer p.Push(paint.Scale(scale, pointer))()

	card := geom.Rc(g.margin, g.margin, g.size.W, g.size.H)
	radius := CardRadius.Get(th)
	fill := CardFill.Get(th)
	shadow := paint.Shadow{Offset: geom.Pt(0, 6), Blur: 18, Color: MenuShadow.Get(th)}
	for i := min(g.Stack, 2); i >= 1; i-- {
		turn := float32(i)*0.06 + fan*float32(i)
		if i == 2 {
			turn = -turn
		}
		func() {
			defer p.Push(paint.Rotate(turn, card.Center()))()
			back := card.Add(geom.Pt(5*float32(i), 4*float32(i)))
			p.ShadowRRect(back, radius, paint.Solid(fill), shadow)
			p.RRectStroke(back, radius, paint.Fill{}, paint.Stroke{Width: 1, Color: MenuBorder.Get(th)})
		}()
	}
	p.ShadowRRect(card, radius, paint.Solid(fill), shadow)
	p.RRectStroke(card, radius, paint.Fill{}, paint.Stroke{Width: 1, Color: MenuBorder.Get(th)})
	kids.At(0).Paint(p)
	g.paintBadge(p, th, card)
}

// paintBadge draws the badge on the card's top right corner.
func (g *DragGhost) paintBadge(p *paint.Painter, th *theme.Live, card geom.Rect) {
	if g.Badge == "" {
		return
	}
	run := g.badgeRun.shape(faceIn(BoldFont, th), g.Badge, TextSize.Get(th)*0.8)
	h := run.Height() + 6
	w := max(h, run.Advance+14)
	pill := geom.Rc(card.Max.X-w+8, card.Min.Y-h/2, w, h)
	pop := min(max(g.in.Value(), 0), 1.3)
	defer p.Push(paint.Scale(pop, pill.Center()))()
	p.ShadowRRect(pill, h/2, paint.Solid(Accent.Get(th)), paint.Shadow{Offset: geom.Pt(0, 2), Blur: 6, Color: MenuShadow.Get(th)})
	run.Paint(p, geom.Pt(pill.Min.X+(w-run.Advance)/2, pill.Min.Y+(h-run.Height())/2), ButtonStrongInk.Get(th))
}

// paintHint draws what a drop would do beside the pointer: a round mark
// of the effect and the words.
func (g *DragGhost) paintHint(p *paint.Painter, th *theme.Live, pointer geom.Point, out float32) {
	on := min(max(g.hintOn.Value(), 0), 1.2)
	if on <= 0.01 || g.hint.Text == "" {
		return
	}
	at := pointer.Add(geom.Pt(g.size.W-g.grab.X+hintGap-8*(1-min(on, 1)), -hintSize/2))
	w := max(hintSize, g.hintW.Value())
	pill := geom.Rc(at.X, at.Y, w, hintSize)
	defer p.Layer(paint.LayerOpts{Bounds: pill.Inset(geom.Uniform(-12)), Opacity: min(on, 1) * (1 - out)})()
	defer p.Push(paint.Scale(0.7+0.3*on, geom.Pt(at.X, at.Y+hintSize/2)))()
	p.ShadowRRect(pill, hintSize/2, paint.Solid(MenuFill.Get(th)), paint.Shadow{Offset: geom.Pt(0, 3), Blur: 10, Color: MenuShadow.Get(th)})
	p.RRectStroke(pill, hintSize/2, paint.Fill{}, paint.Stroke{Width: 1, Color: MenuBorder.Get(th)})
	mark := Accent.Get(th)
	if g.hint.Effect == DropRefused {
		mark = DropRefusedInk.Get(th)
	}
	dot := geom.Rc(at.X+4, at.Y+4, hintSize-8, hintSize-8)
	p.RRect(dot, dot.Size().W/2, paint.Solid(mark))
	drawEffect(p, g.hint.Effect, dot.Center(), ButtonStrongInk.Get(th))
	run := g.hintRun.run
	defer p.Layer(paint.LayerOpts{Bounds: pill, Opacity: 1, Clip: true, Radius: hintSize / 2})()
	run.Paint(p, geom.Pt(at.X+hintSize, at.Y+(hintSize-run.Height())/2), Ink.Get(th))
}

// drawEffect draws the small sign of an effect centred on c: an arrow to
// move, a plus to copy, a curved arrow to link, and a bar to refuse.
func drawEffect(p *paint.Painter, e DropEffect, c geom.Point, ink color.NRGBA) {
	const thick = 1.8
	bar := func(from geom.Point, length, angle float32) {
		defer p.Push(paint.Rotate(angle, from))()
		p.RRect(geom.Rc(from.X, from.Y-thick/2, length, thick), thick/2, paint.Solid(ink))
	}
	switch e {
	case DropMove:
		bar(geom.Pt(c.X-4.5, c.Y), 9, 0)
		bar(geom.Pt(c.X+4.5, c.Y), 4.5, math.Pi*3/4)
		bar(geom.Pt(c.X+4.5, c.Y), 4.5, -math.Pi*3/4)
	case DropCopy:
		bar(geom.Pt(c.X-4.5, c.Y), 9, 0)
		bar(geom.Pt(c.X, c.Y-4.5), 9, math.Pi/2)
	case DropLink:
		tip := geom.Pt(c.X+3.5, c.Y-3.5)
		bar(geom.Pt(c.X-3.5, c.Y+3.5), 9.9, -math.Pi/4)
		bar(tip, 5, math.Pi)
		bar(tip, 5, math.Pi/2)
	case DropRefused:
		bar(geom.Pt(c.X-4.5, c.Y), 9, 0)
	}
}
