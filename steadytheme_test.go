package gunim

import (
	"bytes"
	"image/color"
	"log"
	"testing"

	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// themedInk is the colour a themedLeaf draws, from the window's theme.
var themedInk = theme.Color("test.steady.themed", color.NRGBA{A: 0xff})

// themedBox is the size a themedLeaf draws itself at, to find it among the window's ops.
var themedBox = geom.Sz(37, 23)

// themedLeaf is a Steady node keyed on the window's theme alone, drawing itself in themedInk.
type themedLeaf struct{}

func (themedLeaf) SteadyKey(f Frame) (any, bool) { return f.Theme, true }
func (themedLeaf) Layout(c Constraints, _ Frame, _ Children) geom.Size {
	return c.Constrain(themedBox)
}
func (themedLeaf) Paint(p *paint.Painter, f Frame, _ geom.Size, _ Children) {
	p.RRect(geom.Rect{Max: themedBox.Point()}, 0, paint.Solid(themedInk.Get(f.Theme)))
}

// themedColour is the colour the window's last frame drew the themedLeaf in.
func themedColour(t *testing.T, w *Window) color.NRGBA {
	t.Helper()
	for _, op := range w.mustOffscreen(t).Ops() {
		if r, ok := op.(*paint.RRectOp); ok && r.Rect.Size() == themedBox {
			return r.Fill.Solid
		}
	}
	t.Fatal("the window's last frame drew no themed leaf")
	return color.NRGBA{}
}

// A Steady node keyed on the window's theme ends a theme switch on the new theme's colour exactly, and its key never
// holds while it draws otherwise: with the theme's own glide, an underdamped one, and two switches close together.
func TestASteadyNodeKeyedOnTheThemeEndsOnTheNewColour(t *testing.T) {
	dark, light := color.NRGBA{R: 0x3f, G: 0x0f, B: 0x0f, A: 0xff}, color.NRGBA{R: 0xf0, G: 0xe8, B: 0xd0, A: 0xff}
	themeOf := func(name string, c color.NRGBA, sets ...theme.Entry) theme.Theme {
		return theme.Make(name, append([]theme.Entry{theme.Set(themedInk, c)}, sets...)...)
	}
	bouncy := theme.Set(theme.Switch, anim.Spring{Response: 0.3, Damping: 0.4})
	for _, c := range []struct {
		name  string
		steps []theme.Theme
		gap   int
		want  color.NRGBA
	}{
		{"one switch", []theme.Theme{themeOf("light", light)}, 0, light},
		{"an underdamped switch", []theme.Theme{themeOf("light", light, bouncy)}, 0, light},
		{"two switches close together", []theme.Theme{themeOf("light", light), themeOf("dark", dark)}, 5, dark},
	} {
		for _, debug := range []bool{false, true} {
			var logged bytes.Buffer
			was, wasOut := steadyDebug, log.Writer()
			steadyDebug = debug
			log.SetOutput(&logged)
			w := newTestWindow()
			w.ui.UseTheme(themeOf("dark", dark))
			w.ui.Insert(w.ui.Root(), themedLeaf{})
			w.ui.Insert(w.ui.Root(), &ticker{})
			run(w, 60)
			for i, th := range c.steps {
				if i > 0 {
					run(w, c.gap)
				}
				w.ui.UseTheme(th)
			}
			run(w, 300)
			got := themedColour(t, w)
			steadyDebug = was
			log.SetOutput(wasOut)
			if got != c.want {
				t.Errorf("%s, debug %v: well after the switch the leaf is drawn %v, want %v", c.name, debug, got, c.want)
			}
			if logged.Len() > 0 {
				t.Errorf("%s, debug %v: the steady check logged %q", c.name, debug, logged.String())
			}
		}
	}
}
