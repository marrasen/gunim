//go:build js && wasm

package web_test

import (
	"context"
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver/web"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/widget"
)

// page is what the view shows.
type page struct{ Label string }

// frameSpy counts the animation frames the driver asks for, and when the last was.
type frameSpy struct{}

func spyOnFrames() frameSpy {
	js.Global().Call("eval", `(() => {
		const ask = window.requestAnimationFrame
		window.frameCount = 0
		window.frameAt = performance.now()
		window.requestAnimationFrame = f => { window.frameCount++; return ask.call(window, t => { window.frameAt = performance.now(); f(t) }) }
	})()`)
	return frameSpy{}
}

func (frameSpy) count() int { return js.Global().Get("frameCount").Int() }

// quietFor is how long the page has gone with no frame.
func (frameSpy) quietFor() time.Duration {
	ms := js.Global().Get("performance").Call("now").Float() - js.Global().Get("frameAt").Float()
	return time.Duration(ms * float64(time.Millisecond))
}

func element(name string) js.Value {
	return js.Global().Get("document").Call("querySelector", `#gunim-access [aria-label^="`+name+`"]`)
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for give := time.Now().Add(60 * time.Second); !ok(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(give) {
			t.Fatalf("waiting for %s", what)
		}
	}
}

// With the driver running real windows, through real frames and real widgets.
func TestThroughRealFrames(t *testing.T) {
	spy := spyOnFrames()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{Title: "Real", Size: geom.Sz(600, 400), Root: widget.NewSurface()})
		if err != nil {
			return err
		}
		w.RegisterTheme(widget.Dark())
		toggle := widget.NewButton("Bold")
		toggle.Active = true
		plain := widget.NewButton("Plain")
		seg := widget.NewSegmented("Left", "Right")
		seg.Tooltip = "Align"
		drop := widget.NewDropdown([]widget.MenuItem{{Label: "One"}, {Label: "Two", Checked: true}, {Label: "Three"}})
		drop.Label = "Number"
		label := widget.NewLabel("before")
		code := widget.NewCodeEditor()
		code.Label = "Code"
		long := strings.Repeat("the quick brown fox\n", 10000)
		gunim.RegisterView(w, "v", func(page) gunim.Node {
			return widget.Column(label, toggle, plain, seg, drop, widget.NewSized(code, 500, 60))
		}, func(_ gunim.Node, p page, u *gunim.UI) {
			label.Text = p.Label
			if code.Text() == "" {
				code.SetText(long, u)
			}
		})
		if err := w.Client().Mount(gunim.Root, "v", "v", page{"before"}); err != nil {
			return err
		}
		doc := js.Global().Get("document")
		has := func(name string) func() bool { return func() bool { return !element(name).IsNull() } }

		t.Run("the first tree waits for rest", func(t *testing.T) {
			start := time.Now()
			waitFor(t, "the first tree on the page", has("Bold"))
			if took := time.Since(start); took < 200*time.Millisecond {
				t.Errorf("the first tree came %v after the window opened", took)
			}
		})
		t.Run("handing over the tree wakes no frame", func(t *testing.T) {
			text := func() string {
				el := doc.Call("querySelector", "#gunim-access")
				return el.Get("textContent").String()
			}
			waitFor(t, "the page to rest", func() bool { return spy.quietFor() > 500*time.Millisecond })
			if err := w.Client().Update("v", page{"after"}); err != nil {
				t.Fatal(err)
			}
			before := spy.count()
			waitFor(t, "a frame for the change", func() bool { return spy.count() > before })
			waitFor(t, "the change on the page", func() bool { return strings.Contains(text(), "after") })
			if q := spy.quietFor(); q < 100*time.Millisecond {
				t.Errorf("the tree came %v after a frame", q)
			}
			at := spy.count()
			time.Sleep(600 * time.Millisecond)
			if got := spy.count(); got != at {
				t.Errorf("%d frames came after the tree, with nothing moving", got-at)
			}
		})
		t.Run("a long text stays off the layer", func(t *testing.T) {
			if got := len(element("Code").Get("textContent").String()); got > 5000 {
				t.Errorf("the code editor's element holds %d characters", got)
			}
		})
		t.Run("Chrome reads what the widgets say", func(t *testing.T) {
			ax := web.AskAX(t)
			checks := []struct{ role, name, prop, want string }{
				{"button", "Bold", "pressed", "true"},
				{"button", "Left", "pressed", "true"},
				{"button", "Right", "pressed", "false"},
				{"combobox", "Number: One", "expanded", "false"},
			}
			for _, c := range checks {
				n := ax.Find(c.role, c.name)
				if n == nil {
					t.Errorf("Chrome has no %s called %q", c.role, c.name)
					continue
				}
				if got, ok := n.Prop(c.prop); !ok || got != c.want {
					t.Errorf("Chrome's %s %q has %s %q, want %q", c.role, c.name, c.prop, got, c.want)
				}
			}
			if n := ax.Find("button", "Plain"); n != nil {
				if got, ok := n.Prop("pressed"); ok {
					t.Errorf("a plain button says pressed %q", got)
				}
			}
		})
		t.Run("a screen reader's click opens a menu, which says what's ticked", func(t *testing.T) {
			element("Number").Call("click")
			waitFor(t, "the menu's items on the page", has("Two"))
			ax := web.AskAX(t)
			n := ax.Find("menuitemcheckbox", "Two")
			if n == nil {
				t.Fatal("Chrome has no ticked item called Two")
			}
			if got, ok := n.Prop("checked"); !ok || got != "true" {
				t.Errorf("Chrome's ticked item has checked %q", got)
			}
			if ax.Find("menuitem", "One") == nil {
				t.Error("Chrome has no plain item called One")
			}
			// Keys move what's lit, which the canvas names.
			key := func(name string) {
				canvas := doc.Call("getElementById", "gunim")
				target := doc.Get("activeElement")
				if target.IsNull() || target.Equal(doc.Get("body")) {
					target = canvas
				}
				target.Call("dispatchEvent", js.Global().Get("KeyboardEvent").New("keydown", map[string]any{"bubbles": true, "cancelable": true, "key": name, "code": name}))
				target.Call("dispatchEvent", js.Global().Get("KeyboardEvent").New("keyup", map[string]any{"bubbles": true, "cancelable": true, "key": name, "code": name}))
			}
			key("ArrowDown")
			names := func() string {
				id := doc.Call("getElementById", "gunim").Call("getAttribute", "aria-activedescendant")
				if id.IsNull() {
					return ""
				}
				el := doc.Call("getElementById", id.String())
				if el.IsNull() {
					return "gone: " + id.String()
				}
				return el.Call("getAttribute", "aria-label").String()
			}
			waitFor(t, "a lit item for the canvas to name", func() bool { return names() != "" && !strings.HasPrefix(names(), "Number") })
			lit := names()
			key("Escape")
			waitFor(t, "the menu to leave the page", func() bool {
				el := element("Two")
				return el.IsNull() || el.Call("getClientRects").Length() == 0
			})
			waitFor(t, "the canvas to stop naming its item", func() bool { return names() != lit })
		})
		w.Client().Close()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
