package main

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// pathBarHeight is the path bar's height.
const pathBarHeight = 44

func registerPathBar(w *gunim.Window) {
	gunim.RegisterPatch(w, "browser", func(b *browser, s Banner, u *gunim.UI) { b.banner.set(s, u) })
}

// pathBar is the row under the title bar: back, forward and up, the
// folders of the path, and the filter.
type pathBar struct {
	b             *browser
	back, fwd, up *widget.IconButton
	crumbs        *crumbBar
	field         *pathField
	slot          *pathSlot
	filter        *filterField
	row           *widget.Flex
	path          string
}

func newPathBar(b *browser) *pathBar {
	p := &pathBar{b: b}
	p.back = newNavButton(icon.ArrowLeft, "Back (Alt+Left)", CmdBack)
	p.fwd = newNavButton(icon.ArrowRight, "Forward (Alt+Right)", CmdForward)
	p.up = newNavButton(icon.ArrowUp, "Up (Alt+Up)", CmdUp)
	p.crumbs = &crumbBar{bar: p}
	p.field = &pathField{TextField: widget.NewTextField(), bar: p}
	p.slot = newPathSlot(widget.NewThemed(p.crumbs, crumbTheme), p.field)
	p.filter = &filterField{TextField: widget.NewTextField(), bar: p}
	p.filter.Placeholder = "Filter this folder"
	p.filter.Icon, p.filter.Clearable = icon.Search, true
	p.filter.OnChange = func(s string) gunim.Intent { return FilterChanged{Text: s} }
	nav := func(b *widget.IconButton) gunim.Node { return widget.NewSized(b, navSize, navSize) }
	p.row = widget.Row(nav(p.back), nav(p.fwd), nav(p.up), p.slot, widget.NewSized(p.filter, 220, 0)).Grow(p.slot, 1)
	p.row.Cross = widget.CrossCenter
	p.row.Gap = smallGap
	return p
}

// smallGap is the gap between the path bar's parts.
var smallGap = theme.Length("files.gap.small", 4)

// navSize is the size of the back, forward and up buttons.
const navSize = 32

// newNavButton returns a button that sends cmd, which leaves the keyboard with the listing when clicked.
func newNavButton(ic *icon.Icon, tooltip, cmd string) *widget.IconButton {
	b := widget.NewIconButton(ic, tooltip)
	b.On, b.KeepFocus, b.Disabled = Command{Name: cmd}, true, true
	return b
}

// setListing shows the folder of l.
func (p *pathBar) setListing(l Listing, u *gunim.UI) {
	p.back.Disabled, p.fwd.Disabled, p.up.Disabled = !l.CanBack, !l.CanForward, !l.CanUp
	u.Invalidate()
	if l.Path != p.path {
		p.path = l.Path
		p.crumbs.set(l.Crumbs, u)
		p.filter.SetText(l.Filter)
	}
}

// edit turns the folders into a field holding the path, all selected.
func (p *pathBar) edit(u *gunim.UI) {
	p.field.SetText(p.path)
	p.field.Select(0, len([]rune(p.path)))
	p.slot.show(true, u)
	u.Focus(p.field)
}

// stopEdit turns the field back into the folders, and gives the keyboard
// back to the listing when refocus is set.
func (p *pathBar) stopEdit(u *gunim.UI, refocus bool) {
	p.slot.show(false, u)
	if refocus {
		p.b.focusListing(u)
	}
}

// Children implements [gunim.Composite].
func (p *pathBar) Children() []gunim.Node { return []gunim.Node{p.row} }

// Layout implements [gunim.Node].
func (p *pathBar) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	kid.Layout(gunim.Tight(geom.Sz(c.Max.W-16, pathBarHeight-10)))
	kid.Place(geom.Pt(8, 5))
	return geom.Sz(c.Max.W, pathBarHeight)
}

// Paint implements [gunim.Node].
func (p *pathBar) Paint(pt *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	pt.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(widget.Background.Get(f.Theme)))
	pt.RRect(geom.Rc(0, box.H-1, box.W, 1), 0, paint.Solid(widget.SplitLine.Get(f.Theme)))
	kids.At(0).Paint(pt)
}

// pathSlot shows the folders of the path, or the field that edits it,
// crossfading between them.
type pathSlot struct {
	anim.Group
	crumbs  gunim.Node
	field   *pathField
	editing bool
	mix     *anim.Float
}

func newPathSlot(c gunim.Node, f *pathField) *pathSlot {
	s := &pathSlot{crumbs: c, field: f, mix: anim.NewFloat(0)}
	s.Add(s.mix)
	return s
}

func (s *pathSlot) show(editing bool, u *gunim.UI) {
	s.editing = editing
	s.mix.Animate(map[bool]float32{false: 0, true: 1}[editing], widget.Quick.Get(u.Theme()))
	u.Invalidate()
}

// Children implements [gunim.Composite].
func (s *pathSlot) Children() []gunim.Node { return []gunim.Node{s.crumbs, s.field} }

// Layout implements [gunim.Node].
func (s *pathSlot) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	size := geom.Sz(c.Max.W, 34)
	for k := range kids.All {
		k.Layout(gunim.Tight(size))
		k.Place(geom.Point{})
	}
	return size
}

