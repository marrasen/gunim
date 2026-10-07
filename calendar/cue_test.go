package calendar

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// cues records the cues a window plays.
type cues struct {
	mu  sync.Mutex
	got []gunim.Cue
}

func (c *cues) PlayCue(cue gunim.Cue, _ float32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.got = append(c.got, cue)
}

// take returns the cues played since it was last asked.
func (c *cues) take() []gunim.Cue {
	c.mu.Lock()
	defer c.mu.Unlock()
	got := c.got
	c.got = nil
	return got
}

// A day picked in a small month, or by a heading of the week, sounds the select cue.
func TestPickingADaySounds(t *testing.T) {
	m := NewMiniMonth(monday)
	m.OnPick = func(day time.Time, _ *gunim.UI) gunim.Intent { return picked{day} }
	w, run, _ := stage(t, &frame{child: m, size: geom.Sz(300, 300)})
	l := &cues{}
	w.SetCues(l)
	w.Input(input.KeyPress{Key: input.KeyTab})
	run(1)
	l.take()
	w.Input(input.KeyPress{Key: input.KeyRight})
	run(1)
	if got := l.take(); !slices.Equal(got, []gunim.Cue{gunim.CueSelect}) {
		t.Fatalf("Right in a small month played %v, want the select cue", got)
	}
	week := newWeek()
	w, run, _ = stage(t, week)
	w.SetCues(l)
	click(w, run, geom.Pt(week.colX(2)+week.colW()/2, 10))
	if got := l.take(); !slices.Equal(got, []gunim.Cue{gunim.CueSelect}) {
		t.Fatalf("a click on a day's heading played %v, want the select cue", got)
	}
}
