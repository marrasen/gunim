package widget

import (
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
)

// TextArea is editable text over several lines, wrapped to its width.
//
// It edits as [TextField] does, and adds lines: Enter starts one, Up and
// Down move between them toward the column the caret came from, Page Up
// and Page Down move by what shows, Home and End go to the ends of the
// line on screen and with Ctrl to the ends of the text. The caret glides
// between lines, and the text scrolls to keep it in view.
type TextArea struct {
	Placeholder string
	// Placeholders are shown in turn after Placeholder while the area is empty, each for [PlaceholderHold].
	Placeholders []string
	// Face is the face the text is set in, and the theme's [Font] when unset.
	Face theme.Token[*text.Face]
	// OnChange turns the text into an intent to send when it changes.
	OnChange func(text string) gunim.Intent
	// OnSubmit, when set, turns the text into an intent to send when Enter is pressed, and Shift+Enter starts a
	// line instead.
	OnSubmit func(text string) gunim.Intent
	// OnPasteImage, when set, turns a picture pasted with Ctrl+V, as PNG, into an intent to send. Without a
	// picture on the clipboard, Ctrl+V pastes text.
	OnPasteImage func(png []byte) gunim.Intent
	// Triggers are the characters that begin a word to complete, such as "@:" for mentions and emoji, at the start
	// of the text or after a space. Complete, when set, returns what the word typed after the trigger may become.
	// The suggestions open in a list at the caret, and while it is open Up and Down move through them, Enter or Tab
	// puts one in, and Escape closes the list; the keyboard stays in the text.
	Triggers string
	Complete func(trigger rune, query string) []Completion
	// CompleteAbove opens the suggestions above the caret, for a message box along the bottom of a window.
	CompleteAbove bool
	// Rows is how many lines tall the area is. MaxRows, when above Rows, lets the area grow with its text up to
	// that many lines, and scroll past them, as a chat's message box does.
	Rows, MaxRows int

	editor
	held bool

	focus   *anim.Float
	caretAt *anim.Point
	scroll  *anim.Float
	// lines carries how many lines tall the area is, as it grows and shrinks with its text; laid says it has been
	// laid out once.
	lines *anim.Float
	laid0 bool

	laid laidText
	// para is the text as last laid out, and view how tall its window
	// is, for navigating.
	para text.Paragraph
	view float32
	// followed is the caret the view last scrolled to keep in sight.
	followed int
	// completing is the list of suggestions open at the caret, and dismissedAt where the trigger was of the word
	// Escape closed it for, or -1.
	completing  *completing
	dismissedAt int
	// shownFrom is when the placeholders started taking turns.
	shownFrom time.Time
}

// PlaceholderHold is how long, in seconds, a text area shows each of its placeholders.
var PlaceholderHold = theme.Number("field.placeholder.hold", 15)

// placeholderTurn is how long one placeholder takes to give way to the next.
const placeholderTurn = 400 * time.Millisecond

// NewTextArea returns an empty text area five lines tall.
func NewTextArea() *TextArea {
	a := &TextArea{
		dismissedAt: -1,
		Rows:        5,
		focus:       anim.NewFloat(0),
		caretAt:     anim.NewPoint(geom.Point{}),
		scroll:      anim.NewFloat(0),
		lines:       anim.NewFloat(0),
	}
	a.multiline = true
	a.pasteImage = func(u *gunim.UI) bool {
		if a.OnPasteImage == nil {
			return false
		}
		b, err := u.ClipboardImage()
		if err != nil || len(b) == 0 {
			// With no picture to read, the paste is the clipboard's text.
			return false
		}
		u.Send(a, a.OnPasteImage(b))
		return true
	}
	a.changed = func(u *gunim.UI) {
		if a.OnChange != nil {
			u.Send(a, a.OnChange(a.Text()))
		}
	}
	return a
}

// Focusable implements [gunim.Focusable].
func (a *TextArea) Focusable() bool { return true }

// TakesText implements [gunim.TextTaker].
func (a *TextArea) TakesText() bool { return true }

// TextCaret implements [gunim.CaretReporter].
func (a *TextArea) TextCaret() geom.Rect {
	at := a.caretAt.Value().Add(a.origin(FieldPadding.Default()))
	h := a.para.LineHeight
	if len(a.para.Lines) > 0 {
		h = a.para.Lines[0].Run.Height()
	}
	return geom.Rc(at.X, at.Y, 1.5, h)
}

// caretRect returns where a caret before rune i would stand, in the
// area's space.
func (a *TextArea) caretRect(i int) geom.Rect {
	_, at := a.para.Caret(i)
	at = at.Add(a.origin(FieldPadding.Default()))
	h := a.para.LineHeight
	if len(a.para.Lines) > 0 {
		h = a.para.Lines[0].Run.Height()
	}
	return geom.Rc(at.X, at.Y, 1.5, h)
}

