package main

import (
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

func TestAPhoneShowsOneScreenAtATime(t *testing.T) {
	h := newHarnessSized(t, geom.Sz(390, 800))
	if !h.v.screens.narrow || h.v.screens.showing() {
		t.Fatal("a phone opened on the conversation, want the list")
	}
	// Picking the open conversation again still slides it in.
	h.a.handle(ConversationChosen{ID: h.a.current.ID})
	h.frames(60)
	if !h.v.screens.showing() || h.v.screens.open.Value() != 1 {
		t.Fatalf("picking a conversation left it at %v, want it slid all the way in", h.v.screens.open.Value())
	}
	// The system's back gesture arrives as Escape, and goes back to the list.
	h.w.Input(input.KeyPress{Key: input.KeyEscape})
	h.frames(60)
	if h.v.screens.showing() || h.v.screens.open.Value() != 0 {
		t.Fatalf("Escape left the conversation at %v, want the list back", h.v.screens.open.Value())
	}
}

func TestTheDesktopShowsEverythingSideBySide(t *testing.T) {
	h := newHarness(t)
	if h.v.screens.narrow {
		t.Fatal("a desktop window shows one screen at a time")
	}
	h.a.handle(ConversationChosen{ID: h.a.current.ID})
	h.frames(30)
	h.w.Input(input.KeyPress{Key: input.KeyEscape})
	h.frames(5)
	if !h.v.screens.showing() {
		t.Fatal("Escape on the desktop moved the screens, which only a phone does")
	}
}
