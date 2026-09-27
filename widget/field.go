package widget

import (
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
)

// TextField is a single line of editable text.
//
// It takes typing, deleting, the arrow, Home and End keys with Shift to
// select and Ctrl to move by word, clicks and drags, double and triple
// clicks, and Ctrl+A, C, X and V. The arrow keys follow the screen, so
// in text that mixes directions the caret moves the way the key points.
// The caret and the selection glide to where they go with the theme's
// [Caret] motion, the text slides sideways to keep the caret in view,
// and focus grows a ring around the field.
//
// While the field has focus, keys pressed without Ctrl, Alt or Super
// stop at it, so typing never sets off a window's shortcuts.
//
// An input method composes in place: the composition shows at the caret,
// underlined, with the input method's own caret or highlight inside it,
// until it commits.
type TextField struct {
	Placeholder string
	// Face is the face the text is set in, and the theme's [Font] when unset.
	Face theme.Token[*text.Face]
	// Secret shows each character as a dot, as a password field does,
	// keeps the text off the clipboard, and gives a screen reader no
	// value to read.
	Secret bool
	// OnChange and OnSubmit turn the text into an intent to send when
	// it changes, and when Enter is pressed. They run on the UI
	// goroutine; what reaches the application is the value they return.
	OnChange func(text string) gunim.Intent
	OnSubmit func(text string) gunim.Intent
	// OnEdit runs on the UI goroutine each time the text changes, for a
	// widget around the field that reacts at once, as a palette filters
	// its list.
	OnEdit func(text string, u *gunim.UI)
	// Ghost is a suggestion for the rest of the text, such as the rest
	// of a folder's name, shown faintly after the caret while the caret
	// is at the end. Tab or Right takes it. Set it from OnEdit.
	Ghost string

	editor
	held bool

	focus   *anim.Float
	caretAt *anim.Float
	selA    *anim.Float
	selB    *anim.Float
	scroll  *anim.Float
	// flash runs from 1 down to 0 after Flash, tinting the field.
	flash *anim.Float

	shaped shapedText
	// size is the field's size at its last layout.
	size geom.Size
	// line is the text as last laid out, for navigating.
	line text.Run
}

// NewTextField returns an empty field.
func NewTextField() *TextField {
	t := &TextField{
		focus:   anim.NewFloat(0),
		caretAt: anim.NewFloat(0),
		selA:    anim.NewFloat(0),
		selB:    anim.NewFloat(0),
		scroll:  anim.NewFloat(0),
		flash:   anim.NewFloat(0),
	}
	t.changed = func(u *gunim.UI) {
		if t.OnEdit != nil {
			t.OnEdit(t.Text(), u)
		}
		if t.OnChange != nil {
			u.Send(t, t.OnChange(t.Text()))
		}
	}
	return t
}

// Focusable implements [gunim.Focusable].
func (t *TextField) Focusable() bool { return true }

// TakesText implements [gunim.TextTaker], so an input method composes
// into the field.
func (t *TextField) TakesText() bool { return true }

// TextCaret implements [gunim.CaretReporter].
func (t *TextField) TextCaret() geom.Rect {
	pad := FieldPadding.Default()
	h := t.line.Height()
	y := (t.size.H - h) / 2
	return geom.Rc(pad-t.scroll.Value()+t.caretAt.Value(), y, 1.5, h)
}

// Text returns the field's text.
func (t *TextField) Text() string { return string(t.text) }

// SetText replaces the text and puts the caret at its end. Call it from
// a view's update function. An application that sets the text on every
// change it hears about should skip the ones the field sent, or it will
// move the caret under the user's fingers.
func (t *TextField) SetText(s string) {
	t.text = []rune(s)
	t.set(len(t.text), false)
}

// Select selects the text from rune start to rune end, with the caret at
// end, such as a file's name without its extension. Call it from a
// view's update function.
func (t *TextField) Select(start, end int) {
	t.set(start, false)
	t.set(end, true)
}

