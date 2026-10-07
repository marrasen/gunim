package widget

import (
	"image/color"
	"math"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
)

// Overview tokens.
var (
	OverviewWidth = theme.Length("overview.width", 20)
	OverviewFill  = theme.Color("overview.fill", color.NRGBA{R: 0x12, G: 0x14, B: 0x19, A: 0x99})
	// OverviewBox fills the box showing the rows in view, and
	// OverviewBoxEdge draws its top and bottom.
	OverviewBox     = theme.Color("overview.box", color.NRGBA{R: 0xb4, G: 0xc4, B: 0xff, A: 0x29})
	OverviewBoxEdge = theme.Color("overview.box.edge", color.NRGBA{R: 0xd8, G: 0xe2, B: 0xff, A: 0x8c})
	// OverviewMark lights the lane at the left edge.
	OverviewMark = theme.Color("overview.mark", color.NRGBA{R: 0x7c, G: 0x9c, B: 0xff, A: 0xe6})
	// OverviewHover marks the band under the pointer.
	OverviewHover = theme.Color("overview.hover", color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x73})
	// OverviewMinBox is the least height the box showing the view is
	// drawn at.
	OverviewMinBox = theme.Length("overview.box.min", 6)
)

// A Scroller is a widget that scrolls through rows by index, such as a
// [DataGrid], for an [Overview] to follow and move.
type Scroller interface {
	gunim.Node
	Rows() int
	// Top is the row at the top of the view, with a fraction for a row
	// partly scrolled off, and Visible how many rows the view holds.
	Top() float64
	Visible() float64
	JumpTo(row float64, center bool, u *gunim.UI)
}

// OverviewPart is one share of an [OverviewBand]'s width, in a colour.
type OverviewPart struct {
	// Share is the part's fraction of the band, from 0 to 1.
	Share float32
	Color theme.Token[color.NRGBA]
	// Faint draws the part at half strength.
	Faint bool
}

// OverviewBand is what an [Overview] shows for one stretch of rows.
type OverviewBand struct {
	// Parts fill the band from the left, each its share wide.
	Parts []OverviewPart
	// Alarm, when set, draws a mark in the gutter at the right edge,
	// Strength of the gutter wide.
	Alarm    theme.Token[color.NRGBA]
	Strength float32
	// Mark lights the lane at the left edge, as for a restart.
	Mark bool
}

// Overview is a strip that shows the shape of everything a [Scroller]
// holds, as a code editor's minimap does, and stands in for its
// scrollbar.
//
// The bands spread evenly over the Scroller's rows, top to bottom. A box
// shows the rows in view; a press moves the view there, and a drag moves
// it along, holding the box where it was taken hold of. The wheel
// scrolls the Scroller, and under the pointer Readout says what a band
// holds.
type Overview struct {
	Target Scroller
	Bands  []OverviewBand
	// Readout returns what band b holds, shown beside it under the
	// pointer; nil or an empty string shows nothing.
	Readout func(b int) string
	// Top leaves room above the strip, as beside a grid's header; unset leaves none.
	Top theme.Token[float32]

	// inset and height are where the strip starts and how tall it is, from the last layout.
	inset, height float32
	hover         int
	grab          float32
	grabbing      bool
	size          geom.Size
	readoutText   laidText
}

// NewOverview returns an overview of target.
func NewOverview(target Scroller) *Overview { return &Overview{Target: target, hover: -1} }

// Layout implements [gunim.Node]. The strip is the theme's
// [OverviewWidth] wide and fills the height it is given.
func (o *Overview) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	o.size = c.Constrain(geom.Sz(OverviewWidth.Get(f.Theme), c.Max.H))
	o.inset = 0
	if o.Top.Key() != "" {
		o.inset = min(o.Top.Get(f.Theme), o.size.H)
	}
	o.height = o.size.H - o.inset
	return o.size
}

// box returns the top and height of the box showing the view, and how
// far its top can travel.
func (o *Overview) box(th *theme.Live) (top, size, travel float32) {
	h := o.height
	rows := float64(0)
	if o.Target != nil {
		rows = float64(o.Target.Rows())
	}
	if rows <= 0 || h <= 0 {
		return 0, h, h
	}
	vis := o.Target.Visible()
	size = min(h, max(float32(vis/rows)*h, OverviewMinBox.Get(th)))
	travel = max(1, h-size)
	scrolled := max(1, rows-vis)
	return float32(o.Target.Top()/scrolled) * travel, size, travel
}

