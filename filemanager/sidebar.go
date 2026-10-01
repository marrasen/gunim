package filemanager

import (
	"image/color"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

func registerSidebar(w *gunim.Window) {
	gunim.RegisterPatch(w, "browser", func(b *browser, s Places, u *gunim.UI) { b.side.set(s, u) })
}

// sidebar lists the places to go: the user's folders and the volumes,
// under headings of their groups after the first, then the favourites,
// which a drag puts in order.
type sidebar struct {
	places *widget.List
	favs   *widget.List
	hint   *widget.Label
	// first is the heading of the first group of places.
	first  *widget.Label
	col    *widget.Flex
	scroll *widget.Scroll
	// group makes the places and the favourites one stop for Tab, walked with Up and Down.
	group *widget.Group
	// fs is the ID of the window's file system, and ps how it writes
	// paths.
	fs string
	ps PathStyle
	// items holds the rows of both lists by their keys.
	items map[widget.Key]placeItem
}

// placeItem is a place and whether it is the folder showing. A heading
// is a row that names the group of the places after it.
type placeItem struct {
	Place
	current bool
	heading bool
	// away is set for a place on another file system than the window's.
	away bool
}

// key is the row's key in its list: its path, for a place on the
// window's file system, as the drops and the favourites need.
func (i placeItem) key() widget.Key {
	switch {
	case i.heading:
		return widget.Key("\x00group\x00" + i.Group)
	case i.away:
		return widget.Key("\x00" + i.FS + "\x00" + i.Path)
	}
	return widget.Key(i.Path)
}

// goes says where a click on the row with key k goes: a place on the
// window's file system, one on another, or for a heading nowhere.
func (s *sidebar) goes(k widget.Key) gunim.Intent {
	i, ok := s.items[k]
	switch {
	case !ok:
		return Navigate{Path: string(k)}
	case i.heading:
		return nil
	case i.away:
		return Visit{FS: i.FS, Path: i.Path}
	}
	return Navigate{Path: i.Path}
}

// sideSpacing is the gap between the sidebar's rows.
var sideSpacing = theme.Length("files.side.spacing", 2)

func newSidebar() *sidebar {
	s := &sidebar{places: widget.NewList(), favs: widget.NewList(), items: map[widget.Key]placeItem{}}
	s.places.OnClick = s.goes
	s.favs.OnClick = s.places.OnClick
	s.favs.Reorder = func(keys []widget.Key) gunim.Intent {
		paths := make([]string, len(keys))
		for i, k := range keys {
			paths[i] = string(k)
		}
		return FavouritesReordered{Paths: paths}
	}
	s.hint = widget.NewLabel("Pin a folder here with Ctrl+D.")
	s.hint.Color = Caption
	s.hint.Size = SmallText
	places, favs := widget.NewLabel("PLACES"), widget.NewLabel("FAVOURITES")
	for _, l := range []*widget.Label{places, favs} {
		l.Color, l.Size, l.Face = Caption, SmallText, widget.BoldFont
	}
	s.first = places
	s.col = widget.Column(places, s.places, widget.NewSized(widget.NewSpacer(), 0, 6), favs, s.favs, s.hint)
	s.col.Cross = widget.CrossStretch
	s.col.Gap = sideSpacing
	s.scroll = widget.NewScroll(&sidePad{child: widget.NewThemed(s.col, sideTheme())})
	s.group = widget.NewGroup(widget.Vertical, s.scroll)
	return s
}

func (s *sidebar) set(p Places, u *gunim.UI) {
	clear(s.items)
	items := func(ps []Place, headed bool) []placeItem {
		out := make([]placeItem, 0, len(ps))
		for i, pl := range ps {
			if headed && i > 0 && pl.Group != ps[i-1].Group {
				out = append(out, placeItem{Place: Place{Name: strings.ToUpper(pl.Group), Group: pl.Group}, heading: true})
			}
			away := pl.FS != s.fs
			out = append(out, placeItem{Place: pl, away: away, current: !away && s.ps.Same(pl.Path, p.Current)})
		}
		for _, it := range out {
			s.items[it.key()] = it
		}
		return out
	}
	first := "PLACES"
	if len(p.Places) > 0 && p.Places[0].Group != "" {
		first = strings.ToUpper(p.Places[0].Group)
	}
	s.first.SetText(first)
	key := placeItem.key
	widget.Sync(s.places, u, items(p.Places, true), key, newPlaceRow, (*placeRow).set)
	widget.Sync(s.favs, u, items(p.Favourites, false), key, newPlaceRow, (*placeRow).set)
	if len(p.Favourites) == 0 {
		s.hint.SetText("Pin a folder here with Ctrl+D.")
	} else {
		s.hint.SetText("")
	}
}

// Children implements [gunim.Composite].
func (s *sidebar) Children() []gunim.Node { return []gunim.Node{s.group} }

// Layout implements [gunim.Node].
func (s *sidebar) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	kid.Layout(gunim.Tight(c.Max))
	kid.Place(geom.Point{})
	return c.Max
}

