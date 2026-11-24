//go:build js && wasm

package web

import (
	"log"
	"syscall/js"

	"github.com/marrasen/gunim/driver"
)

// This file is what a window asks of the page it is in: to fill the screen, and to ask
// the user before it closes.

// asked calls a browser method that answers with a promise, for a request the browser
// may refuse and the program goes on without. The log says why it was refused, and
// then, where it is not nil, runs once the browser has answered.
func asked(what string, then func(), target js.Value, method string, args ...any) {
	p, err := call(target, method, args...)
	if err != nil {
		log.Printf("gunim: web: %s: %v", what, err)
	}
	if then == nil {
		then = func() {}
	}
	if err != nil || !p.Truthy() {
		then()
		return
	}
	go func() {
		if _, err := await(p, 0); err != nil {
			log.Printf("gunim: web: %s: %v", what, err)
		}
		then()
	}()
}

// SetFullScreen implements [driver.FullScreener]: the page fills the screen, and the
// canvas the page. The whole page does, as the text mirror, and what the page lays over
// the canvas, take no keyboard beside a canvas that fills the screen alone.
//
// The browser takes it only while it handles a key, a click or a tap of the user's, and
// answers a moment later, so FullScreen says how it went from the [driver.Redraw] it
// brings, and [driver.FullScreenLeft] that the user left or the browser refused. What
// is asked while the browser has yet to answer holds once it does. An iPhone's browser
// has none of it, and a window that does not fill the canvas takes no screen: those
// read as refused.
func (w *Window) SetFullScreen(on bool) {
	d := w.d
	doc := js.Global().Get("document")
	root := doc.Get("documentElement")
	if fills := w.fillsCanvas(); !fills || on && root.Get("requestFullscreen").IsUndefined() {
		if on {
			w.in.Push(driver.FullScreenLeft{})
		}
		return
	}
	d.mu.Lock()
	same := on == d.fullWant
	d.fullWant = on
	if on && !same {
		d.fullOurs = true
		d.fullPending++
	}
	d.mu.Unlock()
	switch {
	case same:
	case !on:
		// With a request the browser has yet to answer, its answer ends this.
		if d.fullScreen() {
			d.leaveFullScreen()
		}
	default:
		asked("full screen", func() {
			d.mu.Lock()
			d.fullPending--
			unwanted := d.fullPending == 0 && !d.fullWant
			d.mu.Unlock()
			if unwanted && d.fullScreen() {
				// The program changed its mind before the browser answered.
				d.leaveFullScreen()
			}
			d.fullScreenChanged()
		}, root, "requestFullscreen", map[string]any{"navigationUI": "hide"})
	}
}

// leaveFullScreen asks the browser to leave full screen. Asked twice while the browser
// has yet to leave, it leaves once.
func (d *Driver) leaveFullScreen() {
	asked("leaving full screen", d.fullScreenChanged, js.Global().Get("document"), "exitFullscreen")
}

// FullScreen implements [driver.FullScreener].
func (w *Window) FullScreen() bool { return w.fillsCanvas() && w.d.fullScreen() }

// fillsCanvas says the window fills the canvas, as popups don't.
func (w *Window) fillsCanvas() bool {
	w.d.mu.Lock()
	defer w.d.mu.Unlock()
	return w.fills
}

// fullScreen says the page fills the screen, as a window asked.
func (d *Driver) fullScreen() bool {
	d.mu.Lock()
	ours := d.fullOurs
	d.mu.Unlock()
	if !ours {
		return false
	}
	doc := js.Global().Get("document")
	return doc.Get("documentElement").Equal(doc.Get("fullscreenElement"))
}

// fullStyle has the canvas fill the page in full screen, wherever the page lays it out.
var fullStyle = map[string]string{
	"position": "fixed", "left": "0", "top": "0", "width": "100%", "height": "100%", "margin": "0",
	"min-width": "0", "min-height": "0", "max-width": "none", "max-height": "none",
}

// fullScreenChanged follows the page filling the screen or leaving it, and the browser
// refusing.
func (d *Driver) fullScreenChanged() {
	full := d.fullScreen()
	d.mu.Lock()
	settled := d.fullPending == 0
	left := !full && settled && d.fullWant
	if !full && settled {
		d.fullOurs, d.fullWant = false, false
	}
	pins, pinned := full && d.fullWant, d.styleBefore != nil
	changed := full != d.fullShown
	d.fullShown = full
	ws := append([]*Window(nil), d.windows...)
	d.mu.Unlock()
	if pins != pinned {
		if pins {
			d.pin()
		} else {
			d.unpin()
		}
	}
	d.lockKeys()
	if changed || pins != pinned {
		for _, w := range ws {
			w.in.Push(driver.Redraw{})
		}
		d.resized()
	}
	if left {
		// The screen is the page's, whichever window asked, which may have closed.
		for _, w := range ws {
			if w.fillsCanvas() {
				w.in.Push(driver.FullScreenLeft{})
			}
		}
	}
}

// pin has the canvas fill the page.
func (d *Driver) pin() {
	style := d.canvas.Get("style")
	was := map[string][2]string{}
	for name, value := range fullStyle {
		was[name] = [2]string{style.Call("getPropertyValue", name).String(), style.Call("getPropertyPriority", name).String()}
		style.Call("setProperty", name, value, "important")
	}
	d.mu.Lock()
	d.styleBefore = was
	d.mu.Unlock()
}

// unpin gives the canvas back the place the page has for it, if it was pinned.
func (d *Driver) unpin() {
	d.mu.Lock()
	was := d.styleBefore
	d.styleBefore = nil
	d.mu.Unlock()
	style := d.canvas.Get("style")
	for name, v := range was {
		style.Call("setProperty", name, v[0], v[1])
	}
}

// SetCloseAsks implements [driver.CloseAsker]: the browser asks the user before the
// page closes, reloads or goes elsewhere, in its own words, once they have clicked or
// typed in it.
func (w *Window) SetCloseAsks(on bool) {
	w.d.mu.Lock()
	w.closeAsks = on
	w.d.mu.Unlock()
	w.d.askBeforeClosing()
}

// askBeforeClosing has the browser ask before the page closes while some window wants
// it to. It listens only then, as a page that listens is one the browser throws away
// when the user leaves it, and loads anew when they come back.
func (d *Driver) askBeforeClosing() {
	d.mu.Lock()
	on := false
	for _, w := range d.windows {
		on = on || w.closeAsks
	}
	on = on && !d.stopped
	same := on == d.closeAsked
	d.closeAsked = on
	d.mu.Unlock()
	if same {
		return
	}
	if on {
		js.Global().Call("addEventListener", "beforeunload", d.closing)
	} else {
		js.Global().Call("removeEventListener", "beforeunload", d.closing)
	}
}

// makeClosing makes what hears the page closing.
func (d *Driver) makeClosing() {
	d.closing = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer survive("beforeunload")
		args[0].Call("preventDefault")
		// For browsers from before preventDefault said it.
		args[0].Set("returnValue", true)
		return nil
	})
}

var (
	_ driver.KeyClaimer   = (*Window)(nil)
	_ driver.CloseAsker   = (*Window)(nil)
	_ driver.FullScreener = (*Window)(nil)
)
