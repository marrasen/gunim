package input

import "time"

// TextState is the focused text node's text as the platform's input
// method reads it: the text around the caret, the selection, and the
// part being composed. Every offset is in bytes into the node's whole
// text.
//
// A phone's keyboard asks for this at any moment and wants the answer
// at once, on the platform's own thread. So the engine hands each new
// state to a driver that keeps a copy, and the driver answers from the
// copy. The keyboard's changes come back as [TextEdit]s.
type TextState struct {
	// Text is the node's text, or for a long text the stretch of it
	// around the selection.
	Text string
	// Start is where Text starts in the whole text.
	Start int
	// Selection holds the anchor and the caret. Equal ends are a caret
	// with nothing selected.
	Selection [2]int
	// Composing is the range the input method is composing, start
	// before end. Equal ends mean no composition.
	Composing [2]int
	// Multiline says Enter starts a new line.
	Multiline bool
	// Secret says the text is a password, which a keyboard keeps out of
	// its suggestions and its memory.
	Secret bool
}

// End returns where Text ends in the whole text.
func (s TextState) End() int { return s.Start + len(s.Text) }

// Commit returns the edit that puts text in place of the composition,
// or of the selection when nothing is being composed, with the caret
// after it: what typing a key or finishing a composition does.
func (s TextState) Commit(text string) TextEdit {
	at, end := s.target()
	c := at + len(text)
	return TextEdit{Replace: [2]int{at, end}, With: text, Selection: [2]int{c, c}, Composing: [2]int{c, c}}
}

// Compose returns the edit that puts text in place of the composition,
// or of the selection when nothing is being composed, as the
// composition. sel is the part of text the input method highlights,
// in bytes into text; equal ends are its caret. An empty text ends the
// composition, leaving nothing in its place.
func (s TextState) Compose(text string, sel [2]int) TextEdit {
	at, end := s.target()
	clamp := func(b int) int { return at + max(0, min(b, len(text))) }
	return TextEdit{
		Replace:   [2]int{at, end},
		With:      text,
		Selection: [2]int{clamp(sel[0]), clamp(sel[1])},
		Composing: [2]int{at, at + len(text)},
	}
}

// target returns the range new text goes in: the composition while
// there is one, and the selection otherwise.
func (s TextState) target() (start, end int) {
	if s.Composing[0] < s.Composing[1] {
		return s.Composing[0], s.Composing[1]
	}
	return min(s.Selection[0], s.Selection[1]), max(s.Selection[0], s.Selection[1])
}

// Apply returns s with e made, as a driver's copy takes an edit the
// moment it sends it. It reports false, leaving s as it was, when e
// replaces text outside the stretch s holds.
func (s TextState) Apply(e TextEdit) (TextState, bool) {
	a, b := e.Replace[0]-s.Start, e.Replace[1]-s.Start
	if a < 0 || b < a || b > len(s.Text) {
		return s, false
	}
	s.Text = s.Text[:a] + e.With + s.Text[b:]
	s.Selection = e.Selection
	s.Composing = e.Composing
	return s, true
}

// TextEdit carries a change the platform's input method made to the
// focused node's text: the bytes Replace covers become With. After it,
// Selection and Composing hold as in [TextState]. Replace is in bytes
// into the text before the edit; Selection and Composing are in bytes
// into the text after it.
//
// Every text node takes this one event for what the input method does:
// typing, composing, autocorrect swapping a word, swipe typing, and a
// keyboard's backspace that deletes text in place of pressing a key.
type TextEdit struct {
	Replace   [2]int
	With      string
	Selection [2]int
	Composing [2]int
	// Seq counts the edits a driver sends, from 1. The engine hands it
	// back with the next [TextState], so the driver can tell a state
	// that has caught up with its edits from one still behind them.
	// Edits made from [TextInput] and [Composing] carry 0.
	Seq  uint64
	Time time.Time
}

func (TextEdit) isEvent() {}
