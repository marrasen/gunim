// Command widgets is a gallery of gunim's layout widgets: a header row,
// and a scrolling column of cards, each a row with a label that wraps
// and a button. The toggle switches between the dark and light themes,
// and every size, colour and motion animates to the new one.
//
//	CGO_ENABLED=0 go run ./example/widgets
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
	// Gallery is the state the gallery view renders.
	Gallery struct{ Items []string }
	// ThemeToggled travels when the user presses the theme button.
	ThemeToggled struct{}
	// Opened travels when the user opens an item.
	Opened struct{ Item string }
	// Confirm is the state of the confirm dialog.
	Confirm struct{ Title string }
	// Closed travels when the confirm dialog closes.
	Closed struct{}
)

func init() {
	gunim.RegisterType[Gallery]("gallery")
	gunim.RegisterType[ThemeToggled]("gallery.theme")
	gunim.RegisterType[Opened]("gallery.open")
	gunim.RegisterType[Confirm]("gallery.confirm")
	gunim.RegisterType[Closed]("gallery.closed")
}

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
			Title: "gunim widgets",
			Size:  geom.Sz(720, 560),
			Root:  widget.NewSurface(),
		})
		if err != nil {
			return err
		}
		registerViews(w)
		return serve(ctx, w.Client())
	})
}

// registerViews is the window half: the views and the themes.
func registerViews(w *gunim.Window) {
	w.RegisterTheme(widget.Dark())
	w.RegisterTheme(widget.Light())

	gunim.RegisterView(w, "gallery", buildGallery, nil)
	gunim.RegisterView(w, "confirm",
		func(s Confirm) *widget.Dialog {
			d := widget.NewDialog(s.Title)
			d.Accept, d.Dismiss = Closed{}, Closed{}
			return d
		}, nil)
}

// buildGallery lays the gallery out: a heading and a theme button over a
// scrolling column of cards.
func buildGallery(s Gallery) *widget.Pad {
	title := widget.NewLabel("Widgets")
	title.Size = widget.HeadingSize
	toggle := widget.NewButton("Switch theme")
	toggle.On = ThemeToggled{}
	spacer := widget.NewSpacer()
	header := widget.Row(title, spacer, toggle).Grow(spacer, 1)
	header.Cross = widget.CrossCenter

	cards := make([]gunim.Node, 0, len(s.Items))
	for _, item := range s.Items {
		label := widget.NewLabel(item)
		open := widget.NewButton("Open")
		open.On = Opened{Item: item}
		row := widget.Row(label, open).Grow(label, 1)
		row.Cross = widget.CrossCenter
		cards = append(cards, widget.NewCard(row))
	}
	items := widget.Column(cards...)
	items.Cross = widget.CrossStretch
	scroll := widget.NewScroll(items)

	page := widget.Column(header, scroll).Grow(scroll, 1)
	page.Cross = widget.CrossStretch
	return widget.NewPad(page)
}

// serve is the application half.
func serve(ctx context.Context, c gunim.Client) error {
	items := make([]string, 40)
	for i := range items {
		items[i] = fmt.Sprintf("Item %d. %s", i+1, blurb[i%len(blurb)])
	}
	if err := c.Mount(gunim.Root, "gallery", "gallery", Gallery{Items: items}); err != nil {
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
			case ThemeToggled:
				light = !light
				_ = c.SetTheme(map[bool]string{false: "dark", true: "light"}[light])
			case Opened:
				_ = c.Mount(gunim.Root, "confirm", "confirm", Confirm{Title: "Open " + v.Item})
				_ = c.Focus("confirm")
			case gunim.CommandFailed:
				log.Printf("command %s failed: %s", v.Command, v.Reason)
			}
		}
	}
}

var blurb = []string{
	"A short one.",
	"This one runs long enough that it wraps onto a second line when the window is narrow, and reflows as the window grows.",
	"Scroll with the wheel: the list glides to where the wheel sends it.",
	"Nothing here but a button.",
}