// Paint implements [gunim.Node]. Only the side showing is painted at
// rest, so only it takes the pointer.
func (s *pathSlot) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	t := min(max(s.mix.Value(), 0), 1)
	r := geom.Rect{Max: box.Point()}
	if t < 0.999 {
		func() {
			defer p.Layer(paint.LayerOpts{Bounds: r, Opacity: 1 - t})()
			kids.At(0).Paint(p)
		}()
	}
	if t > 0.001 {
		defer p.Layer(paint.LayerOpts{Bounds: r.Inset(geom.Uniform(-4)), Opacity: t})()
		kids.At(1).Paint(p)
	}
}

// pathField is the field that edits the path: Enter goes there, and
// Escape or a click elsewhere turns it back into the folders.
type pathField struct {
	*widget.TextField
	bar *pathBar
}

// Handle implements [gunim.Handler].
func (f *pathField) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.KeyPress:
		switch e.Key {
		case input.KeyEscape:
			f.bar.stopEdit(u, true)
			return true
		case input.KeyEnter, input.KeyKPEnter:
			u.Send(f, Navigate{Path: f.Text()})
			f.bar.stopEdit(u, true)
			return true
		default:
		}
	case input.FocusLost:
		f.TextField.Handle(e, u)
		f.bar.slot.show(false, u)
		return true
	}
	return f.TextField.Handle(e, u)
}

// filterField is the filter: Escape empties it, and Down goes to the
// listing.
type filterField struct {
	*widget.TextField
	bar *pathBar
}

// Handle implements [gunim.Handler].
func (f *filterField) Handle(e input.Event, u *gunim.UI) bool {
	if k, ok := e.(input.KeyPress); ok {
		switch k.Key {
		case input.KeyEscape:
			if f.Text() != "" {
				f.SetText("")
				u.Send(f, FilterChanged{})
			}
			f.bar.b.focusListing(u)
			return true
		case input.KeyDown, input.KeyEnter, input.KeyKPEnter:
			f.bar.b.focusListing(u)
			return true
		default:
		}
	}
	return f.TextField.Handle(e, u)
}

// crumbBar shows the folders along the path, each a link, with the last
// one at the right edge when they do not all fit. A click beside them
// edits the path.
type crumbBar struct {
	bar    *pathBar
	crumbs []*crumb
}

// set shows the folders of cs. The folders both paths share stay, and
// the rest leave and arrive.
func (c *crumbBar) set(cs []Crumb, u *gunim.UI) {
	keep := 0
	for keep < len(cs) && keep < len(c.crumbs) && c.crumbs[keep].path == cs[keep].Path {
		keep++
	}
	for _, old := range c.crumbs[keep:] {
		u.Remove(old)
	}
	c.crumbs = c.crumbs[:keep]
	for i := keep; i < len(cs); i++ {
		n := newCrumb(cs[i], c.bar)
		c.crumbs = append(c.crumbs, n)
		u.Insert(c, n)
	}
	for i, n := range c.crumbs {
		n.setLast(i == len(c.crumbs)-1)
	}
	u.Invalidate()
}

// Layout implements [gunim.Node].
func (c *crumbBar) Layout(cs gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	size := cs.Max
	total := float32(0)
	for k := range kids.All {
		if k.Presence() == gunim.Exiting {
			continue
		}
		total += k.Layout(gunim.Loose(size)).W
	}
	// When the folders overflow, the last ones show.
	x := min(float32(8), size.W-total-8)
	for k := range kids.All {
		s := k.Layout(gunim.Loose(size))
		if k.Presence() == gunim.Exiting {
			if cr, ok := k.Node().(*crumb); ok {
				k.Place(geom.Pt(cr.x, (size.H-s.H)/2))
			}
			continue
		}
		if cr, ok := k.Node().(*crumb); ok {
			cr.x = x
		}
		k.Place(geom.Pt(x, (size.H-s.H)/2))
		x += s.W
	}
	return size
}

// Paint implements [gunim.Node].
func (c *crumbBar) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	r := geom.Rect{Max: box.Point()}
	p.RRect(r, widget.FieldRadius.Get(f.Theme), paint.Solid(widget.FieldFill.Get(f.Theme)))
	defer p.Layer(paint.LayerOpts{Bounds: r, Opacity: 1, Clip: true, Radius: widget.FieldRadius.Get(f.Theme)})()
	for k := range kids.All {
		k.Paint(p)
	}
}

// Handle implements [gunim.Handler]: a press beside the folders edits
// the path, and the arrow keys, Home and End move between the folders.
func (c *crumbBar) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		c.bar.edit(u)
		return true
	case input.KeyPress:
		if e.Mods != 0 {
			return false
		}
		switch e.Key {
		case input.KeyLeft, input.KeyRight:
			u.FocusWithin(c, e.Key == input.KeyRight)
			return true
		case input.KeyHome, input.KeyEnd:
			for u.FocusWithin(c, e.Key == input.KeyEnd) {
			}
			return true
		default:
		}
	}
	return false
}

