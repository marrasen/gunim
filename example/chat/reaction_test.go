package main

import (
	"testing"

	"github.com/marrasen/gunim/widget"
)

const thumbs, party = "\U0001F44D", "\U0001F389"

func TestReactingAddsAChipAndReactingAgainTakesItBack(t *testing.T) {
	h := newHarness(t)
	last := h.a.current.msgs[len(h.a.current.msgs)-1]
	h.a.handle(ReactionToggled{ID: last.ID, Emoji: thumbs})
	h.frames(20)
	it := h.v.items[widget.Key(last.ID)]
	if len(it.Reactions) != 1 || it.Reactions[0] != (Reaction{Emoji: thumbs, Count: 1, Mine: true, Who: "You"}) {
		t.Fatalf("reactions %+v, want the user's thumbs up", it.Reactions)
	}
	n, ok := h.v.list.Row(widget.Key(last.ID))
	if !ok {
		t.Fatal("the message is not built")
	}
	bar := n.(*msgRow).reactions
	if len(bar.chips) != 1 {
		t.Fatalf("%d chips, want 1", len(bar.chips))
	}

	// A colleague joins in with the same emoji, and another.
	last.toggle(thumbs, "Anna Berg")
	last.toggle(party, "Anna Berg")
	h.a.publish()
	h.frames(20)
	it = h.v.items[widget.Key(last.ID)]
	if len(it.Reactions) != 2 || it.Reactions[0].Count != 2 || it.Reactions[0].Who != "You, Anna Berg" {
		t.Fatalf("reactions %+v, want two thumbs up and a party popper", it.Reactions)
	}
	if bar.order[0] != thumbs || bar.order[1] != party {
		t.Fatalf("chips in order %q, want the order they came in", bar.order)
	}

	h.a.handle(ReactionToggled{ID: last.ID, Emoji: thumbs})
	h.a.handle(ReactionToggled{ID: last.ID, Emoji: party})
	h.frames(20)
	it = h.v.items[widget.Key(last.ID)]
	if len(it.Reactions) != 2 || it.Reactions[0].Mine || it.Reactions[1].Count != 2 {
		t.Fatalf("reactions %+v, want Anna's thumbs up and both party poppers", it.Reactions)
	}
}

func TestAMessageBuiltWithReactionsShowsThem(t *testing.T) {
	it := Item{Key: "m1", Message: Message{ID: "m1", Author: "Erik Lund", Body: "Shipped",
		Reactions: []Reaction{{Emoji: party, Count: 3, Who: "Anna Berg, Sara Nyström, Erik Lund"}}}}
	r := newMsgRow(it, nil, nil, nil, nil)
	bar := r.reactions
	if len(bar.chips) != 1 || len(bar.Children()) != 2 {
		t.Fatalf("%d chips and %d children, want the party popper and the chip that adds", len(bar.chips),
			len(bar.Children()))
	}
}
