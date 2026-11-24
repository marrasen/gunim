//go:build js && wasm

package web

import (
	"slices"
	"syscall/js"
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/input"
)

// claimWindow opens a window that claims the keys, with the canvas focused. With text
// it takes typing as it comes, as a terminal does, and the mirror is focused.
func claimWindow(t *testing.T, text bool) (*Driver, *Window) {
	t.Helper()
	d, w := testWindow(t)
	d.mu.Lock()
	d.lastTouch = false
	d.mu.Unlock()
	d.canvas.Call("focus")
	if text {
		w.SetTextState(nil, w.editSeq)
		w.SetTextInput(true)
		if !d.plain.Equal(active()) {
			t.Fatalf("the mirror is not focused, %v is", active().Get("tagName"))
		}
	}
	w.SetKeysClaimed(true)
	drain(w)
	return d, w
}

// active is the element with the page's focus.
func active() js.Value { return js.Global().Get("document").Get("activeElement") }

// asApple has the tests run as on a Mac, or as elsewhere.
func asApple(t *testing.T, on bool) {
	t.Helper()
	apple, option := onApple, optionTypes
	onApple, optionTypes = on, on
	t.Cleanup(func() { onApple, optionTypes = apple, option })
}

// send dispatches a key to what has the focus, and reports whether it was cancelled,
// which keeps it from the browser.
func send(kind, name, code string, m input.Mods) (cancelled bool) {
	e := js.Global().Get("KeyboardEvent").New(kind, map[string]any{
		"bubbles": true, "cancelable": true, "key": name, "code": code,
		"ctrlKey": m.Has(input.ModControl), "shiftKey": m.Has(input.ModShift),
		"altKey": m.Has(input.ModAlt), "metaKey": m.Has(input.ModSuper),
	})
	active().Call("dispatchEvent", e)
	return e.Get("defaultPrevented").Bool()
}

// presses returns the key presses among events.
func presses(events []any) []input.KeyPress {
	var out []input.KeyPress
	for _, e := range events {
		if k, ok := e.(input.KeyPress); ok {
			out = append(out, k)
		}
	}
	return out
}

// pasteEvent dispatches the browser's paste of text to what has the focus, and reports
// whether it was cancelled, which keeps the browser from putting the text in.
func pasteEvent(text string) (cancelled bool) {
	data := js.Global().Get("DataTransfer").New()
	data.Call("setData", "text/plain", text)
	e := js.Global().Get("ClipboardEvent").New("paste", map[string]any{"bubbles": true, "cancelable": true, "clipboardData": data})
	active().Call("dispatchEvent", e)
	return e.Get("defaultPrevented").Bool()
}

const (
	ctrl, shift, alt, super = input.ModControl, input.ModShift, input.ModAlt, input.ModSuper
)

// browserKeys are keys the browser keeps from a program that claims none.
var browserKeys = []struct {
	name, code string
	m          input.Mods
}{
	{"r", "KeyR", ctrl},
	{"R", "KeyR", ctrl | shift},
	{"l", "KeyL", ctrl},
	{"w", "KeyW", ctrl},
	{"t", "KeyT", ctrl},
	{"n", "KeyN", ctrl},
	{"C", "KeyC", ctrl | shift},
	{"I", "KeyI", ctrl | shift},
	{"J", "KeyJ", ctrl | shift},
	{"A", "KeyA", ctrl | shift},
	{"5", "Digit5", ctrl},
	{"&", "Digit1", ctrl},
	{"к", "KeyR", ctrl},
	{"Tab", "Tab", ctrl},
	{"Tab", "Tab", ctrl | shift},
	{"PageUp", "PageUp", ctrl},
	{"PageDown", "PageDown", ctrl},
	{"F5", "F5", 0},
	{"F5", "F5", ctrl},
	{"F6", "F6", 0},
	{"F12", "F12", 0},
	{"F11", "F11", shift},
	{"ArrowLeft", "ArrowLeft", alt},
	{"ArrowRight", "ArrowRight", alt},
	{"Home", "Home", alt},
	{"d", "KeyD", alt},
}

func TestAClaimTakesTheBrowsersKeys(t *testing.T) {
	asApple(t, false)
	for _, text := range []bool{false, true} {
		d, w := claimWindow(t, text)
		for _, k := range browserKeys {
			if !browserKey(k.name, k.code, k.m) {
				t.Errorf("%q with %v is no key of the browser's", k.name, k.m)
			}
			if !send("keydown", k.name, k.code, k.m) {
				t.Errorf("text %v: %q with %v was left to the browser too", text, k.name, k.m)
			}
			got := presses(drain(w))
			if len(got) != 1 || got[0].Mods != k.m || got[0].Typed {
				t.Errorf("text %v: %q with %v reached the program as %+v", text, k.name, k.m, got)
			}
			send("keyup", k.name, k.code, k.m)
			drain(w)
		}
		// Let go, they are the browser's again, from the next key.
		w.SetKeysClaimed(false)
		for _, k := range browserKeys {
			if send("keydown", k.name, k.code, k.m) || len(presses(drain(w))) != 0 {
				t.Errorf("text %v: with no claim, %q with %v did not stay the browser's", text, k.name, k.m)
			}
		}
		if text {
			w.SetTextInput(false)
		}
		_ = w.Close()
		d.frame()
	}
}

func TestWhatAClaimLeavesTheBrowser(t *testing.T) {
	asApple(t, false)
	_, w := claimWindow(t, false)
	for _, k := range []struct {
		name, code string
		m          input.Mods
	}{
		// Zooming.
		{"+", "Equal", ctrl | shift},
		{"=", "Equal", ctrl},
		{"-", "Minus", ctrl},
		{"_", "Minus", ctrl | shift},
		{"0", "Digit0", ctrl},
		{"à", "Digit0", ctrl},
		{"0", "Numpad0", ctrl},
		{"+", "NumpadAdd", ctrl},
		// Full screen, and closing the window.
		{"F11", "F11", 0},
		{"F4", "F4", alt},
		// Command's, which is never a terminal's.
		{"r", "KeyR", super},
		{"w", "KeyW", super},
		{"t", "KeyT", super},
		{"l", "KeyL", super},
		{"f", "KeyF", super | ctrl},
		{"h", "KeyH", super},
		{"i", "KeyI", super | alt},
	} {
		if send("keydown", k.name, k.code, k.m) {
			t.Errorf("%q with %v was kept from the browser", k.name, k.m)
		}
		if got := presses(drain(w)); len(got) != 0 {
			t.Errorf("%q with %v reached the program as %+v", k.name, k.m, got)
		}
	}
	// Filling the screen, the program has F11 as well.
	if keptKey("F11", "F11", 0, true) {
		t.Error("F11 stayed the browser's in the program's full screen")
	}
	if !keptKey("f", "KeyF", super|ctrl, true) {
		t.Error("a Mac's full screen key is the program's")
	}
}