// Paint implements [gunim.Node].
func (s *sidebar) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(SidebarFill.Get(f.Theme)))
	kids.At(0).Paint(p)
}

// sidePad leaves room around the sidebar's rows.
type sidePad struct {
	child gunim.Node
}

// Children implements [gunim.Composite].
func (s *sidePad) Children() []gunim.Node { return []gunim.Node{s.child} }

// Layout implements [gunim.Node].
func (s *sidePad) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	size := kid.Layout(gunim.Constraints{Min: geom.Sz(c.Max.W-16, 0), Max: geom.Sz(c.Max.W-16, 0)})
	kid.Place(geom.Pt(8, 12))
	return geom.Sz(c.Max.W, size.H+24)
}

// Paint implements [gunim.Node].
func (s *sidePad) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}

// placeRow is one place in the sidebar: a mark in its colour, its name,
// and for a volume a bar of how full it is.
type placeRow struct {
	anim.Group
	item  placeItem
	hover *anim.Float
	on    *anim.Float
	used  *anim.Float
	name  text.Run
	note  text.Run
	shown string
	size  float32
}

func newPlaceRow(i placeItem) *placeRow {
	r := &placeRow{item: i, hover: anim.NewFloat(0), on: anim.NewFloat(0), used: anim.NewFloat(0)}
	r.Add(r.hover, r.on, r.used)
	if i.current {
		r.on.Jump(1)
	}
	return r
}

func (r *placeRow) set(i placeItem, u *gunim.UI) {
	r.item = i
	r.on.Animate(map[bool]float32{false: 0, true: 1}[i.current], widget.Quick.Get(u.Theme()))
	r.shown = ""
	u.Invalidate()
}

// note is the small line under a volume's name, or a place's note.
func (r *placeRow) noteText() string {
	switch {
	case r.item.heading:
		return ""
	case r.item.Err != "":
		return r.item.Err
	case r.item.Total > 0:
		return humanBytes(int64(r.item.Free)) + " free of " + humanBytes(int64(r.item.Total))
	}
	return r.item.Note
}

func (r *placeRow) shape(th *theme.Live) {
	size := widget.TextSize.Get(th)
	key := r.item.Name + "\x00" + r.noteText()
	if key == r.shown && size == r.size {
		return
	}
	r.shown, r.size = key, size
	face := text.Default()
	if r.item.heading {
		// A heading looks as the Places and Favourites labels do.
		size = SmallText.Get(th)
		if f := widget.BoldFont.Get(th); f != nil {
			face = f
		}
	}
	r.name = face.Shape(r.item.Name, size)
	r.note = text.Default().Shape(r.noteText(), SmallText.Get(th))
}

// tall reports whether the row has a second line.
func (r *placeRow) tall() bool { return r.noteText() != "" }

