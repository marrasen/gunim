//go:build js && wasm

package web_test

import (
	"fmt"
	"slices"
	"syscall/js"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver/web"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// shell is a node that claims the keys as a terminal does: Ctrl+Shift+V pastes and
// Ctrl+Shift+C copies, and with text it takes typing, as it comes. heard is what an
// input method and the keys brought it, in order.
type shell struct {
	text  bool
	keys  []input.KeyPress
	typed string
	read  []string
	heard []string
}

var keyNames = map[input.Key]string{input.KeyBackspace: "Backspace", input.KeyDelete: "Delete", input.KeyEnter: "Enter"}

func (s *shell) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size { return c.Max }
func (s *shell) Paint(*paint.Painter, gunim.Frame, geom.Size, gunim.Children)          {}
func (s *shell) Focusable() bool                                                       { return true }
func (s *shell) TakesText() bool                                                       { return s.text }
func (s *shell) ClaimsKeys() bool                                                      { return true }

func (s *shell) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.KeyPress:
		s.keys = append(s.keys, e)
		name := keyNames[e.Key]
		if e.Char != 0 {
			name = string(e.Char)
		}
		s.heard = append(s.heard, "key "+name)
		switch {
		case e.Mods == input.ModControl|input.ModShift && e.Char == 'v':
			text, err := u.ReadClipboard()
			if err != nil {
				text = err.Error()
			}
			s.read = append(s.read, text)
		case e.Mods == input.ModControl|input.ModShift && e.Char == 'c':
			u.SetClipboard("copied")
		}
		return true
	case input.TextInput:
		s.typed += e.Text
		s.heard = append(s.heard, "text "+e.Text)
		return true
	case input.Composing:
		h := "composing " + e.Text
		if e.Selected != [2]int{len(e.Text), len(e.Text)} {
			h += fmt.Sprint(" ", e.Selected)
		}
		s.heard = append(s.heard, h)
		return true
	case input.PointerDown:
		return true
	}
	return false
}

// down dispatches a key to what has the focus, and reports whether it was cancelled.
func down(name, code string, mods ...string) (cancelled bool) {
	init := map[string]any{"key": name, "code": code}
	for _, m := range mods {
		init[m] = true
	}
	e := eventOf("KeyboardEvent", "keydown", init)
	activeElement().Call("dispatchEvent", e)
	return e.Get("defaultPrevented").Bool()
}

// TestATerminalInTheBrowser: through a real engine, with nothing between the steps, as
// the browser's own events come.
func TestATerminalInTheBrowser(t *testing.T) {
	if web.OnApple() {
		t.Skip("Command is the key that pastes here")
	}
	for _, text := range []bool{false, true} {
		sh := &shell{text: text}
		d := show(t, sh)
		// Before the click nothing is claimed. From it on, the very next key is.
		for give := time.Now().Add(20 * time.Second); ; time.Sleep(20 * time.Millisecond) {
			click(d, 40, 20)
			if down("r", "KeyR", "ctrlKey") {
				break
			}
			if time.Now().After(give) {
				t.Fatalf("text %v: Ctrl+R stays the browser's", text)
			}
		}
		if len(sh.keys) == 0 || sh.keys[len(sh.keys)-1].Key != input.KeyR {
			t.Fatalf("text %v: Ctrl+R was cancelled, and the terminal heard %+v", text, sh.keys)
		}
		// Copying asks the browser while it handles the key, which it must.
		var wrote []string
		write := js.FuncOf(func(_ js.Value, a []js.Value) any {
			wrote = append(wrote, a[0].String())
			return js.Global().Get("Promise").Call("resolve")
		})
		asked := js.FuncOf(func(js.Value, []js.Value) any {
			t.Errorf("text %v: the clipboard was read, which asks the user", text)
			return js.Global().Get("Promise").Call("resolve", "")
		})
		nav := js.Global().Get("navigator")
		js.Global().Get("Object").Call("defineProperty", nav, "clipboard", map[string]any{
			"configurable": true, "value": map[string]any{"writeText": write, "readText": asked},
		})
		down("C", "KeyC", "ctrlKey", "shiftKey")
		if len(wrote) != 1 || wrote[0] != "copied" {
			t.Errorf("text %v: as the key was handled, the browser was asked to copy %q", text, wrote)
		}
		time.Sleep(20 * time.Millisecond)
		// The paste comes after the key, as the browser's default action, while the
		// terminal's handler waits for it.
		for _, want := range []string{"one\ntwo", "three"} {
			if down("V", "KeyV", "ctrlKey", "shiftKey") {
				t.Fatalf("text %v: the browser pastes nothing on a key cancelled", text)
			}
			data := js.Global().Get("DataTransfer").New()
			data.Call("setData", "text/plain", want)
			e := eventOf("ClipboardEvent", "paste", map[string]any{"clipboardData": data})
			activeElement().Call("dispatchEvent", e)
			if !e.Get("defaultPrevented").Bool() {
				t.Errorf("text %v: the browser pasted it too", text)
			}
			time.Sleep(20 * time.Millisecond)
			if n := len(sh.read); n == 0 || sh.read[n-1] != want {
				t.Errorf("text %v: the terminal read %q, want %q", text, sh.read, want)
			}
			activeElement().Call("dispatchEvent", eventOf("KeyboardEvent", "keyup", map[string]any{"key": "V", "code": "KeyV", "ctrlKey": true, "shiftKey": true}))
		}
		// Typing goes on.
		if !down("a", "KeyA") && text {
			k := &typist{t: t, d: d}
			k.insert("a")
		}
		if sh.typed != "a" {
			t.Errorf("text %v: the terminal was typed %q", text, sh.typed)
		}
		js.Global().Get("Reflect").Call("deleteProperty", nav, "clipboard")
		write.Release()
		asked.Release()
	}
}

