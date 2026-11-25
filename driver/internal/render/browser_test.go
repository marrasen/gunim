//go:build linux || windows || darwin

package render

import (
	"testing"

	"github.com/marrasen/gunim/text"
)

// The masks kept of a browser's glyphs stay in bounds, and those asked for all along stay.
func TestKeptMasksStayInBounds(t *testing.T) {
	var k keptMasks
	const size = 64 << 10
	first := glyphKey{id: 1}
	k.put(first, text.Mask{Pix: make([]byte, size), W: 256, H: 256})
	for i := range uint32(1000) {
		k.put(glyphKey{id: 2 + i}, text.Mask{Pix: make([]byte, size)})
		if m, ok := k.get(first); !ok || m.W != 256 {
			t.Fatalf("after %d masks one asked for all along is gone", i)
		}
		held := 0
		for _, masks := range []map[glyphKey]text.Mask{k.now, k.old} {
			for _, m := range masks {
				held += len(m.Pix)
			}
		}
		if held > 2*keptBytes+2*size {
			t.Fatalf("after %d masks %d bytes are held, with room for %d", i, held, 2*keptBytes)
		}
	}
	if k.has(glyphKey{id: 2}) {
		t.Error("a mask not asked for since is still there")
	}
	// Glyphs with nothing to draw count too.
	k = keptMasks{}
	for i := range uint32(1_000_000) {
		k.put(glyphKey{id: i}, text.Mask{})
	}
	if n := len(k.now) + len(k.old); n > 2*keptBytes/100 {
		t.Errorf("%d masks of nothing are held", n)
	}
}

// Glyphs with nothing to draw do not fill an atlas's list for ever.
func TestAnAtlasOfNothingStartsAgain(t *testing.T) {
	var s Shared
	none := func() text.Mask { return text.Mask{} }
	for i := range uint32(2*atlasSlots + 10) {
		s.glyph(glyphKey{id: i}, none)
		if n := len(s.atlas.slots); n > atlasSlots {
			t.Fatalf("the atlas lists %d glyphs", n)
		}
	}
	if s.atlas.epoch < 2 {
		t.Error("the atlas never started again")
	}
}
