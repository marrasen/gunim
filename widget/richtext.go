package widget

import (
	"image/color"
	"slices"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
)

// RichSpan is a piece of a [RichText] in a style of its own.
type RichSpan struct {
	Text string
	// Face, Size and Ink default to the theme's [Font], [TextSize] and
	// [Ink], and to [LinkInk] for a link.
	Face theme.Token[*text.Face]
	Size theme.Token[float32]
	Ink  theme.Token[color.NRGBA]
	// Mark, when set, colours the span's background.
	Mark      theme.Token[color.NRGBA]
	Underline bool
	// On makes the span a link: a click on it sends On.
	On gunim.Intent
}

// RichText is text in several styles, wrapped to the width it is given:
// bold within a sentence, a word in code, a link.
type RichText struct {
	Spans []RichSpan
	Align text.Align
	// LineHeight scales the lines' spacing. Zero means 1.
	LineHeight float32

	laid  text.SpanParagraph
	key   []text.Span
	width float32
	// hover is the link under the pointer, or -1.
	hover int
}

// NewRichText returns text made of spans.
func NewRichText(spans ...RichSpan) *RichText { return &RichText{Spans: spans, hover: -1} }

// SetSpans changes the text. Call it from a view's update function.
func (r *RichText) SetSpans(spans ...RichSpan) { r.Spans = spans }

func (r *RichText) paragraph(th *theme.Live, width float32) text.SpanParagraph {
	spans := make([]text.Span, len(r.Spans))
	for i, s := range r.Spans {
		size := TextSize.Get(th)
		if s.Size.Key() != "" {
			size = s.Size.Get(th)
		}
		spans[i] = text.Span{Text: s.Text, Face: faceIn(s.Face, th), Size: size}
	}
	if !slices.Equal(spans, r.key) || width != r.width {
		r.key, r.width = spans, width
		r.laid = text.LayoutSpans(spans, text.Style{Align: r.Align, LineHeight: r.LineHeight}, width)
	}
	return r.laid
}

// Layout implements [gunim.Node].
func (r *RichText) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(r.paragraph(f.Theme, c.Max.W).Size)
}

// Paint implements [gunim.Node].
func (r *RichText) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	laid := r.paragraph(th, box.W)
	for _, l := range laid.Lines {
		for _, pc := range l.Pieces {
			if pc.Span >= len(r.Spans) {
				continue
			}
			s := r.Spans[pc.Span]
			ink := Ink.Get(th)
			switch {
			case s.Ink.Key() != "":
				ink = s.Ink.Get(th)
			case s.On != nil:
				ink = LinkInk.Get(th)
			}
			if s.Mark.Key() != "" {
				p.RRect(geom.Rc(pc.At.X, pc.At.Y, pc.Run.Advance, pc.Run.Height()), 2, paint.Solid(s.Mark.Get(th)))
			}
			pc.Run.Paint(p, pc.At, ink)
			if s.Underline || s.On != nil && pc.Span == r.hover {
				p.RRect(geom.Rc(pc.At.X, pc.At.Y+pc.Run.Ascent+1.5, pc.Run.Advance, 1), 0, paint.Solid(ink))
			}
		}
	}
}

// linkAt returns the link span under pt, or -1.
func (r *RichText) linkAt(pt geom.Point) int {
	for _, l := range r.laid.Lines {
		for _, pc := range l.Pieces {
			if pc.Span < len(r.Spans) && r.Spans[pc.Span].On != nil &&
				geom.Rc(pc.At.X, pc.At.Y, pc.Run.Advance, pc.Run.Height()).Contains(pt) {
				return pc.Span
			}
		}
	}
	return -1
}

// Cursor implements [gunim.CursorShaper]: a hand over a link.
func (r *RichText) Cursor(pt geom.Point) input.Cursor {
	if r.linkAt(pt) >= 0 {
		return input.CursorHand
	}
	return input.CursorArrow
}

// Handle implements [gunim.Handler]: a click on a link sends it.
func (r *RichText) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		if at := r.linkAt(e.Pos); at != r.hover {
			r.hover = at
			u.Invalidate()
		}
	case input.PointerLeave:
		if r.hover >= 0 {
			r.hover = -1
			u.Invalidate()
		}
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		if at := r.linkAt(e.Pos); at >= 0 {
			u.Send(r, r.Spans[at].On)
			return true
		}
	}
	return false
}

// Access implements [gunim.Accessible]: the text, read as a label.
func (r *RichText) Access() access.Info {
	var b strings.Builder
	for _, s := range r.Spans {
		b.WriteString(s.Text)
	}
	return access.Info{Role: access.RoleLabel, Name: b.String()}
}
