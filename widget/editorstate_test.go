package widget

import (
	"math/rand/v2"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/marrasen/gunim/input"
)

// wholeTextState is TextState worked out the long way, from the whole
// text as drawn, to check the editor's against.
func wholeTextState(e *editor) input.TextState {
	rs, at := e.shown()
	caret, anchor := e.drawnCaret()
	lo := max(0, min(caret, anchor)-textWindow)
	hi := min(len(rs), max(caret, anchor)+textWindow)
	b := func(i int) int { return len(string(rs[:i])) }
	s := input.TextState{Text: string(rs[lo:hi]), Start: b(lo), Multiline: e.multiline, Secret: e.secret}
	s.Selection = [2]int{b(anchor), b(caret)}
	if len(e.preedit) > 0 {
		s.Composing = [2]int{b(at), b(at + len(e.preedit))}
	} else {
		s.Composing = [2]int{s.Selection[1], s.Selection[1]}
	}
	return s
}

// randomRunes returns n runes of a few widths in UTF-8, and newlines.
func randomRunes(r *rand.Rand, n int) []rune {
	pick := []rune("ab \nåé€😀")
	out := make([]rune, n)
	for i := range out {
		out[i] = pick[r.IntN(len(pick))]
	}
	return out
}

// shake makes a random edit, move or composition in e.
func shake(r *rand.Rand, e *editor) {
	n := len(e.text)
	switch r.IntN(5) {
	case 0, 1:
		a := r.IntN(n + 1)
		b := min(n, a+r.IntN(3))
		e.last = otherEdit
		e.replace(a, b, randomRunes(r, r.IntN(4)), nil)
	case 2:
		e.set(r.IntN(n+1), false)
		e.set(r.IntN(n+1), true)
	case 3:
		e.preedit = randomRunes(r, r.IntN(4))
		e.preSel = [2]int{len(e.preedit), len(e.preedit)}
	case 4:
		e.preedit = nil
	}
}

func TestTextStateCountsBytesAsTheWholeTextDoes(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	// A text longer than the window either side of the selection, so
	// the state is cut from it.
	e := &editor{text: randomRunes(r, 3*textWindow), multiline: true}
	for i := range 2000 {
		shake(r, e)
		if got, want := e.TextState(), wholeTextState(e); got != want {
			t.Fatalf("after %d changes, TextState is %+v, want %+v", i+1, brief(got), brief(want))
		}
	}
}

// brief is s with its text cut to its length, for a message.
func brief(s input.TextState) input.TextState {
	s.Text = string(rune('0' + utf8.RuneCountInString(s.Text)%10))
	return s
}

func TestShownChangeKeepsWhatStayed(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	e := &editor{text: randomRunes(r, 200), multiline: true}
	shown := func() []rune {
		rs, _ := e.shown()
		return slices.Clone(rs)
	}
	if _, _, _, ok := e.shownChange(); !ok {
		t.Fatal("the first shownChange reported nothing changed")
	}
	was := shown()
	for i := range 3000 {
		for range r.IntN(3) + 1 {
			shake(r, e)
		}
		now := shown()
		head, tail, n, ok := e.shownChange()
		if !ok {
			if !slices.Equal(now, was) {
				t.Fatalf("after %d rounds, nothing changed, but the text went from %q to %q", i, string(was), string(now))
			}
			continue
		}
		if n != len(was) || head+tail > min(len(was), len(now)) ||
			!slices.Equal(was[:head], now[:head]) || !slices.Equal(was[len(was)-tail:], now[len(now)-tail:]) {
			t.Fatalf("after %d rounds, head %d, tail %d and length %d for %q becoming %q", i, head, tail, n, string(was), string(now))
		}
		was = now
	}
}

func TestUndoAndRedoWalkTheWholeHistory(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 6))
	e := &editor{text: randomRunes(r, 50), multiline: true}
	// texts holds the text after each step.
	texts := []string{string(e.text)}
	note := func() {
		if len(e.undo) == len(texts) {
			texts = append(texts, string(e.text))
		} else {
			texts[len(texts)-1] = string(e.text)
		}
	}
	for range 200 {
		e.last = otherEdit
		at := r.IntN(len(e.text) + 1)
		e.set(at, false)
		e.replace(at, min(len(e.text), at+r.IntN(3)), randomRunes(r, r.IntN(3)), nil)
		note()
		// Typing on joins the step before, when that was typing too.
		for range r.IntN(3) {
			e.insert("a", nil)
			note()
		}
	}
	if len(e.undo) != len(texts)-1 {
		t.Fatalf("%d steps of undo, want %d", len(e.undo), len(texts)-1)
	}
	for i := len(texts) - 2; i >= 0; i-- {
		e.undoEdit(nil)
		if string(e.text) != texts[i] {
			t.Fatalf("undoing to step %d gave %q, want %q", i, string(e.text), texts[i])
		}
	}
	for i := 1; i < len(texts); i++ {
		e.redoEdit(nil)
		if string(e.text) != texts[i] {
			t.Fatalf("redoing to step %d gave %q, want %q", i, string(e.text), texts[i])
		}
	}
}