// Paint implements [gunim.Node].
func (o *Overview) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	defer p.Push(paint.Translate(geom.Pt(0, o.inset)))()
	box.H = o.height
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(OverviewFill.Get(th)))
	n := len(o.Bands)
	// One device pixel, so a band of one line among thousands still shows.
	px := 1 / max(f.Scale, 1)
	lane, gutter := float32(3), float32(6)
	band := max(px, box.W-gutter-lane-1)
	colors := map[string]color.NRGBA{}
	colorOf := func(t theme.Token[color.NRGBA], faint bool) color.NRGBA {
		c, ok := colors[t.Key()]
		if !ok {
			c = t.Get(th)
			colors[t.Key()] = c
		}
		if faint {
			c.A /= 2
		}
		return c
	}
	for b, bd := range o.Bands {
		y0 := snapDown(float32(b)*box.H/float32(n), px)
		y1 := max(y0+px, snapDown(float32(b+1)*box.H/float32(n), px))
		x := lane + 1
		for _, part := range bd.Parts {
			w := part.Share * band
			if w <= 0 {
				continue
			}
			p.RRect(geom.Rc(x, y0, max(w, 0.6*px), y1-y0), 0, paint.Solid(colorOf(part.Color, part.Faint)))
			x += w
		}
		if bd.Alarm.Key() != "" && bd.Strength > 0 {
			w := gutter * (0.45 + 0.55*min(bd.Strength, 1))
			p.RRect(geom.Rc(box.W-w, y0, w, y1-y0), 0, paint.Solid(colorOf(bd.Alarm, false)))
		}
		if bd.Mark {
			p.RRect(geom.Rc(0, y0, lane, y1-y0), 0, paint.Solid(OverviewMark.Get(th)))
		}
	}
	if o.hover >= 0 && o.hover < n {
		y := snapDown(float32(o.hover)*box.H/float32(n), px)
		p.RRect(geom.Rc(0, y, box.W, px), 0, paint.Solid(OverviewHover.Get(th)))
	}
	if o.Target != nil && float64(o.Target.Rows()) > o.Target.Visible() {
		top, size, _ := o.box(th)
		p.RRect(geom.Rc(0, top, box.W, size), 0, paint.Solid(OverviewBox.Get(th)))
		edge := OverviewBoxEdge.Get(th)
		p.RRect(geom.Rc(0, top, box.W, px), 0, paint.Solid(edge))
		p.RRect(geom.Rc(0, top+size-px, box.W, px), 0, paint.Solid(edge))
	}
	o.paintReadout(p, f, box)
}

// paintReadout floats what the band under the pointer holds to the left
// of the strip, or to its right where the window has no room on the left.
func (o *Overview) paintReadout(p *paint.Painter, f gunim.Frame, box geom.Size) {
	if o.hover < 0 || o.hover >= len(o.Bands) || o.grabbing || o.Readout == nil {
		return
	}
	s := o.Readout(o.hover)
	if s == "" {
		return
	}
	th := f.Theme
	para := o.readoutText.layout(faceIn(Font, th), s, text.Style{Size: TooltipSize.Get(th)}, 0)
	pad := float32(6)
	w, h := para.Size.W+2*pad, para.Size.H+2*pad
	y := (float32(o.hover) + 0.5) * box.H / float32(len(o.Bands))
	y = max(0, min(box.H-h, y-h/2))
	at := p.Transform().Apply(geom.Pt(-w-4, y))
	if at.X < 0 {
		at = p.Transform().Apply(geom.Pt(box.W+4, y))
	}
	fill, ink := TooltipFill.Get(th), TooltipInk.Get(th)
	p.Float(func(p *paint.Painter) {
		defer p.Push(paint.Translate(at))()
		p.ShadowRRect(geom.Rc(0, 0, w, h), 5, paint.Solid(fill),
			paint.Shadow{Offset: geom.Pt(0, 2), Blur: 8, Color: color.NRGBA{A: 0x60}})
		para.Paint(p, geom.Pt(pad, pad), ink)
	})
}

// snapDown rounds v down to a whole number of device pixels, px wide.
func snapDown(v, px float32) float32 { return float32(math.Floor(float64(v/px))) * px }

// Handle implements [gunim.Handler].
func (o *Overview) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		if e.Button != input.ButtonPrimary || o.Target == nil {
			return false
		}
		e.Pos.Y -= o.inset
		top, size, _ := o.box(u.Theme())
		// Inside the box the grip holds; outside it the box centres on the press.
		o.grab = size / 2
		if e.Pos.Y >= top && e.Pos.Y <= top+size {
			o.grab = e.Pos.Y - top
		}
		o.grabbing = true
		o.follow(e.Pos.Y, u)
		return true
	case input.PointerMove:
		e.Pos.Y -= o.inset
		if o.grabbing {
			o.follow(e.Pos.Y, u)
			return true
		}
		if n := len(o.Bands); n > 0 && o.height > 0 {
			hover := max(0, min(n-1, int(e.Pos.Y/o.height*float32(n))))
			if hover != o.hover {
				o.hover = hover
				u.Invalidate()
			}
		}
		return false
	case input.PointerUp:
		if !o.grabbing {
			return false
		}
		o.grabbing = false
		u.Invalidate()
		return true
	case input.PointerLeave:
		if o.hover >= 0 {
			o.hover = -1
			u.Invalidate()
		}
	case input.Scroll:
		if h, ok := o.Target.(gunim.Handler); ok {
			return h.Handle(e, u)
		}
	}
	return false
}

// follow moves the view so the box's grip is at y.
func (o *Overview) follow(y float32, u *gunim.UI) {
	_, _, travel := o.box(u.Theme())
	frac := float64(max(0, min(1, (y-o.grab)/travel)))
	o.Target.JumpTo(frac*max(0, float64(o.Target.Rows())-o.Target.Visible()), false, u)
}

// Cursor implements [gunim.CursorShaper].
func (o *Overview) Cursor(geom.Point) input.Cursor { return input.CursorHand }
