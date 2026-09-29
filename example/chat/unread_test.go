package main

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/widget"
)

// newLineAt returns where the line over new messages is in the timeline's rows, or -1.
func newLineAt(items []Item) int {
	for i, it := range items {
		if it.New {
			return i
		}
	}
	return -1
}

func TestTheNewLineGoesOverTheFirstUnreadMessage(t *testing.T) {
	h := newHarness(t)
	c := h.a.projects[0].convs[1]
	c.msgs[len(c.msgs)-1].Author, c.msgs[len(c.msgs)-2].Author, c.msgs[len(c.msgs)-3].Author = "Anna Berg", "Anna Berg", "Anna Berg"
	h.a.leaveUnread(c, 3)
	h.a.handle(ConversationChosen{ID: c.ID})
	s := h.a.state()
	at := newLineAt(s.Items)
	if at < 0 || s.Items[at+1].ID != c.msgs[len(c.msgs)-3].ID || s.Unread != 3 || s.NewKey != s.Items[at].Key {
		t.Fatalf("new line at %d, unread %d, key %q; want it over the third message from the end, and 3 unread", at,
			s.Unread, s.NewKey)
	}
	if c.unread != 0 {
		t.Fatalf("%d unread after opening, want none", c.unread)
	}
	// Away and back, it is all read.
	h.a.handle(ConversationChosen{ID: h.a.projects[0].convs[0].ID})
	h.a.handle(ConversationChosen{ID: c.ID})
	if at := newLineAt(h.a.state().Items); at >= 0 {
		t.Fatal("the new line is back after reading")
	}
}

func TestALongCatchUpOpensAtTheNewLineAndThePillCounts(t *testing.T) {
	h := newHarness(t)
	c := h.a.current
	for _, m := range c.msgs[len(c.msgs)-40:] {
		m.Author = "Anna Berg"
	}
	h.a.leaveUnread(c, 40)
	// Away and back, to open it with 40 unread.
	h.a.handle(ConversationChosen{ID: h.a.projects[0].convs[1].ID})
	h.frames(5)
	h.a.handle(ConversationChosen{ID: c.ID})
	h.frames(30)
	if h.v.list.AtEnd() {
		t.Fatal("the timeline opened at its end, past the 40 unread")
	}
	if n, ok := h.v.list.Row(widget.Key(newKey(c))); !ok || n == nil {
		t.Fatal("the new line is not in view")
	}
	if got := h.v.catchUp.count; got != 40 {
		t.Fatalf("the pill counts %d, want 40", got)
	}
	h.v.list.ScrollToEnd(widget.Quick.Default())
	h.frames(90)
	if got := h.v.catchUp.count; got != 0 {
		t.Fatalf("the pill counts %d at the end, want none", got)
	}
}

func TestMessagesThatComeWhileScrolledUpWaitBelow(t *testing.T) {
	h := newHarness(t)
	h.v.list.ScrollTo(0, widget.Quick.Default())
	h.frames(60)
	c := h.a.current
	h.a.add(c, "Anna Berg", "Are you there?", time.Now())
	h.a.publish()
	h.frames(5)
	if got := h.v.catchUp.count; got != 1 {
		t.Fatalf("the pill counts %d, want the message that came", got)
	}
}