// TestTabWithAClaim: a node that takes Tab hears it as it does without a claim.
func TestTabWithAClaim(t *testing.T) {
	asApple(t, false)
	_, w := claimWindow(t, false)
	for _, m := range []input.Mods{0, shift} {
		for _, claimed := range []bool{false, true} {
			w.SetKeysClaimed(claimed)
			if !send("keydown", "Tab", "Tab", m) {
				t.Errorf("claimed %v: Tab with %v was left to the browser", claimed, m)
			}
			if got := presses(drain(w)); len(got) != 1 || got[0].Key != input.KeyTab {
				t.Errorf("claimed %v: Tab with %v reached the program as %+v", claimed, m, got)
			}
		}
	}
}

// TestAltWithAClaim: Alt and a letter is a shortcut, as a shell's Alt+B, and types
// nothing, but for where Option types.
func TestAltWithAClaim(t *testing.T) {
	for _, apple := range []bool{false, true} {
		asApple(t, apple)
		for _, text := range []bool{false, true} {
			d, w := claimWindow(t, text)
			cancelled := send("keydown", "b", "KeyB", alt)
			events := drain(w)
			got := presses(events)
			if len(got) != 1 || got[0].Mods != alt || got[0].Typed != apple {
				t.Fatalf("apple %v, text %v: Alt+B reached the program as %+v", apple, text, got)
			}
			// In the mirror on a Mac the browser types it.
			if want := !apple || !text; cancelled != want {
				t.Errorf("apple %v, text %v: Alt+B cancelled %v, want %v", apple, text, cancelled, want)
			}
			typed := slices.ContainsFunc(events, func(e any) bool { _, ok := e.(input.TextInput); return ok })
			if want := apple && !text; typed != want {
				t.Errorf("apple %v, text %v: Alt+B typed %v, want %v", apple, text, typed, want)
			}
			// Alt alone moves the keyboard to the browser's menus.
			if !send("keydown", "Alt", "AltLeft", alt) || !send("keyup", "Alt", "AltLeft", 0) {
				t.Errorf("apple %v, text %v: Alt alone was left to the browser", apple, text)
			}
			if text {
				w.SetTextInput(false)
			}
			_ = w.Close()
			d.frame()
		}
	}
}

// TestTypingWithAClaim: what types goes on typing through the browser.
func TestTypingWithAClaim(t *testing.T) {
	asApple(t, false)
	d, w := claimWindow(t, true)
	defer w.SetTextInput(false)
	for _, k := range []struct {
		name, code string
		m          input.Mods
	}{{"a", "KeyA", 0}, {"A", "KeyA", shift}, {" ", "Space", 0}, {"Dead", "Equal", 0}, {"Process", "KeyA", 0}, {"Unidentified", "", 0}} {
		if send("keydown", k.name, k.code, k.m) {
			t.Errorf("%q was cancelled, which types it", k.name)
		}
		drain(w)
	}
	// AltGr's, from a browser that says and one that doesn't.
	for _, init := range []map[string]any{
		{"key": "@", "code": "Digit2", "modifierAltGraph": true},
		{"key": "@", "code": "Digit2", "ctrlKey": true, "altKey": true},
		{"key": "0", "code": "Digit2", "modifierAltGraph": true, "ctrlKey": true, "altKey": true},
	} {
		init["bubbles"], init["cancelable"] = true, true
		e := js.Global().Get("KeyboardEvent").New("keydown", init)
		d.text.Call("dispatchEvent", e)
		if e.Get("defaultPrevented").Bool() {
			t.Errorf("AltGr's %v was cancelled", init["key"])
		}
		if got := presses(drain(w)); len(got) != 1 || !got[0].Typed || got[0].Mods != 0 {
			t.Errorf("AltGr's %v reached the program as %+v", init["key"], got)
		}
	}
	// A word composed.
	for _, kind := range []string{"compositionstart", "compositionupdate"} {
		d.text.Call("dispatchEvent", js.Global().Get("CompositionEvent").New(kind, map[string]any{"bubbles": true}))
	}
	e := js.Global().Get("KeyboardEvent").New("keydown", map[string]any{"bubbles": true, "cancelable": true, "key": "r", "code": "KeyR", "ctrlKey": true, "isComposing": true})
	d.text.Call("dispatchEvent", e)
	if e.Get("defaultPrevented").Bool() || len(presses(drain(w))) != 0 {
		t.Error("a key of an input method's was taken from it")
	}
	d.text.Set("value", "日本")
	d.text.Call("dispatchEvent", js.Global().Get("CompositionEvent").New("compositionend", map[string]any{"bubbles": true, "data": "日本"}))
	var typed []string
	for _, e := range drain(w) {
		if in, ok := e.(input.TextInput); ok {
			typed = append(typed, in.Text)
		}
	}
	if !slices.Equal(typed, []string{"日本"}) {
		t.Errorf("the word composed came as %q", typed)
	}
}

// TestAPopupPassesTheClaimOn: a tooltip open over a terminal has the topmost window,
// and no keyboard of its own.
func TestAPopupPassesTheClaimOn(t *testing.T) {
	asApple(t, false)
	d, w := claimWindow(t, false)
	p := testPopup(t, d, w)
	defer func() { _ = p.Close(); d.frame() }()
	drain(p)
	if !send("keydown", "r", "KeyR", ctrl) || len(presses(drain(p))) != 1 {
		t.Error("with a popup open, Ctrl+R was the browser's")
	}
	// A window of its own over it doesn't.
	_, over := testWindow(t)
	if d.claimed(over) {
		t.Error("a window over one that claims the keys claims them too")
	}
	if send("keydown", "r", "KeyR", ctrl) {
		t.Error("a window that claims none has Ctrl+R")
	}
}

