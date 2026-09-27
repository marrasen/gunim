package widget

import (
	"image/color"
	"math"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Theme tokens for a chromeless window's buttons.
var (
	// WindowButtonHot lights the minimize and maximize buttons under
	// the pointer, and WindowCloseHot the close button, red as Windows
	// makes it.
	WindowButtonHot = theme.Color("window.button.hot", color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x16})
	WindowCloseHot  = theme.Color("window.close.hot", color.NRGBA{R: 0xc4, G: 0x2b, B: 0x1c, A: 0xff})
)

// windowButtonWidth is each button's width, as Windows 11 has it.
const windowButtonWidth = 46

// WindowControls are a chromeless window's minimize, maximize and close
// buttons, for the end of its title bar, drawn as Windows 11 draws them.
// Maximize shows restore while the window is maximized. It is the
// window's [gunim.MaximizeButton], for the snap layouts Windows 11 hangs
// on it. Close does what the system's does: see [gunim.UI.AskToClose].
type WindowControls struct {
	anim.Group
	hot  [3]*anim.Float
	over int
	down int
	// size and maxed are the buttons' size and whether the window was
	// maximized, at the last layout.
	size  geom.Size
	maxed bool
}

// NewWindowControls returns the three buttons.
func NewWindowControls() *WindowControls {
	c := &WindowControls{over: -1, down: -1}
	for i := range c.hot {
		c.hot[i] = anim.NewFloat(0)
		c.Add(c.hot[i])
	}
	return c
}

// Layout implements [gunim.Node]: three buttons as tall as the title
// bar.
func (c *WindowControls) Layout(cs gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	c.size, c.maxed = geom.Size{}, f.Maximized()
	if !f.Chromeless() {
		// The system's title bar has its own.
		return cs.Constrain(geom.Size{})
	}
	h := cs.Max.H
	if h <= 0 {
		h = MenubarHeight.Get(f.Theme)
	}
	c.size = cs.Constrain(geom.Sz(3*windowButtonWidth, h))
	return c.size
}

// MaximizeRect implements [gunim.MaximizeButton].
func (c *WindowControls) MaximizeRect(size geom.Size) geom.Rect {
	return geom.Rc(windowButtonWidth, 0, windowButtonWidth, size.H)
}

// buttonAt is the button under p, or -1.
func (c *WindowControls) buttonAt(p geom.Point, size geom.Size) int {
	if p.Y < 0 || p.Y >= size.H || p.X < 0 {
		return -1
	}
	if i := int(p.X / windowButtonWidth); i < 3 {
		return i
	}
	return -1
}

// Handle implements [gunim.Handler]: a button lights under the pointer,
// and acts when the button pressed on it is let go on it.
func (c *WindowControls) Handle(e input.Event, u *gunim.UI) bool {
	size := geom.Sz(3*windowButtonWidth, MenubarHeight.Get(u.Theme()))
	if b, ok := u.Bounds(c); ok {
		size = b.Size()
	}
	switch e := e.(type) {
	case input.PointerEnter:
		c.light(c.buttonAt(e.Pos, size), u)
	case input.PointerMove:
		c.light(c.buttonAt(e.Pos, size), u)
	case input.PointerLeave:
		c.light(-1, u)
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		c.down = c.buttonAt(e.Pos, size)
	case input.PointerUp:
		at := c.buttonAt(e.Pos, size)
		down := c.down
		c.down = -1
		if at < 0 || at != down {
			return true
		}
		switch at {
		case 0:
			u.Minimize()
		case 1:
			u.ToggleMaximize()
		case 2:
			u.AskToClose()
		}
	default:
		return false
	}
	return true
}

// light lights button i, and dims the rest.
func (c *WindowControls) light(i int, u *gunim.UI) {
	if i == c.over {
		return
	}
	c.over = i
	for k, h := range c.hot {
		to := float32(0)
		if k == i {
			to = 1
		}
		h.Animate(to, Quick.Get(u.Theme()))
	}
	u.Invalidate()
}

