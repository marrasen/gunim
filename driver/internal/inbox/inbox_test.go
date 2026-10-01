package inbox

import "testing"

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