// refuseReading fails the test if the driver asks the browser for the clipboard, which
// may ask the user.
func refuseReading(t *testing.T) {
	t.Helper()
	f := js.FuncOf(func(js.Value, []js.Value) any {
		t.Error("the driver read the clipboard, which asks the user")
		return js.Global().Get("Promise").Call("resolve", "read")
	})
	wrote := js.FuncOf(func(js.Value, []js.Value) any { return js.Global().Get("Promise").Call("resolve") })
	restore := define(js.Global().Get("navigator"), "clipboard", js.ValueOf(map[string]any{"readText": f, "writeText": wrote}))
	t.Cleanup(func() {
		restore()
		f.Release()
		wrote.Release()
	})
}

// pasteKeys are the keys a browser pastes on.
var pasteKeys = []struct {
	name, code string
	m          input.Mods
}{{"v", "KeyV", ctrl}, {"V", "KeyV", ctrl | shift}, {"Insert", "Insert", shift}}

func TestAPasteKeyOfTheProgramsBringsTheText(t *testing.T) {
	asApple(t, false)
	refuseReading(t)
	for _, text := range []bool{false, true} {
		d, w := claimWindow(t, text)
		had := active()
		for _, k := range pasteKeys {
			// The program hears the key, and reads the clipboard on it. The browser
			// pastes after.
			if send("keydown", k.name, k.code, k.m) {
				t.Fatalf("text %v: %q with %v was cancelled, so the browser pastes nothing", text, k.name, k.m)
			}
			if got := presses(drain(w)); len(got) != 1 || got[0].Mods != k.m {
				t.Fatalf("text %v: %q with %v reached the program as %+v", text, k.name, k.m, got)
			}
			if active().Equal(d.canvas) {
				t.Errorf("text %v: Firefox pastes nothing on %q with %v", text, k.name, k.m)
			}
			got := make(chan string, 1)
			go func() {
				s, err := w.Clipboard()
				if err != nil {
					t.Error(err)
				}
				got <- s
			}()
			time.Sleep(10 * time.Millisecond)
			if !pasteEvent("a\nb") {
				t.Errorf("text %v: the browser pastes %q with %v too", text, k.name, k.m)
			}
			if s := <-got; s != "a\nb" {
				t.Errorf("text %v: %q with %v read %q", text, k.name, k.m, s)
			}
			// Read again, or later, it is the same.
			if s, _ := w.Clipboard(); s != "a\nb" {
				t.Errorf("text %v: read again it is %q", text, s)
			}
			send("keyup", k.name, k.code, k.m)
			if !had.Equal(active()) {
				t.Errorf("text %v: after a paste %v has the focus", text, active().Get("tagName"))
			}
			for _, e := range drain(w) {
				if in, ok := e.(input.TextInput); ok {
					t.Errorf("text %v: the paste typed %q too", text, in.Text)
				}
			}
			if v := d.plain.Get("value").String(); v != "" {
				t.Errorf("text %v: the mirror holds %q", text, v)
			}
		}
		if text {
			w.SetTextInput(false)
		}
		_ = w.Close()
		d.frame()
	}
}

// TestAPasteKeyTheProgramUsesOtherwise: Ctrl+V is a key of the shell's, and the paste
// the browser does for it goes nowhere.
func TestAPasteKeyTheProgramUsesOtherwise(t *testing.T) {
	asApple(t, false)
	d, w := claimWindow(t, true)
	defer w.SetTextInput(false)
	send("keydown", "v", "KeyV", ctrl)
	if len(presses(drain(w))) != 1 {
		t.Fatal("the program did not hear Ctrl+V")
	}
	if !pasteEvent("secret") {
		t.Error("the browser pasted on a key of the program's")
	}
	send("keyup", "v", "KeyV", ctrl)
	// The browser's own menu pastes as typing, as it does without a claim.
	d.forgetPaste(nil)
	if pasteEvent("menu") {
		t.Error("the browser's own paste was cancelled")
	}
}

// TestHeldPasteKeys: a paste key held down goes down again and again, and an event
// comes for each, whether or not the program has read the one before.
func TestHeldPasteKeys(t *testing.T) {
	asApple(t, false)
	refuseReading(t)
	_, w := claimWindow(t, true)
	defer w.SetTextInput(false)
	for range 3 {
		send("keydown", "V", "KeyV", ctrl|shift)
	}
	for range 3 {
		if !pasteEvent("x") {
			t.Fatal("the browser pasted one of them itself")
		}
	}
	if pasteEvent("menu") {
		t.Error("a paste with no key left was cancelled")
	}
	if s, err := w.Clipboard(); s != "x" || err != nil {
		t.Errorf("read %q, %v", s, err)
	}
	for _, text := range []string{"one", "two"} {
		send("keydown", "V", "KeyV", ctrl|shift)
		pasteEvent(text)
		if s, _ := w.Clipboard(); s != text {
			t.Errorf("read %q, want %q", s, text)
		}
	}
	drain(w)
}

// fakeClipboard answers the driver's reads of the clipboard with what the clipboard
// holds, and takes its writes.
func fakeClipboard(t *testing.T, holds string) {
	t.Helper()
	read := js.FuncOf(func(js.Value, []js.Value) any { return js.Global().Get("Promise").Call("resolve", holds) })
	wrote := js.FuncOf(func(js.Value, []js.Value) any { return js.Global().Get("Promise").Call("resolve") })
	restore := define(js.Global().Get("navigator"), "clipboard", js.ValueOf(map[string]any{"readText": read, "writeText": wrote}))
	t.Cleanup(func() {
		restore()
		read.Release()
		wrote.Release()
	})
}

// keptPaste is the paste the driver keeps, nil for none.
func keptPaste(d *Driver) *keyPaste {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.paste
}

// setFor sets a time of the driver's for the length of a test.
func setFor(t *testing.T, v *time.Duration, to time.Duration) {
	t.Helper()
	was := *v
	*v = to
	t.Cleanup(func() { *v = was })
}

