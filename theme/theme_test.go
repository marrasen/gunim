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

var ink = Foreground("test.ink", color.NRGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff})

func TestAForegroundFadesThroughASwitch(t *testing.T) {
	dark := color.NRGBA{R: 0x11, G: 0x11, B: 0x11, A: 0xff}
	l := NewLive(Make("dark"))
	if got := ink.Get(l); got.A != 0xff {
		t.Fatalf("setup: ink %v, want opaque", got)
	}
	l.Use(Make("light", Set(ink, dark), Set(Switch, anim.Spring{Response: 0.5, Damping: 1})))

	// Somewhere in the switch the ink is out of sight, and it is never
	// a grey between the two.
	minAlpha := uint8(0xff)
	for range 120 {
		l.Step(time.Second / 120)
		c := ink.Get(l)
		minAlpha = min(minAlpha, c.A)
		if c.A > 0x20 && c.R != 0xee && c.R != 0x11 {
			t.Fatalf("ink %v is visible part way between its two colours", c)
		}
	}
	if minAlpha > 0x08 {
		t.Fatalf("ink stayed at least %d opaque through the switch, want it faded out", minAlpha)
	}
	step(l, 300)
	if got := ink.Get(l); got != dark {
		t.Fatalf("once settled, ink is %v, want %v", got, dark)
	}
}