// Paint implements [gunim.Node].
func (c *WindowControls) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	// The title bar's colour, for the row they end to read as one.
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(MenubarFill.Get(th)))
	ink := Ink.Get(th)
	for i := range 3 {
		r := geom.Rc(float32(i)*windowButtonWidth, 0, windowButtonWidth, box.H)
		t := min(max(c.hot[i].Value(), 0), 1)
		glyph := ink
		if t > 0.01 {
			hot := WindowButtonHot.Get(th)
			if i == 2 {
				hot = WindowCloseHot.Get(th)
				glyph = anim.Mix(anim.ColorCodec, ink, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}, t)
			}
			hot.A = uint8(float32(hot.A) * t)
			p.RRect(r, 0, paint.Solid(hot))
		}
		mid := geom.Pt(float32(math.Round(float64(r.Center().X))), float32(math.Round(float64(r.Center().Y))))
		const g, w = 10, 1
		switch i {
		case 0:
			p.RRect(geom.Rc(mid.X-g/2, mid.Y, g, w), 0, paint.Solid(glyph))
		case 1:
			if f.Maximized() {
				// Restore: a square with another behind it.
				p.RRect(geom.Rc(mid.X-g/2+2, mid.Y-g/2, g-2, w), 0, paint.Solid(glyph))
				p.RRect(geom.Rc(mid.X+g/2-w, mid.Y-g/2, w, g-2), 0, paint.Solid(glyph))
				p.RRectStroke(geom.Rc(mid.X-g/2, mid.Y-g/2+2, g-2, g-2), 1, paint.Fill{}, paint.Stroke{Width: w, Color: glyph})
			} else {
				p.RRectStroke(geom.Rc(mid.X-g/2, mid.Y-g/2, g, g), 1, paint.Fill{}, paint.Stroke{Width: w, Color: glyph})
			}
		case 2:
			// Two strokes crossed.
			for _, a := range []float32{math.Pi / 4, -math.Pi / 4} {
				func() {
					defer p.Push(paint.Rotate(a, mid))()
					p.RRect(geom.Rc(mid.X-g*0.7, mid.Y-w/2, g*1.4, w), 0, paint.Solid(glyph))
				}()
			}
		}
	}
}

// WindowTitle is a chromeless window's title, centred in the room it is
// given, all of which is title bar: a press on it moves the window, and
// a double click maximizes it.
type WindowTitle struct {
	label *Label
}

// NewWindowTitle returns a title showing text.
func NewWindowTitle(text string) *WindowTitle {
	l := NewLabel(text)
	l.MaxLines = 1
	return &WindowTitle{label: l}
}

// SetText changes the title.
func (t *WindowTitle) SetText(text string) { t.label.SetText(text) }

// Label is the label showing the title, to style.
func (t *WindowTitle) Label() *Label { return t.label }

// Children implements [gunim.Composite].
func (t *WindowTitle) Children() []gunim.Node { return []gunim.Node{t.label} }

// Layout implements [gunim.Node]: the title is centred, and cut short
// when the room runs out.
func (t *WindowTitle) Layout(cs gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	own := cs.Max
	k := kids.At(0)
	if !f.Chromeless() {
		// The system's title bar says it.
		k.Layout(gunim.Tight(geom.Size{}))
		return cs.Constrain(geom.Sz(own.W, 0))
	}
	size := k.Layout(gunim.Constraints{Max: own})
	k.Place(geom.Pt(max(0, (own.W-size.W)/2), max(0, (own.H-size.H)/2)))
	return own
}

// Paint implements [gunim.Node].
func (t *WindowTitle) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}

// CaptionRects implements [gunim.Caption]: all of it.
//
// It is room left over, so a row holding it should let it grow.
func (t *WindowTitle) CaptionRects(size geom.Size) []geom.Rect {
	return []geom.Rect{{Max: size.Point()}}
}
