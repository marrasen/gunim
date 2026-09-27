package widget

import (
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
)

type followed struct{}

func TestAClickOnALinkRunsItAndSendsItsIntent(t *testing.T) {
	l := NewLink("show 12 more")
	l.On = followed{}
	ran := 0
	l.OnActivate(func(*gunim.UI) { ran++ })
	w, run := stage(t, &frame{child: Row(l), size: geom.Sz(400, 40)})
	click(w, 5, 5)
	run(1)
	if ran != 1 {
		t.Fatalf("the link ran %d times for one click, want once", ran)
	}
	if got := sent(w); len(got) != 1 || got[0] != (followed{}) {
		t.Fatalf("intents %v, want the link's", got)
	}
	if role := l.Access().Role; role != access.RoleLink {
		t.Fatalf("a link reads as a %v", role)
	}
}