// shellAt starts a shell that takes text, with the keyboard, and returns a function that
// checks what it has heard since, and that the mirror holds nothing then.
func shellAt(t *testing.T) (k *typist, heard func(step string, want ...string)) {
	t.Helper()
	sh := &shell{text: true}
	k = &typist{t: t, d: show(t, sh)}
	for give := time.Now().Add(20 * time.Second); !k.d.Mirror().Equal(activeElement()); time.Sleep(20 * time.Millisecond) {
		click(k.d, 40, 20)
		if time.Now().After(give) {
			t.Fatal("the shell never took the keyboard")
		}
	}
	// The claim comes with the focus, at the latest by the next frame.
	for give := time.Now().Add(20 * time.Second); !down("r", "KeyR", "ctrlKey"); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(give) {
			t.Fatal("Ctrl+R stays the browser's")
		}
	}
	sh.heard = nil
	return k, func(step string, want ...string) {
		t.Helper()
		if !slices.Equal(sh.heard, want) {
			t.Fatalf("%s: the shell heard %q, want %q", step, sh.heard, want)
		}
		sh.heard = nil
		if v := k.d.Mirror().Get("value").String(); len(want) > 0 && want[len(want)-1][0] != 'c' && v != "" {
			t.Fatalf("%s: the mirror holds %q", step, v)
		}
	}
}

