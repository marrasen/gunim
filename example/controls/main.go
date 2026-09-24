// Command controls shows gunim's controls on three tabs. The first has
// a drop-down, a context menu and a tooltip, each opening a popup
// window that can reach past the edge of the main one. The second has
// a checkbox, a switch and a slider. The third has a picture that
// crossfades to the next; the application draws the pictures and
// hands them to the window in its state, by reference.
//
//	CGO_ENABLED=0 go run ./example/controls
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	"log"
	"math"
	"os"
	"os/signal"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// The vocabulary the two halves share.
type (
	// Page is the state the page renders.
	Page struct {
		Status  string
		Picture *paint.Image
	}
	// Next travels when the picture button is pressed.
	Next struct{}
	// Toggled travels when a checkbox or switch flips.
	Toggled struct {
		Name string
		On   bool
	}
	// Slid travels as the slider moves.
	Slid struct{ Value float32 }
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
	gunim.RegisterType[Next]("controls.next")
	gunim.RegisterType[Toggled]("controls.toggled")
	gunim.RegisterType[Slid]("controls.slid")
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
			Size:  geom.Sz(560, 440),
			Root:  widget.NewSurface(),
		})
		if err != nil {
			return err
		}
		w.RegisterTheme(widget.Dark())
		w.RegisterTheme(widget.Light())
		gunim.RegisterView(w, "page", buildPage, func(p *page, s Page, u *gunim.UI) {
			p.status.Text = s.Status
			p.picture.SetSource(s.Picture, u)
		})
		return serve(ctx, w.Client())
	})
}

// page is the page view, with handles on what updates change.
type page struct {
	*widget.Pad
	status  *widget.Label
	picture *widget.Image
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
	popups := widget.Column(fruitRow, menu, widget.Row(tipped))
	popups.Cross = widget.CrossStretch

	check := widget.NewCheckbox("Send me the newsletter")
	check.OnChange = func(on bool) gunim.Intent { return Toggled{Name: "Newsletter", On: on} }
	sw := widget.NewSwitch("Dark mode")
	sw.On = true
	sw.OnChange = func(on bool) gunim.Intent { return Toggled{Name: "Dark mode", On: on} }
	slider := widget.NewSlider(0, 100)
	slider.Snap = 1
	slider.OnChange = func(v float32) gunim.Intent { return Slid{Value: v} }
	toggles := widget.Column(check, sw, widget.NewLabel("Volume"), slider)
	toggles.Cross = widget.CrossStretch

	picture := widget.NewImage(s.Picture)
	picture.Fit, picture.Radius, picture.Size = widget.FitCover, 10, geom.Sz(240, 150)
	next := widget.NewButton("Next picture")
	next.On = Next{}
	pictureRow := widget.Row(picture, next)
	pictureRow.Cross = widget.CrossEnd

	pad := func(n gunim.Node) gunim.Node {
		p := widget.NewPad(n)
		p.Padding = widget.CardPadding
		return p
	}
	tabs := widget.NewTabs([]string{"Popups", "Toggles", "Pictures"}, pad(popups), pad(toggles), pad(pictureRow))

	status := widget.NewLabel(s.Status)
	col := widget.Column(title, tabs, status).Grow(tabs, 1)
	col.Cross = widget.CrossStretch
	return &page{Pad: widget.NewPad(col), status: status, picture: picture}
}

// pictures draws a few pictures to page through: soft bands of colour
// in different hues.
func pictures() []*paint.Image {
	hues := []float64{0.58, 0.05, 0.33, 0.8}
	out := make([]*paint.Image, 0, len(hues))
	for _, hue := range hues {
		m := image.NewRGBA(image.Rect(0, 0, 480, 300))
		for y := range 300 {
			for x := range 480 {
				fx, fy := float64(x)/480, float64(y)/300
				v := 0.5 + 0.5*math.Sin(9*fx+4*math.Sin(5*fy+hue*6))
				r, g, b := hsv(hue+0.08*v, 0.55+0.3*fy, 0.45+0.5*v)
				i := m.PixOffset(x, y)
				m.Pix[i], m.Pix[i+1], m.Pix[i+2], m.Pix[i+3] = r, g, b, 0xff
			}
		}
		out = append(out, paint.NewImage(m))
	}
	return out
}

func hsv(h, s, v float64) (r, g, b uint8) {
	h = (h - math.Floor(h)) * 6
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h, 2)-1))
	var rf, gf, bf float64
	switch int(h) {
	case 0:
		rf, gf = c, x
	case 1:
		rf, gf = x, c
	case 2:
		gf, bf = c, x
	case 3:
		gf, bf = x, c
	case 4:
		rf, bf = x, c
	default:
		rf, bf = c, x
	}
	m := v - c
	return uint8((rf + m) * 255), uint8((gf + m) * 255), uint8((bf + m) * 255)
}

func serve(ctx context.Context, c gunim.Client) error {
	pics := pictures()
	state := Page{Status: "Nothing chosen yet.", Picture: pics[0]}
	if err := c.Mount(gunim.Root, "page", "page", state); err != nil {
		return err
	}
	shown := 0
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
				state.Status = fmt.Sprintf("Chose %s.", fruits[v.Fruit])
				_ = c.Update("page", state)
			case Picked:
				state.Status = fmt.Sprintf("Picked %s.", actions[v.Action])
				_ = c.Update("page", state)
			case Toggled:
				state.Status = fmt.Sprintf("%s is %s.", v.Name, map[bool]string{false: "off", true: "on"}[v.On])
				_ = c.Update("page", state)
				if v.Name == "Dark mode" {
					light = !v.On
					_ = c.SetTheme(map[bool]string{false: "dark", true: "light"}[light])
				}
			case Slid:
				state.Status = fmt.Sprintf("Volume %.0f.", v.Value)
				_ = c.Update("page", state)
			case Next:
				shown = (shown + 1) % len(pics)
				state.Picture = pics[shown]
				_ = c.Update("page", state)
			case ThemeToggled:
				light = !light
				_ = c.SetTheme(map[bool]string{false: "dark", true: "light"}[light])
			case gunim.CommandFailed:
				log.Printf("command %s failed: %s", v.Command, v.Reason)
			}
		}
	}
}
