package widget

import (
	"strings"
	"unicode"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/input"
)

// editor is the editing behind TextField and TextArea: the text, the
// caret and selection, the input method's composition, and every edit
// and key. What differs between the two is how the text is laid out,
// which each supplies as a navigator.
type editor struct {
	text          []rune
	caret, anchor int
	multiline     bool
	// secret keeps the text off the clipboard.
	secret bool

	// hintX is the caret's x as an arrow key left it. Where text of two
	// directions meets, one rune index has two places on screen, and
	// hintX says which. hinted is false once the caret moves otherwise.
	hintX  float32
	hinted bool
	// goalX is the x Up and Down aim for, kept across lines of different
	// lengths so the caret comes back to its column.
	goalX float32
	goal  bool

	// preedit is the input method's composition, and preSel the part of
	// it the input method highlights, in runes; both ends are its caret
	// when nothing is highlighted.
	preedit []rune
	preSel  [2]int

	// changed is called after every edit.
	changed func(u *gunim.UI)
	// edited is set by typing, deleting and composing, and cleared by
	// the widget's next layout. The caret jumps after an edit, so it
	// keeps up with the text; it glides when it only moves.
	edited bool
}

// navigator is what the editor needs from laid-out text.
type navigator interface {
	// caretX returns the x of a caret before rune i on its line.
	caretX(i int) float32
	// beside steps the caret one place on screen, as text.Run.Beside
	// does, crossing to the next or previous line at an edge.
	beside(i int, x float32, right bool) (int, float32)
	// lineStart and lineEnd return the ends of the line holding i.
	lineStart(i int) int
	lineEnd(i int) int
	// vertical moves n lines down, or up when n is negative, from i,
	// toward x. It reports false for text of one line.
	vertical(i int, x float32, n int) (int, bool)
	// page is how many lines a page holds.
	page() int
}

// Selection returns the selected runes' range, start before end.
func (e *editor) Selection() (start, end int) {
	return min(e.caret, e.anchor), max(e.caret, e.anchor)
}

// shown returns the text as drawn, with any composition in place of the
// selection, and where the composition starts.
func (e *editor) shown() (runes []rune, at int) {
	start, end := e.Selection()
	if len(e.preedit) == 0 {
		return e.text, start
	}
	out := make([]rune, 0, len(e.text)-(end-start)+len(e.preedit))
	out = append(out, e.text[:start]...)
	out = append(out, e.preedit...)
	return append(out, e.text[end:]...), start
}

// drawnCaret returns the caret and anchor as drawn: the input method's,
// inside its composition, while one is in progress.
func (e *editor) drawnCaret() (caret, anchor int) {
	if len(e.preedit) > 0 {
		_, at := e.shown()
		return at + e.preSel[1], at + e.preSel[0]
	}
	return e.caret, e.anchor
}

// caretX returns where the caret is drawn on its line.
func (e *editor) caretX(n navigator) float32 {
	if e.hinted {
		return e.hintX
	}
	return n.caretX(e.caret)
}

// set moves the caret to i, keeping the anchor when extend is true.
func (e *editor) set(i int, extend bool) {
	e.caret = max(0, min(i, len(e.text)))
	if !extend {
		e.anchor = e.caret
	}
	e.hinted, e.goal = false, false
}

// compose takes the input method's latest composition, whose selection
// arrives in bytes.
func (e *editor) compose(c input.Composing) {
	e.preedit = []rune(c.Text)
	runeAt := func(b int) int {
		b = max(0, min(b, len(c.Text)))
		return len([]rune(c.Text[:b]))
	}
	e.preSel = [2]int{runeAt(c.Selected[0]), runeAt(c.Selected[1])}
	e.edited = true
}

// press places the caret for a click at rune i: selecting a word on a
// double click and everything on a triple, and extending with Shift.
func (e *editor) press(i, clicks int, shift bool) {
	if clicks < 2 {
		e.set(i, shift)
		return
	}
	e.anchor, e.caret = clickRange(e.text, i, clicks)
	e.hinted, e.goal = false, false
}

// clickRange returns what a click at rune i of rs selects: nothing on a
// single click, a word on a double and everything on a triple.
func clickRange(rs []rune, i, clicks int) (anchor, caret int) {
	switch {
	case clicks >= 3:
		return 0, len(rs)
	case clicks == 2:
		return wordStart(rs, i), wordEnd(rs, i)
	}
	return i, i
}