// hostIndex returns the rune a press at p in the area's space is before.
func (a *TextArea) hostIndex(p geom.Point, u *gunim.UI) int { return a.indexAt(p, u) }

// Text returns the area's text.
func (a *TextArea) Text() string { return string(a.text) }

// SetText replaces the text and puts the caret at its end. Call it from
// a view's update function; see [TextField.SetText].
func (a *TextArea) SetText(s string) {
	a.closeCompletion()
	a.dismissedAt = -1
	a.text = []rune(s)
	a.set(len(a.text), false)
	a.forget()
}

// Step implements [gunim.Animator].
func (a *TextArea) Step(dt time.Duration) bool {
	f, c, s, b, l := a.focus.Step(dt), a.caretAt.Step(dt), a.scroll.Step(dt), a.blink.step(dt), a.lines.Step(dt)
	return f || c || s || b || l
}

// Handle implements [gunim.Handler].
func (a *TextArea) Handle(e input.Event, u *gunim.UI) bool {
	if a.blink.windowFocus(e, u) {
		return false
	}
	switch e := e.(type) {
	case input.FocusGained:
		a.focus.Animate(1, Quick.Get(u.Theme()))
		a.blink.restart(u)
	case input.FocusLost:
		a.focus.Animate(0, Settle.Get(u.Theme()))
		a.blink.halt()
		a.anchor = a.caret
		a.preedit = nil
		a.closeCompletion()
		a.closeMenu()
		a.wording = false
		a.closeHandles()
	case input.PointerDown:
		if e.Button == input.ButtonSecondary {
			a.closeCompletion()
			a.contextPress(a, a.indexAt(e.Pos, u), e.Pos, e.Touch, u)
			break
		}
		a.closeHandles()
		a.press(a.indexAt(e.Pos, u), e.Clicks, e.Mods.Has(input.ModShift))
		a.held = true
		a.complete(u)
	case input.PointerMove:
		if a.wording {
			a.dragWords(a.indexAt(e.Pos, u))
			break
		}
		if !a.held {
			return false
		}
		a.set(a.indexAt(e.Pos, u), true)
	case input.PointerUp:
		a.held = false
		if a.endWords(a, e.Pos, u) {
			break
		}
		a.complete(u)
	case input.Scroll:
		to := max(0, min(a.scroll.Target()-e.Delta.Y, a.para.Size.H-a.view))
		if to == a.scroll.Target() {
			return false // at an end: for whatever scrolls outside
		}
		a.scroll.Animate(to, Quick.Get(u.Theme()))
	case input.TextInput:
		a.commit(e.Text, u)
		a.complete(u)
	case input.Composing:
		a.closeCompletion()
		a.compose(e)
	case input.TextEdit:
		a.edit(e, u)
		if len(a.preedit) > 0 {
			a.closeCompletion()
		} else {
			a.complete(u)
		}
	case input.KeyPress:
		if a.completionKey(e, u) {
			return true
		}
		if a.submits(e) {
			u.Send(a, a.OnSubmit(a.Text()))
			return true
		}
		if !a.key(e, u, areaNav{a}) {
			return false
		}
		a.complete(u)
	default:
		return false
	}
	if _, lost := e.(input.FocusLost); !lost {
		a.blink.restart(u)
	}
	u.Invalidate()
	return true
}

// submits reports whether k is an Enter that sends OnSubmit.
func (a *TextArea) submits(k input.KeyPress) bool {
	enter := k.Key == input.KeyEnter || k.Key == input.KeyKPEnter
	return enter && a.OnSubmit != nil && !k.Mods.Has(input.ModShift) && len(a.preedit) == 0
}

// origin returns where the paragraph's top-left sits in the area.
func (a *TextArea) origin(pad float32) geom.Point {
	return geom.Pt(pad, pad-a.scroll.Value())
}

// indexAt returns the rune index a pointer at p, in the area's space,
// puts the caret at.
func (a *TextArea) indexAt(p geom.Point, u *gunim.UI) int {
	return a.para.Index(p.Sub(a.origin(FieldPadding.Get(u.Theme()))))
}

// areaNav navigates an area's lines.
type areaNav struct{ a *TextArea }

func (n areaNav) caretX(i int) float32 {
	_, at := n.a.para.Caret(i)
	return at.X
}

