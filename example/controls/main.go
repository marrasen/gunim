// Command controls shows gunim's controls on four tabs. The first has
// a drop-down, a context menu and a tooltip, each opening a popup
// window that can reach past the edge of the main one. The second has
// a checkbox, a switch and a slider. The third has pictures: click one
// and it flies from its thumbnail to fill the window, and back when
// clicked again. The application draws the pictures and
// hands them to the window in its state, by reference. The fourth is a
// list of a hundred thousand items, of which only those in view are
// built; adding and removing items animates, and a drag flings it. The
// fifth is a list to put in order by dragging its rows.
//
// A second window, the basket, takes pictures dragged to it from the
// Pictures tab, and image files dropped on it from a file manager.
//
//	CGO_ENABLED=0 go run ./example/controls
package main

import (
	"context"
	"flag"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"log"
	"math"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// The vocabulary the two halves share.
type (
	// Page is the state the page renders.
	Page struct {
		Status   string
		Pictures []*paint.Image
		// Items are the long list's keys, and Tasks the order of the
		// list to arrange.
		Items []widget.Key
		Tasks []widget.Key
	}
	// Added travels when the long list's Add button is pressed.
	Added struct{}
	// Removed travels when an item's Remove button is pressed.
	Removed struct{ Item widget.Key }
	// Arranged travels when a task is dropped in a new place.
	Arranged struct{ Tasks []widget.Key }
	// Basket is the state of the basket window.
	Basket struct{ Items []Kept }
	// Kept is one picture in the basket.
	Kept struct {
		Key     widget.Key
		Picture *paint.Image
	}
	// Basketed travels when a picture is dropped in the basket: one of
	// the page's, by Index, or image files, by Paths.
	Basketed struct {
		Index int
		Paths []string
	}
	// pictureRef is what a thumbnail carries when it is dragged. It
	// stays inside the process, so it needs no registered name.
	pictureRef struct{ Index int }
	// Opened travels when a thumbnail is clicked.
	Opened struct{ Index int }
	// Closed travels when the open picture is clicked.
	Closed struct{}
	// Photo is the state of the open picture's view.
	Photo struct {
		Index   int
		Picture *paint.Image
	}
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
	gunim.RegisterType[Opened]("controls.opened")
	gunim.RegisterType[Closed]("controls.closed")
	gunim.RegisterType[Photo]("controls.photo")
	gunim.RegisterType[Toggled]("controls.toggled")
	gunim.RegisterType[Slid]("controls.slid")
	gunim.RegisterType[Added]("controls.added")
	gunim.RegisterType[Removed]("controls.removed")
	gunim.RegisterType[Arranged]("controls.arranged")
	gunim.RegisterType[Basket]("controls.basket")
	gunim.RegisterType[Basketed]("controls.basketed")
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
		gunim.RegisterView(w, "photo", newPhoto, nil)
		gunim.RegisterView(w, "page", buildPage, func(p *page, s Page, u *gunim.UI) {
			p.status.Text = s.Status
			p.items.SetKeys(s.Items, u)
			widget.Sync(p.tasks, u, s.Tasks, func(k widget.Key) widget.Key { return k }, newTask, nil)
		})
		b, err := a.NewWindow(gunim.WindowOptions{
			Title:  "gunim basket",
			Size:   geom.Sz(260, 440),
			Kind:   driver.KindUtility,
			Parent: w,
			Anchor: geom.Pt(580, 0),
			Root:   widget.NewSurface(),
		})
		if err != nil {
			return err
		}
		b.RegisterTheme(widget.Dark())
		b.RegisterTheme(widget.Light())
		gunim.RegisterView(b, "basket", buildBasket, func(k *basket, s Basket, u *gunim.UI) {
			widget.Sync(k.list, u, s.Items, func(i Kept) widget.Key { return i.Key }, newKept, nil)
		})
		return serve(ctx, w.Client(), b.Client())
	})
}

