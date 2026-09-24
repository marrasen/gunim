// Command widgets is a gallery of gunim's widgets: a header row with a
// search field, and a scrolling list of cards, each a row with a label
// that wraps and a button. Typing in the field filters the cards, which
// collapse and grow back as they leave and return. The toggle switches
// between the dark and light themes, and every size, colour and motion
// animates to the new one.
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
	"strings"
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
	// Filtered travels as the user types in the search field.
	Filtered struct{ Text string }
)

func init() {
	gunim.RegisterType[Gallery]("gallery")
	gunim.RegisterType[ThemeToggled]("gallery.theme")
	gunim.RegisterType[Opened]("gallery.open")
	gunim.RegisterType[Confirm]("gallery.confirm")
	gunim.RegisterType[Closed]("gallery.closed")
	gunim.RegisterType[Filtered]("gallery.filter")
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

	gunim.RegisterView(w, "gallery", buildGallery,
		func(g *gallery, s Gallery, u *gunim.UI) {
			// Cards are keyed by their text, so a filter that hides some
			// collapses them and one that brings them back grows them in.
			widget.Sync(g.list, u, s.Items,
				func(item string) widget.Key { return widget.Key(item) },
				newCard, nil)
		})
	gunim.RegisterView(w, "confirm",
		func(s Confirm) *widget.Dialog {
			d := widget.NewDialog(s.Title)
			d.Accept, d.Dismiss = Closed{}, Closed{}
			return d
		}, nil)
}

// gallery is the gallery view: a padded page, with a handle on the list
// of cards so updates can sync it.
type gallery struct {
	*widget.Pad
	list *widget.List
}

// buildGallery lays the gallery out: a heading, a search field and a
// theme button over a scrolling list of cards.
func buildGallery(Gallery) *gallery {
	title := widget.NewLabel("Widgets")
	title.Size = widget.HeadingSize
	search := widget.NewTextField()
	search.Placeholder = "Filter"
	search.OnChange = func(s string) gunim.Intent { return Filtered{Text: s} }
	toggle := widget.NewButton("Switch theme")
	toggle.On = ThemeToggled{}
	spacer := widget.NewSpacer()
	header := widget.Row(title, spacer, search, toggle).Grow(spacer, 1)
	header.Cross = widget.CrossCenter

	list := widget.NewList()
	scroll := widget.NewScroll(list)
	page := widget.Column(header, scroll).Grow(scroll, 1)
	page.Cross = widget.CrossStretch
	return &gallery{Pad: widget.NewPad(page), list: list}
}

// newCard makes the card for one item: its text, wrapping, and a button.
func newCard(item string) *widget.Card {
	label := widget.NewLabel(item)
	open := widget.NewButton("Open")
	open.On = Opened{Item: item}
	row := widget.Row(label, open).Grow(label, 1)
	row.Cross = widget.CrossCenter
	return widget.NewCard(row)
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
	filter := func(text string) Gallery {
		var out []string
		for _, item := range items {
			if strings.Contains(strings.ToLower(item), strings.ToLower(text)) {
				out = append(out, item)
			}
		}
		return Gallery{Items: out}
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
			case Filtered:
				_ = c.Update("gallery", filter(v.Text))
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
