//go:build js && wasm

package web

import (
	"strings"
	"syscall/js"
	"time"

	"github.com/marrasen/gunim/input"
)

// This file is the keys a program claims from the browser, as a terminal does, and
// the paste such a program's own key does. See [driver.KeyClaimer].
//
// While the node with the keyboard claims them, every key a page can take goes to
// the program alone. The browser still keeps zooming, and F11 until the program
// fills the screen. The system keeps Alt+F4, Alt+Space and Alt with the number pad's
// digits. Every shortcut with Command is whose it is without a claim, as Command is
// the system's and no terminal's, but for Command+V, which the program hears: see
// keyPaste. Keys that open the developer tools, F12 and Ctrl+Shift+I, J and C, are
// claimed: programs in a terminal use F12, a terminal copies with Ctrl+Shift+C, and
// the tools still open from the browser's menu. A browser hands a page no Ctrl+W, T
// or N, and no Ctrl+Tab or Ctrl+Page Up, outside full screen. What it does hand over,
// the program gets.
//
// Keys that type still type through the browser: input methods, dead keys, AltGr and
// soft keyboards work as they do without a claim.

// SetKeysClaimed implements [driver.KeyClaimer].
func (w *Window) SetKeysClaimed(on bool) {
	w.d.mu.Lock()
	w.claims = on
	w.d.mu.Unlock()
	if !on {
		w.d.claimEnded(w)
	}
	w.d.lockKeys()
}

// claimed says the keys that go to w are claimed.
func (d *Driver) claimed(w *Window) bool { return d.claimer(w) != nil }

// claimer returns the window that claims the keys that go to w: the window, or for a
// popup, which passes its keys on to the window it belongs to, that one. It is nil
// where nothing claims them.
func (d *Driver) claimer(w *Window) *Window {
	d.mu.Lock()
	defer d.mu.Unlock()
	for w != nil && !w.claims && w.popup {
		w = w.parent
	}
	if w == nil || !w.claims {
		return nil
	}
	return w
}

// keptKey says a key stays the browser's or the system's even from a window that
// claims the keys. full says the program fills the screen.
func keptKey(name, code string, m input.Mods, full bool) bool {
	if m&input.ModSuper != 0 {
		return browserKey(name, code, m)
	}
	switch ctrl, alt := m.Has(input.ModControl), m.Has(input.ModAlt); {
	case ctrl && !alt:
		// Zooming. The 0 key picks the page's own size, whatever a layout has on it.
		switch shortcutName(name, code) {
		case "+", "=", "-", "_", "0":
			return true
		}
		return m&input.ModShift == 0 && code == "Digit0"
	case alt && !ctrl:
		// Closing the window, its menu, and on Windows a character by its number.
		return name == "F4" || !optionTypes && (code == "Space" || altCodeKey(code))
	case !ctrl:
		// Full screen, and the way out of the browser's own.
		return name == "F11" && m == 0 && !full
	}
	return false
}

// altCodeKey says a key is one of the number pad's that, with Alt held, spell a
// character's number on Windows.
func altCodeKey(code string) bool {
	n, ok := strings.CutPrefix(code, "Numpad")
	return ok && (len(n) == 1 || n == "Add")
}

// claimedKey says a key of a window that claims the keys is one to keep from the
// browser, past those [gunimKey] says: with Alt, the menus, going back and forward and
// the address bar, and Alt alone, which moves the keyboard to the menus. typed says
// the key types, as Option does on a Mac, and the browser types it.
func claimedKey(name string, m input.Mods, typed bool) bool {
	return name == "Alt" || m&input.ModAlt != 0 && !typed
}

// optionTypes says Alt is a key that types on the user's keyboard, as Option on an
// Apple one and Alt on a phone's, where Alt+L can be @. Elsewhere AltGr is that key,
// and Alt makes shortcuts alone.
var optionTypes = func() bool {
	p := platformName()
	return applePlatform(p) || strings.Contains(p, "Android")
}()

// onApple says the browser's shortcuts are Command's.
var onApple = applePlatform(platformName())

// platformName is what the browser says it runs on. Android calls itself Linux as a
// platform, and its name is in the browser's own.
func platformName() string {
	nav := js.Global().Get("navigator")
	if !nav.Truthy() {
		return ""
	}
	word := func(key string) string {
		if v := nav.Get(key); v.Type() == js.TypeString {
			return v.String()
		}
		return ""
	}
	return word("platform") + " " + word("userAgent")
}

func applePlatform(name string) bool {
	return strings.HasPrefix(name, "Mac") || strings.HasPrefix(name, "iP")
}

// modifierKey says a key only changes what others do, and types nothing.
func modifierKey(name string) bool {
	switch name {
	case "Shift", "Control", "Alt", "AltGraph", "Meta", "CapsLock", "NumLock", "ScrollLock", "Fn":
		return true
	}
	return false
}