// TestAPasteIsForgottenAsTheClipboardChanges: what a key pasted answers for a moment
// only, and never for text put there since.
func TestAPasteIsForgottenAsTheClipboardChanges(t *testing.T) {
	asApple(t, false)
	fakeClipboard(t, "the clipboard's")
	d, w := claimWindow(t, true)
	defer w.SetTextInput(false)
	paste := func() *keyPaste {
		t.Helper()
		send("keydown", "V", "KeyV", ctrl|shift)
		pasteEvent("pasted")
		if s, _ := w.Clipboard(); s != "pasted" {
			t.Fatalf("read %q", s)
		}
		return keptPaste(d)
	}
	check := func(what string, p *keyPaste) {
		t.Helper()
		if keptPaste(d) != nil || p.text != "" {
			t.Errorf("after %s the driver keeps %q", what, p.text)
		}
		if s, _ := w.Clipboard(); s != "the clipboard's" {
			t.Errorf("after %s the clipboard reads %q", what, s)
		}
	}
	p := paste()
	if err := w.SetClipboard("new"); err != nil {
		t.Fatal(err)
	}
	check("a copy of the program's", p)
	p = paste()
	js.Global().Call("dispatchEvent", js.Global().Get("Event").New("blur"))
	check("another window's turn", p)
	// Its time is up: it goes by itself, read or not, as it can be a password.
	setFor(t, &pasteKept, 50*time.Millisecond)
	p = paste()
	time.Sleep(200 * time.Millisecond)
	check("its time", p)
	send("keydown", "V", "KeyV", ctrl|shift)
	pasteEvent("unread")
	p = keptPaste(d)
	if p == nil || p.text != "unread" {
		t.Fatalf("the driver keeps %+v", p)
	}
	time.Sleep(200 * time.Millisecond)
	check("its time, unread", p)
	// A browser that pastes nothing on the key is asked for the clipboard.
	setFor(t, &pasteWait, 20*time.Millisecond)
	send("keydown", "V", "KeyV", ctrl|shift)
	if s, _ := w.Clipboard(); s != "the clipboard's" {
		t.Errorf("with no paste the clipboard reads %q", s)
	}
	drain(w)
}

// TestNoPasteIsTakenAfterItsKey: the program's key is over, and what is pasted next is
// nothing of its.
func TestNoPasteIsTakenAfterItsKey(t *testing.T) {
	over := func(name string, end func(w *Window)) {
		t.Run(name, func(t *testing.T) {
			asApple(t, name == "Command coming up")
			d, w := claimWindow(t, true)
			defer w.SetTextInput(false)
			m := ctrl | shift
			if onApple {
				m = super
			}
			// A key the browser pastes nothing on after all.
			send("keydown", "V", "KeyV", m)
			end(w)
			w.SetTextState(&input.TextState{Text: "a field"}, w.editSeq)
			defer w.SetTextState(nil, w.editSeq)
			if pasteEvent("for the field") {
				t.Error("a later paste was taken from a text field")
			}
			if p := keptPaste(d); p != nil {
				t.Errorf("the driver still keeps %+v", p)
			}
			drain(w)
		})
	}
	over("the key coming up", func(*Window) { send("keyup", "V", "KeyV", ctrl|shift) })
	// A Mac sends no keyup for V while Command is down.
	over("Command coming up", func(*Window) { send("keyup", "Meta", "MetaLeft", 0) })
	over("the claim's end", func(w *Window) {
		w.SetKeysClaimed(false)
		time.Sleep(2 * claimEndWait)
	})
	over("focus leaving the page", func(*Window) { js.Global().Call("dispatchEvent", js.Global().Get("Event").New("blur")) })
}

// TestAKeyThatEndsTheClaim: the program has the key, as to open a field of its own, and
// the browser pastes nothing there as it ends the key.
func TestAKeyThatEndsTheClaim(t *testing.T) {
	asApple(t, false)
	_, w := claimWindow(t, true)
	defer w.SetTextInput(false)
	defer w.SetTextState(nil, w.editSeq)
	send("keydown", "V", "KeyV", ctrl|shift)
	w.SetTextState(&input.TextState{Text: "a field"}, w.editSeq)
	w.SetKeysClaimed(false)
	if !pasteEvent("the key's") {
		t.Error("the browser pastes the key's text into the field the key opened")
	}
	drain(w)
}

func TestWhichKeysPaste(t *testing.T) {
	for _, c := range []struct {
		name, code string
		m          input.Mods
		apple      bool
		want       bool
	}{
		{"v", "KeyV", ctrl, false, true},
		{"V", "KeyV", ctrl | shift, false, true},
		{"м", "KeyV", ctrl, false, true},
		{"Insert", "Insert", shift, false, true},
		{"Insert", "Insert", shift | ctrl, false, false},
		{"v", "KeyV", ctrl | alt, false, false},
		{"v", "KeyV", super, false, false},
		{"v", "KeyV", super, true, true},
		{"V", "KeyV", super | shift, true, true},
		{"◊", "KeyV", super | shift | alt, true, true},
		// Moving a file in the Finder, and nothing in a browser.
		{"√", "KeyV", super | alt, true, false},
		{"v", "KeyV", ctrl, true, false},
		{"v", "KeyV", super | ctrl, true, false},
		{"k", "KeyV", super, true, false},
		{"Insert", "Insert", shift, true, false},
	} {
		if got := pasteKey(c.name, c.code, c.m, c.apple); got != c.want {
			t.Errorf("%q with %v, apple %v: pastes %v, want %v", c.name, c.m, c.apple, got, c.want)
		}
	}
}

// texts returns the text typed among events.
func texts(events []any) []string {
	var out []string
	for _, e := range events {
		if in, ok := e.(input.TextInput); ok {
			out = append(out, in.Text)
		}
	}
	return out
}

