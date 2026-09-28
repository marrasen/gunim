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
	back, fwd, up *navButton
	crumbs        *crumbBar
	field         *pathField
	slot          *pathSlot
	filter        *filterField
	row           *widget.Flex
	path          string
}

func newPathBar(b *browser) *pathBar {
	p := &pathBar{b: b}
	p.back = newNavButton("←", CmdBack)
	p.fwd = newNavButton("→", CmdForward)
	p.up = newNavButton("↑", CmdUp)
	p.crumbs = &crumbBar{bar: p}
	p.field = &pathField{TextField: widget.NewTextField(), bar: p}
	p.slot = newPathSlot(p.crumbs, p.field)
	p.filter = &filterField{TextField: widget.NewTextField(), bar: p}
	p.filter.Placeholder = "Filter this folder"
	p.filter.Icon, p.filter.Clearable = icon.Search, true
	p.filter.OnChange = func(s string) gunim.Intent { return FilterChanged{Text: s} }
	p.row = widget.Row(p.back, p.fwd, p.up, p.slot, widget.NewSized(p.filter, 220, 0)).Grow(p.slot, 1)
	p.row.Cross = widget.CrossCenter
	p.row.Gap = smallGap
	return p
}

// smallGap is the gap between the path bar's parts.
var smallGap = theme.Length("files.gap.small", 4)

// setListing shows the folder of l.
func (p *pathBar) setListing(l Listing, u *gunim.UI) {
	p.back.enable(l.CanBack, u)
	p.fwd.enable(l.CanForward, u)
	p.up.enable(l.CanUp, u)
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

// navButton is a round button with an arrow on it, dimmed while there
// is nowhere for it to go.
type navButton struct {
	anim.Group
	glyph string
	cmd   string
	on    bool
	lit   *anim.Float
	hover *anim.Float
	press *anim.Float
	run   text.Run
	size  float32
}

func newNavButton(glyph, cmd string) *navButton {
	b := &navButton{glyph: glyph, cmd: cmd, lit: anim.NewFloat(0), hover: anim.NewFloat(0), press: anim.NewFloat(0)}
	b.Add(b.lit, b.hover, b.press)
	return b
}

func (b *navButton) enable(on bool, u *gunim.UI) {
	b.on = on
	b.lit.Animate(map[bool]float32{false: 0, true: 1}[on], widget.Quick.Get(u.Theme()))
}

// Layout implements [gunim.Node].
func (b *navButton) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(geom.Sz(32, 32))
}

// Paint implements [gunim.Node].
func (b *navButton) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	r := geom.Rect{Max: box.Point()}
	scale := 1 - 0.08*b.press.Value()
	defer p.Push(paint.Scale(scale, r.Center()))()
	if h := b.hover.Value() * b.lit.Value(); h > 0.01 {
		c := widget.ButtonHover.Get(th)
		c.A = uint8(float32(c.A) * min(h, 1))
		p.RRect(r, box.H/2, paint.Solid(c))
	}
	size := widget.TextSize.Get(th) + 3
	if b.size != size || len(b.run.Glyphs) == 0 {
		b.run, b.size = text.Default().Shape(b.glyph, size), size
	}
	ink := widget.Ink.Get(th)
	ink.A = uint8(float32(ink.A) * (0.3 + 0.7*min(max(b.lit.Value(), 0), 1)))
	b.run.Paint(p, geom.Pt((box.W-b.run.Advance)/2, (box.H-b.run.Height())/2), ink)
}

// Handle implements [gunim.Handler].
func (b *navButton) Handle(e input.Event, u *gunim.UI) bool {
	th := u.Theme()
	switch e := e.(type) {
	case input.PointerEnter:
		b.hover.Animate(1, widget.Quick.Get(th))
	case input.PointerLeave:
		b.hover.Animate(0, widget.Settle.Get(th))
	case input.PointerDown:
		if e.Button != input.ButtonPrimary || !b.on {
			return false
		}
		b.press.Animate(1, widget.Quick.Get(th))
	case input.PointerUp:
		b.press.Animate(0, widget.Bounce.Get(th))
		if b.on {
			u.Send(b, Command{Name: b.cmd})
		}
	default:
		return false
	}
	return true
}

