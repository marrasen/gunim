package widget

import (
	"image/color"
	"unicode/utf8"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
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
	// Face is the face the text is set in, and the theme's [Font] when unset.
	Face  theme.Token[*text.Face]
	Size  theme.Token[float32]
	Color theme.Token[color.NRGBA]
	Align text.Align
	// MaxLines cuts the text after that many lines with an ellipsis.
	// Zero means no limit.
	MaxLines int
	// Selectable lets the mouse select the text and Ctrl+C copy it: a
	// drag selects a range, a double click a word and a triple click all.
	Selectable bool

	laid laidText
	sel  textSelection
}

// textSelection is the selection in text that is read but not edited.
type textSelection struct {
	caret, anchor int
	// held is set while a press drags the selection.
	held bool
	// x is where the paragraph was last painted, across the node.
	x float32
}

// NewLabel returns a label showing s.
func NewLabel(s string) *Label { return &Label{Text: s, Size: TextSize, Color: Ink} }

// SetText changes the text. Call it from a view's update function.
func (l *Label) SetText(s string) { l.Text = s }

func (l *Label) paragraph(f gunim.Frame, width float32) text.Paragraph {
	return l.laid.layout(faceIn(l.Face, f.Theme), l.Text, text.Style{Size: l.Size.Get(f.Theme), Align: l.Align, MaxLines: l.MaxLines}, width)
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
	l.sel.x = x
	if start, end := l.Selection(); start != end {
		sel := paint.Solid(Selection.Get(f.Theme))
		paraSpans(para, start, end, func(r geom.Rect) { p.RRect(r.Add(geom.Pt(x, 0)), 3, sel) })
	}
	para.Paint(p, geom.Pt(x, 0), l.Color.Get(f.Theme))
}

// Selection returns the selected runes' range, start before end.
func (l *Label) Selection() (start, end int) {
	n := utf8.RuneCountInString(l.Text)
	return min(l.sel.caret, l.sel.anchor, n), min(max(l.sel.caret, l.sel.anchor), n)
}

// SelectedText returns the selected text.
func (l *Label) SelectedText() string {
	start, end := l.Selection()
	return string([]rune(l.Text)[start:end])
}

// Focusable implements [gunim.Focusable]: a selectable label takes
// focus from a click, so Ctrl+C reaches it.
func (l *Label) Focusable() bool { return l.Selectable }

// SkipsTab implements [gunim.TabSkipper].
func (l *Label) SkipsTab() {}

// DragHeld implements [gunim.DragHolder], so a scroll view scrolls
// while a selection is dragged past its edge.
func (l *Label) DragHeld() bool { return l.sel.held }

// Handle implements [gunim.Handler]: on a selectable label, the mouse
// selects, Ctrl+C copies and Ctrl+A selects all.
func (l *Label) Handle(e input.Event, u *gunim.UI) bool {
	if !l.Selectable {
		return false
	}
	switch e := e.(type) {
	case input.FocusLost:
		l.sel.anchor, l.sel.held = l.sel.caret, false
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		i := l.laid.p.Index(e.Pos.Sub(geom.Pt(l.sel.x, 0)))
		if e.Clicks < 2 && e.Mods.Has(input.ModShift) {
			l.sel.caret = i
		} else {
			l.sel.anchor, l.sel.caret = clickRange([]rune(l.Text), i, e.Clicks)
		}
		l.sel.held = true
	case input.PointerMove:
		if !l.sel.held {
			return false
		}
		l.sel.caret = l.laid.p.Index(e.Pos.Sub(geom.Pt(l.sel.x, 0)))
	case input.PointerUp:
		if !l.sel.held {
			return false
		}
		l.sel.held = false
	case input.KeyPress:
		if e.Typed || !e.Mods.Has(input.ModControl) && !e.Mods.Has(input.ModSuper) {
			return false
		}
		switch start, end := l.Selection(); {
		case e.Key == input.KeyC && start != end:
			u.SetClipboard(l.SelectedText())
		case e.Key == input.KeyA:
			l.sel.anchor, l.sel.caret = 0, utf8.RuneCountInString(l.Text)
		default:
			return false
		}
	default:
		return false
	}
	u.Invalidate()
	return true
}
