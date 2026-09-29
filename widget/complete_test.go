package widget

import (
	"strings"
	"testing"

	"github.com/marrasen/gunim/input"
)

// people are who the completion tests can mention.
var people = []string{"Anna Berg", "Anders Ek", "Erik Lund"}

func newMentioner(t *testing.T) *writer {
	t.Helper()
	wr := newWriter(t, 400)
	wr.area.Triggers = "@"
	wr.area.Complete = func(trigger rune, query string) []Completion {
		var out []Completion
		for _, p := range people {
			if strings.HasPrefix(strings.ToLower(p), strings.ToLower(query)) {
				out = append(out, Completion{Text: "@" + p, Label: p})
			}
		}
		return out
	}
	return wr
}

func TestTypingAfterATriggerOffersCompletionsAtTheCaret(t *testing.T) {
	wr := newMentioner(t)
	wr.typeText("Ask @an")
	c := wr.area.completing
	if c == nil || len(c.items) != 2 {
		t.Fatalf("after @an the list offers %v, want Anna and Anders", c)
	}
	wr.w.Input(input.KeyPress{Key: input.KeyDown})
	wr.run(1)
	wr.w.Input(input.KeyPress{Key: input.KeyEnter})
	wr.run(1)
	if got := wr.area.Text(); got != "Ask @Anders Ek " {
		t.Fatalf("Down and Enter left %q, want the second name put in", got)
	}
	if wr.area.completing != nil {
		t.Fatal("the list stayed open after a pick")
	}
}

func TestEscapeClosesTheListForTheWordAndATriggerInAWordBeginsNothing(t *testing.T) {
	wr := newMentioner(t)
	wr.typeText("@e")
	if wr.area.completing == nil {
		t.Fatal("no list after @e")
	}
	wr.w.Input(input.KeyPress{Key: input.KeyEscape})
	wr.run(1)
	wr.typeText("r")
	if wr.area.completing != nil {
		t.Fatal("the list opened again for the word Escape closed it for")
	}
	wr.typeText(" mail me at a@an")
	if wr.area.completing != nil {
		t.Fatal("an @ inside a word opened the list")
	}
	wr.typeText(" @")
	if wr.area.completing == nil || len(wr.area.completing.items) != 3 {
		t.Fatal("a new @ after a space did not offer everyone")
	}
}
