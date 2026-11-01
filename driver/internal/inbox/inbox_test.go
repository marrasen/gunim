package inbox

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

func TestInboxDeliversInOrderAndCloses(t *testing.T) {
	quit := make(chan struct{})
	defer close(quit)
	q := New(quit)
	for i := range 100 {
		q.Push(i)
	}
	q.Close()
	q.Push("after close")

	want := 0
	for ev := range q.Out() {
		if ev != want {
			t.Fatalf("got %v, want %d", ev, want)
		}
		want++
	}
	if want != 100 {
		t.Fatalf("delivered %d events, want 100", want)
	}
}

func TestInboxMergesQueuedMoves(t *testing.T) {
	move := func(x float32, mods input.Mods) input.PointerMove {
		return input.PointerMove{Pos: geom.Pt(x, 0), Mods: mods}
	}
	down := input.PointerDown{Pos: geom.Pt(3, 0), Button: input.ButtonPrimary}
	up := input.PointerUp{Pos: geom.Pt(6, 0), Button: input.ButtonPrimary}
	q := &Inbox{items: []any{
		move(1, 0), move(2, 0), move(3, 0),
		down,
		move(4, 0), move(5, 0), move(6, input.ModShift),
		up,
		move(7, 0),
	}}

	var got []any
	for len(q.items) > 0 {
		ev := q.items[0]
		q.items = q.items[1:]
		got = append(got, q.lastMove(ev))
	}
	want := []any{move(3, 0), down, move(5, 0), move(6, input.ModShift), up, move(7, 0)}
	if len(got) != len(want) {
		t.Fatalf("got %d events %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestInboxSwapsAWaitingMove(t *testing.T) {
	quit := make(chan struct{})
	defer close(quit)
	q := New(quit)
	for x := range 4 {
		q.Push(input.PointerMove{Pos: geom.Pt(float32(x), 0)})
		time.Sleep(10 * time.Millisecond)
	}
	q.Close()

	var got []any
	for ev := range q.Out() {
		got = append(got, ev)
	}
	if want := (input.PointerMove{Pos: geom.Pt(3, 0)}); len(got) != 1 || got[0] != want {
		t.Fatalf("got %v, want only %v", got, want)
	}
}
