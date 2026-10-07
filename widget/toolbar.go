package widget

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Toolbar tokens.
var (
	ToolbarFill    = theme.Color("toolbar.fill", color.NRGBA{R: 0x2a, G: 0x2f, B: 0x3b, A: 0xff})
	ToolbarBorder  = theme.Color("toolbar.border", color.NRGBA{R: 0x3f, G: 0x46, B: 0x57, A: 0xff})
	ToolbarShadow  = theme.Color("toolbar.shadow", color.NRGBA{A: 0x70})
	ToolbarRadius  = theme.Length("toolbar.radius", 8)
	ToolbarPadding = theme.Insets("toolbar.padding", geom.Uniform(2))
	ToolbarGap     = theme.Length("toolbar.gap", 0)
)

// Toolbar is a small floating strip of buttons on a raised surface, such as the actions that show over a message
// under the pointer. It casts a shadow and holds its items in a row, each at its own size.
//
// The motion and the keys are its buttons': each lights under the pointer and squashes as it is pressed, and Tab
// reaches each in turn, Space or Enter pressing it.
type Toolbar struct {
	row *Flex
}

// NewToolbar returns a toolbar holding items in a row.
func NewToolbar(items ...gunim.Node) *Toolbar {
	row := Row(items...)
	row.Gap, row.Cross = ToolbarGap, CrossCenter
	return &Toolbar{row: row}
}

// Children implements [gunim.Composite].
func (t *Toolbar) Children() []gunim.Node { return []gunim.Node{t.row} }

// Layout implements [gunim.Node]: the items at their own sizes, with the padding around them.
func (t *Toolbar) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	return inset(gunim.Loose(c.Max), ToolbarPadding.Get(f.Theme), kids)
}

// Paint implements [gunim.Node].
func (t *Toolbar) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	th := f.Theme
	r, radius := geom.Rect{Max: box.Point()}, ToolbarRadius.Get(th)
	p.ShadowRRect(r, radius, paint.Solid(ToolbarFill.Get(th)),
		paint.Shadow{Offset: geom.Pt(0, 2), Blur: 6, Color: ToolbarShadow.Get(th)})
	p.RRectStroke(r, radius, paint.Fill{}, paint.Stroke{Width: 1, Color: ToolbarBorder.Get(th)})
	kids.At(0).Paint(p)
}
