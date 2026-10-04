package gunim

import (
	"testing"
	"time"

	"github.com/marrasen/gunim/geom"
)

func TestAViewWatchingATopicNamedForItselfHearsEachPublishOnce(t *testing.T) {
	w := NewOffscreen(geom.Sz(200, 100), nil)
	updates := 0
	RegisterView(w, "counter",
		func(int) *box { return &box{} },
		func(_ *box, _ int, _ *UI) { updates++ })
	c := w.Client()
	if err := c.Mount(Root, "same", "counter", 0, "same"); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	updates = 0
	if err := c.Publish("same", 1); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	if updates != 1 {
		t.Fatalf("one publish updated the view %d times, want once", updates)
	}
}
