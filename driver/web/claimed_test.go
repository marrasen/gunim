//go:build js && wasm

package web_test

import (
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
// Ctrl+Shift+C copies, and with text it takes typing, as it comes.
type shell struct {
	text  bool
	keys  []input.KeyPress
	typed string
	read  []string
}

func (s *shell) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size { return c.Max }
func (s *shell) Paint(*paint.Painter, gunim.Frame, geom.Size, gunim.Children)          {}
func (s *shell) Focusable() bool                                                       { return true }
func (s *shell) TakesText() bool                                                       { return s.text }
func (s *shell) ClaimsKeys() bool                                                      { return true }

func (s *shell) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.KeyPress:
		s.keys = append(s.keys, e)
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