// page is the page view, with handles on what updates change.
type page struct {
	*widget.Pad
	status *widget.Label
	items  *widget.VirtualList
	tasks  *widget.List
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

	cells := make([]gunim.Node, 0, len(s.Pictures))
	for i, pic := range s.Pictures {
		img := widget.NewImage(pic)
		img.Fit, img.Radius, img.Size = widget.FitCover, 8, geom.Sz(112, 70)
		drag := widget.NewDraggable(widget.NewHero(heroTag(i), img), pictureRef{Index: i})
		drag.OnClick = Opened{Index: i}
		drag.Ghost = func() gunim.Node {
			ghost := widget.NewImage(pic)
			ghost.Fit, ghost.Radius, ghost.Size = widget.FitCover, 8, geom.Sz(112, 70)
			return ghost
		}
		cells = append(cells, drag)
	}
	thumbs := widget.Row(cells...)
	pictureRow := widget.Column(thumbs, widget.NewLabel("Click a picture to open it, or drag it to the basket."))

	pad := func(n gunim.Node) gunim.Node {
		p := widget.NewPad(n)
		p.Padding = widget.CardPadding
		return p
	}
	items := widget.NewVirtualList(func(k widget.Key) gunim.Node {
		label := widget.NewLabel("Item " + string(k))
		remove := widget.NewButton("Remove")
		remove.On = Removed{Item: k}
		row := widget.Row(label, remove).Grow(label, 1)
		row.Cross = widget.CrossCenter
		return widget.NewCard(row)
	})
	add := widget.NewButton("Add at the top")
	add.On = Added{}
	items.DragScroll = true
	long := widget.Column(widget.Row(add), items).Grow(items, 1)
	long.Cross = widget.CrossStretch

	tasks := widget.NewList()
	tasks.Reorder = func(keys []widget.Key) gunim.Intent { return Arranged{Tasks: keys} }
	arrange := widget.Column(widget.NewLabel("Drag the tasks into order."), tasks)
	arrange.Cross = widget.CrossStretch

	tabs := widget.NewTabs([]string{"Popups", "Toggles", "Pictures", "Long list", "Arrange"},
		pad(popups), pad(toggles), pad(pictureRow), pad(long), pad(arrange))

	status := widget.NewLabel(s.Status)
	col := widget.Column(title, tabs, status).Grow(tabs, 1)
	col.Cross = widget.CrossStretch
	return &page{Pad: widget.NewPad(col), status: status, items: items, tasks: tasks}
}

// newTask makes the row for a task: its name on a card.
func newTask(k widget.Key) *widget.Card { return widget.NewCard(widget.NewLabel(string(k))) }

func heroTag(i int) string { return "picture-" + strconv.Itoa(i) }

// photo is the open picture: the window dims, and the picture, a hero
// with its thumbnail's tag, flies in to fill most of it. A click
// anywhere closes it.
type photo struct {
	anim.Group
	fade *anim.Float
	hero *widget.Hero
}

func newPhoto(s Photo) *photo {
	img := widget.NewImage(s.Picture)
	img.Fit, img.Radius = widget.FitCover, 14
	p := &photo{fade: anim.NewFloat(0), hero: widget.NewHero(heroTag(s.Index), img)}
	p.Add(p.fade)
	return p
}

func (p *photo) Children() []gunim.Node { return []gunim.Node{p.hero} }

func (p *photo) Transition(pr gunim.Presence, f gunim.Frame) bool {
	switch pr {
	case gunim.Entering:
		p.fade.Animate(1, widget.Settle.Get(f.Theme))
	case gunim.Exiting:
		p.fade.Animate(0, widget.Settle.Get(f.Theme))
	case gunim.Present:
	}
	return !p.fade.Active()
}

func (p *photo) Handle(e input.Event, u *gunim.UI) bool {
	if _, ok := e.(input.PointerDown); ok {
		u.Send(p, Closed{})
		return true
	}
	return false
}

func (p *photo) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	box := c.Max
	// As large as fits in nine tenths of the window, at the pictures'
	// shape of 480 by 300.
	s := min(box.W*0.9/480, box.H*0.9/300)
	size := geom.Sz(480*s, 300*s)
	k := kids.At(0)
	k.Layout(gunim.Tight(size))
	k.Place(geom.Pt((box.W-size.W)/2, (box.H-size.H)/2))
	return box
}

func (p *photo) Paint(pt *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	dim := widget.Scrim.Get(f.Theme)
	dim.A = uint8(float32(dim.A) * min(max(p.fade.Value(), 0), 1))
	pt.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(dim))
	kids.At(0).Paint(pt)
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

