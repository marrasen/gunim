package filemanager

import (
	"slices"
	"testing"

	"github.com/marrasen/gunim/input"
)

func TestGoneFindsTheRowsThatWent(t *testing.T) {
	es := func(names ...string) []entry {
		out := make([]entry, len(names))
		for i, n := range names {
			out[i] = entry{Name: n}
		}
		return out
	}
	if got := gone(es("a", "b", "c", "d"), es("a", "d", "e")); !slices.Equal(got, []int{1, 2}) {
		t.Fatalf("gone = %v, want b and c, 1 and 2", got)
	}
	if got := gone(nil, es("a")); got != nil {
		t.Fatalf("a first listing has %v gone", got)
	}
}

func TestTrashedRowsLeaveTheListing(t *testing.T) {
	h := newHarness(t, "a.txt", "b.txt", "c.txt", "d.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 4 })
	h.pick("b.txt", "c.txt")
	h.w.Input(input.KeyPress{Key: input.KeyDelete})
	grid := func() int { return h.b.listing.cur.grid.Leaving() }
	h.until("the trashed rows leave", func() bool { return grid() == 2 })
	h.until("they have gone", func() bool { return grid() == 0 && slices.Equal(h.shown(), []string{"a.txt", "d.txt"}) })
}

func TestFilteredRowsLeaveTheListing(t *testing.T) {
	h := newHarness(t, "apple.txt", "banana.txt", "cherry.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 3 })
	h.do(FilterChanged{Text: "an"})
	h.until("the rows the filter drops leave", func() bool { return h.b.listing.cur.grid.Leaving() == 2 })
}
