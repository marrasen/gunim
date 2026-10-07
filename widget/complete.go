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
	lines := make([]MenuItem, len(items))
	for i, c := range items {
		lines[i] = MenuItem{Label: c.Label, Hint: c.Hint, Icon: c.Icon, Swatch: c.Swatch}
		if lines[i].Label == "" {
			lines[i].Label = c.Text
		}
	}
	c := a.completing
	if c == nil || c.popup == nil || !c.popup.Open() {
		c = &completing{}
		c.menu = NewMenu(nil)
		c.menu.Pick = func(i int, u *gunim.UI) { a.accept(i, u) }
		c.popup = u.OpenPopup(a, c.menu, gunim.PopupOptions{
			Anchor:  a.triggerBox(start),
			Max:     geom.Sz(360, 320),
			Above:   a.CompleteAbove,
			Dismiss: func(*gunim.UI) { a.closeCompletion() },
		})
		a.completing = c
	}
	c.items, c.start = items, start
	c.menu.SetItems(lines)
	c.menu.Highlight(0)
	u.Invalidate()
}

// triggerBox returns where the trigger at index i is in the area, from the text as last laid out, for the list to
// hang from.
func (a *TextArea) triggerBox(i int) geom.Rect {
	_, at := a.para.Caret(i)
	at = at.Add(a.origin(FieldPadding.Default()))
	h := a.para.lineHeight()
	return geom.Rc(at.X, at.Y, 1, h+2)
}

// followTrigger moves the open list to where its trigger is now, as the text wraps, scrolls or grows.
func (a *TextArea) followTrigger() {
	if c := a.completing; c != nil && c.popup != nil && c.popup.Open() && c.start <= len(a.text) {
		c.popup.Move(a.triggerBox(c.start))
	}
}

// completionKey works the open list with k, and reports whether it took the key: Up and Down move the highlight,
// Enter and Tab without modifiers put the highlighted suggestion in, and Escape closes the list.
func (a *TextArea) completionKey(k input.KeyPress, u *gunim.UI) bool {
	c := a.completing
	if c == nil || c.popup == nil || !c.popup.Open() || len(a.preedit) > 0 {
		return false
	}
	plain := k.Mods&(input.ModShift|input.ModControl|input.ModAlt) == 0
	switch k.Key {
	case input.KeyUp, input.KeyDown:
		c.menu.Key(k, u)
	case input.KeyEnter, input.KeyKPEnter, input.KeyTab:
		if !plain {
			a.closeCompletion()
			return false
		}
		return a.accept(max(c.menu.Highlighted(), 0), u)
	case input.KeyEscape:
		a.dismissedAt = c.start
		a.closeCompletion()
	default:
		return false
	}
	return true
}

// accept puts suggestion i in place of the word it completes, and reports whether it did: not when the word has
// changed under the list, which then closes.
func (a *TextArea) accept(i int, u *gunim.UI) bool {
	c := a.completing
	a.closeCompletion()
	if c == nil || i < 0 || i >= len(c.items) {
		return false
	}
	if start, _, _, ok := a.completionAt(); !ok || start != c.start {
		return false
	}
	a.replace(c.start, a.caret, []rune(c.items[i].Text+" "), u)
	u.Focus(a)
	u.Invalidate()
	return true
}

// closeCompletion closes the list of suggestions.
func (a *TextArea) closeCompletion() {
	if a.completing != nil && a.completing.popup != nil {
		a.completing.popup.Close()
	}
	a.completing = nil
}