// TabGroup implements [gunim.TabGroup]: the folders are one stop for Tab.
func (c *crumbBar) TabGroup() {}

// Cursor implements [gunim.CursorShaper].
func (c *crumbBar) Cursor(geom.Point) input.Cursor { return input.CursorText }

// crumbTheme sizes the crumbs' buttons to fit the path bar.
var crumbTheme = theme.Make("files.crumbs",
	theme.Set(widget.ButtonHeight, 26),
	theme.Set(widget.ButtonPadding, 6),
	theme.Set(widget.ButtonRadius, 6),
)

// crumb is one folder of the path: a button that goes there, and a
// chevron after all but the last. The last one's button edits the path.
type crumb struct {
	anim.Group
	name, path string
	last       bool
	btn        *widget.Button
	// x is where the crumb was placed, and pill its button's width, at the last layout.
	x, pill float32
	in      *anim.Float
	sep     text.Run
	size    float32
}

func newCrumb(c Crumb, bar *pathBar) *crumb {
	n := &crumb{name: c.Name, path: c.Path, btn: widget.NewButton(c.Name), in: anim.NewFloat(0)}
	n.Add(n.in)
	n.btn.Ghost, n.btn.KeepFocus = true, true
	n.btn.OnActivate(func(u *gunim.UI) {
		if n.last {
			bar.edit(u)
		}
	})
	return n
}

// setLast makes the crumb the last, the folder showing, or one before it.
func (n *crumb) setLast(last bool) {
	n.last = last
	n.btn.Ink, n.btn.On = widget.Ink, nil
	if !last {
		n.btn.Ink, n.btn.On = Faint, Navigate{Path: n.path}
	}
}

// Children implements [gunim.Composite].
func (n *crumb) Children() []gunim.Node { return []gunim.Node{n.btn} }

// Transition implements [gunim.Transitioner]: a crumb slides in from the
// left and fades up, and fades as it leaves.
func (n *crumb) Transition(p gunim.Presence, f gunim.Frame) bool {
	switch p {
	case gunim.Entering:
		n.in.Animate(1, widget.Bounce.Get(f.Theme))
	case gunim.Exiting:
		n.in.Animate(0, widget.Quick.Get(f.Theme))
	case gunim.Present:
	}
	return !n.in.Active()
}

// Layout implements [gunim.Node].
func (n *crumb) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	if size := widget.TextSize.Get(f.Theme); n.size != size || len(n.sep.Glyphs) == 0 {
		n.size, n.sep = size, text.Default().Shape("›", size)
	}
	kid := kids.At(0)
	s := kid.Layout(gunim.Loose(c.Max))
	kid.Place(geom.Point{})
	n.pill = s.W
	w := s.W
	if !n.last {
		w += n.sep.Advance + 8
	}
	return c.Constrain(geom.Sz(w, s.H))
}

// Paint implements [gunim.Node].
func (n *crumb) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	t := min(max(n.in.Value(), 0), 1)
	if t <= 0.001 {
		return
	}
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}.Inset(geom.Uniform(-4)), Opacity: t})()
	defer p.Push(paint.Translate(geom.Pt(-10*(1-t), 0)))()
	kids.At(0).Paint(p)
	if !n.last {
		n.sep.Paint(p, geom.Pt(n.pill+4, (box.H-n.sep.Height())/2), Caption.Get(f.Theme))
	}
}

// Cursor implements [gunim.CursorShaper].
func (n *crumb) Cursor(p geom.Point) input.Cursor {
	if p.X > n.pill || n.last {
		return input.CursorText
	}
	return input.CursorHand
}

// bannerView is the line under the path bar that says what went wrong.
type bannerView struct {
	fold  *fold
	label *widget.Label
	seq   int
}

func newBannerView() *bannerView {
	b := &bannerView{label: widget.NewLabel("")}
	b.label.Color = ErrorInk
	dismiss := widget.NewLink("Dismiss")
	row := widget.Row(b.label, dismiss).Grow(b.label, 1)
	row.Cross = widget.CrossCenter
	b.fold = newFold(&bannerBox{child: row})
	dismiss.OnActivate(func(u *gunim.UI) { b.fold.set(false, u) })
	return b
}

func (b *bannerView) set(s Banner, u *gunim.UI) {
	if s.Seq < b.seq {
		return
	}
	b.seq = s.Seq
	if s.Text != "" {
		b.label.SetText(s.Text)
	}
	b.fold.set(s.Text != "", u)
}

// bannerBox pads the banner and paints its tinted background.
type bannerBox struct {
	child gunim.Node
}

// Children implements [gunim.Composite].
func (b *bannerBox) Children() []gunim.Node { return []gunim.Node{b.child} }

// Layout implements [gunim.Node].
func (b *bannerBox) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	s := kid.Layout(gunim.Constraints{Min: geom.Sz(c.Max.W-24, 0), Max: geom.Sz(c.Max.W-24, 0)})
	kid.Place(geom.Pt(12, 8))
	return geom.Sz(c.Max.W, s.H+16)
}

// Paint implements [gunim.Node].
func (b *bannerBox) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(ErrorFill.Get(f.Theme)))
	kids.At(0).Paint(p)
}
