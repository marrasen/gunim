package main

import (
	"image/color"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/widget"
)

// registerViews is the window half: the calculator's view.
func registerViews(w *gunim.Window) {
	gunim.RegisterView(w, "calc",
		func(Calc) *calcRoot { return newCalcRoot() },
		func(r *calcRoot, s Calc, u *gunim.UI) { r.show(s, u) })
}

// runs shapes text once for each string and size it is asked for, for
// nodes that paint text every frame. It is reached from the UI
// goroutine alone.
var runs = map[runKey]text.Run{}

type runKey struct {
	s    string
	size float32
}

// shaped is s shaped at size, from the cache.
func shaped(s string, size float32) text.Run {
	k := runKey{s, size}
	if r, ok := runs[k]; ok {
		return r
	}
	if len(runs) > 2000 {
		// Axis labels at every zoom add up; start again.
		clear(runs)
	}
	r := text.Default().Shape(s, size)
	runs[k] = r
	return r
}

// hues are the colours curves take, in turn.
var hues = []color.NRGBA{
	{R: 0x5e, G: 0x9c, B: 0xff, A: 0xff},
	{R: 0xff, G: 0x7a, B: 0x6b, A: 0xff},
	{R: 0x4f, G: 0xd6, B: 0x9c, A: 0xff},
	{R: 0xf5, G: 0xc2, B: 0x42, A: 0xff},
	{R: 0xc4, G: 0x8b, B: 0xff, A: 0xff},
	{R: 0x3f, G: 0xd0, B: 0xe0, A: 0xff},
}

func hue(i int) color.NRGBA { return hues[i%len(hues)] }

// faded is c at alpha a, from 0 to 1.
func faded(c color.NRGBA, a float32) color.NRGBA {
	c.A = uint8(float32(c.A) * min(max(a, 0), 1))
	return c
}

// titleHeight is the height of the window's own title bar.
const titleHeight = 40

// calcRoot is the whole window: the title bar, and under it the
// calculator and the graph, one of which shows. Switching, the keypad
// slides away as the graph grows out of the display into the window,
// or the other way.
type calcRoot struct {
	anim.Group
	bar   *titleBar
	calc  *calcBody
	graph *graphBody
	// mode is 0 showing the calculator, 1 the graph.
	mode  *anim.Float
	state Calc
	// body is where the calculator and the graph go, and from the rect
	// the graph grows out of: the display's.
	body, from geom.Rect
}

func newCalcRoot() *calcRoot {
	r := &calcRoot{mode: anim.NewFloat(0)}
	r.bar = newTitleBar(r)
	r.calc = newCalcBody(r)
	r.graph = newGraphBody(r)
	r.Add(r.mode)
	return r
}

// show takes the application's state.
func (r *calcRoot) show(s Calc, u *gunim.UI) {
	was := r.state
	r.state = s
	to := float32(0)
	if s.Graph {
		to = 1
	}
	if s.Graph != was.Graph || r.mode.Target() != to {
		r.mode.Animate(to, anim.Spring{Response: 0.55, Damping: 0.82})
		if s.Graph {
			r.graph.arrive()
		}
	}
	r.bar.show(s.Graph)
	r.calc.show(was, s, u)
	r.graph.show(s, u)
	u.Invalidate()
}

// Children implements [gunim.Composite].
func (r *calcRoot) Children() []gunim.Node { return []gunim.Node{r.bar, r.calc, r.graph} }

// Focusable implements [gunim.Focusable]: the window's keys come here.
func (r *calcRoot) Focusable() bool { return true }

// Layout implements [gunim.Node].
func (r *calcRoot) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	size := c.Max
	bar := kids.At(0)
	bar.Layout(gunim.Tight(geom.Sz(size.W, titleHeight)))
	bar.Place(geom.Point{})
	r.body = geom.Rc(0, titleHeight, size.W, size.H-titleHeight)
	for i := 1; i < 3; i++ {
		k := kids.At(i)
		k.Layout(gunim.Tight(r.body.Size()))
		k.Place(r.body.Min)
	}
	r.from = r.calc.display.Add(r.body.Min)
	return size
}

// Paint implements [gunim.Node]: the calculator slides off to the left
// and fades as the graph grows out of the display's card, until it
// fills the window.
func (r *calcRoot) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	th := f.Theme
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(widget.Background.Get(th)))
	m := min(max(r.mode.Value(), -0.1), 1.1)
	if m < 1 {
		func() {
			defer p.Layer(paint.LayerOpts{Bounds: r.body, Opacity: min(max(1-m*1.4, 0), 1)})()
			defer p.Push(paint.Translate(geom.Pt(-60*m, 0)))()
			defer p.Push(paint.Scale(1-0.06*m, r.body.Center()))()
			kids.At(1).Paint(p)
		}()
	}
	if m > 0 {
		// The rect the graph fills, from the display's to the body's.
		at := mixRect(r.from, r.body, m)
		sx := at.Size().W / max(r.body.Size().W, 1)
		sy := at.Size().H / max(r.body.Size().H, 1)
		func() {
			defer p.Layer(paint.LayerOpts{Bounds: at, Opacity: min(max(m*1.6, 0), 1), Clip: true, Radius: 18 * (1 - min(m, 1))})()
			defer p.Push(paint.Transform{A: sx, C: at.Min.X - r.body.Min.X*sx, E: sy, F: at.Min.Y - r.body.Min.Y*sy})()
			kids.At(2).Paint(p)
		}()
	}
	kids.At(0).Paint(p)
}

