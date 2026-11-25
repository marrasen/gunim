// Command secret shows a password field, and keeps the window out of screenshots while its button says so. Its other
// buttons copy the password, marked sensitive, and take it off the clipboard again.
//
//	go run ./tools/gunimapk -run -name Secret -permissions hide-overlays ./example/secret
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// secureIntent is the first button clicked.
type secureIntent struct{}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{Title: "gunim secret", Size: geom.Sz(360, 420), Root: widget.NewSurface()})
		if err != nil {
			return err
		}
		gunim.RegisterView(w, "secret", func(string) *secret { return newSecret() }, func(v *secret, s string, _ *gunim.UI) {
			v.text.Text = s
		})
		c := w.Client()
		if err := c.Mount(gunim.Root, "secret", "secret", "Screenshots show this window."); err != nil {
			return err
		}
		on := false
		for e := range c.Intents() {
			if _, ok := e.Intent.(secureIntent); !ok {
				continue
			}
			on = !on
			err := c.Secure(on)
			s := "Screenshots show this window."
			if on {
				s = "Screenshots show nothing of this window."
			}
			if err != nil {
				s += " " + err.Error()
			}
			log.Print(s)
			if err := c.Update("secret", s); err != nil {
				return err
			}
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

// secret is a password field and buttons, over what the window says.
type secret struct {
	field *widget.TextField
	kids  []gunim.Node
	text  *widget.Label
}

func newSecret() *secret {
	v := &secret{field: widget.NewTextField(), text: widget.NewLabel("")}
	v.field.Secret = true
	v.field.Placeholder = "A password"
	button := func(label string, do func(u *gunim.UI)) {
		b := widget.NewButton(label)
		b.OnClick = func(u *gunim.UI) gunim.Intent {
			do(u)
			return nil
		}
		v.kids = append(v.kids, b)
	}
	v.kids = append(v.kids, v.field)
	secure := widget.NewButton("Keep out of screenshots")
	secure.OnClick = widget.Sends(secureIntent{})
	v.kids = append(v.kids, secure)
	button("Copy as a secret", func(u *gunim.UI) {
		v.say(u, fmt.Sprintf("Copied, marked %v: %v", u.MarksClipboardSensitive(), u.SetClipboardSensitive(v.field.Text())))
	})
	button("Take off the clipboard", func(u *gunim.UI) {
		took, err := u.ClearClipboardIf(v.field.Text())
		v.say(u, fmt.Sprintf("Taken off: %v, %v", took, err))
	})
	v.kids = append(v.kids, v.text)
	return v
}

func (v *secret) say(u *gunim.UI, s string) {
	log.Print(s)
	v.text.Text = s
	u.Invalidate()
}

// Children implements [gunim.Composite].
func (v *secret) Children() []gunim.Node { return v.kids }

// Layout implements [gunim.Node]: one under the other. The surface under the view keeps it clear of the system's bars.
func (v *secret) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	box := c.Max
	const margin, rowH = 12, 48
	w := box.W - 2*margin
	y := float32(margin)
	for i := range v.kids {
		h := float32(rowH)
		if i == len(v.kids)-1 {
			h = max(box.H-y-margin, 0)
		}
		kids.At(i).Layout(gunim.Tight(geom.Sz(w, h)))
		kids.At(i).Place(geom.Pt(margin, y))
		y += rowH + margin
	}
	return box
}

// Paint implements [gunim.Node].
func (v *secret) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for kid := range kids.All {
		kid.Paint(p)
	}
}
