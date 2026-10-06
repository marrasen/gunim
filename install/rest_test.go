package install

import (
	"testing"
	"time"
)

// The icon floats a while after a page arrives, and then rests, so a
// window at rest asks for no frames; a new page wakes it.
func TestTheStageComesToRest(t *testing.T) {
	was := restAfter
	restAfter = 200 * time.Millisecond
	t.Cleanup(func() { restAfter = was })
	s := newStage(nil, "studio", accentFor(&App{}))
	s.wake()
	frames := 0
	for s.Step(time.Second/60) && frames < 60*30 {
		frames++
	}
	if frames >= 60*30 || s.lively() > 0.01 {
		t.Fatalf("after %d frames the icon is still at %v", frames, s.lively())
	}
	if frames < 10 {
		t.Fatalf("the icon rested after %d frames", frames)
	}
}
