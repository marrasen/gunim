// Command widgets is a gallery of gunim's widgets: a header row with a
// search field, and a scrolling list of cards, each a row with a label
// that wraps and a button. Typing in the field filters the cards, which
// collapse and grow back as they leave and return. The toggle switches
// between the dark and light themes, and every size, colour and motion
// animates to the new one. A row of icons shows them at 16 and 48 pixels,
// drawing themselves on, spinning, and in buttons, a link and a menu.
//
//	CGO_ENABLED=0 go run ./example/widgets
//	CGO_ENABLED=0 go run ./example/widgets -shot widgets.png
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"image/png"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/theme"
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
	shot := flag.String("shot", "", "write the window to this PNG file after -after, and quit")
	after := flag.Duration("after", 2*time.Second, "how long -shot waits")
	zoom := flag.Float64("zoom", 1, "zoom the window, as Ctrl with + and - does")
	flag.Parse()
	if err := run(*runFor, *shot, *after, float32(*zoom)); err != nil {
		log.Fatal(err)
	}
}

func run(runFor time.Duration, shot string, after time.Duration, zoom float32) error {
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
		if zoom != 1 {
			if err := w.Client().SetZoom(zoom); err != nil {
				return err
			}
		}
		return serve(ctx, w.Client(), shot, after)
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
	toggle.Icon = icon.SunMoon
	toggle.On = ThemeToggled{}
	spacer := widget.NewSpacer()
	header := widget.Row(title, spacer, search, widget.NewIconButton(icon.Filter, "Filter"),
		widget.NewIconButton(icon.Columns3, "Columns"), toggle).Grow(spacer, 1)
	header.Cross = widget.CrossCenter

	// The callout wears a theme of its own that sets only its colours,
	// so a theme switch still moves its padding and corners with the
	// rest of the page.
	callout := widget.NewThemed(
		widget.NewCard(widget.NewLabel("This card wears a theme of its own. Switch the theme: its colours stay, its padding and corners move with the page.")),
		theme.Make("callout",
			theme.Set(widget.CardFill, color.NRGBA{R: 0x2f, G: 0x6f, B: 0xe0, A: 0xff}),
			theme.Set(widget.Ink, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})))

	notes := widget.NewTextArea()
	notes.Rows = 3
	notes.Placeholder = "Notes: several lines, wrapped to the width"

	list := widget.NewList()
	scroll := widget.NewScroll(list)
	page := widget.Column(header, icons(), callout, notes, scroll).Grow(scroll, 1)
	page.Cross = widget.CrossStretch
	return &gallery{Pad: widget.NewPad(page), list: list}
}

// bigIcon is the size of the large icons in the gallery.
var bigIcon = theme.Length("gallery.icon.big", 48)

// icons lays out a card of icons: small ones, large ones that draw themselves on, a spinner, and icons in a link and
// a button that draws the large ones on again.
func icons() *widget.Card {
	row := make([]gunim.Node, 0, 16)
	for _, ic := range []*icon.Icon{icon.Database, icon.HardDrive, icon.ShieldAlert, icon.Target, icon.ScrollText,
		icon.Copy, icon.RefreshCw, icon.ChevronRight, icon.X, icon.Check} {
		row = append(row, widget.NewIcon(ic, ic.Name))
	}
	spin := widget.NewIcon(icon.Loader2, "Loading")
	spin.Spin = true
	big := make([]*widget.Icon, 0, 3)
	for _, ic := range []*icon.Icon{icon.TriangleAlert, icon.FolderOpen, icon.CircleCheck} {
		i := widget.NewIcon(ic, ic.Name)
		i.Size = bigIcon
		i.DrawOn(1200 * time.Millisecond)
		big = append(big, i)
		row = append(row, i)
	}
	again := widget.NewIconButton(icon.RotateCcw, "Draw the icons on again")
	again.OnActivate(func(*gunim.UI) {
		for _, i := range big {
			i.DrawOn(1200 * time.Millisecond)
		}
	})
	link := widget.NewLink("Open folder")
	link.Icon = icon.FolderOpen
	row = append(row, spin, again, link)
	r := widget.Row(row...)
	r.Cross = widget.CrossCenter
	return widget.NewCard(r)
}

// newCard makes the card for one item: its text, wrapping, and a button.
func newCard(item string) *widget.Card {
	label := widget.NewLabel(item)
	open := widget.NewButton("Open")
	open.Icon = icon.ExternalLink
	open.On = Opened{Item: item}
	row := widget.Row(label, open).Grow(label, 1)
	row.Cross = widget.CrossCenter
	return widget.NewCard(row)
}

// serve is the application half. Given a shot path, it writes the window there after a while and quits.
func serve(ctx context.Context, c gunim.Client, shot string, after time.Duration) error {
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
	var shoot <-chan time.Time
	if shot != "" {
		shoot = time.After(after)
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-shoot:
			if err := writeShot(ctx, c, shot); err != nil {
				return err
			}
			c.Close()
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

// writeShot writes what the window shows to a PNG file.
func writeShot(ctx context.Context, c gunim.Client, path string) error {
	img, err := c.Shot(ctx)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return errors.Join(png.Encode(f, img), f.Close())
}
