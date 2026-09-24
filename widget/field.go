package widget

import (
	"strings"
	"time"
	"unicode"

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
// clicks, and Ctrl+A, C, X and V. The caret and the selection glide to
// where they go with the theme's [Caret] motion, the text slides
// sideways to keep the caret in view, and focus grows a ring around the
// field.
//
// While the field has focus, keys pressed without Ctrl, Alt or Super
// stop at it, so typing never sets off a window's shortcuts.
type TextField struct {
	Placeholder string
	// OnChange and OnSubmit turn the text into an intent to send when
	// it changes, and when Enter is pressed. They run on the UI
	// goroutine; what reaches the application is the value they return.
	OnChange func(text string) gunim.Intent
	OnSubmit func(text string) gunim.Intent

	text          []rune
	caret, anchor int
	held          bool

	focus  *anim.Float
	caretX *anim.Float
	selA   *anim.Float
	selB   *anim.Float
	scroll *anim.Float

	shaped shapedText
}

// NewTextField returns an empty field.
func NewTextField() *TextField {
	return &TextField{
		focus:  anim.NewFloat(0),
		caretX: anim.NewFloat(0),
		selA:   anim.NewFloat(0),
		selB:   anim.NewFloat(0),
		scroll: anim.NewFloat(0),
	}
}

// Focusable implements [gunim.Focusable].
func (t *TextField) Focusable() bool { return true }

// Text returns the field's text.
func (t *TextField) Text() string { return string(t.text) }

// SetText replaces the text and puts the caret at its end. Call it from
// a view's update function. An application that sets the text on every
// change it hears about should skip the ones the field sent, or it will
// move the caret under the user's fingers.
func (t *TextField) SetText(s string) {
	t.text = []rune(s)
	t.caret, t.anchor = len(t.text), len(t.text)
}

// Selection returns the selected runes' range, start before end.
func (t *TextField) Selection() (start, end int) {
	return min(t.caret, t.anchor), max(t.caret, t.anchor)
}

// Step implements [gunim.Animator].
func (t *TextField) Step(dt time.Duration) bool {
	moving := false
	for _, a := range []*anim.Float{t.focus, t.caretX, t.selA, t.selB, t.scroll} {
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
	case input.PointerDown:
		i := t.indexAt(e.Pos, u)
		switch {
		case e.Clicks >= 3:
			t.anchor, t.caret = 0, len(t.text)
		case e.Clicks == 2:
			t.anchor, t.caret = wordStart(t.text, i), wordEnd(t.text, i)
		case e.Mods.Has(input.ModShift):
			t.caret = i
		default:
			t.caret, t.anchor = i, i
		}
		t.held = true
	case input.PointerMove:
		if !t.held {
			return false
		}
		t.caret = t.indexAt(e.Pos, u)
	case input.PointerUp:
		t.held = false
	case input.TextInput:
		t.insert(e.Text, u)
	case input.KeyPress:
		return t.key(e, u)
	default:
		return false
	}
	u.Invalidate()
	return true
}

// indexAt returns the rune index a pointer at p, in the field's space,
// puts the caret at.
func (t *TextField) indexAt(p geom.Point, u *gunim.UI) int {
	run := t.run(u.Theme())
	return run.Index(p.X - FieldPadding.Get(u.Theme()) + t.scroll.Value())
}

// key handles a key press. It reports false for a shortcut the field
// leaves to its ancestors.
func (t *TextField) key(e input.KeyPress, u *gunim.UI) bool {
	ctrl := e.Mods.Has(input.ModControl) || e.Mods.Has(input.ModSuper)
	shift := e.Mods.Has(input.ModShift)
	start, end := t.Selection()
	move := func(to int) {
		t.caret = max(0, min(to, len(t.text)))
		if !shift {
			t.anchor = t.caret
		}
	}
	switch e.Key {
	case input.KeyLeft:
		switch {
		case ctrl:
			move(wordStart(t.text, t.caret-1))
		case start != end && !shift:
			move(start)
		default:
			move(t.caret - 1)
		}
	case input.KeyRight:
		switch {
		case ctrl:
			move(wordEnd(t.text, t.caret+1))
		case start != end && !shift:
			move(end)
		default:
			move(t.caret + 1)
		}
	case input.KeyHome:
		move(0)
	case input.KeyEnd:
		move(len(t.text))
	case input.KeyBackspace:
		switch {
		case start != end:
			t.replace(start, end, nil, u)
		case ctrl:
			t.replace(wordStart(t.text, t.caret-1), t.caret, nil, u)
		case t.caret > 0:
			t.replace(t.caret-1, t.caret, nil, u)
		}
	case input.KeyDelete:
		switch {
		case start != end:
			t.replace(start, end, nil, u)
		case ctrl:
			t.replace(t.caret, wordEnd(t.text, t.caret+1), nil, u)
		case t.caret < len(t.text):
			t.replace(t.caret, t.caret+1, nil, u)
		}
	case input.KeyTab:
		return false // focus moves on
	case input.KeyEnter:
		if t.OnSubmit != nil {
			u.Send(t, t.OnSubmit(t.Text()))
		}
	case input.KeyA, input.KeyC, input.KeyX, input.KeyV:
		if !ctrl {
			return true // typing: the letter arrives as TextInput
		}
		t.clipboard(e.Key, start, end, u)
	default:
		// Keys with a modifier are shortcuts for someone else; the rest
		// are typing, which arrives as TextInput.
		return !ctrl && !e.Mods.Has(input.ModAlt)
	}
	return true
}

func (t *TextField) clipboard(k input.Key, start, end int, u *gunim.UI) {
	switch k {
	case input.KeyA:
		t.anchor, t.caret = 0, len(t.text)
	case input.KeyC:
		if start != end {
			u.SetClipboard(string(t.text[start:end]))
		}
	case input.KeyX:
		if start != end {
			u.SetClipboard(string(t.text[start:end]))
			t.replace(start, end, nil, u)
		}
	case input.KeyV:
		t.insert(u.Clipboard(), u)
	default:
	}
}

// insert puts s in place of the selection, as one line.
func (t *TextField) insert(s string, u *gunim.UI) {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return ' '
		case unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
	if s == "" {
		return
	}
	start, end := t.Selection()
	t.replace(start, end, []rune(s), u)
}

// replace swaps runes start to end for with, leaves the caret after it,
// and tells the application.
func (t *TextField) replace(start, end int, with []rune, u *gunim.UI) {
	if start < 0 || end > len(t.text) || start > end {
		return
	}
	out := make([]rune, 0, len(t.text)-(end-start)+len(with))
	out = append(out, t.text[:start]...)
	out = append(out, with...)
	out = append(out, t.text[end:]...)
	t.text = out
	t.caret = start + len(with)
	t.anchor = t.caret
	if t.OnChange != nil {
		u.Send(t, t.OnChange(t.Text()))
	}
}

// wordStart returns where the word at or before i starts, skipping
// spaces before it.
func wordStart(rs []rune, i int) int {
	i = max(0, min(i, len(rs)))
	for i > 0 && unicode.IsSpace(rs[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(rs[i-1]) {
		i--
	}
	return i
}

// wordEnd returns where the word at or after i ends, skipping spaces
// before it.
func wordEnd(rs []rune, i int) int {
	i = max(0, min(i, len(rs)))
	for i < len(rs) && unicode.IsSpace(rs[i]) {
		i++
	}
	for i < len(rs) && !unicode.IsSpace(rs[i]) {
		i++
	}
	return i
}

func (t *TextField) run(th *theme.Live) text.Run {
	return t.shaped.shape(string(t.text), TextSize.Get(th))
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

	// Aim the caret, the selection and the scroll. Each glides there.
	run := t.run(th)
	inner := own.W - 2*FieldPadding.Get(th)
	motion := Caret.Get(th)
	cx := run.CaretX(t.caret)
	t.caretX.Animate(cx, motion)
	a, b := run.CaretX(t.anchor), cx
	t.selA.Animate(min(a, b), motion)
	t.selB.Animate(max(a, b), motion)

	scroll := t.scroll.Target()
	switch {
	case cx < scroll:
		scroll = cx
	case cx > scroll+inner:
		scroll = cx - inner
	}
	scroll = max(0, min(scroll, run.Advance-inner))
	t.scroll.Animate(scroll, motion)
	return own
}

// Paint implements [gunim.Node].
func (t *TextField) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	focus := t.focus.Value()
	r := geom.Rect{Max: box.Point()}
	radius := FieldRadius.Get(th)
	border := anim.Mix(anim.ColorCodec, FieldBorder.Get(th), Accent.Get(th), min(focus, 1))
	p.RRectStroke(r, radius, paint.Solid(FieldFill.Get(th)), paint.Stroke{Width: 1 + focus, Color: border})

	pad := FieldPadding.Get(th)
	inner := geom.Rect{Min: geom.Pt(pad/2, 0), Max: geom.Pt(box.W-pad/2, box.H)}
	defer p.Layer(paint.LayerOpts{Bounds: inner, Opacity: 1, Clip: true})()

	run := t.run(th)
	x := pad - t.scroll.Value()
	y := (box.H - run.Height()) / 2
	if len(t.text) == 0 && t.Placeholder != "" {
		ph := text.Default().Shape(t.Placeholder, TextSize.Get(th))
		ph.Paint(p, geom.Pt(pad, y), Placeholder.Get(th))
	}

	if a, b := t.selA.Value(), t.selB.Value(); b-a > 0.5 && focus > 0 {
		sel := Selection.Get(th)
		sel.A = uint8(float32(sel.A) * min(focus, 1))
		p.RRect(geom.Rc(x+a, y, b-a, run.Height()), 3, paint.Solid(sel))
	}
	run.Paint(p, geom.Pt(x, y), Ink.Get(th))

	if focus > 0.01 {
		c := Accent.Get(th)
		c.A = uint8(float32(c.A) * min(focus, 1))
		p.RRect(geom.Rc(x+t.caretX.Value()-0.75, y, 1.5, run.Height()), 0.75, paint.Solid(c))
	}
}
