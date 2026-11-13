package text

import (
	"sync"
	"testing"
)

// A face read on its first use stays unread until something sets text in it, and then reads once, however many
// goroutines ask at the same moment.
func TestInterIsReadOnItsFirstUse(t *testing.T) {
	f := parseLate(interRegular, "Inter")
	if f.face != nil {
		t.Fatal("the face was read before anything used it")
	}
	var wg sync.WaitGroup
	runs := make([]Run, 8)
	for i := range runs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runs[i] = f.Shape("Revoke laptop", 17)
		}()
	}
	wg.Wait()
	for i, r := range runs {
		if r.Advance <= 0 || r.Advance != runs[0].Advance {
			t.Fatalf("run %d is %v wide, want %v, more than 0", i, r.Advance, runs[0].Advance)
		}
		for _, g := range r.Glyphs {
			if g.Face != f.id {
				t.Fatalf("run %d sets a glyph in face %d, want Inter's own %d", i, g.Face, f.id)
			}
		}
	}
	if got, ok := Lookup(f.id); !ok || got != f {
		t.Fatalf("Lookup(%d) = %p, %v; want the face itself", f.id, got, ok)
	}
}

// Inter's semibold is wider than its regular: the two are different weights, each with metrics of its own.
func TestInterHasTwoWeights(t *testing.T) {
	regular, semibold := Inter(false).Shape("Plants", 24), Inter(true).Shape("Plants", 24)
	if semibold.Advance <= regular.Advance {
		t.Fatalf("semibold is %v wide, regular %v; want semibold wider", semibold.Advance, regular.Advance)
	}
	if a, d, _ := Inter(false).Metrics(17); a <= 0 || d <= 0 {
		t.Fatalf("Inter's extents at 17 are %v above and %v below, want both above 0", a, d)
	}
}
