package gunim

import (
	"testing"
	"time"
)

// counted is a view that records how often its update ran, so a test
// can tell one reconciliation from a hundred.
type counted struct {
	probe
	updates int
}

func newCountedWindow(t *testing.T) *Window {
	t.Helper()
	w := newTestWindow()
	RegisterView(w, "counted",
		func(s panelState) *counted {
			c := &counted{probe: *newProbe(s.Label)}
			return c
		},
		func(c *counted, s panelState, _ *UI) {
			c.updates++
			c.label = s.Label
		})
	return w
}

func countedAt(t *testing.T, w *Window, id ID) *counted {
	t.Helper()
	s, ok := w.ui.ids[id]
	if !ok {
		t.Fatalf("view %q is missing", id)
	}
	c, ok := s.node.(*counted)
	if !ok {
		t.Fatalf("view %q is a %T, want a *counted", id, s.node)
	}
	return c
}

func TestPublishBurstCostsOneUpdatePerFrame(t *testing.T) {
	w := newCountedWindow(t)
	c := w.Client()
	if err := c.Mount(Root, "panel", "counted", panelState{Label: "start"}, "jobs"); err != nil {
		t.Fatal(err)
	}
	run(w, 1)
	before := countedAt(t, w, "panel").updates

	// The application pushes far faster than any display refreshes.
	for i := range 100 {
		if err := c.Publish("jobs", panelState{Label: string(rune('a' + i%26))}); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Publish("jobs", panelState{Label: "last"}); err != nil {
		t.Fatal(err)
	}
	run(w, 1)

	view := countedAt(t, w, "panel")
	if got := view.updates - before; got != 1 {
		t.Fatalf("view updated %d times for 101 publishes in one frame, want 1", got)
	}
	if view.label != "last" {
		t.Fatalf("label = %q, want the newest state to be the one that landed", view.label)
	}
	if s := w.Stats(); s.Coalesced != 100 {
		t.Fatalf("Coalesced = %d, want 100 of the 101 publishes superseded", s.Coalesced)
	}
}

func TestMountKeepsEarlierStateFromBeingCoalescedAway(t *testing.T) {
	// Coalescing must stop at a command that changes what exists, so
	// state queued ahead of a mount still reaches the views that were
	// already there.
	w := newCountedWindow(t)
	c := w.Client()
	if err := c.Mount(Root, "first", "counted", panelState{Label: "start"}, "jobs"); err != nil {
		t.Fatal(err)
	}
	run(w, 1)
	before := countedAt(t, w, "first").updates

	mustPublish(t, c, "a")
	mustPublish(t, c, "b") // supersedes a
	if err := c.Mount(Root, "second", "counted", panelState{Label: "new"}, "jobs"); err != nil {
		t.Fatal(err)
	}
	mustPublish(t, c, "c") // lands after the mount, so it stands alone
	run(w, 1)

	if got := countedAt(t, w, "first").updates - before; got != 2 {
		t.Fatalf("first view updated %d times, want 2: b before the mount and c after", got)
	}
	if got := countedAt(t, w, "first").label; got != "c" {
		t.Fatalf("label = %q, want c", got)
	}
}

func TestPatchesAreNeverCoalesced(t *testing.T) {
	// Two patches on one topic may aim at different rows, so each one
	// has to be applied.
	w := newProbeWindow(t)
	c := w.Client()
	if err := c.Mount(Root, "panel", "panel", panelState{}, "jobs"); err != nil {
		t.Fatal(err)
	}
	run(w, 1)
	coalescedBefore := w.Stats().Coalesced

	for range 5 {
		if err := c.Patch("jobs", tick{At: 1}); err != nil {
			t.Fatal(err)
		}
	}
	run(w, 1)

	if got := w.Stats().Coalesced - coalescedBefore; got != 0 {
		t.Fatalf("%d patches were coalesced away; every patch carries something of its own", got)
	}
}

func TestLoopDrawsOncePerDisplayRefresh(t *testing.T) {
	w := newCountedWindow(t)
	d := w.offscreen(t)
	c := w.Client()
	go w.loop()
	defer w.Close()

	if err := c.Mount(Root, "panel", "counted", panelState{Label: "start"}, "jobs"); err != nil {
		t.Fatal(err)
	}
	for range 50 {
		mustPublish(t, c, "push")
	}

	// Nothing reaches the screen until the display asks for it, however
	// much the application queued.
	time.Sleep(25 * time.Millisecond)
	if got := w.Stats().Frames; got != 0 {
		t.Fatalf("drew %d frames before the display ticked once", got)
	}

	d.Tick()
	waitFor(t, func() bool { return w.Stats().Frames >= 1 }, "the first frame")
	time.Sleep(25 * time.Millisecond)
	if got := w.Stats().Frames; got != 1 {
		t.Fatalf("drew %d frames for one display refresh, want 1", got)
	}

	d.Tick()
	waitFor(t, func() bool { return w.Stats().Frames >= 2 }, "the second frame")
	time.Sleep(25 * time.Millisecond)
	if got := w.Stats().Frames; got != 2 {
		t.Fatalf("drew %d frames for two display refreshes, want 2", got)
	}
}

// mustPublish sends state to the topic these tests share.
func mustPublish(t *testing.T, c Client, label string) {
	t.Helper()
	if err := c.Publish("jobs", panelState{Label: label}); err != nil {
		t.Fatal(err)
	}
}

func waitFor(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}
