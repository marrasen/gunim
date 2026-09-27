package widget

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
)

// LinkInk is the colour of a link's text.
var LinkInk = theme.Foreground("link.ink", color.NRGBA{R: 0x7c, G: 0x9c, B: 0xff, A: 0xff})

// Link is text that does something when clicked, underlined under the
// pointer: a small action among other text, where a button would be
// too big, such as "show 12 more".
type Link struct {
	anim.Group
	Text string
	// Size and Face default to the theme's [TextSize] and [Font].
	Size theme.Token[float32]
	Face theme.Token[*text.Face]
	// On is the intent sent when the link is clicked. Leave it nil for a
	// link whose whole job is local; see [Link.OnActivate].
	On gunim.Intent

	activate func(*gunim.UI)
	hover    *anim.Float
	shaped   shapedText
}

// NewLink returns a link showing s.
func NewLink(s string) *Link {
	l := &Link{Text: s, Size: TextSize, hover: anim.NewFloat(0)}
	l.Add(l.hover)
	return l
}

// OnActivate wires behaviour that runs inside the window when the link
// is clicked.
func (l *Link) OnActivate(fn func(*gunim.UI)) { l.activate = fn }

func (l *Link) run(th *theme.Live) text.Run {
	size := TextSize.Get(th)
	if l.Size.Key() != "" {
		size = l.Size.Get(th)
	}
	return l.shaped.shape(faceIn(l.Face, th), l.Text, size)
}

// Layout implements [gunim.Node].
func (l *Link) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(l.run(f.Theme).Box())
}

// Paint implements [gunim.Node].
func (l *Link) Paint(p *paint.Painter, f gunim.Frame, _ geom.Size, _ gunim.Children) {
	run := l.run(f.Theme)
	ink := LinkInk.Get(f.Theme)
	run.Paint(p, geom.Point{}, ink)
	if t := l.hover.Value(); t > 0.01 {
		ink.A = uint8(float32(ink.A) * min(t, 1))
		p.RRect(geom.Rc(0, run.Ascent+1.5, run.Advance, 1), 0, paint.Solid(ink))
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
	}
	return false
}

func (l *Link) fire(u *gunim.UI) {
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
