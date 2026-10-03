package main

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// press presses keys in turn on a fresh calculator.
func press(keys ...string) *calcState {
	s := &calcState{}
	for _, k := range keys {
		s.press(k)
	}
	return s
}

func TestKeysWorkASumOutOntoTheTape(t *testing.T) {
	s := press("1", "2", "×", "(", "3", "+", "4", ")")
	if s.Preview != "84" {
		t.Fatalf("typing, the preview is %q", s.Preview)
	}
	s.press("=")
	if s.Expr != "84" || len(s.Tape) != 1 || s.Tape[0].Expr != "12×(3+4)" || s.Tape[0].Result != "84" {
		t.Fatalf("worked out, the state is %+v", s.Calc)
	}
	// A digit next starts afresh; a sign carries on from the answer.
	s.press("5")
	if s.Expr != "5" {
		t.Fatalf("a digit after = makes %q", s.Expr)
	}
	s.press("=")
	s.press("+")
	if s.Expr != "5+" {
		t.Fatalf("a sign after = makes %q", s.Expr)
	}
}

func TestASumOfXGoesOnTheGraph(t *testing.T) {
	s := press("sin", "x", ")")
	if s.Expr != "sin(x)" || s.Draft != "" {
		t.Fatalf("typed on the keypad, the state is %+v", s.Calc)
	}
	s.press("=")
	if !s.Graph || len(s.Plots) != 1 || s.Plots[0].Expr != "sin(x)" || s.Expr != "" {
		t.Fatalf("worked out, the state is %+v", s.Calc)
	}
	// On the graph, what is typed is drawn live while it reads, and the
	// last that read stays while it does not.
	s.press("x")
	s.press("^")
	if s.Draft != "x" {
		t.Fatalf("typing x^, the draft is %q", s.Draft)
	}
	s.press("2")
	if s.Draft != "x^2" {
		t.Fatalf("typing x^2, the draft is %q", s.Draft)
	}
	s.press("=")
	if len(s.Plots) != 2 || s.Plots[1].Hue == s.Plots[0].Hue || s.Draft != "" {
		t.Fatalf("kept, the plots are %+v and the draft %q", s.Plots, s.Draft)
	}
	s.removePlot(s.Plots[0].ID)
	if len(s.Plots) != 1 || s.Plots[0].Expr != "x^2" {
		t.Fatalf("one taken off, the plots are %+v", s.Plots)
	}
}

func TestASumWithNoAnswerSaysSo(t *testing.T) {
	s := press("1", "÷", "0", "=")
	if s.Error == "" || s.Errors != 1 || len(s.Tape) != 0 {
		t.Fatalf("1÷0 left %+v", s.Calc)
	}
	s.press("⌫")
	if s.Error != "" || s.Expr != "1÷" {
		t.Fatalf("a key after clears the error: %+v", s.Calc)
	}
	s.press("sin")
	s.press("⌫")
	if s.Expr != "1÷" {
		t.Fatalf("⌫ after sin( leaves %q", s.Expr)
	}
}

func TestKeysOnTheCommandLineSplitAsTyped(t *testing.T) {
	if got := splitKeys("2sin(3)=√(4)"); !slices.Equal(got, []string{"2", "sin", "3", ")", "=", "√", "4", ")"}) {
		t.Fatalf("split into %q", got)
	}
}

// viewStage mounts the calculator's view offscreen with s.
func viewStage(t *testing.T, s Calc) (w *gunim.Window, publish func(Calc), run func(int)) {
	t.Helper()
	w = gunim.NewOffscreen(geom.Sz(980, 660), nil)
	registerViews(w)
	c := w.Client()
	if err := c.Mount(gunim.Root, "calc", "calc", s, calcTopic); err != nil {
		t.Fatal(err)
	}
	run = func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	publish = func(s Calc) {
		t.Helper()
		if err := c.Publish(calcTopic, s); err != nil {
			t.Fatal(err)
		}
		run(1)
	}
	run(2)
	return w, publish, run
}

// A click on a key sends it, and typing sends keys too.
func TestTheKeypadAndTheKeyboardSendKeys(t *testing.T) {
	w, _, run := viewStage(t, Calc{})
	if err := w.Client().Focus("calc"); err != nil {
		t.Fatal(err)
	}
	run(10)
	// "7" is the first key of the third row.
	at := geom.Pt(18+20, 40+18+160+16+2*((660-40-18-160-16-18-50)/6+10)+20)
	w.Input(input.PointerMove{Pos: at, Time: time.Now()})
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary, Time: time.Now()})
	w.Input(input.TextInput{Text: "+2"})
	w.Input(input.KeyPress{Key: input.KeyEnter})
	run(2)
	var got []string
	for len(w.Client().Intents()) > 0 {
		env := <-w.Client().Intents()
		if p, ok := env.Intent.(Pressed); ok {
			got = append(got, p.Key)
		}
	}
	if !slices.Equal(got, []string{"7", "+", "2", "="}) {
		t.Fatalf("the keys sent were %q", got)
	}
}

// The calculator and the graph each run their animations through, from
// state to state, and switching between them.
func TestTheViewsAnimateFromStateToState(t *testing.T) {
	_, publish, run := viewStage(t, Calc{})
	publish(Calc{Expr: "12×3", Preview: "36"})
	run(10)
	publish(Calc{Expr: "36", Tape: []Line{{ID: 1, Expr: "12×3", Result: "36"}}})
	run(60)
	publish(Calc{Expr: "1÷0", Error: "that is too big", Errors: 1, Tape: []Line{{ID: 1, Expr: "12×3", Result: "36"}}})
	run(30)
	graph := Calc{Graph: true, Plots: []Plot{{ID: 2, Expr: "sin(x)", Hue: 0}}}
	publish(graph)
	run(20)
	graph.Expr, graph.Draft = "x^2", "x^2"
	publish(graph)
	run(10)
	graph.Expr, graph.Draft = "x^2−1", "x^2−1"
	publish(graph)
	run(10)
	graph.Plots = append(graph.Plots, Plot{ID: 3, Expr: "x^2−1", Hue: 1})
	graph.Expr, graph.Draft = "", ""
	publish(graph)
	run(60)
	graph.Plots = graph.Plots[1:]
	graph.Graph = false
	publish(graph)
	run(90)
}

func TestTheGraphShakesBackAndSaysWhy(t *testing.T) {
	// A sum with no curve shakes the line being typed and says why, as
	// show sets them going; the graph's Step runs both through.
	g := newGraphBody(nil)
	g.why.set("× needs a number before it")
	g.shake.Jump(1)
	g.shake.Animate(0, anim.Spring{Response: 0.35, Damping: 0.2})
	for range 120 {
		g.Step(time.Second / 60)
	}
	if v := g.shake.Value(); math.Abs(float64(v)) > 0.01 {
		t.Fatalf("two seconds after a sum with no curve the line is shaken %v, want it back at rest", v)
	}
	if a := g.why.a.Value(); a < 0.99 {
		t.Fatalf("two seconds after a sum with no curve why shows at %v, want it in full", a)
	}
	g.why.set("")
	for range 120 {
		g.Step(time.Second / 60)
	}
	if a := g.why.a.Value(); a > 0.01 {
		t.Fatalf("two seconds after typing on why still shows at %v, want it gone", a)
	}
}