// pasteKey says the browser pastes on a key: Ctrl+V, with Shift or not, and
// Shift+Insert, or on an Apple keyboard Command+V, with Shift, or with Shift and Option.
func pasteKey(name, code string, m input.Mods, apple bool) bool {
	if apple {
		return (m == input.ModSuper || m == input.ModSuper|input.ModShift || m == input.ModSuper|input.ModShift|input.ModAlt) &&
			shortcutName(name, code) == "v"
	}
	return m&^input.ModShift == input.ModControl && shortcutName(name, code) == "v" || m == input.ModShift && name == "Insert"
}

// keyPaste is what the browser's paste brings a program for a key of the program's
// own, as Ctrl+Shift+V in a terminal. A page may read what the user pastes, and asks
// them nothing, where reading the clipboard itself brings up a prompt, or a Paste
// button. So the program gets the key, the browser pastes on it too, and the text
// its paste event holds answers the program's [Window.Clipboard]. It can be a password,
// so it stays in memory no longer than it can answer.
type keyPaste struct {
	// w is the window that claimed the key, and reads the clipboard for it, and code
	// the key's place.
	w    *Window
	code string
	// due is how many paste events keys have yet to bring, until the key comes up.
	due int
	// at is when the key last went down, and came closes as the first event comes
	// and text holds what it pasted.
	at   time.Time
	came chan struct{}
	text string
	// read says the program has read the clipboard for it.
	read bool
	// command says the key was Command+V, whose text is typed where the program
	// doesn't read the clipboard, as the browser does without a claim. toType says it
	// is there to type still, and typed that it has been, so that a read of it from
	// here on brings nothing more.
	command, toType, typed bool
	// expiry forgets it, and typing types it.
	expiry, typing *time.Timer
}

// pasteKept is how long the text of a key's paste answers [Window.Clipboard], for a
// program that reads it a little after the key.
var pasteKept = 2 * time.Second

// pasteWait is how long [Window.Clipboard] waits for the browser's paste event, which
// comes as the browser ends the key.
var pasteWait = time.Second

// commandWait is how long a program has to read what Command+V pasted before it is
// typed. A key that follows types it first.
var commandWait = 200 * time.Millisecond

// claimEndWait is how long after a claim ends the paste of the key that ended it is
// still the program's. A key can move the focus to a text field, where the browser
// would paste as it ends that key, though the program has the key.
const claimEndWait = 100 * time.Millisecond

// expectPaste notes that w, which claims the keys, gets one the browser pastes on.
// Firefox pastes on Ctrl+Shift+V only into a field, so the mirror has the focus until
// it has pasted, where the canvas had it.
func (d *Driver) expectPaste(w *Window, code string, command bool) {
	if d.canvas.Equal(js.Global().Get("document").Get("activeElement")) {
		d.movingFocus, d.lentFocus = true, true
		d.text.Call("focus", map[string]any{"preventScroll": true})
		d.movingFocus = false
	}
	// The one before, held down or pressed again, is typed first.
	d.typePaste()
	d.mu.Lock()
	p := d.paste
	if p == nil || p.due == 0 || p.w != w || p.command != command {
		d.mu.Unlock()
		d.forgetPaste(nil)
		d.mu.Lock()
		p = &keyPaste{w: w, came: make(chan struct{}), command: command}
		d.paste = p
	}
	defer d.mu.Unlock()
	p.due++
	p.at, p.code = time.Now(), code
	if p.expiry != nil {
		p.expiry.Stop()
	}
	p.expiry = time.AfterFunc(pasteKept, func() { d.forgetPaste(p) })
}

// takeFocusBack has the canvas take the focus back from a mirror that had it for a
// paste, unless a window took text meanwhile.
func (d *Driver) takeFocusBack() {
	if !d.lentFocus {
		return
	}
	d.lentFocus = false
	if d.typingWindow() == nil && d.text.Equal(js.Global().Get("document").Get("activeElement")) {
		d.movingFocus = true
		d.canvas.Call("focus", map[string]any{"preventScroll": true})
		d.movingFocus = false
	}
}

