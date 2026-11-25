package gunim

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// phoneWindow is an offscreen window that keeps, in order, all the engine says of text
// being taken, as a phone's driver hears it.
type phoneWindow struct {
	*driver.OffscreenWindow
	said []string
}

func (w *phoneWindow) SetTextInput(on bool) { w.said = append(w.said, fmt.Sprint("text input ", on)) }
func (w *phoneWindow) ShowKeyboard()        { w.said = append(w.said, "keyboard") }
func (w *phoneWindow) SetTextCaret(geom.Rect) {
	w.said = append(w.said, "caret")
}

func (w *phoneWindow) SetTextState(s *input.TextState, _ uint64) {
	switch {
	case s == nil:
		w.said = append(w.said, "no text")
	case s.Secret:
		w.said = append(w.said, "secret text")
	default:
		w.said = append(w.said, "text")
	}
}

// take returns what the engine has said since the last take.
func (w *phoneWindow) take() []string {
	s := w.said
	w.said = nil
	return s
}

// typeTo is a node that takes text while on says so, with a caret, and no text of its
// own, as a terminal has. Space turns it over, and a press, or a release, as byPress and
// byRelease say.
type typeTo struct {
	recorder
	on, byPress, byRelease bool
}

func (s *typeTo) TakesText() bool                               { return s.on }
func (s *typeTo) Focusable() bool                               { return true }
func (s *typeTo) TextCaret() geom.Rect                          { return geom.Rc(3, 4, 1, 16) }
func (s *typeTo) Layout(Constraints, Frame, Children) geom.Size { return geom.Sz(200, 100) }

func (s *typeTo) Handle(e input.Event, u *UI) bool {
	switch e := e.(type) {
	case input.KeyPress:
		if e.Key == input.KeySpace {
			s.on = !s.on
		}
	case input.PointerDown:
		s.on = s.on != s.byPress
	case input.PointerUp:
		s.on = s.on != s.byRelease
	}
	s.recorder.Handle(e, u)
	return true
}

// TestTheDriverHearsAFocusedNodeStartAndStopTakingText: without the focus moving.
func TestTheDriverHearsAFocusedNodeStartAndStopTakingText(t *testing.T) {
	pw := &phoneWindow{OffscreenWindow: driver.Offscreen(geom.Sz(800, 600))}
	w := newWindow(pw, nil)
	term, plain := &typeTo{}, &recorder{}
	w.ui.Insert(w.ui.Root(), term)
	w.ui.Insert(w.ui.Root(), plain)
	frame := func() { w.Frame(time.Second / 60) }
	check := func(what string, want ...string) {
		t.Helper()
		if got := pw.take(); !slices.Equal(got, want) {
			t.Fatalf("%s, the driver heard %q, want %q", what, got, want)
		}
	}
	frame()
	w.ui.Focus(term)
	frame()
	check("as one that takes no text took the focus", "text input false")
	for range 2 {
		// The program has it take text. The caret the driver has yet to hear comes too.
		term.on = true
		w.ui.Invalidate()
		frame()
		check("as the program had it take text", "text input true", "caret")
		frame()
		check("a frame with nothing changed")
		term.on = false
		w.ui.Invalidate()
		frame()
		check("as the program had it stop", "text input false")
	}
	// A key changes its mind, with no frame between.
	w.Input(input.KeyPress{Key: input.KeySpace})
	check("as a key had it take text", "text input true")
	w.Input(input.KeyPress{Key: input.KeyEnter})
	check("a key that changed nothing")
	w.Input(input.KeyPress{Key: input.KeySpace})
	check("as a key had it stop", "text input false")
	// One that has stopped, unheard, ends as the focus goes, and one that has started
	// has nothing to end.
	term.on = true
	w.ui.Focus(plain)
	check("as the focus left one that had started, unheard", "text input false")
	w.ui.Focus(term)
	check("as one that takes text took the focus", "text input true")
	term.on = false
	w.ui.Focus(plain)
	check("as the focus left one that had stopped, unheard", "text input false")
	// Taken out of the tree while it has the focus.
	term.on = true
	w.ui.Focus(term)
	w.ui.Remove(term)
	frame()
	if got := pw.take(); len(got) == 0 || got[len(got)-1] != "text input false" {
		t.Fatalf("as it left the tree, the driver heard %q, want text input off last", got)
	}
}

