package plugui

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// around holds a control at its own size, and counts the presses and keys the control passes on to it.
type around struct {
	child gunim.Node
	downs map[input.Button]int
	keys  int
}

func (a *around) Children() []gunim.Node { return []gunim.Node{a.child} }

func (a *around) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kids.At(0).Layout(gunim.Constraints{Max: c.Max})
	kids.At(0).Place(geom.Point{})
	return c.Max
}

func (a *around) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}

func (a *around) Handle(e input.Event, _ *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		a.downs[e.Button]++
		return true
	case input.KeyPress:
		a.keys++
		return true
	}
	return false
}

// look is a control of the package as a test of its looks works it greyed.
type look struct {
	name string
	node gunim.Node
	grey func(bool)
	// at is a point on the control, at its own size, and hover how lit it is under the pointer, by where its light
	// is heading.
	at    func() geom.Point
	hover func() float32
}

// looks returns one of each control of the package, greyed by its lock or its Disabled, and a knob greyed but
// turning last.
func looks() []look {
	h := &host{values: map[uint32]float64{3: 100}}
	k, turning := NewKnob(h, release), NewKnob(h, release)
	sel := NewSelector("Stereo", []string{"Left", "Both"}, nil)
	p, d := NewPower("gate"), NewActionDial("Learn", nil)
	capAt := func() geom.Point { return geom.Pt(KnobW/2, 21) }
	return []look{
		{"locked knob", k, func(on bool) { k.Locked = on }, capAt, k.hover.Target},
		{"selector", sel, func(on bool) { sel.Locked = on }, func() geom.Point { return sel.cells[1].Center() },
			func() float32 { return float32(anim.OnOff(sel.hot >= 0)) }},
		{"power lamp", p, func(on bool) { p.Disabled = on },
			func() geom.Point { return geom.Pt(PowerSize/2, PowerSize/2) }, p.hover.Target},
		{"action dial", d, func(on bool) { d.Disabled = on }, capAt, d.hover.Target},
		{"knob greyed but turning", turning, func(on bool) { turning.Disabled = on }, capAt, turning.hover.Target},
	}
}

// stageAround mounts c inside an around, and returns the window and the around.
func stageAround(t *testing.T, c look) (*gunim.Window, *around) {
	t.Helper()
	a := &around{child: c.node, downs: map[input.Button]int{}}
	return stageNode(t, a), a
}

// A greyed control keeps a click to itself, as a disabled widget does, and passes a right click and the keys on to
// what is round it, even with the keyboard on it.
func TestAGreyedControlKeepsAClickAndPassesTheRest(t *testing.T) {
	for _, c := range looks()[:4] {
		w, a := stageAround(t, c)
		w.Input(input.KeyPress{Key: input.KeyTab})
		frames(w, 2)
		c.grey(true)
		frames(w, 2)
		at := c.at()
		for _, b := range []input.Button{input.ButtonPrimary, input.ButtonSecondary} {
			w.Input(input.PointerMove{Pos: at, Time: time.Now()})
			w.Input(input.PointerDown{Pos: at, Button: b, Clicks: 1, Time: time.Now()})
			w.Input(input.PointerUp{Pos: at, Button: b, Time: time.Now()})
			frames(w, 1)
		}
		for _, k := range []input.Key{input.KeySpace, input.KeyEnter, input.KeyUp} {
			w.Input(input.KeyPress{Key: k})
		}
		frames(w, 1)
		if a.downs[input.ButtonPrimary] != 0 || a.downs[input.ButtonSecondary] != 1 || a.keys != 3 {
			t.Errorf("round the greyed %s, %d clicks, %d right clicks and %d keys arrived, want 0, 1 and 3",
				c.name, a.downs[input.ButtonPrimary], a.downs[input.ButtonSecondary], a.keys)
		}
	}
}

