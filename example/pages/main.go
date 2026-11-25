// Command pages stacks pages, as a phone's settings do. Back, the system's gesture or button, goes a page back and
// closes the dialog, and leaves the program from the first page. The text is as large as the user has it in the
// system's settings, and follows as they change it.
//
//	go run ./tools/gunimapk -run -name Pages ./example/pages
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
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// look is the theme for the user's settings, and what they are.
func look(l gunim.SystemLook, err error) (th theme.Theme, says string) {
	if err != nil {
		return widget.Dark().With(widget.Touch()...), fmt.Sprintf("The system's settings: %v", err)
	}
	base := widget.Light()
	if l.Dark {
		base = widget.Dark()
	}
	return widget.ForLook(base.With(widget.Touch()...), l),
		fmt.Sprintf("Text %.2f, dark %v, less motion %v", l.TextScale, l.Dark, l.LessMotion)
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	err := gunim.Main(ctx, func(a *gunim.App) error {
		th, says := look(a.SystemLook())
		log.Print(says)
		w, err := a.NewWindow(gunim.WindowOptions{
			Title: "gunim pages", Size: geom.Sz(420, 640), Root: widget.NewSurface(), Theme: th, LeaveOnBack: true,
		})
		if err != nil {
			return err
		}
		gunim.RegisterView(w, "pages", func(string) *pages { return newPages() }, func(v *pages, s string, _ *gunim.UI) { v.first.look.Text = s })
		c := w.Client()
		if err := c.Mount(gunim.Root, "pages", "pages", says); err != nil {
			return err
		}
		stop, err := a.WatchSystemLook(func(l gunim.SystemLook, err error) {
			th, says := look(l, err)
			log.Print(says)
			w.RegisterTheme(th)
			_ = c.SetTheme(th.Name)
			_ = c.Update("pages", says)
		})
		if err == nil {
			defer stop()
		}
		for range c.Intents() {
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

// pages is a stack of pages, the first of which shows the system's settings.
type pages struct {
	*widget.Stack
	first *page
}

func newPages() *pages {
	v := &pages{}
	v.first = v.page(1)
	v.Stack = widget.NewStack(v.first)
	v.OnPop = func(*gunim.UI) gunim.Intent {
		log.Printf("Back to page %d", v.Len())
		return nil
	}
	return v
}

// page is a page: a title, two buttons and a label for the system's settings.
type page struct {
	title, look    *widget.Label
	deeper, dialog *widget.Button
}

// page returns the page numbered n.
func (v *pages) page(n int) *page {
	p := &page{title: widget.NewLabel(fmt.Sprintf("Page %d", n)), look: widget.NewLabel(""),
		deeper: widget.NewButton("A page deeper"), dialog: widget.NewButton("A dialog")}
	p.deeper.OnClick = func(u *gunim.UI) gunim.Intent {
		log.Printf("On to page %d", n+1)
		v.Push(v.page(n+1), u)
		return nil
	}
	p.dialog.OnClick = func(u *gunim.UI) gunim.Intent {
		d := widget.NewDialog("Back closes this")
		d.OnDismiss = func(*gunim.UI) gunim.Intent {
			log.Print("The dialog closes")
			return nil
		}
		u.Insert(u.Root(), d)
		return nil
	}
	return p
}

// Children implements [gunim.Composite].
func (p *page) Children() []gunim.Node { return []gunim.Node{p.title, p.deeper, p.dialog, p.look} }

// Layout implements [gunim.Node]: one under another.
func (p *page) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	const margin = 16
	y := float32(margin)
	for kid := range kids.All {
		size := kid.Layout(gunim.Constraints{Max: geom.Sz(c.Max.W-2*margin, c.Max.H)})
		kid.Place(geom.Pt(margin, y))
		y += size.H + margin
	}
	return c.Max
}

// Paint implements [gunim.Node]: on a ground of its own, over the page below.
func (p *page) Paint(pt *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	pt.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(widget.Background.Get(f.Theme)))
	for kid := range kids.All {
		kid.Paint(pt)
	}
}
