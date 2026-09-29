package main

import (
	"strconv"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/widget"
)

// Sizes of a poll's card.
const (
	pollMaxW    = 420
	pollPad     = 12
	pollOptionH = 34
	pollGap     = 6
)

// pollCard is a poll in a message: its question, and an option a row, each with a bar as long as its share of the
// votes, how many voted for it, and a tick on the user's own. A click on an option votes for it, and on the user's
// own takes the vote back. The bars spring to new lengths as votes come.
type pollCard struct {
	anim.Group
	id    string
	p     Poll
	share []*anim.Float
	hover int

	question text.Paragraph
	options  []text.Run
	counts   []text.Run
	foot     text.Run
	rows     []geom.Rect
	card     geom.Rect
}

func newPollCard(id string, p Poll) *pollCard {
	c := &pollCard{id: id, hover: -1}
	c.take(p, nil)
	return c
}

// take keeps p, and sends each option's bar toward its share, at once without u.
func (c *pollCard) take(p Poll, u *gunim.UI) {
	c.p = p
	for len(c.share) < len(p.Options) {
		s := anim.NewFloat(0)
		c.share = append(c.share, s)
		c.Add(s)
	}
	for i, o := range p.Options {
		to := float32(0)
		if p.Voters > 0 {
			to = float32(o.Votes) / float32(p.Voters)
		}
		if u == nil {
			c.share[i].Jump(to)
		} else {
			c.share[i].Animate(to, widget.Settle.Get(u.Theme()))
		}
	}
}

// set shows p.
func (c *pollCard) set(p Poll, u *gunim.UI) {
	c.take(p, u)
	u.Invalidate()
}

// Layout implements [gunim.Node].
func (c *pollCard) Layout(cs gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	if c.p.Question == "" {
		return geom.Sz(cs.Max.W, 0)
	}
	th := f.Theme
	w := min(cs.Max.W, pollMaxW)
	inner := w - 2*pollPad
	size := widget.TextSize.Get(th)
	c.question = widget.BoldFont.Get(th).Layout(c.p.Question, text.Style{Size: size}, inner)
	y := pollPad + c.question.Size.H + pollGap
	c.options, c.counts, c.rows = c.options[:0], c.counts[:0], c.rows[:0]
	for _, o := range c.p.Options {
		c.counts = append(c.counts, widget.BoldFont.Get(th).Shape(strconv.Itoa(o.Votes), SmallText.Get(th)))
		c.options = append(c.options, shapeFit(widget.Font.Get(th), o.Text, size, inner-60))
		c.rows = append(c.rows, geom.Rc(pollPad, y, inner, pollOptionH))
		y += pollOptionH + pollGap
	}
	voters := strconv.Itoa(c.p.Voters) + " votes"
	if c.p.Voters == 1 {
		voters = "1 vote"
	}
	c.foot = widget.Font.Get(th).Shape(voters, SmallText.Get(th))
	y += c.foot.Height() + pollPad - pollGap/2
	c.card = geom.Rc(0, 0, w, y)
	return geom.Sz(cs.Max.W, y)
}

// Paint implements [gunim.Node].
func (c *pollCard) Paint(p *paint.Painter, f gunim.Frame, _ geom.Size, _ gunim.Children) {
	if c.p.Question == "" {
		return
	}
	th := f.Theme
	p.RRect(c.card, 10, paint.Solid(QuoteFill.Get(th)))
	ink, faint, accent := widget.Ink.Get(th), Faint.Get(th), widget.Accent.Get(th)
	c.question.Paint(p, geom.Pt(pollPad, pollPad), ink)
	for i, row := range c.rows {
		o := c.p.Options[i]
		p.RRect(row, 8, paint.Solid(ChipFillOff.Get(th)))
		if i == c.hover {
			p.RRect(row, 8, paint.Solid(fade(ink, 0.06)))
		}
		if s := min(max(c.share[i].Value(), 0), 1); s > 0.001 {
			bar := fade(accent, 0.28)
			if o.Mine {
				bar = fade(accent, 0.5)
			}
			p.RRect(geom.Rc(row.Min.X, row.Min.Y, row.Size().W*s, row.Size().H), 8, paint.Solid(bar))
		}
		if o.Mine {
			p.RRectStroke(row, 8, paint.Fill{}, paint.Stroke{Width: 1, Color: accent})
		}
		x := row.Min.X + 12
		if o.Mine {
			s := float32(16)
			widget.PaintIcon(p, th, icon.Check, geom.Rc(x, row.Center().Y-s/2, s, s), accent)
			x += s + 6
		}
		run := c.options[i]
		run.Paint(p, geom.Pt(x, row.Center().Y-run.Height()/2), ink)
		n := c.counts[i]
		n.Paint(p, geom.Pt(row.Max.X-12-n.Advance, row.Center().Y-n.Height()/2), faint)
	}
	last := c.rows[len(c.rows)-1]
	c.foot.Paint(p, geom.Pt(pollPad, last.Max.Y+pollGap/2), faint)
}

// at returns the option under pt, or -1.
func (c *pollCard) at(pt geom.Point) int {
	for i, r := range c.rows {
		if r.Contains(pt) {
			return i
		}
	}
	return -1
}

// Handle implements [gunim.Handler]: an option lights under the pointer, and a click votes for it.
func (c *pollCard) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		if i := c.at(e.Pos); i != c.hover {
			c.hover = i
			u.Invalidate()
		}
	case input.PointerLeave:
		c.hover = -1
		u.Invalidate()
	case input.PointerDown:
		if i := c.at(e.Pos); i >= 0 && e.Button == input.ButtonPrimary {
			u.Send(c, PollVoted{ID: c.id, Option: i})
			return true
		}
	}
	return false
}

// Cursor implements [gunim.CursorShaper].
func (c *pollCard) Cursor(pt geom.Point) input.Cursor {
	if c.at(pt) >= 0 {
		return input.CursorHand
	}
	return input.CursorArrow
}