// pasted takes the browser's paste event for a key the program got, and leaves any
// other, as from the browser's own menu, to the browser.
func (d *Driver) pasted(e js.Value) {
	defer d.takeFocusBack()
	d.mu.Lock()
	p := d.paste
	ours := p != nil && p.due > 0
	if ours {
		p.due--
	}
	d.mu.Unlock()
	if !ours {
		if d.lentFocus {
			// Into a mirror that only has the canvas's focus, where nobody types.
			e.Call("preventDefault")
		}
		return
	}
	// The program pastes it, or had the key do something else.
	e.Call("preventDefault")
	text := ""
	if data := e.Get("clipboardData"); data.Truthy() {
		text = data.Call("getData", "text/plain").String()
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	p.text, p.typed = text, false
	if p.command && !p.read {
		p.toType = true
		if p.typing != nil {
			p.typing.Stop()
		}
		p.typing = time.AfterFunc(commandWait, d.typePaste)
	}
	select {
	case <-p.came:
	default:
		close(p.came)
	}
}

// typePaste types what Command+V pasted, where the program has not read it. The paste
// stays as typed until its time is up, for a program that reads it late.
func (d *Driver) typePaste() {
	d.mu.Lock()
	p := d.paste
	if p == nil || !p.toType {
		d.mu.Unlock()
		return
	}
	text, w := p.text, p.w
	p.toType, p.typed, p.text = false, true, ""
	d.mu.Unlock()
	if text != "" {
		w.in.Push(input.TextInput{Text: fieldText(text), Time: time.Now()})
	}
}

// pastedText returns the text of the browser's paste on a key w got just now, waiting
// for it to come, and false where no such key went down. It is empty where the text has
// been typed, as reading the clipboard would paste it again.
func (d *Driver) pastedText(w *Window) (string, bool) {
	d.mu.Lock()
	p := d.paste
	if p == nil || p.w != w {
		d.mu.Unlock()
		return "", false
	}
	if p.typed {
		d.mu.Unlock()
		return "", true
	}
	p.read, p.toType = true, false
	d.mu.Unlock()
	t := time.NewTimer(pasteWait)
	defer t.Stop()
	select {
	case <-p.came:
		d.mu.Lock()
		defer d.mu.Unlock()
		return p.text, true
	case <-t.C:
		// The browser pasted nothing on the key.
		d.forgetPaste(p)
		return "", false
	}
}

// forgetPaste forgets p, or whatever paste there is for nil, as the clipboard holds
// something else now, or may, or its time is up.
func (d *Driver) forgetPaste(p *keyPaste) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.paste == nil || p != nil && d.paste != p {
		return
	}
	p, d.paste = d.paste, nil
	for _, t := range []*time.Timer{p.expiry, p.typing} {
		if t != nil {
			t.Stop()
		}
	}
	p.due, p.toType = 0, false
	select {
	case <-p.came:
		p.text = ""
	default:
	}
}

// pasteKeyUp ends the wait for a paste as its key comes up, by which the browser has
// pasted. A Mac sends no keyup for a key while Command is held, and Command's own
// stands in for it.
func (d *Driver) pasteKeyUp(name, code string) {
	d.mu.Lock()
	ours := d.paste != nil && (d.paste.code == code || d.paste.command && name == "Meta")
	d.mu.Unlock()
	if ours {
		d.endPasteKey()
	}
}

// endPasteKey has no paste event from here on be the program's. What it has been
// pasted stays its, and so does one it is waiting for.
func (d *Driver) endPasteKey() {
	d.mu.Lock()
	defer d.mu.Unlock()
	p := d.paste
	if p == nil {
		return
	}
	select {
	case <-p.came:
		p.due = 0
	default:
		if p.read {
			return
		}
		p.due = 0
		d.paste = nil
		p.expiry.Stop()
	}
}

// claimEnded ends w's claim on the paste of a key, once the browser has ended the key
// that may have ended the claim. What was to be typed there is let go, as the focus is
// elsewhere.
func (d *Driver) claimEnded(w *Window) {
	d.mu.Lock()
	p := d.paste
	if p == nil || p.w != w {
		d.mu.Unlock()
		return
	}
	p.command, p.toType = false, false
	d.mu.Unlock()
	time.AfterFunc(claimEndWait, func() {
		d.mu.Lock()
		same := d.paste == p
		d.mu.Unlock()
		if same {
			d.endPasteKey()
		}
	})
}

// clipboardChanges readies for the clipboard to hold something else: what Command+V
// pasted is typed now, and nothing pasted answers for it any more.
func (d *Driver) clipboardChanges() {
	d.typePaste()
	d.forgetPaste(nil)
}

// leftForAnother is clipboardChanges as the window loses the keyboard to another, which
// may put something on the clipboard. What has been typed still answers for its key.
func (d *Driver) leftForAnother() {
	d.typePaste()
	d.mu.Lock()
	typed := d.paste != nil && d.paste.typed
	d.mu.Unlock()
	if !typed {
		d.forgetPaste(nil)
	}
}

// lockedKeys are the keys a window that fills the screen and claims the keys asks the
// browser for, by their places: Escape, which otherwise leaves the full screen. Chrome
// hands a page in full screen its other shortcuts unasked, as Ctrl+W and Ctrl+Tab, and
// acts on those the page leaves it. One locked it acts on no more, so none of them is.
var lockedKeys = []any{"Escape"}

// lockKeys asks the browser for Escape once the program fills the screen and claims the
// keys, and gives it back as full screen ends. Meanwhile the focus can go where nothing
// claims them, as to a dialog, and back, and the lock stays: the browser can tell the
// user of a lock each time one is made. Escape then reaches the dialog, and the
// browser's own shortcuts are the browser's. Only Chrome and its kin have the Keyboard
// Lock API.
func (d *Driver) lockKeys() {
	full := d.fullScreen()
	claimed := full && d.claimed(d.keyWindow())
	d.mu.Lock()
	on := full && (claimed || d.keysLocked)
	same := on == d.keysLocked
	d.keysLocked = on
	d.mu.Unlock()
	kb := js.Global().Get("navigator").Get("keyboard")
	if same || !kb.Truthy() || kb.Get("lock").IsUndefined() {
		return
	}
	if !on {
		_, _ = call(kb, "unlock")
		return
	}
	asked("keeping the keys", nil, kb, "lock", lockedKeys)
}
