package widget

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
)

// Label shows text, wrapped to the width it is given.
//
// Given no width, as a child of a row that does not grow it, it sets
// each line unbroken.
type Label struct {
	Text string
	// Size and Color default to the theme's [TextSize] and [Ink]; set
	// them to tokens of your own for a heading or a caption.
	Size  theme.Token[float32]
	Color theme.Token[color.NRGBA]
	Align text.Align
	// MaxLines cuts the text after that many lines with an ellipsis.
	// Zero means no limit.
	MaxLines int

	laid laidText
}

// NewLabel returns a label showing s.
func NewLabel(s string) *Label { return &Label{Text: s, Size: TextSize, Color: Ink} }

// SetText changes the text. Call it from a view's update function.
func (l *Label) SetText(s string) { l.Text = s }

func (l *Label) paragraph(f gunim.Frame, width float32) text.Paragraph {
	return l.laid.layout(l.Text, text.Style{Size: l.Size.Get(f.Theme), Align: l.Align, MaxLines: l.MaxLines}, width)
}

// Layout implements [gunim.Node].
func (l *Label) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(l.paragraph(f, c.Max.W).Size)
}

// Paint implements [gunim.Node]. Given more width than its text needs,
// it places the text by its alignment.
func (l *Label) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	para := l.paragraph(f, box.W)
	var x float32
	switch l.Align {
	case text.AlignCenter:
		x = (box.W - para.Size.W) / 2
	case text.AlignEnd:
		x = box.W - para.Size.W
	case text.AlignStart:
	}
	para.Paint(p, geom.Pt(x, 0), l.Color.Get(f.Theme))
}
