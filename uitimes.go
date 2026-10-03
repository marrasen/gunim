package gunim

import (
	"log"
	"os"
	"time"
)

// uiFramesDebug is set by GUNIM_DEBUG_FRAMES, which also logs, each
// second, how long each window took to build its frames: applying the
// application's commands, and laying out and painting.
var uiFramesDebug = os.Getenv("GUNIM_DEBUG_FRAMES") != ""

// uiTimes sums a window's frame building for uiFramesDebug.
type uiTimes struct {
	from              time.Time
	n                 int
	apply, build      time.Duration
	applyMax, buildMx time.Duration
}

// add counts a frame that took apply to apply commands and build to lay
// out and paint, and logs the second's sums once one has passed.
func (t *uiTimes) add(w *Window, apply, build time.Duration) {
	now := time.Now()
	if t.from.IsZero() {
		t.from = now
	}
	t.n++
	t.apply += apply
	t.build += build
	t.applyMax = max(t.applyMax, apply)
	t.buildMx = max(t.buildMx, build)
	if now.Sub(t.from) < time.Second {
		return
	}
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	n := time.Duration(t.n)
	log.Printf("gunim ui %p %q: %d frames built; commands %.1f ms (max %.1f), layout and paint %.1f ms (max %.1f)",
		w, w.title, t.n, ms(t.apply/n), ms(t.applyMax), ms(t.build/n), ms(t.buildMx))
	*t = uiTimes{from: now}
}