// TestATerminalHearsWhatIsComposed: it keeps no text for the mirror to hold, and shows
// what an input method composes by [input.Composing], as on a desktop.
func TestATerminalHearsWhatIsComposed(t *testing.T) {
	k, heard := shellAt(t)
	m := k.d.Mirror()
	for _, round := range []string{"first", "again"} {
		c := compose(m)
		heard(round + ", as it starts")
		c.step("k")
		heard(round+", composing", "composing k")
		c.step("日本")
		m.Call("setSelectionRange", 0, 1)
		m.Call("dispatchEvent", eventOf("InputEvent", "input", map[string]any{"inputType": "insertCompositionText", "data": "日本", "isComposing": true}))
		heard(round+", converting", "composing 日本", "composing 日本 [0 3]")
		// The keys are the input method's while it composes.
		for _, e := range []map[string]any{
			{"key": "Process", "code": "Enter"},
			{"key": "Enter", "code": "Enter", "isComposing": true},
			{"key": "c", "code": "KeyC", "ctrlKey": true, "isComposing": true},
		} {
			key := eventOf("KeyboardEvent", "keydown", e)
			m.Call("dispatchEvent", key)
			if key.Get("defaultPrevented").Bool() {
				t.Errorf("%s: the input method's key %v is cancelled", round, e)
			}
		}
		heard(round + ", keys of the input method's")
		c.end("日本")
		heard(round+", composed", "composing ", "text 日本")
	}
	// Firefox says what was composed once more, after the composition.
	m.Call("dispatchEvent", eventOf("InputEvent", "input", map[string]any{"inputType": "insertCompositionText", "data": "日本"}))
	heard("an input event after it")
	// Safari takes the composition out and puts the text in, before its end.
	c := compose(m)
	c.step("ka")
	c.step("")
	c.step("か")
	c.end("か")
	heard("composed as Safari does", "composing ka", "composing ", "composing か", "composing ", "text か")
	// What is dropped leaves nothing, whether or not it is emptied first.
	c = compose(m)
	c.step("ni")
	c.step("")
	c.end("")
	heard("dropped", "composing ni", "composing ")
	c = compose(m)
	c.step("ni")
	m.Set("value", "")
	c.end("")
	heard("dropped at once", "composing ni", "composing ")
	// Two with nothing between, as Korean's syllables come.
	c = compose(m)
	c.step("한")
	c.end("한")
	c = compose(m)
	c.step("그")
	c.end("그")
	heard("two in a row", "composing 한", "composing ", "text 한", "composing 그", "composing ", "text 그")
	// The program still has Ctrl+C, and Ctrl+V, which the browser pastes on.
	if !down("c", "KeyC", "ctrlKey") {
		t.Error("Ctrl+C is the browser's")
	}
	heard("Ctrl+C", "key c")
	if !web.OnApple() {
		if down("v", "KeyV", "ctrlKey") {
			t.Error("Ctrl+V is cancelled, so the browser pastes nothing")
		}
		heard("Ctrl+V", "key v")
	}
}

// TestATerminalOnASoftKeyboard: one composes every word, and sends keys with no name.
func TestATerminalOnASoftKeyboard(t *testing.T) {
	k, heard := shellAt(t)
	m := k.d.Mirror()
	unnamed := func() {
		m.Call("dispatchEvent", eventOf("KeyboardEvent", "keydown", map[string]any{"key": "Unidentified", "code": ""}))
	}
	word := func(steps ...string) *composer {
		c := compose(m)
		for _, s := range steps {
			unnamed()
			c.step(s)
		}
		return c
	}
	c := word("l", "ls", "l", "ls")
	heard("a word", "composing l", "composing ls", "composing l", "composing ls")
	c.end("ls")
	k.insert(" ")
	heard("a space after it", "composing ", "text ls", "text  ")
	c = word("a", "")
	c.end("")
	heard("a word rubbed out", "composing a", "composing ")
	k.key("Backspace", "")
	heard("backspace on an empty line", "key Backspace")
	for range 2 {
		unnamed()
		for kind, want := range map[string]string{"deleteContentBackward": "key Backspace", "deleteContentForward": "key Delete"} {
			e := eventOf("InputEvent", "beforeinput", map[string]any{"inputType": kind})
			m.Call("dispatchEvent", e)
			if !e.Get("defaultPrevented").Bool() {
				t.Errorf("%s is the browser's", kind)
			}
			heard(kind, want)
		}
	}
	// A delete in a composition is the composition's.
	c = word("ab")
	m.Call("dispatchEvent", eventOf("InputEvent", "beforeinput", map[string]any{"inputType": "deleteContentBackward"}))
	c.step("a")
	c.end("a")
	heard("a delete in a composition", "composing ab", "composing a", "composing ", "text a")
	k.key("Enter", "")
	heard("enter", "key Enter")
}

// TestATerminalLosingTheKeyboardMidCompositionKeepsNothingOfIt: the next composition
// starts from nothing.
func TestATerminalLosingTheKeyboardMidCompositionKeepsNothingOfIt(t *testing.T) {
	k, heard := shellAt(t)
	m := k.d.Mirror()
	c := compose(m)
	c.step("ni")
	// The browser ends it, with what there is, as the focus leaves.
	m.Call("blur")
	c.end("ni")
	heard("left", "composing ni", "composing ", "text ni")
	for give := time.Now().Add(20 * time.Second); !m.Equal(activeElement()); time.Sleep(20 * time.Millisecond) {
		click(k.d, 40, 20)
		if time.Now().After(give) {
			t.Fatal("the shell never took the keyboard back")
		}
	}
	c = compose(m)
	c.step("ka")
	c.end("か")
	heard("back", "composing ka", "composing ", "text ka")
}
