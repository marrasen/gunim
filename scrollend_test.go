package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// ends counts the ScrollEnds r heard, and the scrolls it heard after the last of them, and how many of those were
// flung.
func ends(r *recorder) (n, after, flung int) {
	for _, e := range r.events {
		switch e := e.(type) {
		case input.ScrollEnd:
			n, after, flung = n+1, 0, 0
		case input.Scroll:
			if n > 0 {
				after++
				if e.Fling {
					flung++
				}
			}
		}
	}
	return n, after, flung
}

// A finger that scrolled says so once as it lifts, before the fling it leaves; the scrolls of the fling say they
// are flung, and those of the finger do not.
func TestAFingerThatScrolledSaysItLifted(t *testing.T) {
	r := &recorder{}
	w, finger := touchStage(t, r)
	swipe(finger, geom.Pt(100, 300), geom.Pt(100, 200), 5, time.Now(), 0)
	for _, e := range r.events {
		if s, ok := e.(input.Scroll); ok && s.Fling {
			t.Fatal("a scroll of the finger itself says it was flung")
		}
	}
	for range 10 {
		w.Frame(time.Second / 60)
	}
	n, after, flung := ends(r)
	if n != 1 || after == 0 || flung != after {
		t.Fatalf("the finger lifted with %d ends, then %d scrolls, %d flung; want one end and a fling after it", n, after,
			flung)
	}
	// A tap says nothing of a scroll ending, and a finger coming down mid-scroll ends the one before.
	swipe(finger, geom.Pt(100, 100), geom.Pt(102, 101), 2, time.Now(), 0)
	if n, _, _ := ends(r); n != 1 {
		t.Fatalf("a tap made %d ends in all, want still 1", n)
	}
	at := time.Now()
	finger(input.PointerDown{Pos: geom.Pt(100, 300), Button: input.ButtonPrimary, Clicks: 1, Touch: true, Time: at})
	finger(input.PointerMove{Pos: geom.Pt(100, 260), Touch: true, Time: at.Add(16 * time.Millisecond)})
	finger(input.PointerMove{Pos: geom.Pt(100, 230), Touch: true, Time: at.Add(32 * time.Millisecond)})
	finger(input.PointerDown{Pos: geom.Pt(50, 50), Button: input.ButtonPrimary, Clicks: 1, Touch: true, Time: at.Add(48 * time.Millisecond)})
	if n, _, _ := ends(r); n != 2 {
		t.Fatalf("a finger down mid-scroll made %d ends in all, want 2", n)
	}
}
