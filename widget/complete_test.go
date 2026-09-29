package widget

import (
	"strings"
	"testing"

	"github.com/marrasen/gunim"
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

func TestNewTextClosesTheListAndEnterSends(t *testing.T) {
	wr := newMentioner(t)
	sent := ""
	wr.area.OnSubmit = func(s string) gunim.Intent { sent = s; return nil }
	wr.typeText("Ask @an")
	wr.area.SetText("hi")
	wr.run(1)
	if wr.area.completing != nil {
		t.Fatal("the list stayed open over new text")
	}
	wr.w.Input(input.KeyPress{Key: input.KeyEnter})
	wr.run(1)
	if sent != "hi" {
		t.Fatalf("Enter sent %q, want the new text", sent)
	}
}

func TestEscapeIsForgottenWithTheText(t *testing.T) {
	wr := newMentioner(t)
	wr.typeText("@xy")
	wr.w.Input(input.KeyPress{Key: input.KeyEscape})
	wr.run(1)
	wr.area.SetText("")
	wr.run(1)
	wr.typeText("@")
	if wr.area.completing == nil {
		t.Fatal("a new @ at the start of new text opened no list")
	}
}

func TestTheListHangsFromItsTriggerAndShiftEnterIsANewLine(t *testing.T) {
	wr := newMentioner(t)
	wr.typeText("Hello there @an")
	wr.run(5)
	c := wr.area.completing
	if c == nil {
		t.Fatal("no list")
	}
	if got, want := wr.area.triggerBox(c.start).Min.X, wr.area.TextCaret().Min.X; got >= want {
		t.Fatalf("the list hangs at x=%v, want at the @, left of the caret at %v", got, want)
	}
	wr.w.Input(input.KeyPress{Key: input.KeyEnter, Mods: input.ModShift})
	wr.run(1)
	if got := wr.area.Text(); got != "Hello there @an\n" {
		t.Fatalf("Shift+Enter left %q, want a new line", got)
	}
}
