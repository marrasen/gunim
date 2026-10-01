package main

import (
	_ "embed"
	"fmt"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// Lesson 2: a button, and the two halves.
//
// A gunim program is two halves that speak in values. The window half
// owns the nodes and runs on the window's goroutine. The application
// half owns the work and runs wherever it likes. A button's On is the
// intent it sends when clicked; the application hears it on
// Client.Intents, changes its state, and hands the view fresh state
// with Client.Update.

//go:embed lesson2.go
var lesson2Source string

// The vocabulary the two halves share.
type (
	// Counter is what the view shows.
	Counter struct{ Clicks int }
	// Clicked travels when the button is pressed.
	Clicked struct{}
	// Reset travels when the count is set back to zero.
	Reset struct{}
)

func init() {
	gunim.RegisterType[Counter]("tutorial.counter")
	gunim.RegisterType[Clicked]("tutorial.clicked")
	gunim.RegisterType[Reset]("tutorial.reset")
}

var lesson2 = lesson{
	Title:  lessonTitles[1],
	File:   lessonFile(1),
	Source: lesson2Source,
	View:   "lesson2",
	Register: func(w *gunim.Window) {
		gunim.RegisterView(w, "lesson2", buildLesson2, (*counterPage).show)
	},
	State: func(a *app) any { return Counter{Clicks: a.clicks} },
	// Handle is the application half of the lesson.
	Handle: func(a *app, _ gunim.Client, in gunim.Intent) bool {
		switch in.(type) {
		case Clicked:
			a.clicks++
		case Reset:
			a.clicks = 0
		default:
			return false
		}
		return true
	},
}

const lesson2Intro = `A gunim program is two halves that speak only in values. The **window half** owns the nodes and runs on the window's goroutine. The **application half** owns the work and runs on a goroutine of its own, here in ` + "`serve`" + ` in main.go.

A button's ` + "`On`" + ` field is the intent it sends when pressed. It is a plain value, so the wiring is data. The application reads intents from ` + "`Client.Intents`" + `, changes its state, and hands the view fresh state with ` + "`Client.Update`" + `. The view's update function turns that state into what the nodes show.

Press the button: the click travels to the application as ` + "`Clicked`" + `, the count goes up there, and ` + "`Counter`" + ` comes back. The count on screen is the application's, every time.

The view's update function is also where a change gets its motion. Here it sees the count change and gives the label a bump: a spring on a scale, overshooting and settling, that ` + "`bump`" + ` below paints its child through.`

// counterPage is the lesson's view, with handles on the label the
// update function changes and the bump that animates the change.
type counterPage struct {
	*page
	count *widget.Label
	bump  *bump
	shown int
}

// buildLesson2 builds the page. It runs once, when the view is mounted.
func buildLesson2(Counter) *counterPage {
	count := widget.NewLabel("")
	count.Size = widget.HeadingSize
	press := widget.NewButton("Press me")
	press.Kind = widget.ButtonPrimary
	press.On = Clicked{}
	reset := widget.NewButton("Reset")
	reset.On = Reset{}
	b := newBump(count)
	row := widget.Row(press, reset, b)
	row.Cross = widget.CrossCenter
	return &counterPage{
		page:  newPage(1, lesson2Intro, row, lesson2Source),
		count: count,
		bump:  b,
	}
}

// show is the view's update function. It runs after the build, and
// again each time the application sends state.
func (p *counterPage) show(s Counter, u *gunim.UI) {
	switch s.Clicks {
	case 0:
		p.count.Text = "Press it and see"
	case 1:
		p.count.Text = "Pressed once"
	default:
		p.count.Text = fmt.Sprintf("Pressed %d times", s.Clicks)
	}
	if s.Clicks != p.shown {
		p.bump.Bump(u)
	}
	p.shown = s.Clicks
	u.Invalidate()
}

// bump paints its child scaled by a spring, so a change to it can pop.
type bump struct {
	anim.Group
	scale *anim.Float
	child gunim.Node
}

func newBump(child gunim.Node) *bump {
	b := &bump{scale: anim.NewFloat(1), child: child}
	b.Add(b.scale)
	return b
}

// Bump kicks the scale up and lets the theme's bounce bring it back.
// Jump sets a value with no motion; Animate starts the spring.
func (b *bump) Bump(u *gunim.UI) {
	b.scale.Jump(1.35)
	b.scale.Animate(1, widget.Bounce.Get(u.Theme()))
}

// Children implements [gunim.Composite].
func (b *bump) Children() []gunim.Node { return []gunim.Node{b.child} }

// Layout implements [gunim.Node]: the child's size is the bump's.
func (b *bump) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	size := kids.At(0).Layout(c)
	kids.At(0).Place(geom.Point{})
	return size
}

// Paint implements [gunim.Node]: the child, scaled about its left
// middle so the text grows out from where it starts.
func (b *bump) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	defer p.Push(paint.Scale(b.scale.Value(), geom.Pt(0, box.H/2)))()
	kids.At(0).Paint(p)
}
