// Command controls shows gunim's controls: a drop-down, a context
// menu and tooltips, each opening a popup window that can reach past
// the edge of the main one.
//
//	CGO_ENABLED=0 go run ./example/controls
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/widget"
)

// The vocabulary the two halves share.
type (
	// Page is the state the page renders.
	Page struct{ Status string }
	// Chose travels when the drop-down changes.
	Chose struct{ Fruit int }
	// Picked travels when a context menu item is picked.
	Picked struct{ Action int }
	// ThemeToggled travels when the theme button is pressed.
	ThemeToggled struct{}
)

func init() {
	gunim.RegisterType[Page]("controls.page")
	gunim.RegisterType[Chose]("controls.chose")
	gunim.RegisterType[Picked]("controls.picked")
	gunim.RegisterType[ThemeToggled]("controls.theme")
}

var (
	fruits  = []string{"Apple", "Banana", "Cherry", "Dragon fruit", "Elderberry", "Fig", "Grape"}
	actions = []string{"Cut", "Copy", "Paste", "Select all"}
)

func main() {
	runFor := flag.Duration("for", 0, "quit after this long; zero runs until the window closes")
	flag.Parse()
	if err := run(*runFor); err != nil {
		log.Fatal(err)
	}
}

func run(runFor time.Duration) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, runFor)
		defer cancel()
	}
	return gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{
			Title: "gunim controls",
			Size:  geom.Sz(560, 420),
			Root:  widget.NewSurface(),
		})
		if err != nil {
			return err
		}
		w.RegisterTheme(widget.Dark())
		w.RegisterTheme(widget.Light())
		gunim.RegisterView(w, "page", buildPage, func(p *page, s Page, _ *gunim.UI) {
			p.status.Text = s.Status
		})
		return serve(ctx, w.Client())
	})
}

// page is the page view, with a handle on the status line.
type page struct {
	*widget.Pad
	status *widget.Label
}

func buildPage(s Page) *page {
	title := widget.NewLabel("Controls")
	title.Size = widget.HeadingSize

	fruit := widget.NewDropdown(fruits...)
	fruit.OnChange = func(i int) gunim.Intent { return Chose{Fruit: i} }
	fruitRow := widget.Row(widget.NewLabel("Fruit"), fruit)
	fruitRow.Cross = widget.CrossCenter

	area := widget.NewCard(widget.NewLabel("Right-click anywhere in this card for a context menu."))
	menu := widget.NewContextMenu(area, actions...)
	menu.OnPick = func(i int) gunim.Intent { return Picked{Action: i} }

	toggle := widget.NewButton("Switch theme")
	toggle.On = ThemeToggled{}
	tipped := widget.NewTooltip(toggle, "Switches between the dark and light themes")

	status := widget.NewLabel(s.Status)
	col := widget.Column(title, fruitRow, menu, widget.Row(tipped), status)
	col.Cross = widget.CrossStretch
	return &page{Pad: widget.NewPad(col), status: status}
}

func serve(ctx context.Context, c gunim.Client) error {
	if err := c.Mount(gunim.Root, "page", "page", Page{Status: "Nothing chosen yet."}); err != nil {
		return err
	}
	light := false
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-c.Intents():
			if !ok {
				return c.Err()
			}
			switch v := ev.Intent.(type) {
			case Chose:
				_ = c.Update("page", Page{Status: fmt.Sprintf("Chose %s.", fruits[v.Fruit])})
			case Picked:
				_ = c.Update("page", Page{Status: fmt.Sprintf("Picked %s.", actions[v.Action])})
			case ThemeToggled:
				light = !light
				_ = c.SetTheme(map[bool]string{false: "dark", true: "light"}[light])
			case gunim.CommandFailed:
				log.Printf("command %s failed: %s", v.Command, v.Reason)
			}
		}
	}
}
