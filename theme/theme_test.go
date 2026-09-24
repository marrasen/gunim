package theme

import (
	"image/color"
	"testing"
	"time"

	"github.com/marrasen/gunim/anim"
)

var (
	fill = Color("test.fill", color.NRGBA{A: 0xff})
	pad  = Length("test.pad", 8)
)

func step(l *Live, n int) {
	for range n {
		l.Step(time.Second / 60)
	}
}

func TestGetGivesTheDefaultOutsideATheme(t *testing.T) {
	if got := pad.Get(nil); got != 8 {
		t.Fatalf("with no Live, pad = %v, want the default 8", got)
	}
	if got := pad.Get(NewLive(Make("empty"))); got != 8 {
		t.Fatalf("in a theme that leaves pad out, pad = %v, want 8", got)
	}
}

func TestGetGivesTheThemesValue(t *testing.T) {
	l := NewLive(Make("roomy", Set(pad, 20)))
	if got := pad.Get(l); got != 20 {
		t.Fatalf("pad = %v, want the theme's 20", got)
	}
}

func TestUseAnimatesToTheNewTheme(t *testing.T) {
	l := NewLive(Make("tight"))
	if pad.Get(l) != 8 {
		t.Fatal("setup: pad should start at 8")
	}
	l.Use(Make("roomy", Set(pad, 20)))
	step(l, 3)
	if got := pad.Get(l); got <= 8 || got >= 20 {
		t.Fatalf("three frames into the switch, pad = %v, want between 8 and 20", got)
	}
	if !l.Step(time.Second / 60) {
		t.Fatal("Step reports nothing moving in the middle of a switch")
	}
	step(l, 300)
	if got := pad.Get(l); got != 20 {
		t.Fatalf("once settled, pad = %v, want 20", got)
	}
	if l.Step(time.Second / 60) {
		t.Fatal("Step still reports movement after the switch settled")
	}
}

func TestSwitchingBackReturnsToTheDefault(t *testing.T) {
	l := NewLive(Make("roomy", Set(pad, 20)))
	pad.Get(l)
	l.Use(Make("plain"))
	step(l, 300)
	if got := pad.Get(l); got != 8 {
		t.Fatalf("pad = %v, want its default 8 in a theme that leaves it out", got)
	}
}

func TestATokenFirstReadAfterASwitchStartsAtRest(t *testing.T) {
	l := NewLive(Make("plain"))
	l.Use(Make("red", Set(fill, color.NRGBA{R: 0xff, A: 0xff})))
	if got := fill.Get(l); got != (color.NRGBA{R: 0xff, A: 0xff}) {
		t.Fatalf("fill = %v, want the new theme's red at once", got)
	}
	if l.Step(time.Second / 60) {
		t.Fatal("a token read after the switch is animating")
	}
}

func TestTheNewThemeChoosesTheSwitchMotion(t *testing.T) {
	instant := anim.Spring{Response: 0.001, Damping: 1}
	slow := NewLive(Make("a"))
	fast := NewLive(Make("a"))
	pad.Get(slow)
	pad.Get(fast)
	slow.Use(Make("b", Set(pad, 20)))
	fast.Use(Make("b", Set(pad, 20), Set(Switch, instant)))
	step(slow, 2)
	step(fast, 2)
	if s, f := pad.Get(slow), pad.Get(fast); f != 20 || s >= f {
		t.Fatalf("after two frames pad is %v with the default switch and %v with an instant one", s, f)
	}
}

func TestAKeyKeepsOneType(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("declaring test.pad again as a colour did not panic")
		}
	}()
	Color("test.pad", color.NRGBA{})
}
