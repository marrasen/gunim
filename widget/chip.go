package widget

import (
	"image/color"
	"math"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Chip tokens.
var (
	ChipHeight = theme.Length("chip.height", 24)
	ChipFill   = theme.Color("chip.fill", color.NRGBA{R: 0x2b, G: 0x2f, B: 0x3a, A: 0xff})
	ChipHover  = theme.Color("chip.hover", color.NRGBA{R: 0x3d, G: 0x45, B: 0x58, A: 0xff})
	// ChipLead colours a chip's lead, the part before its label, such as
	// the field a filter is on.
	ChipLead = theme.Color("chip.lead", color.NRGBA{R: 0x8a, G: 0x93, B: 0xa6, A: 0xff})
)

// Chip is a small rounded label with a cross that removes it, such as
// a filter in force.
type Chip struct {
	anim.Group
	// Lead is drawn dim before the label.
	Lead  string
	Label string
	// OnRemove turns a click on the cross into an intent.
	OnRemove func() gunim.Intent

	hover               *anim.Float
	leadText, labelText shapedText
	size                geom.Size
	crossX              float32
}

// NewChip returns a chip showing lead and label.
func NewChip(lead, label string) *Chip {
	c := &Chip{Lead: lead, Label: label, hover: anim.NewFloat(0)}
	c.Add(c.hover)
	return c
}

// Layout implements [gunim.Node].
func (c *Chip) Layout(cs gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	h := ChipHeight.Get(th)
	size := TextSize.Get(th) * 0.9
	w := h / 2
	if c.Lead != "" {
		w += c.leadText.shape(faceIn(BoldFont, th), c.Lead, size).Advance + 6
	}
	w += c.labelText.shape(faceIn(Font, th), c.Label, size).Advance
	c.crossX = w + h/2
	c.size = cs.Constrain(geom.Sz(w+h, h))
	return c.size
}

// Paint implements [gunim.Node].
func (c *Chip) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	fill := anim.Mix(anim.ColorCodec, ChipFill.Get(th), ChipHover.Get(th), c.hover.Value())
	p.RRect(geom.Rect{Max: box.Point()}, box.H/2, paint.Solid(fill))
	x := box.H / 2
	if c.Lead != "" {
		run := c.leadText.run
		run.Paint(p, geom.Pt(x, (box.H-run.Height())/2), ChipLead.Get(th))
		x += run.Advance + 6
	}
	run := c.labelText.run
	run.Paint(p, geom.Pt(x, (box.H-run.Height())/2), Ink.Get(th))
	drawCross(p, geom.Pt(c.crossX, box.H/2), box.H/3, Ink.Get(th))
}

// drawCross draws a small ×, size across, centred on at.
func drawCross(p *paint.Painter, at geom.Point, size float32, c color.NRGBA) {
	for _, turn := range [2]float32{math.Pi / 4, -math.Pi / 4} {
		func() {
			defer p.Push(paint.Rotate(turn, at))()
			p.RRect(geom.Rc(at.X-size/2, at.Y-0.75, size, 1.5), 0.75, paint.Solid(c))
		}()
	}
}

// Handle implements [gunim.Handler]: a click on the cross removes the
// chip.
func (c *Chip) Handle(e input.Event, u *gunim.UI) bool {
	th := u.Theme()
	switch e := e.(type) {
	case input.PointerMove:
		to := float32(0)
		if e.Pos.X >= c.crossX-c.size.H/2 {
			to = 1
		}
		c.hover.Animate(to, Quick.Get(th))
	case input.PointerLeave:
		c.hover.Animate(0, Settle.Get(th))
	case input.PointerDown:
		if e.Button != input.ButtonPrimary || e.Pos.X < c.crossX-c.size.H/2 {
			return false
		}
		c.remove(u)
		return true
	}
	return false
}

func (c *Chip) remove(u *gunim.UI) {
	if c.OnRemove != nil {
		if v := c.OnRemove(); v != nil {
			u.Send(c, v)
		}
	}
}

// Access implements [gunim.Accessible]: the chip reads as a button that
// removes it.
func (c *Chip) Access() access.Info {
	name := c.Label
	if c.Lead != "" {
		name = c.Lead + " " + c.Label
	}
	return access.Info{Role: access.RoleButton, Name: "Remove " + name, Actions: []string{access.ActionPress}}
}

// AccessAct implements [gunim.AccessActor].
func (c *Chip) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress {
		return false
	}
	c.remove(u)
	return true
}

// Wrap sets its children in a row, and starts a new row where the next
// child would pass its width, as words wrap. Its children come and go
// with [gunim.UI.Insert] and [gunim.UI.Remove].
type Wrap struct {
	// Gap is the space between children, and the theme's [Gap] when
	// unset.
	Gap theme.Token[float32]
	// Cross puts each child at the top of its line, or with CrossCenter or CrossEnd in its middle or at its bottom.
	Cross Cross
}

// NewWrap returns an empty wrap.
func NewWrap() *Wrap { return &Wrap{Gap: Gap} }

// Layout implements [gunim.Node]. The wrap is as wide as it is given,
// and as tall as its rows.
func (w *Wrap) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	gap := float32(0)
	if w.Gap.Key() != "" {
		gap = w.Gap.Get(f.Theme)
	}
	width := c.Max.W
	var x, y, line float32
	// start is the first child of the line being filled, and at and sizes where each child goes and how big it is
	start := 0
	var at []geom.Point
	var sizes []geom.Size
	endLine := func() {
		for i := start; i < len(at); i++ {
			switch w.Cross {
			case CrossCenter:
				at[i].Y += (line - sizes[i].H) / 2
			case CrossEnd:
				at[i].Y += line - sizes[i].H
			case CrossStart, CrossStretch:
			}
		}
		start = len(at)
	}
	for kid := range kids.All {
		s := kid.Layout(gunim.Loose(geom.Sz(width, 0)))
		if x > 0 && x+s.W > width {
			endLine()
			x, y, line = 0, y+line+gap, 0
		}
		at, sizes = append(at, geom.Pt(x, y)), append(sizes, s)
		x += s.W + gap
		line = max(line, s.H)
	}
	endLine()
	i := 0
	for kid := range kids.All {
		kid.Place(at[i])
		i++
	}
	return c.Constrain(geom.Sz(width, y+line))
}

// Paint implements [gunim.Node].
func (w *Wrap) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for kid := range kids.All {
		kid.Paint(p)
	}
}
