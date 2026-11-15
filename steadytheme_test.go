package gunim

import (
	"bytes"
	"image/color"
	"log"
	"testing"
	"time"

	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// themedInk and themedLength are what a themedLeaf draws in, from the theme it is in.
var (
	themedInk    = theme.Color("test.steady.themed", color.NRGBA{A: 0xff})
	themedLength = theme.Length("test.steady.length", 10)
)

// leafHeight is how tall a themedLeaf draws itself, and springHeight a springLeaf, to find them among the ops.
const leafHeight, springHeight = 23, 17

// themedLeaf is a Steady node keyed on its theme alone, drawing a bar themedLength long in themedInk, and counting
// its paints where painted is set.
type themedLeaf struct{ painted *int }

func (themedLeaf) SteadyKey(f Frame) (any, bool) { return f.Theme, true }
func (themedLeaf) Layout(c Constraints, _ Frame, _ Children) geom.Size {
	return c.Constrain(geom.Sz(200, leafHeight))
}
func (l themedLeaf) Paint(p *paint.Painter, f Frame, _ geom.Size, _ Children) {
	if l.painted != nil {
		*l.painted++
	}
	p.RRect(geom.Rc(0, 0, themedLength.Get(f.Theme), leafHeight), 0, paint.Solid(themedInk.Get(f.Theme)))
}

// themeScope is a ThemeScope that steps its own theme, round its children.
type themeScope struct{ live *theme.Live }

func (b *themeScope) ThemeScope() *theme.Live    { return b.live }
func (b *themeScope) Step(dt time.Duration) bool { return b.live.Step(dt) }
func (b *themeScope) Layout(c Constraints, _ Frame, kids Children) geom.Size {
	for k := range kids.All {
		k.Layout(Constraints{Max: c.Max})
	}
	return c.Max
}
func (b *themeScope) Paint(p *paint.Painter, _ Frame, _ geom.Size, kids Children) {
	for k := range kids.All {
		k.Paint(p)
	}
}

// look is a colour and a length a leaf drew.
type look struct {
	ink    color.NRGBA
	length float32
}

// drawnAt is the look of the last bar of height h the window's last frame drew.
func drawnAt(t *testing.T, w *Window, h float32) look {
	t.Helper()
	var got *look
	for _, op := range w.mustOffscreen(t).Ops() {
		if r, ok := op.(*paint.RRectOp); ok && r.Rect.Size().H == h {
			got = &look{r.Fill.Solid, r.Rect.Size().W}
		}
	}
	if got == nil {
		t.Fatalf("the window's last frame drew no bar %v tall", h)
	}
	return *got
}

var (
	dark  = look{color.NRGBA{R: 0x3f, G: 0x0f, B: 0x0f, A: 0xff}, 60}
	light = look{color.NRGBA{R: 0xf0, G: 0xe8, B: 0xd0, A: 0xff}, 113.5}
)

// themeOf is a theme named name that gives a themedLeaf l, and sets the rest.
func themeOf(name string, l look, sets ...theme.Entry) theme.Theme {
	return theme.Make(name, append([]theme.Entry{theme.Set(themedInk, l.ink), theme.Set(themedLength, l.length)},
		sets...)...)
}

// endsOn runs a window that build mounts, and act changes after it has settled, with the steady check off and on.
// Well after act, the bar of height h must be drawn as want, and the check must have logged nothing.
func endsOn(t *testing.T, name string, h float32, build, act func(w *Window), want look) {
	t.Helper()
	for _, debug := range []bool{false, true} {
		var logged bytes.Buffer
		was, wasOut := steadyDebug, log.Writer()
		steadyDebug = debug
		log.SetOutput(&logged)
		w := newTestWindow()
		build(w)
		w.ui.Insert(w.ui.Root(), &ticker{})
		run(w, 60)
		act(w)
		run(w, 300)
		got := drawnAt(t, w, h)
		steadyDebug = was
		log.SetOutput(wasOut)
		anim.SetScale(1)
		if got != want {
			t.Errorf("%s, debug %v: well after the change the bar is drawn %v, want %v", name, debug, got, want)
		}
		if logged.Len() > 0 {
			t.Errorf("%s, debug %v: the steady check logged %q", name, debug, logged.String())
		}
	}
}

// A Steady node keyed on the window's theme ends a theme switch on the new theme's values exactly, a colour and a
// length, and its key never holds while it draws otherwise: with the theme's own glide, an underdamped one, two
// switches close together, and the glide cut short as animation is turned off.
func TestASteadyNodeKeyedOnTheThemeEndsOnTheNewColour(t *testing.T) {
	bouncy := theme.Set(theme.Switch, anim.Spring{Response: 0.3, Damping: 0.4})
	leafIn := func(l look) func(w *Window) {
		return func(w *Window) {
			w.ui.UseTheme(themeOf("dark", l))
			w.ui.Insert(w.ui.Root(), themedLeaf{})
		}
	}
	endsOn(t, "one switch", leafHeight, leafIn(dark), func(w *Window) { w.ui.UseTheme(themeOf("light", light)) },
		light)
	endsOn(t, "an underdamped switch", leafHeight, leafIn(dark),
		func(w *Window) { w.ui.UseTheme(themeOf("light", light, bouncy)) }, light)
	endsOn(t, "two switches close together", leafHeight, leafIn(dark), func(w *Window) {
		w.ui.UseTheme(themeOf("light", light))
		run(w, 5)
		w.ui.UseTheme(themeOf("dark", dark))
	}, dark)
	endsOn(t, "animation turned off mid-glide", leafHeight, leafIn(dark), func(w *Window) {
		w.ui.UseTheme(themeOf("light", light))
		run(w, 10)
		anim.SetScale(0)
	}, light)
}

// A Steady node keyed on its theme inside a ThemeScope ends on the new values exactly: as the scope switches by
// itself, and as the window's theme it falls back to switches.
func TestASteadyNodeKeyedOnAScopesThemeEndsOnTheNewValues(t *testing.T) {
	var own *themeScope
	endsOn(t, "a scope switching by itself", leafHeight, func(w *Window) {
		w.ui.UseTheme(themeOf("window", dark))
		own = &themeScope{live: theme.NewLive(themeOf("dark", dark))}
		w.ui.Insert(w.ui.Root(), own)
		w.ui.Insert(own, themedLeaf{})
	}, func(*Window) { own.live.Use(themeOf("light", light)) }, light)
	endsOn(t, "the window's theme read through a scope", leafHeight, func(w *Window) {
		w.ui.UseTheme(themeOf("dark", dark))
		through := &themeScope{live: theme.NewLive(theme.Theme{})}
		w.ui.Insert(w.ui.Root(), through)
		w.ui.Insert(through, themedLeaf{})
	}, func(w *Window) { w.ui.UseTheme(themeOf("light", light)) }, light)
}

// A theme switch paints a Steady node keyed on the theme on each frame the theme moves, and on one frame more, the
// frame its last value lands on: no more.
func TestATheSwitchPaintsAPartKeyedOnTheThemeOneFrameMore(t *testing.T) {
	w := newTestWindow()
	w.ui.UseTheme(themeOf("dark", dark))
	painted := 0
	w.ui.Insert(w.ui.Root(), themedLeaf{painted: &painted})
	w.ui.Insert(w.ui.Root(), &ticker{})
	run(w, 60)
	painted = 0
	w.ui.UseTheme(themeOf("light", light))
	moved := 0
	for range 300 {
		run(w, 1)
		if w.ui.themeMoved == w.ui.seq {
			moved++
		}
	}
	if moved == 0 || painted != moved+1 {
		t.Fatalf("the theme moved on %d frames and the part was painted on %d, want %d", moved, painted, moved+1)
	}
}

// springLeaf draws a bar as long as its spring stands.
type springLeaf struct{ length *anim.Float }

func (l *springLeaf) Step(dt time.Duration) bool { return l.length.Step(dt) }
func (l *springLeaf) Layout(c Constraints, _ Frame, _ Children) geom.Size {
	return c.Constrain(geom.Sz(200, springHeight))
}
func (l *springLeaf) Paint(p *paint.Painter, _ Frame, _ geom.Size, _ Children) {
	p.RRect(geom.Rc(0, 0, l.length.Value(), springHeight), 0, paint.Solid(color.NRGBA{B: 0xff, A: 0xff}))
}

// steadyBox is a Steady node whose key never changes, round its children.
type steadyBox struct{ name string }

func (*steadyBox) SteadyKey(Frame) (any, bool) { return 1, true }
func (*steadyBox) Layout(c Constraints, _ Frame, kids Children) geom.Size {
	for k := range kids.All {
		k.Layout(Constraints{Max: c.Max})
	}
	return c.Max
}
func (*steadyBox) Paint(p *paint.Painter, _ Frame, _ geom.Size, kids Children) {
	for k := range kids.All {
		k.Paint(p)
	}
}

// A node's spring inside a Steady node is drawn where it lands, on the frame its last step lands it, though the
// Steady node's key never changes.
func TestASpringInsideASteadyNodeIsDrawnWhereItLands(t *testing.T) {
	var spring *springLeaf
	endsOn(t, "a spring landing", springHeight, func(w *Window) {
		spring = &springLeaf{length: anim.NewFloat(60)}
		box := &steadyBox{name: "box"}
		w.ui.Insert(w.ui.Root(), box)
		w.ui.Insert(box, spring)
	}, func(*Window) { spring.length.Animate(113.5, anim.Spring{Response: 0.3, Damping: 0.4}) }, look{
		color.NRGBA{B: 0xff, A: 0xff}, 113.5})
}
