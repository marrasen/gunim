package widget

import (
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// fingerText is a text widget as a finger works it.
type fingerText struct {
	name string
	node textHost
	ed   *editor
}

func fingerTexts() []fingerText {
	lines := "one two three four five six\nseven eight nine ten eleven\n" + strings.Repeat("more of it here\n", 30)
	f, a, c := NewTextField(), NewTextArea(), NewCodeEditor()
	f.SetText("one two three four five six", nil)
	a.SetText(lines, nil)
	c.SetText(lines, nil)
	return []fingerText{
		{"a field", f, &f.editor},
		{"an area", a, &a.editor},
		{"code", c, &c.editor},
	}
}

// TestAFingersPressWaitsToShowWhatItIs: a tap acts as it lifts; a finger that scrolls, and one held on the selection,
// leave what is selected, and its menu copies it.
func TestAFingersPressWaitsToShowWhatItIs(t *testing.T) {
	for _, c := range fingerTexts() {
		t.Run(c.name, func(t *testing.T) {
			g := &grab{child: c.node}
			w, run := stage(t, &frame{child: g, size: geom.Sz(300, 80)})
			w.TimeByFrames(true)
			run(1)
			g.ui.Focus(c.node)
			c.ed.set(0, false)
			run(120)
			// where is a point of the first line just past rune i's left edge.
			where := func(i int) geom.Point {
				t.Helper()
				box, ok := g.ui.Bounds(c.node)
				r := c.node.caretRect(i)
				p := geom.Pt(r.Min.X+1, r.Center().Y)
				if got := c.node.hostIndex(p, g.ui); !ok || got != i {
					t.Fatalf("at rune %d is %d", i, got)
				}
				return box.Min.Add(p)
			}
			want := func(what string, start, end int) {
				t.Helper()
				if s, e := c.ed.Selection(); s != start || e != end {
					t.Fatalf("%s, %d–%d is selected, want %d–%d", what, s, e, start, end)
				}
			}
			down := func(p geom.Point, clicks int) {
				w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: clicks, Touch: true, Time: time.Now()})
			}
			in := where(5)
			down(in, 1)
			run(3)
			want("as a finger comes down", 0, 0)
			touchUp(w, in)
			want("as it lifts", 5, 5)
			down(in, 2)
			want("as it comes down again", 5, 5)
			touchUp(w, in)
			want("as it lifts from a double tap", 4, 7)
			run(2)

			// A scroll, from elsewhere in the text.
			from := where(10)
			down(from, 1)
			for i := range 8 {
				w.Input(input.PointerMove{Pos: from.Add(geom.Pt(-float32(i)*4, -float32(i)*4)), Touch: true, Time: time.Now()})
				run(1)
				want("as a finger scrolls", 4, 7)
			}
			touchUp(w, from.Add(geom.Pt(-28, -28)))
			want("once a finger has scrolled", 4, 7)
			run(120)
			w.Input(input.Scroll{Pos: from, Delta: geom.Pt(500, 500)})
			run(120)

			// Held on the selection.
			in = where(5)
			down(in, 1)
			for range 40 {
				run(1)
				want("as a finger is held on the selection", 4, 7)
			}
			if c.ed.menuItems != nil {
				t.Fatal("the menu opened before the finger lifted")
			}
			touchUp(w, in)
			run(2)
			want("as it lifts", 4, 7)
			if c.ed.menuItems == nil {
				t.Fatal("no edit menu opened")
			}
			c.ed.menuItems.OnPick(editCopy, g.ui)
			run(2)
			if s, _ := w.Offscreen().Clipboard(); s != "two" {
				t.Fatalf("Copy copied %q", s)
			}
			// Held there and dragged on, it takes more words.
			down(in, 1)
			run(40)
			w.Input(input.PointerMove{Pos: where(10), Touch: true, Time: time.Now()})
			want("dragged on to the next word", 4, 13)
			touchUp(w, where(10))
			run(2)
			c.ed.closeMenu()
			run(2)
			// Held elsewhere, the word there.
			in = where(15)
			down(in, 1)
			run(40)
			want("as a finger is held on another word", 14, 18)
			touchUp(w, in)
			run(2)
		})
	}
}

// TestTheCaretStandsWhereTheTextIsDrawn: in the theme's padding, as the touch theme's is wider, and on every frame of a
// switch to it. The platform's caret, a handle and a list of suggestions go by it.
func TestTheCaretStandsWhereTheTextIsDrawn(t *testing.T) {
	for _, c := range fingerTexts() {
		t.Run(c.name, func(t *testing.T) {
			themed := NewThemed(c.node, Dark())
			g := &grab{child: themed}
			w, run := stage(t, &frame{child: g, size: geom.Sz(300, 80)})
			w.TimeByFrames(true)
			run(1)
			g.ui.Focus(c.node)
			c.ed.set(0, false)
			check := func(when string) {
				t.Helper()
				box, _ := g.ui.Bounds(c.node)
				// Where the first line of the text is drawn: it is long, as a line's number is not.
				first := geom.Pt(1e6, 1e6)
				for _, op := range w.Offscreen().Ops() {
					if op, ok := op.(*paint.TextOp); ok && len(op.Glyphs) > 10 {
						if at := op.Transform.Apply(op.Glyphs[0].At); at.Y < first.Y {
							first = at
						}
					}
				}
				want := first.X - box.Min.X
				for what, r := range map[string]geom.Rect{"the caret": c.node.(gunim.CaretReporter).TextCaret(), "a handle's place": c.node.caretRect(0)} {
					if d := r.Min.X - want; d > 1 || d < -1 {
						t.Fatalf("%s: %s is %v in, and the text %v", when, what, r.Min.X, want)
					}
				}
			}
			run(60)
			check("under the dark theme")
			themed.Use(Dark().With(Touch()...))
			g.ui.Invalidate()
			for range 120 {
				run(1)
				check("switching to the touch theme")
			}
		})
	}
	a := NewTextArea()
	a.Triggers = "@"
	_, g, run := touchStage(t, a, geom.Sz(300, 80))
	a.SetText("@a", g.ui)
	run(60)
	if got := a.triggerBox(0).Min; got != geom.Pt(16, 16) {
		t.Errorf("under the touch theme suggestions hang from %v, and the padding is 16", got)
	}
}