// key handles a key press. It reports false for a key the widget or its
// ancestors should have: a shortcut, Tab, and Enter in a single line.
func (e *editor) key(k input.KeyPress, u *gunim.UI, n navigator) bool {
	// A press that typed text is typing, whatever modifiers it holds:
	// AltGr holds Control and Alt. Its text arrives as TextInput.
	if k.Typed {
		return true
	}
	ctrl := k.Mods.Has(input.ModControl) || k.Mods.Has(input.ModSuper)
	shift := k.Mods.Has(input.ModShift)
	start, end := e.Selection()
	switch k.Key {
	case input.KeyLeft, input.KeyRight:
		right := k.Key == input.KeyRight
		switch {
		case ctrl && right:
			e.set(wordEnd(e.text, e.caret+1), shift)
		case ctrl:
			e.set(wordStart(e.text, e.caret-1), shift)
		case start != end && !shift:
			if right {
				e.set(end, false)
			} else {
				e.set(start, false)
			}
		default:
			i, x := n.beside(e.caret, e.caretX(n), right)
			e.set(i, shift)
			e.hintX, e.hinted = x, true
		}
	case input.KeyUp, input.KeyDown, input.KeyPageUp, input.KeyPageDown:
		lines := 1
		if k.Key == input.KeyPageUp || k.Key == input.KeyPageDown {
			lines = n.page()
		}
		if k.Key == input.KeyUp || k.Key == input.KeyPageUp {
			lines = -lines
		}
		if !e.goal {
			e.goalX = e.caretX(n)
		}
		i, ok := n.vertical(e.caret, e.goalX, lines)
		if !ok {
			return false
		}
		goalX := e.goalX
		e.set(i, shift)
		e.goalX, e.goal = goalX, true
	case input.KeyHome:
		if ctrl {
			e.set(0, shift)
		} else {
			e.set(n.lineStart(e.caret), shift)
		}
	case input.KeyEnd:
		if ctrl {
			e.set(len(e.text), shift)
		} else {
			e.set(n.lineEnd(e.caret), shift)
		}
	case input.KeyBackspace:
		switch {
		case start != end:
			e.replace(start, end, nil, u)
		case ctrl:
			e.replace(wordStart(e.text, e.caret-1), e.caret, nil, u)
		case e.caret > 0:
			e.replace(e.caret-1, e.caret, nil, u)
		}
	case input.KeyDelete:
		switch {
		case start != end:
			e.replace(start, end, nil, u)
		case ctrl:
			e.replace(e.caret, wordEnd(e.text, e.caret+1), nil, u)
		case e.caret < len(e.text):
			e.replace(e.caret, e.caret+1, nil, u)
		}
	case input.KeyEnter, input.KeyKPEnter:
		if !e.multiline {
			return false
		}
		e.insert("\n", u)
	case input.KeyTab:
		return false // focus moves on
	case input.KeyEscape:
		return false // for whatever the field sits in, to close
	case input.KeyA, input.KeyC, input.KeyX, input.KeyV:
		if !ctrl {
			return true // typing: the letter arrives as TextInput
		}
		e.clipboard(k.Key, start, end, u)
	default:
		// Keys with a modifier are shortcuts for someone else, as are the
		// function keys; the rest are typing, which arrives as TextInput.
		return !ctrl && !k.Mods.Has(input.ModAlt) && !functionKey(k.Key)
	}
	return true
}

func (e *editor) clipboard(k input.Key, start, end int, u *gunim.UI) {
	switch k {
	case input.KeyA:
		e.anchor, e.caret = 0, len(e.text)
		e.hinted, e.goal = false, false
	case input.KeyC:
		if start != end && !e.secret {
			u.SetClipboard(string(e.text[start:end]))
		}
	case input.KeyX:
		if start != end && !e.secret {
			u.SetClipboard(string(e.text[start:end]))
			e.replace(start, end, nil, u)
		}
	case input.KeyV:
		e.insert(u.Clipboard(), u)
	default:
	}
}

// insert puts s in place of the selection. A single line turns
// newlines and tabs into spaces; both keep printable text only.
func (e *editor) insert(s string, u *gunim.UI) {
	s = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' && e.multiline:
			return r
		case r == '\n' || r == '\t':
			return ' '
		case r == '\r' || unicode.IsControl(r):
			return -1
		}
		return r
	}, s)
	if s == "" {
		return
	}
	start, end := e.Selection()
	e.replace(start, end, []rune(s), u)
}

// replace swaps runes start to end for with, and leaves the caret after
// it.
func (e *editor) replace(start, end int, with []rune, u *gunim.UI) {
	if start < 0 || end > len(e.text) || start > end {
		return
	}
	out := make([]rune, 0, len(e.text)-(end-start)+len(with))
	out = append(out, e.text[:start]...)
	out = append(out, with...)
	out = append(out, e.text[end:]...)
	e.text = out
	e.set(start+len(with), false)
	e.edited = true
	if e.changed != nil {
		e.changed(u)
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

// aim sends a to v: at once after an edit, gliding with m otherwise.
func (e *editor) aim(a interface {
	Jump(float32)
	Animate(float32, anim.Motion)
}, v float32, m anim.Motion) {
	if e.edited {
		a.Jump(v)
		return
	}
	a.Animate(v, m)
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// functionKey reports whether k is one of F1 to F24.
func functionKey(k input.Key) bool {
	return k >= input.KeyF1 && k <= input.KeyF12 || k >= input.KeyF13 && k <= input.KeyF24
}
