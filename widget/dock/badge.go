package dock

import (
	"image/color"
	"time"

	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// tabBadge is a tab's badge as it shows: a word or two in a pill, as
// package widget's Badge. It grows and fades in, changes its words and
// its colour in a fade, and shrinks and fades out. Where the tab has
// little room it narrows to what the title leaves, or to half where both
// are long, and at last to a dot, which stands at the icon's corner in a
// tab of its icon alone.
type tabBadge struct {
	text string
	ink  *theme.Token[color.NRGBA]
	// on is how far it shows, swap how far its words and colour have come
	// from was and wasInk, and wide the width its words ask for.
	on, swap, wide *anim.Float
	// held says it has other words to say, as want, once the fade it is
	// in ends.
	held bool
	// beside is the room the title beside it needs, which glides as the
	// tab's width does.
	beside         *anim.Float
	run, was, want text.Run
	wasInk         color.NRGBA
	// face, size, said and from are what want was shaped from.
	face *text.Face
	size float32
	said string
	from text.Stamp
	// cut says it last showed less than its words.
	cut bool
}

// badgeGap is the room before a badge.
const badgeGap = 6

func newTabBadge() *tabBadge {
	return &tabBadge{on: anim.NewFloat(0), swap: anim.NewFloat(1), wide: anim.NewFloat(0), beside: anim.NewFloat(0)}
}

func (g *tabBadge) step(dt time.Duration) bool {
	moving := g.on.Step(dt)
	moving = g.swap.Step(dt) || moving
	moving = g.wide.Step(dt) || moving
	return g.beside.Step(dt) || moving || g.held
}

// color is the badge's own colour.
func (g *tabBadge) color(th *theme.Live) color.NRGBA {
	if g.ink != nil {
		return g.ink.Get(th)
	}
	return widget.Accent.Get(th)
}

// shown is how far the badge shows, 0 to 1.
func (g *tabBadge) shown() float32 {
	if g == nil {
		return 0
	}
	return min(max(g.on.Value(), 0), 1)
}

// swapped is how far its words and colour have changed, 0 to 1.
func (g *tabBadge) swapped() float32 { return min(max(g.swap.Value(), 0), 1) }

// say has the badge say s in ink, "" for nothing, and reports whether
// it is gone. Its width sets out at once, and words that change while
// others still fade in wait for them. With still, it is simply there, or
// not.
func (g *tabBadge) say(s string, ink *theme.Token[color.NRGBA], still bool, th *theme.Live) (gone bool) {
	if s == "" {
		// It keeps its words while it goes.
		g.held = false
		if still {
			g.on.Jump(0)
		}
		g.on.Animate(0, widget.Settle.Get(th))
		return g.on.Value() < 0.001 && !g.on.Active()
	}
	face, size := widget.Font.Get(th), widget.BadgeSize.Get(th)
	if g.from.Moved() || g.face != face || g.size != size || g.said != s {
		g.want, g.face, g.size, g.said = face.Shape(s, size), face, size, s
	}
	if still {
		g.on.Jump(1)
	}
	still = still || g.text == "" || g.on.Value() <= 0.001
	changes := g.text != s || g.ink != ink
	g.held = changes && !still && g.swap.Active()
	if !g.held {
		if changes && !still {
			g.was, g.wasInk = g.run, g.color(th)
			g.swap.Jump(0)
		}
		g.run, g.text, g.ink = g.want, s, ink
	}
	wide := g.want.Advance + 2*widget.BadgePadding.Get(th)
	if still {
		g.swap.Jump(1)
		g.wide.Jump(wide)
	}
	g.on.Animate(1, widget.Quick.Get(th))
	g.swap.Animate(1, widget.Quick.Get(th))
	// As its tab's width does.
	g.wide.Animate(wide, Slide.Get(th))
	return false
}

// width is the room the badge asks for: its words, as far as TabBadge
// lets them go.
func (g *tabBadge) width(th *theme.Live) float32 {
	if g == nil || g.on.Target() == 0 {
		return 0
	}
	return min(g.want.Advance+2*widget.BadgePadding.Get(th), TabBadge.Get(th))
}

// stand has the badge stand beside a title that needs room. With still,
// and while it does not show, that is simply so.
func (g *tabBadge) stand(room float32, still bool, th *theme.Live) {
	if still || g.on.Value() <= 0.001 {
		g.beside.Jump(room)
	}
	g.beside.Animate(room, Slide.Get(th))
}

// fit is how wide the badge is drawn in room, all of which the tab has
// for it and its title. It has what the title leaves, or where that is
// less than half of it, up to half.
func (g *tabBadge) fit(room float32, th *theme.Live) float32 {
	room -= badgeGap
	// A hair more than the title needs, so rounding never cuts it.
	w := min(g.wide.Value(), TabBadge.Get(th), max(room-g.beside.Value()-0.25, room/2))
	return max(w, TabBadgeDot.Get(th))
}

// paint draws the badge w wide, its right edge at right and its middle
// at mid, in a tab whose insides are high.
func (g *tabBadge) paint(p *paint.Painter, th *theme.Live, right, mid, w, high, opacity float32) {
	dot, pad := TabBadgeDot.Get(th), widget.BadgePadding.Get(th)
	full := min(widget.BadgeHeight.Get(th), high)
	// A pill narrower than it is tall shrinks to the dot.
	pill := min(max((w-dot)/max(2*pad, 1), 0), 1)
	h := dot + (full-dot)*pill
	on := g.shown()
	opacity *= on
	// Grown in from its middle's height, as wide as it shows.
	r := geom.Rc(right-w*on, mid-h/2, w*on, h)
	ink := anim.MixColor(g.wasInk, g.color(th), g.swapped())
	if g.swapped() >= 1 {
		ink = g.color(th)
	}
	fade := func(c color.NRGBA, by float32) color.NRGBA {
		c.A = uint8(float32(c.A)*by + 0.5)
		return c
	}
	tint := min(max(widget.BadgeTint.Get(th), 0), 1)
	p.RRect(r, h/2, paint.Solid(fade(ink, opacity*(1-(1-tint)*pill))))
	g.cut = w < g.run.Advance+2*pad-0.5
	room := r.Size().W - pad
	if pill <= 0 || room <= 0 {
		return
	}
	say := func(run text.Run, by float32) {
		if c := fade(ink, opacity*pill*by); c.A > 0 && run.Advance > 0 {
			run.PaintFaded(p, geom.Pt(r.Min.X+pad, mid-run.Height()/2), c, room, widget.TextFade.Get(th))
		}
	}
	if g.swapped() < 1 {
		say(g.was, 1-g.swapped())
	}
	say(g.run, g.swapped())
}