// headingGap is the room above a heading, as above the heading of the
// favourites.
const headingGap = 8

// Layout implements [gunim.Node].
func (r *placeRow) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	r.shape(f.Theme)
	if r.item.heading {
		return geom.Sz(c.Max.W, headingGap+r.name.Height())
	}
	h := float32(30)
	if r.tall() {
		h = 48
	}
	if r.item.Total > 0 {
		r.used.Animate(1-float32(float64(r.item.Free)/float64(r.item.Total)), widget.Settle.Get(f.Theme))
	}
	return geom.Sz(c.Max.W, h)
}

// Paint implements [gunim.Node].
func (r *placeRow) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	if r.item.heading {
		r.name.Paint(p, geom.Pt(0, headingGap), Caption.Get(th))
		return
	}
	full := geom.Rect{Max: box.Point()}
	if h := r.hover.Value(); h > 0.01 {
		c := SidebarHot.Get(th)
		c.A = uint8(float32(c.A) * min(h, 1))
		p.RRect(full, 7, paint.Solid(c))
	}
	if on := r.on.Value(); on > 0.01 {
		c := SidebarOn.Get(th)
		c.A = uint8(float32(c.A) * min(on, 1))
		p.RRect(full, 7, paint.Solid(c))
		// The accent edge grows from the middle.
		bar := geom.Rc(0, box.H/2*(1-on)+4*on, 3, (box.H-8)*on)
		p.RRect(bar, 1.5, paint.Solid(widget.Accent.Get(th)))
	}
	mark := placeTint(r.item.Kind).Get(th)
	line := float32(30)
	p.RRect(geom.Rc(12, (line-12)/2, 12, 12), 3.5, paint.Solid(mark))
	ink := widget.Ink.Get(th)
	r.name.Paint(p, geom.Pt(34, (line-r.name.Height())/2), ink)
	if !r.tall() {
		return
	}
	noteInk := Faint.Get(th)
	if r.item.Err != "" {
		noteInk = ErrorInk.Get(th)
	}
	y := line - 2
	if r.item.Total > 0 {
		track := geom.Rc(34, y, box.W-46, 4)
		p.RRect(track, 2, paint.Solid(widget.ProgressTrack.Get(th)))
		used := min(max(r.used.Value(), 0), 1)
		fill := widget.Accent.Get(th)
		if used > 0.9 {
			fill = ErrorInk.Get(th)
		}
		p.RRect(geom.Rc(track.Min.X, y, track.Size().W*used, 4), 2, paint.Solid(fill))
		y += 6
	}
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rc(34, y, box.W-40, box.H-y), Opacity: 1, Clip: true})()
	r.note.Paint(p, geom.Pt(34, y), noteInk)
}

// placeTint is the colour of a place's mark.
func placeTint(kind string) theme.Token[color.NRGBA] {
	switch kind {
	case "drive":
		return tintToken(TintOther)
	case "favourite":
		return widget.Accent
	case "pictures":
		return tintToken(TintImage)
	case "documents":
		return tintToken(TintDocument)
	case "downloads":
		return tintToken(TintCode)
	}
	return tintToken(TintFolder)
}

// Handle implements [gunim.Handler]: the row lights under the pointer,
// and leaves presses to the list, which tells a click from a drag.
func (r *placeRow) Handle(e input.Event, u *gunim.UI) bool {
	if r.item.heading {
		return false
	}
	switch e.(type) {
	case input.PointerEnter:
		r.hover.Animate(1, widget.Quick.Get(u.Theme()))
	case input.PointerLeave:
		r.hover.Animate(0, widget.Settle.Get(u.Theme()))
	}
	return false
}

// Cursor implements [gunim.CursorShaper].
func (r *placeRow) Cursor(geom.Point) input.Cursor {
	if r.item.heading {
		return input.CursorArrow
	}
	return input.CursorHand
}