// TestCommandVWithAClaim: the program hears it, and what it pastes is the program's to
// read, or is typed, and never both.
func TestCommandVWithAClaim(t *testing.T) {
	asApple(t, true)
	refuseReading(t)
	for _, text := range []bool{false, true} {
		d, w := claimWindow(t, text)
		command := func(text string) {
			t.Helper()
			if send("keydown", "v", "KeyV", super) {
				t.Fatal("Command+V was cancelled, so the browser pastes nothing")
			}
			if got := presses(drain(w)); len(got) != 1 || got[0].Mods != super {
				t.Fatalf("Command+V reached the program as %+v", got)
			}
			if !pasteEvent(text) {
				t.Fatal("the browser pasted by itself")
			}
		}
		read := func(want string) {
			t.Helper()
			if s, err := w.Clipboard(); s != want || err != nil {
				t.Errorf("Command+V read %q, %v, want %q", s, err, want)
			}
		}
		// Read in the key's handler, which waits for the event.
		send("keydown", "v", "KeyV", super)
		got := make(chan string, 1)
		go func() {
			s, _ := w.Clipboard()
			got <- s
		}()
		time.Sleep(10 * time.Millisecond)
		if !pasteEvent("at once") {
			t.Error("the program pasted on Command+V, and so did the browser")
		}
		if s := <-got; s != "at once" {
			t.Errorf("Command+V read %q", s)
		}
		// Read outside it, as by a goroutine of the program's, it is not typed too.
		drain(w)
		command("soon after")
		time.Sleep(commandWait / 4)
		read("soon after")
		time.Sleep(commandWait * 2)
		if got := texts(drain(w)); len(got) != 0 {
			t.Errorf("text %v: what the program read was typed too: %q", text, got)
		}
		// Never read, it is typed, once, with its line endings as a browser types them.
		command("a\r\nb")
		time.Sleep(commandWait * 3)
		if got := texts(drain(w)); !slices.Equal(got, []string{"a\nb"}) {
			t.Errorf("text %v: unread, it was typed as %q", text, got)
		}
		// Read after it was typed, it is nothing more, for as long as it would have
		// answered, and the clipboard is not asked for it. Nor is it after a switch to
		// another window.
		read("")
		js.Global().Call("dispatchEvent", js.Global().Get("Event").New("blur"))
		read("")
		if p := keptPaste(d); p == nil || p.text != "" {
			t.Errorf("text %v: typed, the driver keeps %+v", text, p)
		}
		// A press types it first, before the focus can move.
		command("before the press")
		pointer(d, "pointerdown", 9, 5, 5)
		pointer(d, "pointerup", 9, 5, 5)
		d.mu.Lock()
		d.lastTouch = false
		d.mu.Unlock()
		pressed, typed := -1, -1
		for i, e := range drain(w) {
			switch e := e.(type) {
			case input.PointerDown:
				pressed = i
			case input.TextInput:
				if e.Text == "before the press" {
					typed = i
				}
			}
		}
		if typed < 0 || pressed < typed {
			t.Errorf("text %v: the paste was typed at %d, and the press at %d", text, typed, pressed)
		}
		// What is typed next comes after it.
		command("first")
		send("keydown", "x", "KeyX", 0)
		events := drain(w)
		if len(events) == 0 {
			t.Fatal("nothing was typed")
		}
		if in, ok := events[0].(input.TextInput); !ok || in.Text != "first" {
			t.Errorf("text %v: the key after it overtook the paste: %+v", text, events)
		}
		// Held down, each is typed.
		command("1")
		command2 := func(text string) {
			send("keydown", "v", "KeyV", super)
			pasteEvent(text)
		}
		command2("2")
		command2("3")
		time.Sleep(commandWait * 3)
		if got := texts(drain(w)); !slices.Equal(got, []string{"1", "2", "3"}) {
			t.Errorf("text %v: held down it typed %q", text, got)
		}
		// Switching to another window does not lose it.
		command("before the switch")
		js.Global().Call("dispatchEvent", js.Global().Get("Event").New("blur"))
		if got := texts(drain(w)); !slices.Equal(got, []string{"before the switch"}) {
			t.Errorf("text %v: as the window lost the keyboard it typed %q", text, got)
		}
		// With the claim gone the focus is elsewhere, and nothing is typed there.
		command("elsewhere")
		w.SetKeysClaimed(false)
		time.Sleep(commandWait * 3)
		if got := texts(drain(w)); len(got) != 0 {
			t.Errorf("text %v: it was typed where the focus went: %q", text, got)
		}
		w.SetKeysClaimed(true)
		// Control is not the browser's to paste with there.
		if !send("keydown", "v", "KeyV", ctrl) {
			t.Errorf("text %v: Control+V was left to the browser", text)
		}
		drain(w)
		if text {
			w.SetTextInput(false)
		}
		_ = w.Close()
		d.frame()
	}
}

// TestLentFocusComesBack: the mirror has the canvas's focus for a paste that never
// comes, and gives it back as the window loses the keyboard, and as the key comes up.
func TestLentFocusComesBack(t *testing.T) {
	asApple(t, false)
	d, w := claimWindow(t, false)
	for what, end := range map[string]func(){
		"the window losing the keyboard": func() { js.Global().Call("dispatchEvent", js.Global().Get("Event").New("blur")) },
		"the key coming up":              func() { send("keyup", "V", "KeyV", ctrl|shift) },
	} {
		send("keydown", "V", "KeyV", ctrl|shift)
		if !d.plain.Equal(active()) {
			t.Fatalf("the mirror has no focus to paste into, %v has", active().Get("tagName"))
		}
		end()
		if !d.canvas.Equal(active()) {
			t.Errorf("after %s, %v keeps the focus", what, active().Get("tagName"))
		}
		drain(w)
	}
}

// TestAltKeysTheSystemHas: Alt+Space opens the window's menu, and Alt with the number
// pad types a character by its number on Windows, as Alt comes up.
func TestAltKeysTheSystemHas(t *testing.T) {
	asApple(t, false)
	for _, text := range []bool{false, true} {
		d, w := claimWindow(t, text)
		send("keydown", "Alt", "AltLeft", alt)
		drain(w)
		for _, k := range [][2]string{{" ", "Space"}, {"1", "Numpad1"}, {"End", "Numpad1"}, {"+", "NumpadAdd"}} {
			if send("keydown", k[0], k[1], alt) || len(presses(drain(w))) != 0 {
				t.Errorf("text %v: Alt+%s did not stay the system's", text, k[1])
			}
		}
		if send("keyup", "Alt", "AltLeft", 0) {
			t.Errorf("text %v: Alt coming up was cancelled, which types the character", text)
		}
		// Alt alone once more is kept from the menus.
		send("keydown", "Alt", "AltLeft", alt)
		if !send("keyup", "Alt", "AltLeft", 0) {
			t.Errorf("text %v: Alt alone moves the keyboard to the menus", text)
		}
		// Alt+Enter on the number pad, and Alt with the digits above the letters, are
		// the program's.
		drain(w)
		for _, k := range [][2]string{{"Enter", "NumpadEnter"}, {"1", "Digit1"}} {
			if !send("keydown", k[0], k[1], alt) || len(presses(drain(w))) != 1 {
				t.Errorf("text %v: Alt+%s stayed the system's", text, k[1])
			}
		}
		if text {
			w.SetTextInput(false)
		}
		_ = w.Close()
		d.frame()
	}
}

