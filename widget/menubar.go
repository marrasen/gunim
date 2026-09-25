package widget

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Menubar tokens.
var (
	MenubarFill   = theme.Color("menubar.fill", color.NRGBA{R: 0x1b, G: 0x1e, B: 0x26, A: 0xff})
	MenubarHeight = theme.Length("menubar.height", 30)
	// MenubarHot lights the title under the pointer, and the title of
	// the menu that is open.
	MenubarHot = theme.Color("menubar.hot", color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x16})
)

// BarMenu is one menu of a [Menubar]: its title, and its items, which
// take the extras a [Menu] does.
type BarMenu struct {
	Title    string
	Items    []string
	Hints    []string
	Checked  []bool
	Disabled []bool
	Breaks   []int
}

// Menubar is a row of menu titles along the top of a window, each
// opening its menu below it.
//
// A click on a title opens its menu; with one open, the pointer moving
// onto another title opens that one instead, and a light glides along
// the bar to it. While a menu is open the menubar has the keyboard:
// Left and Right move between menus, Up, Down and Enter work the menu,
// and Escape closes it, giving the keyboard back where it was. A click
// on the bar leaves the keyboard where it is, so the bar's commands
// act on what has it.
type Menubar struct {
	anim.Group
	Menus []BarMenu
	// Pick runs on the UI goroutine with the menu and the item picked,
	// once the menu has closed.
	Pick func(menu, item int, u *gunim.UI)

	open  int
	popup *gunim.Popup
	menu  *Menu
	back  gunim.Node
	// over is the title under the pointer, or -1.
	over int

	titles []shapedText
	// spans holds each title's left and right edges.
	spans    [][2]float32
	lightX   *anim.Float
	lightW   *anim.Float
	lightOn  *anim.Float
	lightSet bool
}

// NewMenubar returns a menubar of menus.
func NewMenubar(menus ...BarMenu) *Menubar {
	b := &Menubar{Menus: menus, open: -1, over: -1, lightX: anim.NewFloat(0), lightW: anim.NewFloat(0), lightOn: anim.NewFloat(0)}
	b.Add(b.lightX, b.lightW, b.lightOn)
	return b
}

// KeepsFocus implements [gunim.FocusKeeper].
func (b *Menubar) KeepsFocus() {}

// IsOpen reports whether a menu is open.
func (b *Menubar) IsOpen() bool { return b.open >= 0 }

// Open opens menu i, taking the keyboard, as F10 does in most
// programs.
func (b *Menubar) Open(i int, u *gunim.UI) {
	if i < 0 || i >= len(b.Menus) || i == b.open {
		return
	}
	if b.open < 0 {
		b.back = u.Focused()
	}
	b.shut()
	b.open = i
	m := b.Menus[i]
	menu := NewMenu(m.Items...)
	menu.Hints, menu.Checked, menu.Disabled, menu.Breaks = m.Hints, m.Checked, m.Disabled, m.Breaks
	menu.MinWidth = 180
	menu.Pick = func(item int, u *gunim.UI) {
		b.Close(u)
		if b.Pick != nil {
			b.Pick(i, item, u)
		}
	}
	b.menu = menu
	span := b.span(i)
	b.popup = u.OpenPopup(b, menu, gunim.PopupOptions{
		Anchor:  geom.Rect{Min: geom.Pt(span[0], 0), Max: geom.Pt(span[1], MenubarHeight.Get(u.Theme()))},
		Max:     geom.Sz(600, 800),
		Dismiss: b.Close,
	})
	b.aim(i, u)
	u.Focus(b)
	u.Invalidate()
}

// Close closes the open menu, and gives the keyboard back.
func (b *Menubar) Close(u *gunim.UI) {
	if b.open < 0 {
		return
	}
	b.shut()
	b.open = -1
	if b.back != nil {
		u.Focus(b.back)
		b.back = nil
	}
	b.aim(b.over, u)
	u.Invalidate()
}

func (b *Menubar) shut() {
	if b.popup != nil {
		b.popup.Close()
		b.popup, b.menu = nil, nil
	}
}