func (n areaNav) beside(i int, x float32, right bool) (next int, nextX float32) {
	p := n.a.para
	line, _ := p.Caret(i)
	if line >= len(p.Lines) {
		return i, x
	}
	l := p.Lines[line]
	j, lx := l.Run.Beside(i, x-l.At.X, right)
	if j != i || lx != x-l.At.X {
		return j, lx + l.At.X
	}
	// At the edge of the line, carry on through the text.
	switch {
	case right && i < len(n.a.text):
		i++
	case !right && i > 0:
		i--
	default:
		return i, x
	}
	return i, n.caretX(i)
}

func (n areaNav) lineStart(i int) int {
	line, _ := n.a.para.Caret(i)
	if line >= len(n.a.para.Lines) {
		return 0
	}
	return n.a.para.Lines[line].Run.Start
}

// lineEnd returns the end of i's line on screen. A wrapped line ends
// where the next begins, which is where a caret shows on the next line,
// so the end of a wrapped line is one rune short: before the space the
// line broke at.
func (n areaNav) lineEnd(i int) int {
	p := n.a.para
	line, _ := p.Caret(i)
	if line >= len(p.Lines) {
		return len(n.a.text)
	}
	l := p.Lines[line].Run
	if line+1 < len(p.Lines) && p.Lines[line+1].Run.Start == l.End && l.End > l.Start {
		return l.End - 1
	}
	return l.End
}

func (n areaNav) vertical(i int, x float32, lines int) (int, bool) {
	p := n.a.para
	line, _ := p.Caret(i)
	to := line + lines
	switch {
	case to < 0:
		return 0, true
	case to >= len(p.Lines):
		return len(n.a.text), true
	}
	l := p.Lines[to]
	j, _ := l.Run.Hit(x - l.At.X)
	return j, true
}

func (n areaNav) page() int {
	if n.a.para.LineHeight <= 0 {
		return 1
	}
	return max(1, int(n.a.view/n.a.para.LineHeight)-1)
}

// Layout implements [gunim.Node]. The area fills the width it is given,
// or the theme's [AreaWidth], and is Rows lines tall.
func (a *TextArea) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	pad := FieldPadding.Get(th)
	w := c.Max.W
	if w <= 0 {
		w = AreaWidth.Get(th)
	}
	shown, _ := a.shown()
	a.para = a.laid.layout(faceIn(a.Face, th), string(shown), text.Style{Size: TextSize.Get(th)}, w-2*pad)
	rows := float32(max(a.Rows, 1))
	if a.MaxRows > a.Rows {
		want := float32(min(max(len(a.para.Lines), a.Rows, 1), a.MaxRows))
		if !a.laid0 {
			a.lines.Jump(want)
		} else {
			a.lines.Animate(want, Quick.Get(th))
		}
		rows = a.lines.Value()
	}
	a.laid0 = true
	own := c.Constrain(geom.Sz(w, rows*a.para.LineHeight+2*pad))
	a.view = own.H - 2*pad

	// Aim the caret, then scroll so its line shows.
	motion := Caret.Get(th)
	caret, _ := a.drawnCaret()
	line, at := a.para.Caret(caret)
	if a.hinted && len(a.preedit) == 0 && line < len(a.para.Lines) {
		l := a.para.Lines[line]
		if l.Run.Places(caret, a.hintX-l.At.X) {
			at.X = a.hintX
		}
	}
	if a.edited {
		a.caretAt.Jump(at)
	} else {
		a.caretAt.Animate(at, motion)
	}

	// Follow the caret only when it has moved or the text has changed,
	// so the wheel can scroll away from it; the next keystroke brings
	// it back into view.
	scroll := a.scroll.Target()
	if a.edited || caret != a.followed {
		switch {
		case at.Y < scroll:
			scroll = at.Y
		case at.Y+a.para.LineHeight > scroll+a.view:
			scroll = at.Y + a.para.LineHeight - a.view
		}
	}
	a.followed = caret
	a.aim(a.scroll, max(0, min(scroll, a.para.Size.H-a.view)), motion)
	a.edited = false
	a.followTrigger()
	a.placeHandles(a)
	return own
}

