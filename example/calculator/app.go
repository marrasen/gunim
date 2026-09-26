package main

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/marrasen/gunim"
)

// The vocabulary the two halves share.
type (
	// Calc is what the window shows.
	Calc struct {
		// Expr is what is being typed, and Preview what it works out
		// to so far, empty when it works out to nothing yet.
		Expr    string
		Preview string
		// Error says why the last sum had no answer, and Errors counts
		// them, so the window shakes once for each.
		Error  string
		Errors int
		// Tape is the sums worked out, newest first.
		Tape []Line
		// Graph says the graph is showing. Plots are the curves on it,
		// and Draft the one being typed, drawn live while it reads.
		Graph bool
		Plots []Plot
		Draft string
	}
	// Line is one sum worked out.
	Line struct {
		ID           int
		Expr, Result string
	}
	// Plot is one curve on the graph, and Hue its colour's place in the
	// window's list.
	Plot struct {
		ID   int
		Expr string
		Hue  int
	}

	// Pressed travels when a key is pressed, on the keypad or the
	// keyboard: a digit, a sign, a function, or = C ⌫.
	Pressed struct{ Key string }
	// ShowGraph switches between the keypad and the graph.
	ShowGraph struct{ On bool }
	// RemovePlot takes a curve off the graph.
	RemovePlot struct{ ID int }
	// Recall takes a sum on the tape's answer back to work on.
	Recall struct{ ID int }
)

// calcTopic is what the calculator view watches.
const calcTopic = "calc"

// serve is the application half: it keeps the calculator's state, and
// hears what the window sends.
func serve(ctx context.Context, c gunim.Client, start Calc, keys string) error {
	s := &calcState{Calc: start}
	if err := c.Mount(gunim.Root, "calc", "calc", s.Calc, calcTopic); err != nil {
		return err
	}
	_ = c.Focus("calc")
	// Keys given on the command line, as if typed.
	for _, k := range splitKeys(keys) {
		s.press(k)
	}
	if keys != "" {
		_ = c.Publish(calcTopic, s.Calc)
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-c.Intents():
			if !ok {
				return c.Err()
			}
			switch in := ev.Intent.(type) {
			case Pressed:
				s.press(in.Key)
			case ShowGraph:
				s.Graph = in.On
				s.draft()
			case RemovePlot:
				s.removePlot(in.ID)
			case Recall:
				for _, l := range s.Tape {
					if l.ID == in.ID {
						s.Expr, s.fresh = l.Result, true
						s.work()
					}
				}
			default:
				continue
			}
			_ = c.Publish(calcTopic, s.Calc)
		}
	}
}

// splitKeys splits keys typed on the command line, where a function
// is written with a bracket after it: "2sin(3)=" is 2, sin, 3, ), =.
func splitKeys(keys string) []string {
	var out []string
	for keys != "" {
		took := false
		for name := range functions {
			if strings.HasPrefix(keys, name+"(") {
				out = append(out, name)
				keys = keys[len(name)+1:]
				took = true
				break
			}
		}
		if took {
			continue
		}
		r, n := utf8.DecodeRuneInString(keys)
		out = append(out, string(r))
		keys = keys[n:]
	}
	return out
}

// calcState is the calculator, reached from serve alone.
type calcState struct {
	Calc
	// fresh says Expr is an answer: a digit typed next starts afresh,
	// and a sign carries on from it.
	fresh bool
	ids   int
	hues  int
}

// press carries out one key.
func (s *calcState) press(k string) {
	s.Error = ""
	switch k {
	case "C":
		s.Expr, s.fresh = "", false
	case "⌫":
		s.Expr = dropLast(s.Expr)
		s.fresh = false
	case "=":
		s.equals()
	default:
		if _, ok := functions[k]; ok {
			k += "("
		}
		if s.fresh && !strings.ContainsAny(k, "+−×÷^%") {
			s.Expr = ""
		}
		s.fresh = false
		s.Expr += k
	}
	s.work()
}

// equals works the sum out onto the tape, or puts a curve on the graph.
func (s *calcState) equals() {
	if strings.TrimSpace(s.Expr) == "" {
		return
	}
	if usesX(s.Expr) {
		if _, err := parse(s.Expr); err != nil {
			s.fail(err.Error())
			return
		}
		s.ids++
		s.Plots = append(s.Plots, Plot{ID: s.ids, Expr: s.Expr, Hue: s.hues})
		s.hues++
		s.Expr, s.Graph = "", true
		return
	}
	v, err := evaluate(s.Expr)
	if err != nil {
		s.fail(err.Error())
		return
	}
	s.ids++
	s.Tape = append([]Line{{ID: s.ids, Expr: s.Expr, Result: format(v)}}, s.Tape...)
	s.Expr, s.fresh = format(v), true
}

// fail says why a sum had no answer.
func (s *calcState) fail(why string) {
	s.Error = why
	s.Errors++
}

// work brings the preview and the draft curve up to date with Expr.
func (s *calcState) work() {
	s.Preview = ""
	if !usesX(s.Expr) && strings.ContainsAny(s.Expr, "+−×÷^%(") {
		if v, err := evaluate(s.Expr); err == nil {
			s.Preview = format(v)
		}
	}
	s.draft()
}

// draft draws what is typed on the graph, live, while it reads as a
// curve; a half-typed one leaves the last that read.
func (s *calcState) draft() {
	switch {
	case !s.Graph || strings.TrimSpace(s.Expr) == "":
		s.Draft = ""
	case usesX(s.Expr):
		if _, err := parse(s.Expr); err == nil {
			s.Draft = s.Expr
		}
	}
}

// removePlot takes a curve off the graph.
func (s *calcState) removePlot(id int) {
	for i, p := range s.Plots {
		if p.ID == id {
			s.Plots = append(s.Plots[:i:i], s.Plots[i+1:]...)
			return
		}
	}
}

// dropLast takes the last key's worth off an expression: a function's
// name goes with its bracket.
func dropLast(e string) string {
	for name := range functions {
		if strings.HasSuffix(e, name+"(") {
			return strings.TrimSuffix(e, name+"(")
		}
	}
	_, n := utf8.DecodeLastRuneInString(e)
	return e[:len(e)-n]
}