// mixRect is the rect t of the way from a to b.
func mixRect(a, b geom.Rect, t float32) geom.Rect {
	mix := func(x, y float32) float32 { return x + (y-x)*t }
	return geom.Rect{
		Min: geom.Pt(mix(a.Min.X, b.Min.X), mix(a.Min.Y, b.Min.Y)),
		Max: geom.Pt(mix(a.Max.X, b.Max.X), mix(a.Max.Y, b.Max.Y)),
	}
}

// Handle implements [gunim.Handler]: the keyboard types, as the keypad
// does. Tab switches between the calculator and the graph.
func (r *calcRoot) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.KeyPress:
		var key string
		switch e.Key {
		case input.KeyEnter, input.KeyKPEnter:
			key = "="
		case input.KeyBackspace:
			key = "⌫"
		case input.KeyEscape, input.KeyDelete:
			key = "C"
		case input.KeyTab:
			u.Send(r, ShowGraph{On: !r.state.Graph})
			return true
		default:
			return false
		}
		r.press(key, u)
		return true
	case input.TextInput:
		for _, c := range e.Text {
			if key, ok := typedKey(c); ok {
				r.press(key, u)
			}
		}
		return true
	}
	return false
}

// press sends a key, and lights it on whichever keypad shows.
func (r *calcRoot) press(key string, u *gunim.UI) {
	if r.state.Graph {
		r.graph.keys.flash(key, u)
	} else {
		r.calc.keypad.flash(key, u)
	}
	u.Send(r, Pressed{Key: key})
}

// typedKey is the key a character typed stands for.
func typedKey(c rune) (string, bool) {
	switch c {
	case '*':
		return "×", true
	case '/':
		return "÷", true
	case '-':
		return "−", true
	case 'p':
		return "π", true
	case 's':
		return "sin", true
	case 'c':
		return "cos", true
	case 't':
		return "tan", true
	case 'r':
		return "√", true
	case 'l':
		return "ln", true
	case '=':
		return "=", true
	}
	if strings.ContainsRune("0123456789.+^%()xeπ×÷−", c) {
		return string(c), true
	}
	return "", false
}

// titleBar is the window's own title bar: the switch between the
// calculator and the graph, the title, which moves the window, and the
// window's buttons.
type titleBar struct {
	r        *calcRoot
	sw       *modeSwitch
	title    *widget.WindowTitle
	controls *widget.WindowControls
}

func newTitleBar(r *calcRoot) *titleBar {
	t := &titleBar{r: r, sw: newModeSwitch(r), title: widget.NewWindowTitle("Calculator"), controls: widget.NewWindowControls()}
	t.title.Label().Color = widget.Ink
	return t
}

func (t *titleBar) show(graph bool) { t.sw.show(graph) }

// Children implements [gunim.Composite].
func (t *titleBar) Children() []gunim.Node { return []gunim.Node{t.sw, t.title, t.controls} }

// Layout implements [gunim.Node].
func (t *titleBar) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	size := c.Max
	sw, title, controls := kids.At(0), kids.At(1), kids.At(2)
	cs := controls.Layout(gunim.Constraints{Max: size})
	controls.Place(geom.Pt(size.W-cs.W, 0))
	ss := sw.Layout(gunim.Constraints{Max: geom.Sz(size.W, size.H)})
	sw.Place(geom.Pt(10, (size.H-ss.H)/2))
	left := 10 + ss.W + 10
	title.Layout(gunim.Tight(geom.Sz(max(0, size.W-cs.W-left), size.H)))
	title.Place(geom.Pt(left, 0))
	return size
}

// Paint implements [gunim.Node].
func (t *titleBar) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(widget.MenubarFill.Get(f.Theme)))
	for k := range kids.All {
		k.Paint(p)
	}
}

// modeSwitch is the switch between the calculator and the graph: two
// words, and a pill that glides to the one showing.
type modeSwitch struct {
	anim.Group
	r     *calcRoot
	pill  *anim.Float
	hover *anim.Float
	on    int
}

var modeWords = [2]string{"Calc", "Graph"}

// modeWidth is each word's room in the switch.
const modeWidth, modeHeight = 64, 28

func newModeSwitch(r *calcRoot) *modeSwitch {
	s := &modeSwitch{r: r, pill: anim.NewFloat(0), hover: anim.NewFloat(0)}
	s.Add(s.pill, s.hover)
	return s
}

func (s *modeSwitch) show(graph bool) {
	to := 0
	if graph {
		to = 1
	}
	if to != s.on {
		s.on = to
		s.pill.Animate(float32(to), anim.Bouncy)
	}
}

// Layout implements [gunim.Node].
func (s *modeSwitch) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(geom.Sz(2*modeWidth, modeHeight))
}

// Paint implements [gunim.Node].
func (s *modeSwitch) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	track := widget.Ink.Get(th)
	p.RRect(geom.Rect{Max: box.Point()}, box.H/2, paint.Solid(faded(track, 0.08+0.04*s.hover.Value())))
	x := s.pill.Value() * modeWidth
	p.RRect(geom.Rc(x+2, 2, modeWidth-4, box.H-4), (box.H-4)/2, paint.Solid(widget.Accent.Get(th)))
	for i, word := range modeWords {
		run := shaped(word, 13)
		// The word the pill is on reads on the pill; the other, faint.
		near := 1 - min(abs32(s.pill.Value()-float32(i)), 1)
		ink := faded(track, 0.55+0.45*near)
		run.Paint(p, geom.Pt(float32(i)*modeWidth+(modeWidth-run.Advance)/2, (box.H-run.Height())/2), ink)
	}
}

// Handle implements [gunim.Handler].
func (s *modeSwitch) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerEnter:
		s.hover.Animate(1, anim.Snappy)
	case input.PointerLeave:
		s.hover.Animate(0, anim.Snappy)
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		u.Send(s, ShowGraph{On: e.Pos.X >= modeWidth})
	default:
		return false
	}
	return true
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
