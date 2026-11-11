package gunim

import (
	"slices"
	"testing"
)

func TestAnAnnouncementReachesTheWindow(t *testing.T) {
	w := newTestWindow()
	w.ui.Announce("Syncing 3 files")
	w.ui.Announce("You're up to date")
	if got, want := w.Offscreen().Announced(), []string{"Syncing 3 files", "You're up to date"}; !slices.Equal(got, want) {
		t.Fatalf("the window was told %q, want %q", got, want)
	}
}

func TestAnnouncingOnANilUIDoesNothing(t *testing.T) {
	var u *UI
	u.Announce("nothing hears this")
}