// TestAClaimInATextField: the browser goes on cutting, copying and pasting in a field
// whose text it has.
func TestAClaimInATextField(t *testing.T) {
	asApple(t, false)
	d, w := claimWindow(t, true)
	defer w.SetTextInput(false)
	defer w.SetTextState(nil, w.editSeq)
	w.SetTextState(&input.TextState{Text: "abc", Selection: [2]int{0, 3}}, w.editSeq)
	drain(w)
	for _, k := range append(pasteKeys, pasteKeys[0], struct {
		name, code string
		m          input.Mods
	}{"c", "KeyC", ctrl}) {
		if send("keydown", k.name, k.code, k.m) || len(presses(drain(w))) != 0 {
			t.Errorf("%q with %v in a field left the browser", k.name, k.m)
		}
	}
	if pasteEvent("x") {
		t.Error("a paste in a field was cancelled")
	}
	// The keys it claims it has all the same.
	if !send("keydown", "r", "KeyR", ctrl) || len(presses(drain(w))) != 1 {
		t.Error("Ctrl+R in a field that claims the keys was the browser's")
	}
	_ = d
}

// fakeFullScreen stands in for the browser's full screen, which takes a user's own
// key or click, and for its Keyboard Lock API.
type fakeFullScreen struct {
	asked, left, locks, unlocks int
	locked                      []string
	// refuse has requests refused, hold has them wait for answer, and slow has the way
	// out wait for exit. enter fills the screen, with the request yet to be answered.
	refuse, hold, slow  bool
	answer, enter, exit func()
}

func newFakeFullScreen(t *testing.T) *fakeFullScreen {
	t.Helper()
	f := &fakeFullScreen{}
	doc := js.Global().Get("document")
	root := doc.Get("documentElement")
	resolved := js.Global().Get("Promise").Call("resolve")
	set := func(el any) {
		js.Global().Get("Object").Call("defineProperty", doc, "fullscreenElement", map[string]any{"configurable": true, "value": el})
		doc.Call("dispatchEvent", js.Global().Get("Event").New("fullscreenchange"))
	}
	var resolves []js.Value
	held := js.FuncOf(func(_ js.Value, args []js.Value) any {
		resolves = append(resolves, args[0])
		return nil
	})
	f.answer = func() {
		for _, r := range resolves {
			set(root)
			r.Invoke()
		}
		resolves = nil
	}
	f.enter = func() { set(root) }
	f.exit = func() { set(js.Null()) }
	fns := []js.Func{
		held,
		js.FuncOf(func(js.Value, []js.Value) any {
			f.asked++
			if f.refuse {
				return js.Global().Get("Promise").Call("reject", js.Global().Get("TypeError").New("no gesture"))
			}
			if f.hold {
				return js.Global().Get("Promise").New(held)
			}
			set(root)
			return resolved
		}),
		js.FuncOf(func(js.Value, []js.Value) any {
			f.left++
			if !f.slow {
				set(js.Null())
			}
			return resolved
		}),
		js.FuncOf(func(_ js.Value, args []js.Value) any {
			f.locks++
			f.locked = nil
			for i := range args[0].Length() {
				f.locked = append(f.locked, args[0].Index(i).String())
			}
			return resolved
		}),
		js.FuncOf(func(js.Value, []js.Value) any { f.unlocks++; return nil }),
	}
	restores := []func(){
		define(root, "requestFullscreen", fns[1].Value),
		define(doc, "exitFullscreen", fns[2].Value),
		define(js.Global().Get("navigator"), "keyboard", js.ValueOf(map[string]any{"lock": fns[3], "unlock": fns[4]})),
	}
	t.Cleanup(func() {
		set(js.Null())
		js.Global().Get("Reflect").Call("deleteProperty", doc, "fullscreenElement")
		for _, r := range restores {
			r()
		}
		for _, fn := range fns {
			fn.Release()
		}
	})
	return f
}