// TestATapThatHasANodeTakeTextBringsUpTheKeyboard: the keyboard comes on that tap, and
// for a node the program has take text, on the next one.
func TestATapThatHasANodeTakeTextBringsUpTheKeyboard(t *testing.T) {
	tap := func(w *Window, p geom.Point) {
		w.Input(input.PointerDown{Pos: p, Button: input.ButtonPrimary, Clicks: 1, Touch: true, Time: time.Now()})
		w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary, Touch: true, Time: time.Now()})
	}
	for _, c := range []struct {
		name string
		term *typeTo
		want []string
	}{
		{"by the press", &typeTo{byPress: true}, []string{"text input true", "keyboard"}},
		{"by the release", &typeTo{byRelease: true}, []string{"text input true", "keyboard"}},
		{"by the program", &typeTo{}, []string{"text input true", "keyboard"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			pw := &phoneWindow{OffscreenWindow: driver.Offscreen(geom.Sz(800, 600))}
			w := newWindow(pw, nil)
			w.ui.Insert(w.ui.Root(), c.term)
			w.ui.Focus(c.term)
			w.Frame(time.Second / 60)
			pw.take()
			if !c.term.byPress && !c.term.byRelease {
				c.term.on = true
				w.ui.Invalidate()
				w.Frame(time.Second / 60)
			}
			tap(w, geom.Pt(20, 20))
			got := slices.DeleteFunc(pw.take(), func(s string) bool { return s == "caret" })
			if !slices.Equal(got, c.want) {
				t.Errorf("the driver heard %q, want %q", got, c.want)
			}
		})
	}
}

// TestANodeThatStopsTakingTextHearsItsCompositionEnd: as it would as the focus left.
func TestANodeThatStopsTakingTextHearsItsCompositionEnd(t *testing.T) {
	w := NewOffscreen(geom.Sz(800, 600), nil)
	term := &typeTo{on: true}
	w.ui.Insert(w.ui.Root(), term)
	w.ui.Focus(term)
	w.Frame(time.Second / 60)
	w.Input(input.Composing{Text: "に"})
	term.on = false
	w.ui.Invalidate()
	w.Frame(time.Second / 60)
	var last input.Event
	for _, e := range term.events {
		if c, ok := e.(input.Composing); ok {
			last = c
		}
	}
	if last != (input.Composing{}) {
		t.Errorf("the last it heard of the composition was %#v, want its end", last)
	}
}

// secretText is a node whose text is a password while secret says so.
type secretText struct {
	editsText
	secret bool
}

func (n *secretText) TextState() input.TextState {
	s := n.editsText.TextState()
	s.Secret = n.secret
	return s
}

// TestAFocusedNodesTextTurnsSecret: the driver hears it, and text input stays on, so a
// phone's keyboard stays up and changes.
func TestAFocusedNodesTextTurnsSecret(t *testing.T) {
	pw := &phoneWindow{OffscreenWindow: driver.Offscreen(geom.Sz(800, 600))}
	w := newWindow(pw, nil)
	n := &secretText{}
	w.ui.Insert(w.ui.Root(), n)
	w.ui.Focus(n)
	w.Frame(time.Second / 60)
	if got, want := pw.take(), []string{"text", "text input true"}; !slices.Equal(got, want) {
		t.Fatalf("as it took the focus, the driver heard %q, want %q", got, want)
	}
	for _, secret := range []bool{true, false} {
		n.secret = secret
		w.ui.Invalidate()
		w.Frame(time.Second / 60)
		want := []string{map[bool]string{true: "secret text", false: "text"}[secret]}
		if got := pw.take(); !slices.Equal(got, want) {
			t.Errorf("as it turned secret %v, the driver heard %q, want %q", secret, got, want)
		}
	}
}
