package widget

import (
	"image/color"
	"unicode"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
)

// A Completion is a suggestion for the word being typed in a [TextArea]: what it puts in the text in place of the
// word and the character that began it, and how the list of suggestions shows it.
type Completion struct {
	// Text replaces the word, its trigger included, and a space follows it.
	Text string
	// Label is what the list shows, and Hint a faint word at its right, such as a shortcut or a detail.
	Label, Hint string
	// Icon or Swatch shows before the label.
	Icon   *icon.Icon
	Swatch color.NRGBA
}

// maxQuery is the longest word a completion is asked for.
const maxQuery = 40

// completing is the list of suggestions open at a text area's caret.
type completing struct {
	popup *gunim.Popup
	menu  *Menu
	items []Completion
	// start is where the trigger is in the text.
	start int
}

// completionAt returns the word being typed at the caret after one of the area's triggers: where the trigger is,
// the trigger, and the word after it. A trigger counts at the start of the text or after a space.
func (a *TextArea) completionAt() (start int, trigger rune, query string, ok bool) {
	if a.Complete == nil || a.Triggers == "" || len(a.preedit) > 0 || a.caret != a.anchor {
		return 0, 0, "", false
	}
	for i := a.caret - 1; i >= 0 && a.caret-i <= maxQuery+1; i-- {
		r := a.text[i]
		if unicode.IsSpace(r) {
			return 0, 0, "", false
		}
		if !containsRune(a.Triggers, r) {
			continue
		}
		if i > 0 && !unicode.IsSpace(a.text[i-1]) {
			// A trigger inside a word, such as the @ in an address, begins nothing.
			return 0, 0, "", false
		}
		return i, r, string(a.text[i+1 : a.caret]), true
	}
	return 0, 0, "", false
}

func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}
	return false
}

// complete opens, changes or closes the list of suggestions for the word at the caret, after an edit or a move of
// the caret.
func (a *TextArea) complete(u *gunim.UI) {
	start, trigger, query, ok := a.completionAt()
	if ok && a.dismissedAt == start {
		// Escape closed the list for this word; a new word opens it again.
		ok = false
	} else if !ok || a.dismissedAt != start {
		a.dismissedAt = -1
	}
	var items []Completion
	if ok {
		items = a.Complete(trigger, query)
	}
	if len(items) == 0 {
		a.closeCompletion()
		return
	}
	labels := make([]string, len(items))
	hints := make([]string, len(items))
	icons := make([]*icon.Icon, len(items))
	swatches := make([]color.NRGBA, len(items))
	for i, c := range items {
		labels[i], hints[i], icons[i], swatches[i] = c.Label, c.Hint, c.Icon, c.Swatch
		if labels[i] == "" {
			labels[i] = c.Text
		}
	}
	c := a.completing
	if c == nil || c.popup == nil || !c.popup.Open() {
		c = &completing{}
		c.menu = NewMenu(labels...)
		c.menu.Pick = func(i int, u *gunim.UI) { a.accept(i, u) }
		caret := a.TextCaret()
		c.popup = u.OpenPopup(a, c.menu, gunim.PopupOptions{
			Anchor:  geom.Rc(caret.Min.X, caret.Min.Y, 1, caret.Size().H+2),
			Max:     geom.Sz(360, 320),
			Above:   a.CompleteAbove,
			Dismiss: func(*gunim.UI) { a.closeCompletion() },
		})
		a.completing = c
	}
	c.items, c.start = items, start
	c.menu.Items, c.menu.Hints, c.menu.Icons, c.menu.Swatches = labels, hints, icons, swatches
	c.menu.Highlight(0)
	u.Invalidate()
}

// completionKey works the open list with k, and reports whether it took the key: Up and Down move the highlight,
// Enter and Tab put the highlighted suggestion in, and Escape closes the list.
func (a *TextArea) completionKey(k input.KeyPress, u *gunim.UI) bool {
	c := a.completing
	if c == nil || c.popup == nil || !c.popup.Open() {
		return false
	}
	switch k.Key {
	case input.KeyUp, input.KeyDown:
		c.menu.Key(k, u)
	case input.KeyEnter, input.KeyKPEnter, input.KeyTab:
		a.accept(max(c.menu.Highlighted(), 0), u)
	case input.KeyEscape:
		a.dismissedAt = c.start
		a.closeCompletion()
	default:
		return false
	}
	return true
}

// accept puts suggestion i in place of the word it completes.
func (a *TextArea) accept(i int, u *gunim.UI) {
	c := a.completing
	if c == nil || i < 0 || i >= len(c.items) {
		return
	}
	text := []rune(c.items[i].Text + " ")
	start := c.start
	a.closeCompletion()
	a.replace(start, a.caret, text, u)
	u.Focus(a)
	u.Invalidate()
}

// closeCompletion closes the list of suggestions.
func (a *TextArea) closeCompletion() {
	if a.completing != nil && a.completing.popup != nil {
		a.completing.popup.Close()
	}
	a.completing = nil
}