// A greyed control takes no keyboard, and a greyed press squashes nothing.
func TestAGreyedControlTakesNoKeyboardAndNoPress(t *testing.T) {
	for _, c := range looks()[:4] {
		c.grey(true)
		if f, ok := c.node.(gunim.Focusable); ok && f.Focusable() {
			t.Errorf("the greyed %s takes the keyboard", c.name)
		}
		w, _ := stageAround(t, c)
		at := c.at()
		w.Input(input.PointerMove{Pos: at, Time: time.Now()})
		w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
		frames(w, 2)
		var squash float32
		switch n := c.node.(type) {
		case *ActionDial:
			squash = n.squash.Target()
		case *Power:
			squash = n.squash.Target()
		case *Selector:
			squash = n.press.Target()
		}
		if squash != 0 {
			t.Errorf("pressed while greyed, the %s squashes to %v", c.name, squash)
		}
	}
}

// ringShown is how far the focus ring n paints into box shows, from 0 to 1.
func ringShown(n gunim.Node, box geom.Size) float32 {
	th := theme.NewLive(widget.Dark())
	accent := widget.Accent.Get(th)
	var p paint.Painter
	n.Paint(&p, gunim.Frame{Scale: 1, Theme: th}, box, gunim.Children{})
	most := float32(0)
	for _, op := range p.Ops() {
		r, ok := op.(*paint.RRectOp)
		if !ok || r.Stroke.Width != 2 {
			continue
		}
		if c := r.Stroke.Color; c.R == accent.R && c.G == accent.G && c.B == accent.B {
			most = max(most, float32(c.A)/(float32(accent.A)*0.56))
		}
	}
	return most
}

// A control greyed with the keyboard on it fades its ring out, and brings it back as it is freed.
func TestAGreyedControlsFocusRingFades(t *testing.T) {
	for _, c := range looks()[:4] {
		w := stageNode(t, c.node)
		box := geom.Sz(100, 100)
		w.Input(input.KeyPress{Key: input.KeyTab})
		frames(w, 40)
		if r := ringShown(c.node, box); r < 0.9 {
			t.Fatalf("Tab left the %s's ring at %v", c.name, r)
		}
		c.grey(true)
		frames(w, 2)
		if r := ringShown(c.node, box); r < 0.01 || r > 0.99 {
			t.Errorf("two frames after it greyed with the keyboard on it, the %s's ring shows %v, want part way",
				c.name, r)
		}
		frames(w, 40)
		if r := ringShown(c.node, box); r > 0 {
			t.Errorf("greyed with the keyboard on it, the %s's ring still shows %v", c.name, r)
		}
		c.grey(false)
		frames(w, 40)
		if r := ringShown(c.node, box); r < 0.9 {
			t.Errorf("freed with the keyboard still on it, the %s's ring shows %v", c.name, r)
		}
	}
}

// A greyed control does not light under the pointer, but a knob greyed but turning does; one greyed under the
// pointer lets its light go, and one freed under it lights at once.
func TestAGreyedControlDoesNotLightUnderThePointer(t *testing.T) {
	for _, c := range looks() {
		turns := c.name == "knob greyed but turning"
		c.grey(true)
		w := stageNode(t, c.node)
		w.Input(input.PointerMove{Pos: c.at(), Time: time.Now()})
		frames(w, 2)
		if lit := c.hover(); lit != anim.OnOff(turns) {
			t.Errorf("greyed under the pointer, the %s's light heads to %v", c.name, lit)
		}
		c.grey(false)
		frames(w, 2)
		if c.hover() != 1 {
			t.Errorf("freed under the resting pointer, the %s's light heads to %v, want 1", c.name, c.hover())
		}
		c.grey(true)
		frames(w, 2)
		if lit := c.hover(); lit != anim.OnOff(turns) {
			t.Errorf("greyed again under the pointer, the %s's light heads to %v", c.name, lit)
		}
	}
}

// A locked selector shows the chosen option's name under the pointer, not the name of the lamp under it.
func TestALockedSelectorShowsTheChosenName(t *testing.T) {
	s := NewSelector("Stereo", []string{"Left", "Both"}, nil)
	s.Locked = true
	w := stageNode(t, s)
	w.Input(input.PointerMove{Pos: s.cells[1].Center(), Time: time.Now()})
	frames(w, 30)
	if s.shown.Target() != 0 || s.lit[1].Target() != 0 {
		t.Errorf("locked under the pointer on Both, the selector shows name %v and lights Both %v",
			s.shown.Target(), s.lit[1].Target())
	}
}
