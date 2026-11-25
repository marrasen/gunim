// Command confirm asks, with the system's own prompt, whether it is the user who holds the phone, and says what came of
// it.
//
//	go run ./tools/gunimapk -run -name Confirm -permissions confirm ./example/confirm
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// doIntent is a button clicked, by its label.
type doIntent struct{ what string }

// The buttons.
const (
	ask    = "Is it you?"
	strong = "No weak face or finger"
	short  = "Within five seconds"
	lock   = "Is there a screen lock?"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{Title: "gunim confirm", Size: geom.Sz(420, 640), Root: widget.NewSurface()})
		if err != nil {
			return err
		}
		gunim.RegisterView(w, "out", func(string) *out { return newOut() }, func(v *out, s string, _ *gunim.UI) { v.text.Text = s })
		c := w.Client()
		if err := c.Mount(gunim.Root, "out", "out", ""); err != nil {
			return err
		}
		var mu sync.Mutex
		lines := ""
		tell := func(format string, args ...any) {
			mu.Lock()
			defer mu.Unlock()
			s := fmt.Sprintf(format, args...)
			log.Print(s)
			lines = s + "\n" + lines
			_ = c.Update("out", lines)
		}
		for e := range c.Intents() {
			in, ok := e.Intent.(doIntent)
			if !ok {
				continue
			}
			o := gunim.ConfirmOptions{Title: "Open the notes", Text: "They are yours alone."}
			most := 2 * time.Minute
			switch in.what {
			case lock:
				on, err := a.HasScreenLock()
				tell("Screen lock: %v, %v", on, err)
				continue
			case strong:
				o.Strong = true
			case short:
				most = 5 * time.Second
			}
			// The window goes on meanwhile.
			go func() {
				ctx, cancel := context.WithTimeout(ctx, most)
				defer cancel()
				tell("%s %s", in.what, answer(a.ConfirmPerson(ctx, o)))
			}()
		}
		return c.Err()
	})
	if errors.Is(err, driver.ErrNoDriver) {
		log.Print("gunim has no driver for this operating system yet")
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// answer says what came of a prompt.
func answer(err error) string {
	switch {
	case err == nil:
		return "Yes."
	case errors.Is(err, gunim.ErrNotConfirmed):
		return "No."
	case errors.Is(err, gunim.ErrNoScreenLock):
		return "Set a screen lock in the system's settings first."
	case errors.Is(err, gunim.ErrConfirming):
		return "It is being asked already."
	}
	return err.Error()
}

// out is buttons over what came of them.
type out struct {
	buttons []gunim.Node
	text    *widget.Label
}

func newOut() *out {
	v := &out{text: widget.NewLabel("")}
	for _, what := range []string{ask, strong, short, lock} {
		b := widget.NewButton(what)
		b.OnClick = widget.Sends(doIntent{what})
		v.buttons = append(v.buttons, b)
	}
	return v
}

// Children implements [gunim.Composite].
func (v *out) Children() []gunim.Node { return append(append([]gunim.Node{}, v.buttons...), v.text) }

// Layout implements [gunim.Node]: the buttons on top and the text filling the rest. The surface under the view keeps it
// clear of the system's bars.
func (v *out) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	box := c.Max
	const margin, buttonH = 12, 48
	w := box.W - 2*margin
	y := float32(margin)
	for i := range v.buttons {
		kids.At(i).Layout(gunim.Tight(geom.Sz(w, buttonH)))
		kids.At(i).Place(geom.Pt(margin, y))
		y += buttonH + margin
	}
	last := kids.At(len(v.buttons))
	last.Layout(gunim.Tight(geom.Sz(w, max(box.H-y-margin, 0))))
	last.Place(geom.Pt(margin, y))
	return box
}

// Paint implements [gunim.Node].
func (v *out) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for kid := range kids.All {
		kid.Paint(p)
	}
}
