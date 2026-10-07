package widget

import (
	"image/color"
	"strings"
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
// Given no width, as a child of a row with room to spare, it sets each
// line unbroken; a row that runs short squeezes it, and it wraps.
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
	// NoWrap keeps each line whole, however long, for text whose lines
	// mean something as they are, such as a key or a table of columns.
	// The label is as wide as its longest line where there is room.
	// Where there is not, it shows what fits and scrolls sideways with a
	// sideways scroll, or Shift and the wheel.
	NoWrap bool

	laid laidText
	sel  textSelection
	// word is the width of the longest word, for the text, face and size
	// in wordOf.
	word   float32
	wordOf wordKey
	// across is how far a NoWrap label is scrolled sideways, and over
	// how far it can be: its longest line's width past the box's.
	across, over float32
}

// wordKey is the text, face and size a label's widest word was measured for.
type wordKey struct {
	face *text.Face
	s    string
	size float32
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

func (l *Label) paragraph(f gunim.Frame, width float32) text.Paragraph {
	if l.NoWrap {
		// A width of zero sets each line unbroken.
		width = 0
	}
	return l.laid.layout(faceIn(l.Face, f.Theme), l.Text, text.Style{Size: l.Size.Get(f.Theme), Align: l.Align, MaxLines: l.MaxLines}, width)
}

// MinWidth implements [Shrinker]. It is the width of the label's widest word: a row that runs short
// squeezes the label no narrower, where it can, so it wraps between
// words. A label that keeps its lines whole scrolls, and takes any width.
func (l *Label) MinWidth(f gunim.Frame) float32 {
	if l.NoWrap {
		return 0
	}
	face, size := faceIn(l.Face, f.Theme), l.Size.Get(f.Theme)
	if key := (wordKey{face, l.Text, size}); key != l.wordOf {
		words := strings.Fields(l.Text)
		l.word = 0
		// The widest word is among the longest few.
		for _, i := range longest(words, 8) {
			l.word = max(l.word, face.Shape(words[i], size).Advance)
		}
		l.wordOf = key
	}
	return l.word
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
	if l.NoWrap {
		l.over = max(para.Size.W-box.W, 0)
		l.across = min(l.across, l.over)
		if l.over > 0 {
			x = -l.across
			defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: 1, Clip: true})()
		}
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
	if s, ok := e.(input.Scroll); ok {
		return l.scrollAcross(s, u)
	}
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
		if l.NoWrap && !e.Typed && e.Mods == 0 && l.keyAcross(e.Key, u) {
			return true
		}
		if e.Typed || e.Mods != input.ModControl && e.Mods != input.ModSuper {
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

// scrollAcross scrolls a NoWrap label wider than its box sideways, for
// a sideways scroll or Shift with the wheel, and reports false at its
// end, or with nothing to scroll, for whatever scrolls outside.
func (l *Label) scrollAcross(e input.Scroll, u *gunim.UI) bool {
	dx := e.Delta.X
	if dx == 0 && e.Mods.Has(input.ModShift) {
		dx = e.Delta.Y
	}
	if !l.NoWrap || dx == 0 || l.over <= 0 {
		return false
	}
	to := max(0, min(l.across-dx, l.over))
	if to == l.across {
		return false
	}
	l.across = to
	u.Invalidate()
	return true
}

// keyAcross scrolls a NoWrap label sideways from the keyboard, for
// whoever reads it without a wheel: Left and Right a step, Home and End
// to either end. It reports whether the key moved it.
func (l *Label) keyAcross(k input.Key, u *gunim.UI) bool {
	const step = 40
	to := l.across
	switch k {
	case input.KeyLeft:
		to -= step
	case input.KeyRight:
		to += step
	case input.KeyHome:
		to = 0
	case input.KeyEnd:
		to = l.over
	default:
		return false
	}
	to = max(0, min(to, l.over))
	if to == l.across {
		return false
	}
	l.across = to
	u.Invalidate()
	return true
}