func serve(ctx context.Context, c, bc gunim.Client) error {
	pics := pictures()
	items := make([]widget.Key, 100_000)
	for i := range items {
		items[i] = widget.Key(strconv.Itoa(i + 1))
	}
	added := len(items)
	state := Page{Status: "Nothing chosen yet.", Pictures: pics, Items: items,
		Tasks: []widget.Key{"Water the plants", "Answer the letters", "Bake the bread", "Mend the fence", "Read a chapter"}}
	if err := c.Mount(gunim.Root, "page", "page", state); err != nil {
		return err
	}
	var kept Basket
	if err := bc.Mount(gunim.Root, "basket", "basket", kept); err != nil {
		return err
	}
	light := false
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-bc.Intents():
			if !ok {
				return bc.Err()
			}
			v, ok := ev.Intent.(Basketed)
			if !ok {
				continue
			}
			var add []*paint.Image
			if v.Index >= 0 {
				add = append(add, pics[v.Index])
			}
			for _, path := range v.Paths {
				if !isImage(path) {
					continue
				}
				pic, err := load(path)
				if err != nil {
					log.Print(err)
					continue
				}
				add = append(add, pic)
			}
			for _, pic := range add {
				kept.Items = append(kept.Items, Kept{Key: widget.Key(strconv.Itoa(len(kept.Items) + 1)), Picture: pic})
			}
			_ = bc.Update("basket", kept)
			state.Status = fmt.Sprintf("Pictures in the basket: %d.", len(kept.Items))
			_ = c.Update("page", state)
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
			case Added:
				added++
				k := widget.Key(strconv.Itoa(added))
				state.Items = append([]widget.Key{k}, state.Items...)
				state.Status = fmt.Sprintf("Added item %s.", k)
				_ = c.Update("page", state)
			case Removed:
				state.Items = slices.DeleteFunc(slices.Clone(state.Items), func(k widget.Key) bool { return k == v.Item })
				state.Status = fmt.Sprintf("Removed item %s; %d left.", v.Item, len(state.Items))
				_ = c.Update("page", state)
			case Arranged:
				state.Tasks = v.Tasks
				state.Status = fmt.Sprintf("First up: %s.", v.Tasks[0])
				_ = c.Update("page", state)
			case Opened:
				_ = c.Mount(gunim.Root, "photo", "photo", Photo{Index: v.Index, Picture: pics[v.Index]})
			case Closed:
				_ = c.Unmount("photo")
			case ThemeToggled:
				light = !light
				_ = c.SetTheme(map[bool]string{false: "dark", true: "light"}[light])
			case gunim.CommandFailed:
				log.Printf("command %s failed: %s", v.Command, v.Reason)
			}
		}
	}
}

// basket is the basket window's view: a list of kept pictures that
// takes drops.
type basket struct {
	*widget.DropTarget
	list *widget.List
}

func buildBasket(Basket) *basket {
	list := widget.NewList()
	hint := widget.NewLabel("Drag pictures here, from the Pictures tab or from a file manager.")
	scroll := widget.NewScroll(list)
	col := widget.Column(hint, scroll).Grow(scroll, 1)
	col.Cross = widget.CrossStretch
	target := widget.NewDropTarget(widget.NewPad(col))
	target.Accept = func(data any, paths []string) bool {
		if _, ok := data.(pictureRef); ok {
			return true
		}
		return slices.ContainsFunc(paths, isImage)
	}
	target.OnDrop = func(d input.Drop) gunim.Intent {
		if r, ok := d.Data.(pictureRef); ok {
			return Basketed{Index: r.Index}
		}
		return Basketed{Index: -1, Paths: d.Paths}
	}
	return &basket{DropTarget: target, list: list}
}

// newKept makes a kept picture's row.
func newKept(k Kept) *widget.Image {
	img := widget.NewImage(k.Picture)
	img.Fit, img.Radius, img.Size = widget.FitCover, 8, geom.Sz(220, 120)
	return img
}

func isImage(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif":
		return true
	}
	return false
}

// load reads an image file.
func load(path string) (*paint.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	m, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return paint.NewImage(m), nil
}
