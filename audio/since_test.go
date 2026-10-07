package audio

import (
	"slices"
	"testing"
)

// BlocksSince and ShortTermsSince hand over what was written after the mark, and the mark to pass next.
func TestTheMeterHandsOverOnlyWhatIsNew(t *testing.T) {
	var m LoudnessMeter
	m.Write(sine(SampleRate*4, 1000, 0.1, 0))
	blocks, mark := m.BlocksSince(0)
	shorts, smark := m.ShortTermsSince(0)
	if !slices.Equal(blocks, m.Blocks()) || mark != len(m.Blocks()) || !slices.Equal(shorts, m.ShortTerms()) ||
		smark != len(m.ShortTerms()) {
		t.Fatalf("from 0, handed %d blocks to mark %d and %d windows to %d; want all %d and %d",
			len(blocks), mark, len(shorts), smark, len(m.Blocks()), len(m.ShortTerms()))
	}
	m.Write(sine(SampleRate, 1000, 0.1, 0))
	more, next := m.BlocksSince(mark)
	if all := m.Blocks(); !slices.Equal(more, all[mark:]) || next != len(all) || len(more) != 10 {
		t.Fatalf("a second later, handed %d blocks to mark %d, want the 10 new ones to %d", len(more), next, len(all))
	}
	moreShorts, _ := m.ShortTermsSince(smark)
	if all := m.ShortTerms(); !slices.Equal(moreShorts, all[smark:]) {
		t.Fatalf("a second later, handed %d windows, want the %d new ones", len(moreShorts), len(all)-smark)
	}
	if none, at := m.BlocksSince(next); len(none) != 0 || at != next {
		t.Fatalf("with nothing new, handed %d blocks to mark %d", len(none), at)
	}
}
