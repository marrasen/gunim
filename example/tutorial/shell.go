package main

import (
	"fmt"
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// hueColors are the lessons' colours, one each, and hues the same as
// theme tokens: a theme can set them, and a switch animates them like
// any other.
var hueColors = []color.NRGBA{
	{R: 0x5e, G: 0x9c, B: 0xff, A: 0xff},
	{R: 0xff, G: 0x7a, B: 0x6b, A: 0xff},
	{R: 0x4f, G: 0xd6, B: 0x9c, A: 0xff},
	{R: 0xf5, G: 0xc2, B: 0x42, A: 0xff},
	{R: 0xc4, G: 0x8b, B: 0xff, A: 0xff},
	{R: 0x3f, G: 0xd0, B: 0xe0, A: 0xff},
}

var hues = func() []theme.Token[color.NRGBA] {
	out := make([]theme.Token[color.NRGBA], len(hueColors))
	for i, c := range hueColors {
		out[i] = theme.Color(fmt.Sprintf("tutorial.hue.%d", i+1), c)
	}
	return out
}()

// lessonTitles name the lessons, in order. They live apart from the
// lessons so a page can name its lesson while the lessons are built.
var lessonTitles = []string{
	"A label in a card",
	"A button and the two halves",
	"A list that animates",
	"A node of your own",
	"Themes and tokens",
	"A dialog that mounts",
}

// hue is lesson i's colour token.
func hue(i int) theme.Token[color.NRGBA] { return hues[i%len(hues)] }

// tinted is c at alpha a, from 0 to 1.
func tinted(c color.NRGBA, a float32) color.NRGBA {
	c.A = uint8(float32(c.A) * min(max(a, 0), 1))
	return c
}

// The shell's vocabulary: what the application tells it, and what it
// tells the application.
type (
	// Shell is the shell's state: which lesson is open.
	Shell struct{ Lesson int }
	// Chose travels when a lesson is picked from the list.
	Chose struct{ Lesson int }
)

func init() {
	gunim.RegisterType[Shell]("tutorial.shell")
	gunim.RegisterType[Chose]("tutorial.chose")
}

// registerShell registers the shell's view: a list of the lessons down
// the left, and room for the open lesson beside it.
func registerShell(w *gunim.Window) {
	gunim.RegisterView(w, "shell", newShell, func(s *shell, st Shell, u *gunim.UI) {
		s.list.choose(st.Lesson, u)
	})
}

// sidebarWidth is how wide the lesson list is.
const sidebarWidth = 260

// shell is the frame around the lessons. Views mounted under it go in
// its body, as Slot says.
type shell struct {
	row  *widget.Flex
	body *gunim.Box
	list *lessonList
}

func newShell(Shell) *shell {
	s := &shell{body: &gunim.Box{}, list: newLessonList()}
	title := widget.NewLabel("gunim")
	title.Size = widget.HeadingSize
	col := widget.Column(title, s.list)
	col.Cross = widget.CrossStretch
	side := widget.NewSized(widget.NewPad(col), sidebarWidth, 0)
	s.row = widget.Row(side, s.body).Grow(s.body, 1)
	s.row.Cross = widget.CrossStretch
	return s
}

// lessonList is the list of lessons: a button each, and behind the
// chosen one a pill in the lesson's colour. The pill springs from
// lesson to lesson, and its colour blends on the way, through Oklab.
type lessonList struct {
	anim.Group
	buttons []*widget.Button
	pill    *anim.Rect
	tint    *anim.Color
	// slots are where the last layout put the buttons.
	slots  []geom.Rect
	chosen int
	laid   bool
}

func newLessonList() *lessonList {
	l := &lessonList{pill: anim.NewRect(geom.Rect{}), tint: anim.NewColor(color.NRGBA{}), chosen: -1}
	l.Add(l.pill, l.tint)
	for i := range lessons {
		b := widget.NewButton(lessonTitle(i))
		b.Ghost = true
		// On is the intent the button sends. It is a value, so the
		// wiring is data the application can read back.
		b.On = Chose{Lesson: i}
		l.buttons = append(l.buttons, b)
	}
	return l
}

// choose moves the pill to lesson i. The first choice puts it there
// at once; the rest spring.
func (l *lessonList) choose(i int, u *gunim.UI) {
	l.chosen = i
	for j, b := range l.buttons {
		b.Active = j == i
	}
	th := u.Theme()
	if !l.laid {
		l.tint.Jump(hue(i).Get(th))
	} else {
		l.pill.Animate(l.slots[i], widget.Bounce.Get(th))
		l.tint.Animate(hue(i).Get(th), widget.Settle.Get(th))
	}
	u.Invalidate()
}

// Children implements [gunim.Composite].
func (l *lessonList) Children() []gunim.Node {
	kids := make([]gunim.Node, len(l.buttons))
	for i, b := range l.buttons {
		kids[i] = b
	}
	return kids
}

// Layout implements [gunim.Node]: the buttons stack with the theme's
// gap, each as wide as the list.
func (l *lessonList) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	gap := widget.Gap.Get(f.Theme)
	l.slots = l.slots[:0]
	y := float32(0)
	for k := range kids.All {
		size := k.Layout(gunim.Constraints{Min: geom.Sz(c.Max.W, 0), Max: geom.Sz(c.Max.W, c.Max.H)})
		k.Place(geom.Pt(0, y))
		l.slots = append(l.slots, geom.Rc(0, y, size.W, size.H))
		y += size.H + gap
	}
	if !l.laid && l.chosen >= 0 {
		l.pill.Jump(l.slots[l.chosen])
		l.laid = true
	}
	return c.Constrain(geom.Sz(c.Max.W, y-gap))
}

// Paint implements [gunim.Node]: the pill, then the buttons over it.
func (l *lessonList) Paint(p *paint.Painter, f gunim.Frame, _ geom.Size, kids gunim.Children) {
	r := l.pill.Value()
	c := l.tint.Value()
	p.RRect(r, widget.ButtonRadius.Get(f.Theme), paint.Solid(tinted(c, 0.18)))
	bar := geom.Rc(r.Min.X, r.Min.Y+8, 4, r.Size().H-16)
	p.RRect(bar, 2, paint.Solid(c))
	for k := range kids.All {
		k.Paint(p)
	}
}

// Children implements [gunim.Composite].
func (s *shell) Children() []gunim.Node { return []gunim.Node{s.row} }

// Slot implements [gunim.Slotted]: a view mounted under the shell goes
// in its body.
func (s *shell) Slot() gunim.Node { return s.body }

// Layout implements [gunim.Node].
func (s *shell) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kids.At(0).Layout(gunim.Tight(c.Max))
	kids.At(0).Place(geom.Point{})
	return c.Max
}

// Paint implements [gunim.Node].
func (s *shell) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}