// Flash tints the field in the accent colour and fades it back, to
// show that something other than typing changed it, such as a button
// that fills it in. A hidden field changes only its dots, which are
// easy to miss.
func (t *TextField) Flash() {
	t.flash.Jump(1)
	t.flash.Animate(0, anim.Tween{Duration: flashTime, Ease: anim.EaseInOut})
}

// flashTime is how long a flash takes to fade.
const flashTime = 700 * time.Millisecond

// Step implements [gunim.Animator].
func (t *TextField) Step(dt time.Duration) bool {
	moving := false
	for _, a := range []*anim.Float{t.focus, t.caretAt, t.selA, t.selB, t.scroll, t.flash} {
		if a.Step(dt) {
			moving = true
		}
	}
	return moving
}

// Handle implements [gunim.Handler].
func (t *TextField) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.FocusGained:
		t.focus.Animate(1, Quick.Get(u.Theme()))
	case input.FocusLost:
		t.focus.Animate(0, Settle.Get(u.Theme()))
		t.anchor = t.caret
		t.preedit = nil // the driver ends the composition too
	case input.PointerDown:
		t.press(t.indexAt(e.Pos, u), e.Clicks, e.Mods.Has(input.ModShift))
		t.held = true
	case input.PointerMove:
		if !t.held {
			return false
		}
		t.set(t.indexAt(e.Pos, u), true)
	case input.PointerUp:
		t.held = false
	case input.TextInput:
		t.preedit = nil
		t.insert(e.Text, u)
	case input.Composing:
		t.compose(e)
	case input.KeyPress:
		if e.Key == input.KeyEnter || e.Key == input.KeyKPEnter {
			// With nothing to submit to, Enter is for the nodes around
			// the field, such as a dialog's default button.
			if t.OnSubmit == nil {
				return false
			}
			u.Send(t, t.OnSubmit(t.Text()))
			return true
		}
		if t.Ghost != "" && e.Mods == 0 && (e.Key == input.KeyTab || e.Key == input.KeyRight) && t.atEnd() {
			ghost := t.Ghost
			t.Ghost = ""
			t.insert(ghost, u)
			break
		}
		if !t.key(e, u, fieldNav{t}) {
			return false
		}
	default:
		return false
	}
	u.Invalidate()
	return true
}

// atEnd reports whether the caret is at the end of the text, with
// nothing selected and nothing being composed.
func (t *TextField) atEnd() bool {
	return t.caret == len(t.text) && t.anchor == t.caret && len(t.preedit) == 0
}

// indexAt returns the rune index a pointer at p, in the field's space,
// puts the caret at.
func (t *TextField) indexAt(p geom.Point, u *gunim.UI) int {
	return t.run(u.Theme()).Index(p.X - FieldPadding.Get(u.Theme()) + t.scroll.Value())
}

// fieldNav navigates a field's one line.
type fieldNav struct{ t *TextField }

func (n fieldNav) caretX(i int) float32 { return n.t.line.CaretX(i) }

func (n fieldNav) beside(i int, x float32, right bool) (next int, nextX float32) {
	return n.t.line.Beside(i, x, right)
}

func (n fieldNav) lineStart(int) int { return 0 }

func (n fieldNav) lineEnd(int) int { return len(n.t.text) }

func (n fieldNav) vertical(i int, _ float32, _ int) (int, bool) { return i, false }

func (n fieldNav) page() int { return 1 }

func (t *TextField) run(th *theme.Live) text.Run {
	shown, _ := t.shown()
	t.secret = t.Secret
	if t.Secret {
		shown = []rune(strings.Repeat("•", len(shown)))
	}
	t.line = t.shaped.shape(faceIn(t.Face, th), string(shown), TextSize.Get(th))
	return t.line
}

