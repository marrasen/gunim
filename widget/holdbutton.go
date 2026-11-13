package widget

import (
	"image/color"
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// HoldButton tokens.
var (
	// HoldRing is the width of the ring that fills as a [HoldButton] is held.
	HoldRing = theme.Length("hold.ring", 3)
	// HoldHint is what a screen reader says a [HoldButton] asks for, after its name.
	HoldHint = theme.Choice("hold.hint", "Press and hold to confirm")
)

// HoldButton is a button for the gravest changes, such as revoking a device: it acts only once it has been held
// down for Hold, one second unless set.
//
// While it is held, a ring at its start fills, at an even pace, from the top round. Let go early, and the ring
// drains back on a spring, and nothing happens; press again before it has drained, and it fills on from where it
// is. Once the ring is full the button acts, once, and springs back from its press. It lights and squashes under
// the finger, as [Button] does.
//
// Space or Enter held down holds it from the keyboard. A screen reader's press acts at once: the hold guards against
// a slip of the finger, which a screen reader's request is not.
type HoldButton struct {
	Control

	Label string
	// Icon shows inside the ring.
	Icon *icon.Icon
	// Kind colours the button, as a [Button]'s. NewHoldButton makes it [ButtonDanger].
	Kind ButtonKind
	// Hold is how long the button must be held to act.
	Hold time.Duration
	// OnClick runs once the button has been held for Hold. A non-nil result is sent as the button's intent.
	OnClick func(u *gunim.UI) gunim.Intent

	// fill runs from 0 to 1 as the ring fills, on the clock, and back as it drains.
	fill *anim.Float
	// holding says a press or a key holds the button now, key which key, and fired that this hold has acted.
	holding, fired bool
	key            input.Key
	// stop ends the wait for the hold to be long enough.
	stop  func()
	size  geom.Size
	text  shapedText
	ell   shapedText
	ring  []paint.RibbonPoint
	track []paint.RibbonPoint
}

// NewHoldButton returns a button showing label that acts once held for a second.
func NewHoldButton(label string) *HoldButton {
	b := &HoldButton{Control: newControl(), Label: label, Kind: ButtonDanger, Hold: time.Second, fill: anim.NewFloat(0)}
	// The ring keeps time: it fills over Hold however the theme speeds up or slows down motion.
	b.fill.RealTime()
	b.Add(b.fill)
	return b
}

// Progress returns how full the ring is, from 0 to 1.
func (b *HoldButton) Progress() float32 { return min(max(b.fill.Value(), 0), 1) }

// DragsTouch implements [gunim.TouchDragger]: a finger holding the button keeps holding it, and never turns into a
// long press or a scroll while it does.
func (b *HoldButton) DragsTouch() bool { return b.held }

// Holding reports whether the button is held down now.
func (b *HoldButton) Holding() bool { return b.holding }

// begin starts a hold: the ring fills from where it is, and the button acts once it is full.
func (b *HoldButton) begin(u *gunim.UI) {
	if b.holding {
		return
	}
	b.holding, b.fired = true, false
	hold := max(b.Hold, time.Millisecond)
	rest := time.Duration(float64(hold) * float64(1-b.Progress()))
	b.fill.Retarget(1, anim.Tween{Duration: rest, Ease: anim.Linear})
	b.stop = u.After(rest, b.complete)
	u.Invalidate()
}

// complete acts, once the button has been held long enough.
func (b *HoldButton) complete(u *gunim.UI) {
	b.stop = nil
	if !b.holding || b.fired {
		return
	}
	b.fired = true
	b.fill.Animate(1, Quick.Get(u.Theme()))
	// The press springs back, so the button shows it has acted while the finger is still on it.
	b.squash.Animate(0, Bounce.Get(u.Theme()))
	act0(u, b, gunim.CuePress, b.OnClick)
}

// end ends a hold: a ring short of full drains back, and one that is full drains once let go.
func (b *HoldButton) end(th *theme.Live) {
	if !b.holding {
		return
	}
	b.holding = false
	if b.stop != nil {
		b.stop()
		b.stop = nil
	}
	b.squash.Animate(0, Bounce.Get(th))
	b.fill.Retarget(0, Settle.Get(th))
}

// Handle implements [gunim.Handler].
func (b *HoldButton) Handle(e input.Event, u *gunim.UI) bool {
	b.showTip(e, u, b)
	th := u.Theme()
	if b.Disabled {
		if _, lost := e.(input.FocusLost); lost {
			b.end(th)
		}
		return b.handleDisabled(e, u, nil)
	}
	switch e := e.(type) {
	case input.PointerEnter:
		b.hover.Animate(1, Quick.Get(th))
	case input.PointerLeave:
		b.hover.Animate(0, Settle.Get(th))
		// Dragged off, the hold lets go, though the button keeps the pointer until it is released.
		if b.held {
			b.end(th)
		}
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		b.held = true
		b.squashIn(th)
		b.begin(u)
	case input.PointerMove:
		// A finger that slides off lets go of the hold; the button keeps it until it lifts.
		if b.held && b.holding && over(e.Pos, b.size) < 0 {
			b.end(th)
		}
		return b.held
	case input.PointerUp:
		if !b.held {
			return false
		}
		b.held = false
		b.end(th)
	case input.KeyPress:
		if e.Key != input.KeySpace && e.Key != input.KeyEnter && e.Key != input.KeyKPEnter {
			return false
		}
		if !e.Repeat && !b.holding {
			b.key = e.Key
			b.squash.Animate(1, Quick.Get(th))
			b.begin(u)
		}
	case input.KeyRelease:
		if !b.holding || b.held || e.Key != b.key {
			return false
		}
		b.end(th)
	case input.FocusLost:
		b.ringFollows(e, th)
		if !b.held {
			b.end(th)
		}
	default:
		return b.ringFollows(e, th)
	}
	return true
}

// Layout implements [gunim.Node]: the ring, a gap and the label, inside the button's padding.
func (b *HoldButton) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	b.follow(th)
	if b.Disabled && b.holding {
		b.end(th)
	}
	h := ButtonHeight.Get(th)
	w := 2*ButtonPadding.Get(th) + b.ringSize(th)
	if b.Label != "" {
		w += IconGap.Get(th) + b.text.shape(faceIn(Font, th), b.Label, TextSize.Get(th)).Advance
	}
	b.size = c.Constrain(geom.Sz(w, h))
	return b.size
}

