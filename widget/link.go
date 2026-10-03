package widget

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
)

// LinkInk is the colour of a link's text.
var LinkInk = theme.Foreground("link.ink", color.NRGBA{R: 0x7c, G: 0x9c, B: 0xff, A: 0xff})

// Link is text that does something when clicked, underlined under the
// pointer: a small action among other text, where a button would be
// too big, such as "show 12 more". It takes focus, grows a ring as a
// [Button] does, and Enter or Space activates it.
type Link struct {
	anim.Group
	Text string
	// Icon shows before the text, as tall as the text and in its colour.
	Icon *icon.Icon
	// Size and Face default to the theme's [TextSize] and [Font].
	Size theme.Token[float32]
	Face theme.Token[*text.Face]
	// On is the intent sent when the link is clicked. Leave it nil for a
	// link whose whole job is local; see [Link.OnActivate].
	On gunim.Intent

	activate func(*gunim.UI)
	hover    *anim.Float
	ring     *anim.Float
	shaped   shapedText
	// laid is the text cut to its box, ending in an ellipsis, for a link
	// given less room than its text takes.
	laid laidText
}

// NewLink returns a link showing s.
func NewLink(s string) *Link {
	l := &Link{Text: s, Size: TextSize, hover: anim.NewFloat(0), ring: anim.NewFloat(0)}
	l.Add(l.hover, l.ring)
	return l
}

// OnActivate wires behaviour that runs inside the window when the link
// is clicked.
func (l *Link) OnActivate(fn func(*gunim.UI)) { l.activate = fn }

func (l *Link) run(th *theme.Live) text.Run {
	return l.shaped.shape(faceIn(l.Face, th), l.Text, l.textSize(th))
}

// textSize is the link's text size in th.
func (l *Link) textSize(th *theme.Live) float32 {
	if l.Size.Key() != "" {
		return l.Size.Get(th)
	}
	return TextSize.Get(th)
}

// Layout implements [gunim.Node].
func (l *Link) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	s := l.run(f.Theme).Box()
	s.W += l.iconWidth(f.Theme)
	return c.Constrain(s)
}

// iconWidth is the room the link's icon takes before its text, with the gap.
func (l *Link) iconWidth(th *theme.Live) float32 {
	if l.Icon == nil {
		return 0
	}
	return l.textSize(th) + IconGap.Get(th)
}

// Paint implements [gunim.Node].
// Given less room than its text takes, it ends the text in an ellipsis
// within its box.
func (l *Link) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	focusRing(p, geom.Rect{Max: box.Point()}, 4, l.ring.Value(), f.Theme)
	run := l.run(f.Theme)
	ink := LinkInk.Get(f.Theme)
	x := l.iconWidth(f.Theme)
	if l.Icon != nil {
		s := l.textSize(f.Theme)
		paintIcon(p, f.Theme, l.Icon, geom.Rc(0, (run.Height()-s)/2, s, s), ink, 1)
	}
	wide := run.Advance
	if run.Advance > box.W-x+0.5 {
		para := l.laid.layout(faceIn(l.Face, f.Theme), l.Text, text.Style{Size: l.textSize(f.Theme), MaxLines: 1},
			max(box.W-x, 0))
		para.Paint(p, geom.Pt(x, 0), ink)
		wide = para.Size.W
	} else {
		run.Paint(p, geom.Pt(x, 0), ink)
	}
	if t := l.hover.Value(); t > 0.01 {
		ink.A = uint8(float32(ink.A) * min(t, 1))
		p.RRect(geom.Rc(x, run.Ascent+1.5, wide, 1), 0, paint.Solid(ink))
	}
}

// Cursor implements [gunim.CursorShaper].
func (l *Link) Cursor(geom.Point) input.Cursor { return input.CursorHand }

// Handle implements [gunim.Handler].
func (l *Link) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerEnter:
		l.hover.Animate(1, Quick.Get(u.Theme()))
	case input.PointerLeave:
		l.hover.Animate(0, Settle.Get(u.Theme()))
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		l.fire(u)
		return true
	case input.KeyPress:
		if e.Key != input.KeyEnter && e.Key != input.KeyKPEnter && e.Key != input.KeySpace {
			return false
		}
		l.fire(u)
		return true
	case input.FocusRing:
		l.ring.Animate(ringTo(e), Quick.Get(u.Theme()))
		return true
	case input.FocusLost:
		l.ring.Animate(0, Settle.Get(u.Theme()))
		return true
	}
	return false
}

// Focusable implements [gunim.Focusable].
func (l *Link) Focusable() bool { return true }

func (l *Link) fire(u *gunim.UI) {
	u.Cue(gunim.CuePress, l)
	if l.activate != nil {
		l.activate(u)
	}
	if l.On != nil {
		u.Send(l, l.On)
	}
}

// Access implements [gunim.Accessible].
func (l *Link) Access() access.Info {
	return access.Info{Role: access.RoleLink, Name: l.Text, Actions: []string{access.ActionPress}}
}

// AccessAct implements [gunim.AccessActor].
func (l *Link) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress {
		return false
	}
	l.fire(u)
	return true
}