// Layout implements [gunim.Node]. The field fills the width it is given,
// or the theme's [FieldWidth] when the width is unbounded.
func (t *TextField) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	w := c.Max.W
	if w <= 0 {
		w = FieldWidth.Get(th)
	}
	own := c.Constrain(geom.Sz(w, FieldHeight.Get(th)))
	t.size = own

	// Aim the caret, the selection and the scroll. Each glides there.
	run := t.run(th)
	inner := own.W - 2*FieldPadding.Get(th)
	motion := Caret.Get(th)
	caret, anchor := t.drawnCaret()
	cx := run.CaretX(caret)
	if t.hinted && len(t.preedit) == 0 && run.Places(caret, t.hintX) {
		cx = t.hintX
	}
	t.aim(t.caretAt, cx, motion)
	a, b := run.CaretX(anchor), cx
	t.aim(t.selA, min(a, b), motion)
	t.aim(t.selB, max(a, b), motion)

	scroll := t.scroll.Target()
	switch {
	case cx < scroll:
		scroll = cx
	case cx > scroll+inner:
		scroll = cx - inner
	}
	scroll = max(0, min(scroll, run.Advance-inner))
	t.aim(t.scroll, scroll, motion)
	t.edited = false
	return own
}

// Paint implements [gunim.Node].
func (t *TextField) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	focus := t.focus.Value()
	r := geom.Rect{Max: box.Point()}
	radius := FieldRadius.Get(th)
	border := anim.Mix(anim.ColorCodec, FieldBorder.Get(th), Accent.Get(th), min(focus, 1))
	fill := FieldFill.Get(th)
	if lit := min(max(t.flash.Value(), 0), 1); lit > 0 {
		fill = anim.Mix(anim.ColorCodec, fill, Accent.Get(th), 0.35*lit)
		border = anim.Mix(anim.ColorCodec, border, Accent.Get(th), lit)
	}
	p.RRectStroke(r, radius, paint.Solid(fill), paint.Stroke{Width: 1 + focus, Color: border})

	pad := FieldPadding.Get(th)
	inner := geom.Rect{Min: geom.Pt(pad/2, 0), Max: geom.Pt(box.W-pad/2, box.H)}
	defer p.Layer(paint.LayerOpts{Bounds: inner, Opacity: 1, Clip: true})()

	run := t.run(th)
	x := pad - t.scroll.Value()
	y := (box.H - run.Height()) / 2
	if len(t.text) == 0 && len(t.preedit) == 0 && t.Placeholder != "" {
		ph := faceIn(t.Face, th).Shape(t.Placeholder, TextSize.Get(th))
		ph.Paint(p, geom.Pt(pad, y), Placeholder.Get(th))
	}

	if a, b := t.selA.Value(), t.selB.Value(); b-a > 0.5 && focus > 0 {
		sel := Selection.Get(th)
		sel.A = uint8(float32(sel.A) * min(focus, 1))
		p.RRect(geom.Rc(x+a, y, b-a, run.Height()), 3, paint.Solid(sel))
	}
	run.Paint(p, geom.Pt(x, y), Ink.Get(th))
	if t.Ghost != "" && focus > 0.01 && t.atEnd() && !t.Secret {
		ghost := faceIn(t.Face, th).Shape(t.Ghost, TextSize.Get(th))
		ghost.Paint(p, geom.Pt(x+run.Advance, y), Placeholder.Get(th))
	}
	if len(t.preedit) > 0 {
		// Underline the composition, as input methods expect.
		_, at := t.shown()
		x0, x1 := run.CaretX(at), run.CaretX(at+len(t.preedit))
		p.RRect(geom.Rc(x+min(x0, x1), y+run.Ascent+2, abs32(x1-x0), 1), 0, paint.Solid(Ink.Get(th)))
	}

	if focus > 0.01 {
		c := Accent.Get(th)
		c.A = uint8(float32(c.A) * min(focus, 1))
		p.RRect(geom.Rc(x+t.caretAt.Value()-0.75, y, 1.5, run.Height()), 0.75, paint.Solid(c))
	}
}