func (b *Menubar) span(i int) [2]float32 {
	if i >= 0 && i < len(b.spans) {
		return b.spans[i]
	}
	return [2]float32{}
}

// aim glides the light to title i, or fades it for -1.
func (b *Menubar) aim(i int, u *gunim.UI) {
	motion := Quick.Get(u.Theme())
	if i < 0 {
		b.lightOn.Animate(0, motion)
		return
	}
	s := b.span(i)
	if !b.lightSet || b.lightOn.Value() < 0.05 {
		b.lightX.Jump(s[0])
		b.lightW.Jump(s[1] - s[0])
		b.lightSet = true
	} else {
		b.lightX.Animate(s[0], motion)
		b.lightW.Animate(s[1]-s[0], motion)
	}
	b.lightOn.Animate(1, motion)
}

// titleAt returns the title at p, in the bar's space, or -1.
func (b *Menubar) titleAt(p geom.Point) int {
	for i, s := range b.spans {
		if p.X >= s[0] && p.X < s[1] {
			return i
		}
	}
	return -1
}

// Handle implements [gunim.Handler].
func (b *Menubar) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerEnter:
		b.hover(b.titleAt(e.Pos), u)
		return true
	case input.PointerMove:
		b.hover(b.titleAt(e.Pos), u)
		return true
	case input.PointerLeave:
		b.over = -1
		if b.open < 0 {
			b.aim(-1, u)
		}
		return true
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		i := b.titleAt(e.Pos)
		switch {
		case i < 0:
		case i == b.open:
			b.Close(u)
		default:
			b.Open(i, u)
		}
		return true
	case input.KeyPress:
		return b.key(e, u)
	}
	return false
}

// hover follows the pointer onto title i, or off the titles for -1:
// with a menu open, onto that title's menu.
func (b *Menubar) hover(i int, u *gunim.UI) {
	if i == b.over {
		return
	}
	b.over = i
	switch {
	case b.open >= 0 && i >= 0:
		b.Open(i, u)
	case b.open < 0:
		b.aim(i, u)
	}
}

func (b *Menubar) key(k input.KeyPress, u *gunim.UI) bool {
	if b.open < 0 {
		return false
	}
	n := len(b.Menus)
	switch k.Key {
	case input.KeyLeft:
		b.Open((b.open+n-1)%n, u)
	case input.KeyRight:
		b.Open((b.open+1)%n, u)
	case input.KeyEscape, input.KeyF10:
		b.Close(u)
	case input.KeyTab:
		b.Close(u)
		return false
	default:
		if b.menu != nil && b.menu.Key(k, u) {
			u.Invalidate()
			return true
		}
		// While a menu is open, other keys stop here, so nothing behind
		// the menu acts on them.
		return !k.Mods.Has(input.ModControl)
	}
	return true
}

// Layout implements [gunim.Node]. The bar is as wide as it is given.
func (b *Menubar) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	if len(b.titles) != len(b.Menus) {
		b.titles = make([]shapedText, len(b.Menus))
	}
	b.spans = b.spans[:0]
	pad := MenuRowPadding.Get(th)
	x := float32(4)
	for i, m := range b.Menus {
		w := b.titles[i].shape(m.Title, TextSize.Get(th)).Advance + 2*pad
		b.spans = append(b.spans, [2]float32{x, x + w})
		x += w
	}
	return c.Constrain(geom.Sz(max(c.Max.W, x), MenubarHeight.Get(th)))
}

// Paint implements [gunim.Node].
func (b *Menubar) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(MenubarFill.Get(th)))
	if t := min(max(b.lightOn.Value(), 0), 1); t > 0.01 {
		c := MenubarHot.Get(th)
		c.A = uint8(float32(c.A) * t)
		p.RRect(geom.Rc(b.lightX.Value(), 3, b.lightW.Value(), box.H-6), 5, paint.Solid(c))
	}
	pad := MenuRowPadding.Get(th)
	ink := Ink.Get(th)
	for i, s := range b.spans {
		run := b.titles[i].run
		run.Paint(p, geom.Pt(s[0]+pad, (box.H-run.Height())/2), ink)
	}
}