// ringSize is the ring's width across, with its icon inside.
func (b *HoldButton) ringSize(th *theme.Live) float32 { return IconSize.Get(th) + 4*HoldRing.Get(th) }

// Paint implements [gunim.Node].
func (b *HoldButton) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	defer b.faint(p, box)()
	th := f.Theme
	r := geom.Rect{Max: box.Point()}
	radius := min(ButtonRadius.Get(th), box.H/2)
	if s := 1 - ButtonSquash.Get(th)*b.squash.Value(); s != 1 {
		defer p.Push(paint.Scale(s, r.Center()))()
	}
	b.paintRing(p, r, radius, th)
	rest, hover, ink := kindColours(b.Kind, th)
	fill := anim.Mix(anim.ColorCodec, rest.Get(th), hover.Get(th), min(max(b.hover.Value(), 0), 1))
	p.RRect(r, radius, paint.Solid(fill))
	b.paintGlow(p, r, radius, th)
	inked := ink.Get(th)

	pad := min(ButtonPadding.Get(th), box.W/4)
	d := b.ringSize(th)
	c := geom.Pt(pad+d/2, box.H/2)
	if b.Label == "" {
		c.X = box.W / 2
	}
	w := HoldRing.Get(th)
	faint := inked
	faint.A = uint8(float32(faint.A)*0.35 + 0.5)
	b.track = arc(b.track[:0], c, d/2-w/2, w, 1, faint)
	p.Ribbon(b.track)
	if t := b.Progress(); t > 0.002 {
		b.ring = arc(b.ring[:0], c, d/2-w/2, w, t, inked)
		p.Ribbon(b.ring)
	}
	if b.Icon != nil {
		s := IconSize.Get(th) * 0.75
		drawIcon(p, th, b.Icon, geom.Rc(c.X-s/2, c.Y-s/2, s, s), inked, 1)
	}
	if b.Label == "" {
		return
	}
	x := c.X + d/2 + IconGap.Get(th)
	if end := box.W - pad; x < end {
		run := fitRun(b.text.shape(faceIn(Font, th), b.Label, TextSize.Get(th)), &b.ell, end-x)
		run.Paint(p, geom.Pt(x, (box.H-run.Height())/2), inked)
	}
}

// Access implements [gunim.Accessible].
func (b *HoldButton) Access() access.Info {
	name := b.accessName(b.Label)
	if name == "" {
		name = iconName(b.Icon)
	}
	return access.Info{Role: access.RoleButton, Name: name, Description: HoldHint.Default(), State: b.accessState(),
		Actions: []string{access.ActionPress}}
}

// AccessAct implements [gunim.AccessActor]: a press acts at once.
func (b *HoldButton) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress || b.Disabled {
		return false
	}
	act0(u, b, gunim.CuePress, b.OnClick)
	return true
}

// arc appends to pts a ribbon round c at radius r, w wide, from the top clockwise for frac of the way round, and
// returns it. It takes about one point for each 3 pixels of its length.
func arc(pts []paint.RibbonPoint, c geom.Point, r, w, frac float32, col color.NRGBA) []paint.RibbonPoint {
	frac = min(max(frac, 0), 1)
	n := max(int(math.Ceil(float64(2*math.Pi*r*frac/3))), 2) + 1
	for i := range n {
		a := -math.Pi/2 + 2*math.Pi*float64(frac)*float64(i)/float64(n-1)
		pts = append(pts, paint.RibbonPoint{
			At:    geom.Pt(c.X+r*float32(math.Cos(a)), c.Y+r*float32(math.Sin(a))),
			Width: w,
			Color: col,
		})
	}
	return pts
}