// Cursor implements [gunim.CursorShaper].
func (b *navButton) Cursor(geom.Point) input.Cursor { return input.CursorHand }

// KeepsFocus implements [gunim.FocusKeeper], so a click leaves the
// keyboard with the listing.
func (b *navButton) KeepsFocus() {}

// pathSlot shows the folders of the path, or the field that edits it,
// crossfading between them.
type pathSlot struct {
	anim.Group
	crumbs  *crumbBar
	field   *pathField
	editing bool
	mix     *anim.Float
}

func newPathSlot(c *crumbBar, f *pathField) *pathSlot {
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
		n := newCrumb(cs[i], i-keep)
		c.crumbs = append(c.crumbs, n)
		u.Insert(c, n)
	}
	for i, n := range c.crumbs {
		n.last = i == len(c.crumbs)-1
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
// the path.
func (c *crumbBar) Handle(e input.Event, u *gunim.UI) bool {
	if d, ok := e.(input.PointerDown); ok && d.Button == input.ButtonPrimary {
		c.bar.edit(u)
		return true
	}
	return false
}

// Cursor implements [gunim.CursorShaper].
func (c *crumbBar) Cursor(geom.Point) input.Cursor { return input.CursorText }

// crumb is one folder of the path: its name, a link to it, and a chevron
// after all but the last.
type crumb struct {
	anim.Group
	name, path string
	last       bool
	x          float32
	in         *anim.Float
	hover      *anim.Float
	delay      int
	run, sep   text.Run
	size       float32
}

func newCrumb(c Crumb, delay int) *crumb {
	n := &crumb{name: c.Name, path: c.Path, delay: delay, in: anim.NewFloat(0), hover: anim.NewFloat(0)}
	n.Add(n.in, n.hover)
	return n
}

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

func (n *crumb) shape(th *theme.Live) {
	size := widget.TextSize.Get(th)
	if n.size == size && len(n.run.Glyphs) > 0 {
		return
	}
	n.size = size
	n.run = text.Default().Shape(n.name, size)
	n.sep = text.Default().Shape("›", size)
}

// Layout implements [gunim.Node].
func (n *crumb) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	n.shape(f.Theme)
	w := n.run.Advance + 12
	if !n.last {
		w += n.sep.Advance + 8
	}
	return c.Constrain(geom.Sz(w, 26))
}

// Paint implements [gunim.Node].
func (n *crumb) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	t := min(max(n.in.Value(), 0), 1)
	if t <= 0.001 {
		return
	}
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}.Inset(geom.Uniform(-4)), Opacity: t})()
	defer p.Push(paint.Translate(geom.Pt(-10*(1-t), 0)))()
	pill := geom.Rc(0, 0, n.run.Advance+12, box.H)
	if h := n.hover.Value(); h > 0.01 {
		c := widget.ButtonHover.Get(th)
		c.A = uint8(float32(c.A) * min(h, 1))
		p.RRect(pill, 6, paint.Solid(c))
	}
	ink := widget.Ink.Get(th)
	if !n.last {
		ink = Faint.Get(th)
	}
	n.run.Paint(p, geom.Pt(6, (box.H-n.run.Height())/2), ink)
	if !n.last {
		n.sep.Paint(p, geom.Pt(pill.Max.X+4, (box.H-n.sep.Height())/2), Caption.Get(th))
	}
}

// Handle implements [gunim.Handler].
func (n *crumb) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerEnter:
		n.hover.Animate(1, widget.Quick.Get(u.Theme()))
	case input.PointerLeave:
		n.hover.Animate(0, widget.Settle.Get(u.Theme()))
	case input.PointerDown:
		// A press on the folder showing edits the path, as one beside the
		// folders does.
		if e.Button != input.ButtonPrimary || e.Pos.X > n.run.Advance+12 || n.last {
			return false
		}
		u.Send(n, Navigate{Path: n.path})
	case input.PointerUp:
	default:
		return false
	}
	return true
}

// Cursor implements [gunim.CursorShaper].
func (n *crumb) Cursor(p geom.Point) input.Cursor {
	if p.X > n.run.Advance+12 || n.last {
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
