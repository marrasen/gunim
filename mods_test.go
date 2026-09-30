package gunim

import (
	"testing"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// clicked is what a clicker sends as it is pressed.
type clicked struct{}

// clicker fills its room and sends clicked as it is pressed, counting the presses.
type clicker struct{ presses int }

func (*clicker) Layout(c Constraints, _ Frame, _ Children) geom.Size { return c.Max }
func (*clicker) Paint(*paint.Painter, Frame, geom.Size, Children)    {}
func (c *clicker) Handle(e input.Event, u *UI) bool {
	if _, ok := e.(input.PointerDown); ok {
		c.presses++
		u.Send(c, clicked{})
		return true
	}
	return false
}

func TestAnIntentSaysTheModifiersHeldAsItWasSent(t *testing.T) {
	RegisterType[clicked]("gunim.test.clicked")
	w := newTestWindow()
	w.ui.Insert(w.ui.Root(), &clicker{})
	run(w, 1)
	w.Input(input.PointerDown{Pos: geom.Pt(10, 10), Mods: input.ModControl})
	w.Input(input.PointerUp{Pos: geom.Pt(10, 10), Mods: input.ModControl})
	env := <-w.Client().Intents()
	if _, ok := env.Intent.(clicked); !ok || env.Mods != input.ModControl {
		t.Fatalf("the intent came as %#v with mods %v, want clicked with Control", env.Intent, env.Mods)
	}
	// The modifiers cross a socket with the intent
	b, err := MarshalEnvelope(env)
	if err != nil {
		t.Fatal(err)
	}
	back, err := UnmarshalEnvelope(b)
	if err != nil {
		t.Fatal(err)
	}
	if back.Mods != input.ModControl {
		t.Fatalf("the modifiers came back over the wire as %v", back.Mods)
	}
	// A press with none held sends none
	w.Input(input.PointerDown{Pos: geom.Pt(10, 10)})
	if env := <-w.Client().Intents(); env.Mods != 0 {
		t.Fatalf("a plain press sent mods %v", env.Mods)
	}
}