func TestFullScreen(t *testing.T) {
	d, w := claimWindow(t, false)
	w.SetKeysClaimed(false)
	f := newFakeFullScreen(t)
	style := d.canvas.Get("style")
	before := style.Get("cssText").String()
	waitFull := func(on bool) {
		t.Helper()
		for give := time.Now().Add(5 * time.Second); w.FullScreen() != on; time.Sleep(time.Millisecond) {
			if time.Now().After(give) {
				t.Fatalf("full screen is never %v", on)
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	drain(w)
	w.SetFullScreen(true)
	waitFull(true)
	if f.asked != 1 || f.locks != 0 {
		t.Errorf("full screen was asked for %d times, and the keys locked %d with no claim", f.asked, f.locks)
	}
	if events := drain(w); len(events) == 0 {
		t.Error("the engine heard nothing of the full screen")
	}
	r := d.canvas.Call("getBoundingClientRect")
	if r.Get("left").Float() != 0 || r.Get("top").Float() != 0 ||
		r.Get("width").Float() != js.Global().Get("document").Get("documentElement").Get("clientWidth").Float() ||
		r.Get("height").Float() != js.Global().Get("innerHeight").Float() {
		t.Errorf("in full screen the canvas is at %v, %v, %v×%v", r.Get("left"), r.Get("top"), r.Get("width"), r.Get("height"))
	}
	// Asked again, nothing more.
	w.SetFullScreen(true)
	if f.asked != 1 {
		t.Error("full screen was asked for twice")
	}
	// A claim made then has the keys that come only there, Escape among them.
	w.SetKeysClaimed(true)
	// No more than that: Chrome acts no more on one locked and left to it.
	if f.locks != 1 || !slices.Equal(f.locked, []string{"Escape"}) {
		t.Errorf("a claim in full screen locked the keys %d times: %v", f.locks, f.locked)
	}
	drain(w)
	for _, k := range [][2]string{{"Escape", "Escape"}, {"F11", "F11"}} {
		if !send("keydown", k[0], k[1], 0) || len(presses(drain(w))) != 1 {
			t.Errorf("in full screen, %s did not reach the program alone", k[0])
		}
	}
	if !send("keydown", "w", "KeyW", ctrl) || len(presses(drain(w))) != 1 {
		t.Error("in full screen, Ctrl+W did not reach the program alone")
	}
	// The focus goes where nothing claims them and back, as to a dialog, and the lock
	// stays as it is. Escape is the dialog's, and the browser's keys the browser's.
	w.SetKeysClaimed(false)
	if !send("keydown", "Escape", "Escape", 0) || len(presses(drain(w))) != 1 {
		t.Error("with the claim let go in full screen, Escape did not reach the program")
	}
	for _, k := range [][2]string{{"w", "KeyW"}, {"t", "KeyT"}, {"1", "Digit1"}, {"PageUp", "PageUp"}} {
		if send("keydown", k[0], k[1], ctrl) || len(presses(drain(w))) != 0 {
			t.Errorf("with the claim let go in full screen, Ctrl+%s did not stay the browser's", k[0])
		}
	}
	w.SetKeysClaimed(true)
	if f.locks != 1 || f.unlocks != 0 {
		t.Errorf("the claim let go and made again locked the keys %d times, and unlocked them %d", f.locks, f.unlocks)
	}
	// The user leaves, as by holding Escape.
	js.Global().Get("document").Call("exitFullscreen")
	waitFull(false)
	if f.unlocks != 1 {
		t.Errorf("leaving full screen left the keys locked: unlocked %d times", f.unlocks)
	}
	if got := style.Get("cssText").String(); got != before {
		t.Errorf("after full screen the canvas's style is %q, it was %q", got, before)
	}
	if send("keydown", "F11", "F11", 0) {
		t.Error("out of full screen, F11 is the program's")
	}
	if events := drain(w); len(events) == 0 {
		t.Error("the engine heard nothing of the full screen's end")
	}
	// With the claim held, full screen locks at once.
	w.SetFullScreen(true)
	waitFull(true)
	if f.locks != 2 {
		t.Errorf("the keys were locked %d times", f.locks)
	}
	w.SetFullScreen(false)
	waitFull(false)
	if f.left != 2 {
		t.Errorf("full screen was left %d times", f.left)
	}
}

// waitFor waits for cond.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for give := time.Now().Add(5 * time.Second); !cond(); time.Sleep(time.Millisecond) {
		if time.Now().After(give) {
			t.Fatalf("%s never came", what)
		}
	}
}

// leftIn says whether events say the user left full screen.
func leftIn(events []any) bool { return slices.Contains(events, any(driver.FullScreenLeft{})) }

func TestFullScreenRefused(t *testing.T) {
	d, w := testWindow(t)
	f := newFakeFullScreen(t)
	f.refuse = true
	before := d.canvas.Get("style").Get("cssText").String()
	drain(w)
	w.SetFullScreen(true)
	time.Sleep(20 * time.Millisecond)
	if w.FullScreen() || d.fullOurs || d.fullWant {
		t.Error("refused, the window says it fills the screen")
	}
	if !leftIn(drain(w)) {
		t.Error("the program heard nothing of the refusal")
	}
	if got := d.canvas.Get("style").Get("cssText").String(); got != before {
		t.Errorf("refused, the canvas's style is %q", got)
	}
	// A popup has none of its own.
	p := testPopup(t, d, w)
	defer func() { _ = p.Close(); d.frame() }()
	drain(p)
	p.SetFullScreen(false)
	if leftIn(drain(p)) {
		t.Error("a popup asked out of full screen says it was refused")
	}
	p.SetFullScreen(true)
	if f.asked != 1 {
		t.Error("a popup asked for the screen")
	}
	if !leftIn(drain(p)) {
		t.Error("a popup's program heard nothing of the refusal")
	}
	// Nor has an iPhone's browser.
	root := js.Global().Get("document").Get("documentElement")
	restore := define(root, "requestFullscreen", js.Undefined())
	drain(w)
	w.SetFullScreen(false)
	if leftIn(drain(w)) {
		t.Error("asked out of full screen, a browser with none says it refused")
	}
	w.SetFullScreen(true)
	if !leftIn(drain(w)) || w.FullScreen() || d.fullWant {
		t.Error("the program heard nothing of a browser with no full screen")
	}
	restore()
	// The page's own full screen is not the program's.
	f.refuse = false
	js.Global().Get("document").Get("documentElement").Call("requestFullscreen")
	if w.FullScreen() {
		t.Error("the page's own full screen is the window's")
	}
}

// TestTheProgramHearsTheUserLeaveFullScreen: and not its own leaving.
func TestTheProgramHearsTheUserLeaveFullScreen(t *testing.T) {
	d, w := testWindow(t)
	newFakeFullScreen(t)
	full := func(on bool) func() bool { return func() bool { return w.FullScreen() == on } }
	settle := func() { time.Sleep(20 * time.Millisecond) }
	w.SetFullScreen(true)
	waitFor(t, "full screen", full(true))
	settle()
	if leftIn(drain(w)) {
		t.Error("entering full screen says the user left")
	}
	w.SetFullScreen(false)
	waitFor(t, "its end", full(false))
	settle()
	if leftIn(drain(w)) {
		t.Error("the program leaving says the user left")
	}
	w.SetFullScreen(true)
	waitFor(t, "full screen", full(true))
	settle()
	_, over := testWindow(t)
	drain(w)
	drain(over)
	js.Global().Get("document").Call("exitFullscreen")
	settle()
	if !leftIn(drain(w)) || !leftIn(drain(over)) {
		t.Error("a window that filled the screen heard nothing of the user leaving")
	}
	// The one that asked for it closes, and the other still hears.
	raw, err := d.NewWindow(driver.Options{})
	if err != nil {
		t.Fatal(err)
	}
	raw.(*Window).SetFullScreen(true)
	waitFor(t, "full screen", full(true))
	settle()
	_ = raw.Close()
	d.frame()
	if !w.FullScreen() || !d.fullWant {
		t.Error("full screen went with the window that asked for it")
	}
	drain(w)
	js.Global().Get("document").Call("exitFullscreen")
	settle()
	if !leftIn(drain(w)) {
		t.Error("with the window that asked closed, no one hears the user leave")
	}
	// Asked for again, it comes.
	w.SetFullScreen(true)
	waitFor(t, "full screen again", full(true))
	w.SetFullScreen(false)
	waitFor(t, "its end", full(false))
	settle()
}

// TestFullScreenChangesItsMindWhileTheBrowserAnswers: what the program asked last holds.
func TestFullScreenChangesItsMindWhileTheBrowserAnswers(t *testing.T) {
	d, w := testWindow(t)
	f := newFakeFullScreen(t)
	before := d.canvas.Get("style").Get("cssText").String()
	f.hold = true
	drain(w)
	w.SetFullScreen(true)
	w.SetFullScreen(false)
	f.answer()
	waitFor(t, "the way out", func() bool { return f.left == 1 })
	time.Sleep(20 * time.Millisecond)
	if w.FullScreen() || d.canvas.Get("style").Get("cssText").String() != before || leftIn(drain(w)) {
		t.Errorf("asked out again before the answer, full %v, style %q", w.FullScreen(), d.canvas.Get("style").Get("cssText"))
	}
	// And in again.
	w.SetFullScreen(true)
	w.SetFullScreen(false)
	w.SetFullScreen(true)
	f.answer()
	waitFor(t, "full screen", w.FullScreen)
	time.Sleep(20 * time.Millisecond)
	if !w.FullScreen() || f.left != 1 || d.canvas.Get("style").Get("cssText").String() == before {
		t.Errorf("asked in again before the answer, full %v, left %d times", w.FullScreen(), f.left)
	}
	// Out as it fills the screen, and the request answered before it has left.
	f.exit()
	time.Sleep(20 * time.Millisecond)
	w.SetKeysClaimed(true)
	f.slow = true
	w.SetFullScreen(true)
	f.enter()
	if !d.keysLocked {
		t.Fatal("the keys are not locked")
	}
	w.SetFullScreen(false)
	f.answer()
	time.Sleep(20 * time.Millisecond)
	drain(w)
	f.exit()
	if d.keysLocked || len(drain(w)) == 0 {
		t.Errorf("out of full screen, the keys are locked %v, and the engine heard nothing", d.keysLocked)
	}
	f.slow = false
	// Out, in and out before the browser has answered the first.
	f.hold = false
	w.SetKeysClaimed(true)
	drain(w)
	w.SetFullScreen(false)
	w.SetFullScreen(true)
	w.SetFullScreen(false)
	time.Sleep(50 * time.Millisecond)
	if w.FullScreen() || d.keysLocked || d.canvas.Get("style").Get("cssText").String() != before || leftIn(drain(w)) {
		t.Errorf("asked out last, full %v, keys locked %v", w.FullScreen(), d.keysLocked)
	}
}

// TestTheKeyLockComesWithTheKeyboard: however the keyboard comes to a window that
// claims the keys in full screen, they are locked, and once only until it ends.
func TestTheKeyLockComesWithTheKeyboard(t *testing.T) {
	for what, comes := range map[string]func(d *Driver, w, over *Window){
		"the window over it closing": func(_ *Driver, _, over *Window) { _ = over.Close() },
		"the window over it hiding":  func(_ *Driver, _, over *Window) { _ = over.Hide() },
		"taking text":                func(_ *Driver, w, _ *Window) { w.SetTextInput(true) },
		"the window over it claiming": func(_ *Driver, _, over *Window) {
			over.SetKeysClaimed(true)
		},
		"showing again": func(_ *Driver, w, over *Window) {
			_ = w.Hide()
			_ = over.Close()
			_ = w.Show()
		},
	} {
		t.Run(what, func(t *testing.T) {
			d, w := claimWindow(t, false)
			f := newFakeFullScreen(t)
			raw, err := d.NewWindow(driver.Options{})
			if err != nil {
				t.Fatal(err)
			}
			over := raw.(*Window)
			defer func() { _ = over.Close(); d.frame() }()
			w.SetFullScreen(true)
			waitFor(t, "full screen", w.FullScreen)
			if f.locks != 0 {
				t.Fatal("the keys are locked with a window over the one that claims them")
			}
			comes(d, w, over)
			if f.locks != 1 || f.unlocks != 0 {
				t.Errorf("the keys were locked %d times, and let go %d", f.locks, f.unlocks)
			}
			// A window over it, and gone, changes nothing.
			_, again := testWindow(t)
			_ = again.Hide()
			_ = again.Show()
			_ = again.Close()
			if f.locks != 1 || f.unlocks != 0 {
				t.Errorf("after a window over it, the keys were locked %d times, and let go %d", f.locks, f.unlocks)
			}
			w.SetFullScreen(false)
			waitFor(t, "its end", func() bool { return !w.FullScreen() && f.unlocks == 1 })
			if what == "taking text" {
				w.SetTextInput(false)
			}
		})
	}
}

// TestStoppingInFullScreenLeavesTheCanvasAlone: the page gets its canvas back as it laid
// it out.
func TestStoppingInFullScreenLeavesTheCanvasAlone(t *testing.T) {
	d, w := testWindow(t)
	f := newFakeFullScreen(t)
	before := d.canvas.Get("style").Get("cssText").String()
	w.SetFullScreen(true)
	waitFor(t, "full screen", w.FullScreen)
	time.Sleep(20 * time.Millisecond)
	if d.canvas.Get("style").Get("cssText").String() == before {
		t.Fatal("the canvas is not pinned")
	}
	d.stop()
	if got := d.canvas.Get("style").Get("cssText").String(); got != before {
		t.Errorf("stopped, the canvas's style is %q, it was %q", got, before)
	}
	if f.left != 1 {
		t.Errorf("full screen was left %d times", f.left)
	}
}

func TestAskBeforeThePageCloses(t *testing.T) {
	d, w := testWindow(t)
	asks := func() bool {
		e := js.Global().Get("Event").New("beforeunload", map[string]any{"cancelable": true})
		js.Global().Call("dispatchEvent", e)
		return e.Get("defaultPrevented").Bool()
	}
	if asks() {
		t.Error("the page asks before closing, with no window that wants it to")
	}
	w.SetCloseAsks(true)
	if !asks() {
		t.Error("the page closes unasked")
	}
	_, other := testWindow(t)
	other.SetCloseAsks(true)
	w.SetCloseAsks(false)
	if !asks() {
		t.Error("the page closes unasked, with a window left that wants it asked")
	}
	_ = other.Close()
	if asks() {
		t.Error("the page asks for a window that has closed")
	}
	d.frame()
}
