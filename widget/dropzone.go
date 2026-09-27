package widget

import (
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// DropDwell is how long a drag rests on a spot of a [DropZone] before
// the spot springs open.
const DropDwell = time.Second

// DropSpot is a place inside a [DropZone] that a drag can land on, such
// as a row of a list.
type DropSpot struct {
	// Key names the spot. It must be comparable.
	Key any
	// Rect is where the spot is, in the zone's space, and Radius rounds
	// its corners.
	Rect   geom.Rect
	Radius float32
	// Hint says what a drop on the spot does, for the picture the drag
	// carries, such as a [DropHint].
	Hint any
	// Refused marks a spot that takes no drop. It lights up in
	// DropRefusedInk, and a drop on it is not taken.
	Refused bool
	// Opens says a drag resting on the spot for the zone's Dwell opens
	// it, as a folder springs open.
	Opens bool
}

// DropZone takes drags on spots inside its child: the spot under a drag
// lights up, the light gliding from spot to spot, and a drag resting on
// a spot that opens fills it and then springs it open.
type DropZone struct {
	anim.Group
	// Spot returns the spot a drop d would land on, and false where the
	// zone takes none. It hears a drag inside the application as a drop
	// with Data and no Paths, before it is let go, and files from
	// another program with Paths.
	Spot func(d input.Drop, u *gunim.UI) (DropSpot, bool)
	// OnDrop turns a drop on a spot into an intent.
	OnDrop func(spot DropSpot, d input.Drop) gunim.Intent
	// OnOpen turns a drag resting on a spot that opens into an intent.
	OnOpen func(spot DropSpot) gunim.Intent
	// Dwell is how long a drag rests on a spot before it opens; zero is
	// DropDwell.
	Dwell time.Duration

	child gunim.Node
	lit   DropSpot
	over  bool
	// opened is the key of the spot opened last, which opens again only
	// once the drag has gone elsewhere.
	opened any
	stop   func()
	rect   *anim.Rect
	on     *anim.Float
	red    *anim.Float
	fill   *anim.Float
	pop    *anim.Float
}

// NewDropZone returns child, taking drags on the spots Spot finds.
func NewDropZone(child gunim.Node) *DropZone {
	z := &DropZone{child: child, rect: anim.NewRect(geom.Rect{}), on: anim.NewFloat(0), red: anim.NewFloat(0),
		fill: anim.NewFloat(0), pop: anim.NewFloat(0)}
	z.Add(z.rect, z.on, z.red, z.fill, z.pop)
	return z
}

// Children implements [gunim.Composite].
func (z *DropZone) Children() []gunim.Node { return []gunim.Node{z.child} }

// Over returns the spot a drag is over, and false when there is none.
func (z *DropZone) Over() (DropSpot, bool) { return z.lit, z.over }

// Handle implements [gunim.Handler].
func (z *DropZone) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.DragOver:
		spot, ok := z.find(input.Drop{Pos: e.Pos, Data: e.Data, Mods: e.Mods, Time: e.Time}, u)
		if !ok {
			z.leave(u)
			return false
		}
		z.hover(spot, u)
		u.AnswerDrag(spot.Hint)
		return true
	case input.DragLeave:
		z.leave(u)
	case input.Drop:
		spot, ok := z.find(e, u)
		z.leave(u)
		if !ok || spot.Refused {
			return false
		}
		z.land(spot, u)
		if z.OnDrop != nil {
			if v := z.OnDrop(spot, e); v != nil {
				u.Send(z, v)
			}
		}
	default:
		return false
	}
	u.Invalidate()
	return true
}

func (z *DropZone) find(d input.Drop, u *gunim.UI) (DropSpot, bool) {
	if z.Spot == nil {
		return DropSpot{}, false
	}
	return z.Spot(d, u)
}