// paintPlaceholder draws the placeholder at o, and with several, turns to the next after each hold, the old one
// rising away as the new one rises into its place.
func (a *TextArea) paintPlaceholder(p *paint.Painter, f gunim.Frame, o geom.Point, width float32) {
	th := f.Theme
	all := a.Placeholders
	if a.Placeholder != "" {
		all = append([]string{a.Placeholder}, all...)
	}
	show := func(s string, at geom.Point, alpha float32) {
		c := Placeholder.Get(th)
		c.A = uint8(float32(c.A)*alpha + 0.5)
		faceIn(a.Face, th).Layout(s, text.Style{Size: TextSize.Get(th)}, width).Paint(p, at, c)
	}
	hold := time.Duration(PlaceholderHold.Get(th) * float32(time.Second))
	switch {
	case len(all) == 0:
		return
	case len(all) == 1 || hold <= placeholderTurn:
		show(all[0], o, 1)
		return
	}
	if a.shownFrom.IsZero() {
		a.shownFrom = f.Now
	}
	was, now, t, next := placeholderTurnAt(len(all), hold, f.Now.Sub(a.shownFrom))
	f.RedrawAt(a.shownFrom.Add(next))
	if t >= 1 {
		show(all[now], o, 1)
		return
	}
	const rise = 8
	show(all[was], o.Add(geom.Pt(0, -rise*t)), 1-t)
	show(all[now], o.Add(geom.Pt(0, rise*(1-t))), t)
}

// placeholderTurnAt returns which of n placeholders held for hold each show since after they start: the one giving
// way and the one taking over, how far the turn has got, eased from 0 to 1, and when, since the start, to draw next.
func placeholderTurnAt(n int, hold, since time.Duration) (was, now int, t float32, next time.Duration) {
	k := int(since / hold)
	in := since - time.Duration(k)*hold
	now = k % n
	if k == 0 || in >= placeholderTurn {
		return now, now, 1, time.Duration(k+1) * hold
	}
	t = float32(in) / float32(placeholderTurn)
	return (k - 1) % n, now, t * t * (3 - 2*t), since
}

// Paint implements [gunim.Node].
func (a *TextArea) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	focus := a.focus.Value()
	radius := FieldRadius.Get(th)
	border := anim.Mix(anim.ColorCodec, FieldBorder.Get(th), Accent.Get(th), min(focus, 1))
	p.RRectStroke(geom.Rect{Max: box.Point()}, radius, paint.Solid(FieldFill.Get(th)), paint.Stroke{Width: 1 + focus, Color: border})

	pad := FieldPadding.Get(th)
	inner := geom.Rect{Min: geom.Pt(pad/2, pad/2), Max: geom.Pt(box.W-pad/2, box.H-pad/2)}
	defer p.Layer(paint.LayerOpts{Bounds: inner, Opacity: 1, Clip: true})()

	o := a.origin(pad)
	if len(a.text) == 0 && len(a.preedit) == 0 {
		a.paintPlaceholder(p, f, o, box.W-2*pad)
	}

	caret, anchor := a.drawnCaret()
	if start, end := min(caret, anchor), max(caret, anchor); start != end && focus > 0 {
		sel := Selection.Get(th)
		sel.A = uint8(float32(sel.A) * min(focus, 1))
		a.spans(start, end, func(r geom.Rect) { p.RRect(r.Add(o), 3, paint.Solid(sel)) })
	}
	a.para.Paint(p, o, Ink.Get(th))
	if len(a.preedit) > 0 {
		// Underline the composition, as input methods expect.
		_, at := a.shown()
		ink := Ink.Get(th)
		a.spans(at, at+len(a.preedit), func(r geom.Rect) {
			p.RRect(geom.Rc(r.Min.X+o.X, r.Max.Y+o.Y-2, r.Size().W, 1), 0, paint.Solid(ink))
		})
	}

	if focus > 0.01 {
		c := Accent.Get(th)
		c.A = uint8(float32(c.A) * min(focus, 1) * a.blink.value())
		at := a.caretAt.Value().Add(o)
		h := a.para.LineHeight
		if len(a.para.Lines) > 0 {
			h = a.para.Lines[0].Run.Height()
		}
		p.RRect(geom.Rc(at.X-0.75, at.Y, 1.5, h), 0.75, paint.Solid(c))
	}
}

// spans calls fn with the rectangle, in the paragraph's space, that
// runes start to end cover on each line they touch. A span that carries
// on past a line's end covers a little more, standing for the line
// break.
func (a *TextArea) spans(start, end int, fn func(geom.Rect)) { paraSpans(a.para, start, end, fn) }

// paraSpans is [TextArea.spans] for any paragraph.
func paraSpans(p text.Paragraph, start, end int, fn func(geom.Rect)) {
	const lineBreak = 6
	for _, l := range p.Lines {
		s0, s1 := max(start, l.Run.Start), min(end, l.Run.End)
		if s0 > s1 || (s0 == s1 && end <= l.Run.End) {
			continue
		}
		x0, x1 := l.Run.CaretX(s0), l.Run.CaretX(s1)
		if x0 > x1 {
			x0, x1 = x1, x0
		}
		if end > l.Run.End {
			x1 += lineBreak
		}
		fn(geom.Rc(l.At.X+x0, l.At.Y, x1-x0, l.Run.Height()))
	}
}