// hover lights spot, and starts the wait for it to open.
func (z *DropZone) hover(spot DropSpot, u *gunim.UI) {
	th := u.Theme()
	was, same := z.over, z.over && spot.Key == z.lit.Key
	z.lit, z.over = spot, true
	if !was {
		z.rect.Jump(spot.Rect)
		z.pop.Jump(0)
	} else {
		z.rect.Animate(spot.Rect, Quick.Get(th))
	}
	z.on.Animate(1, Quick.Get(th))
	z.red.Animate(map[bool]float32{false: 0, true: 1}[spot.Refused], Quick.Get(th))
	if same {
		return
	}
	z.opened = nil
	z.wait(spot, u)
}

// wait starts the fill that opens spot once it is full.
func (z *DropZone) wait(spot DropSpot, u *gunim.UI) {
	if z.stop != nil {
		z.stop()
		z.stop = nil
	}
	z.fill.Jump(0)
	if !spot.Opens || spot.Refused || z.OnOpen == nil {
		return
	}
	dwell := z.Dwell
	if dwell <= 0 {
		dwell = DropDwell
	}
	z.fill.Animate(1, anim.Tween{Duration: dwell, Ease: anim.Linear})
	key := spot.Key
	z.stop = u.After(dwell, func(u *gunim.UI) {
		z.stop = nil
		if !z.over || z.lit.Key != key || z.opened == key {
			return
		}
		z.opened = key
		z.fill.Animate(0, Settle.Get(u.Theme()))
		z.land(z.lit, u)
		if v := z.OnOpen(z.lit); v != nil {
			u.Send(z, v)
		}
	})
}

// land flashes the light, for a drop taken or a spot opened.
func (z *DropZone) land(spot DropSpot, u *gunim.UI) {
	z.rect.Jump(spot.Rect)
	z.pop.Jump(1)
	z.pop.Animate(0, anim.Tween{Duration: 450 * time.Millisecond})
	u.Invalidate()
}

// leave puts the light out.
func (z *DropZone) leave(u *gunim.UI) {
	if z.stop != nil {
		z.stop()
		z.stop = nil
	}
	z.over, z.opened = false, nil
	z.on.Animate(0, Settle.Get(u.Theme()))
	z.fill.Animate(0, Settle.Get(u.Theme()))
}

// Layout implements [gunim.Node].
func (z *DropZone) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	k := kids.At(0)
	s := k.Layout(c)
	k.Place(geom.Point{})
	return s
}

// Paint implements [gunim.Node]: the child, and the light over it.
func (z *DropZone) Paint(p *paint.Painter, f gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
	th := f.Theme
	on := min(max(z.on.Value(), 0), 1)
	pop := min(max(z.pop.Value(), 0), 1)
	if on <= 0.01 && pop <= 0.01 {
		return
	}
	r := z.rect.Value()
	radius := z.lit.Radius
	ink := anim.Mix(anim.ColorCodec, Accent.Get(th), DropRefusedInk.Get(th), min(max(z.red.Value(), 0), 1))
	if pop > 0 {
		// A drop taken swells the light and lets it go.
		grow := 6 * (1 - pop)
		flash := ink
		flash.A = uint8(float32(flash.A) * 0.45 * pop)
		p.RRect(r.Inset(geom.Uniform(-grow)), radius+grow, paint.Solid(flash))
	}
	if on <= 0.01 {
		return
	}
	wash := ink
	wash.A = uint8(float32(wash.A) * 0.16 * on)
	p.RRect(r, radius, paint.Solid(wash))
	if t := min(max(z.fill.Value(), 0), 1); t > 0.001 {
		fill := ink
		fill.A = uint8(float32(fill.A) * 0.22 * on)
		func() {
			defer p.Layer(paint.LayerOpts{Bounds: r, Opacity: 1, Clip: true, Radius: radius})()
			p.RRect(geom.Rect{Min: r.Min, Max: geom.Pt(r.Min.X+r.Size().W*t, r.Max.Y)}, 0, paint.Solid(fill))
		}()
	}
	edge := ink
	edge.A = uint8(float32(edge.A) * on)
	p.RRectStroke(r, radius, paint.Fill{}, paint.Stroke{Width: 1.5, Color: edge})
}
