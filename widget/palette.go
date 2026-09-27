package widget

import (
	"image/color"
	"strconv"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/match"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Palette tokens.
var (
	PaletteWidth     = theme.Length("palette.width", 560)
	PaletteRowHeight = theme.Length("palette.row", 32)
	// PaletteMark sits behind the letters a query matched.
	PaletteMark = theme.Color("palette.mark", color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0x50})
	// PaletteHint colours an item's hint, such as its shortcut.
	PaletteHint = theme.Color("palette.hint", color.NRGBA{R: 0x8a, G: 0x93, B: 0xa6, A: 0xff})
)

// paletteRows is how many items show before the list scrolls.
const paletteRows = 10

// PaletteItem is one thing a [Palette] offers: its title, a hint shown
// beside it, such as its shortcut, and other words it answers to.
type PaletteItem struct {
	Title string
	Hint  string
	Also  []string
	// Detail is said faintly after the title, such as the folder a file
	// is in.
	Detail string
	// Key keeps an item's row from one [Palette.SetItems] to the next, so
	// it moves to its new place; unset, the item's index is its key.
	Key Key
	// At holds the runes of Title to mark, for items ranked by the caller.
	At []int
	// Problem draws the title in the colour of something wrong.
	Problem bool
}

// Palette finds one of a list of items as the user types part of its
// name, the way a command palette does. It opens in a popup with a
// field and the items, best match first, the matched letters marked.
// The list reorders as the query changes, items folding away and
// growing back, and the card grows and shrinks with it. With Search set,
// the caller ranks the items, as for a search run in the background, and
// hands them over with SetItems.
//
// The field has the keyboard while the palette is open. Up and Down,
// Page Up and Page Down move the highlight, Enter picks, and Escape, a
// click outside or the window losing the keyboard closes it. Focus goes
// back where it was.
type Palette struct {
	Items       []PaletteItem
	Placeholder string
	// Pick runs with the index of the item chosen, once the palette
	// has closed. An index past Items is Typed's item at i-len(Items).
	Pick func(i int, u *gunim.UI)
	// Typed, when set, returns items made from what has been typed, such
	// as a value to match exactly; they show first, in the order given.
	Typed func(query string) []PaletteItem
	// Search, when set, hears each query in place of the palette ranking
	// Items itself; the answer comes back through SetItems.
	Search func(query string, u *gunim.UI)
	// CtrlPick, when set, runs in place of Pick for Ctrl+Enter.
	CtrlPick func(i int, u *gunim.UI)
	// Status is a line under the field, such as how far a search has got.
	Status string

	popup *gunim.Popup
	card  *paletteCard
	back  gunim.Node
	// typedItems holds Typed's items for the query last typed.
	typedItems []PaletteItem
}

// Open opens the palette in a popup attached to anchor, a rectangle in
// opener's space: centred on it, just below its top edge.
func (p *Palette) Open(opener gunim.Node, anchor geom.Rect, u *gunim.UI) {
	if p.IsOpen() {
		return
	}
	p.back = u.Focused()
	p.card = newPaletteCard(p)
	w := PaletteWidth.Get(u.Theme())
	x := anchor.Center().X - w/2
	p.popup = u.OpenPopup(opener, p.card, gunim.PopupOptions{
		Anchor:  geom.Rect{Min: geom.Pt(x, anchor.Min.Y), Max: geom.Pt(x+w, anchor.Min.Y)},
		Max:     geom.Sz(w+200, 800),
		Dismiss: p.Close,
	})
	p.card.filter("", u)
	u.Focus(p.card.field)
}

// Close closes the palette, and gives the keyboard back.
func (p *Palette) Close(u *gunim.UI) {
	if p.popup == nil {
		return
	}
	p.popup.Close()
	p.popup = nil
	if p.back != nil {
		u.Focus(p.back)
	}
}

// IsOpen reports whether the palette is open.
func (p *Palette) IsOpen() bool { return p.popup != nil && p.popup.Open() }

// SetItems makes items the palette's items, in the order given, for a
// palette whose Search ranks them. Rows move to their new places.
func (p *Palette) SetItems(items []PaletteItem, u *gunim.UI) {
	was := Key("")
	if p.IsOpen() && p.card.hot > 0 {
		if i := p.card.hotIndex(); i >= 0 {
			was = p.keyOf(i)
		}
	}
	p.Items = items
	if p.IsOpen() {
		p.card.show(was, u)
	}
}

// SetStatus sets the line under the field.
func (p *Palette) SetStatus(s string, u *gunim.UI) {
	p.Status = s
	u.Invalidate()
}

// Query returns what is typed in the palette's field.
func (p *Palette) Query() string {
	if p.card == nil {
		return ""
	}
	return p.card.field.Text()
}

// SetQuery puts q in the palette's field, as if it were typed.
func (p *Palette) SetQuery(q string, u *gunim.UI) {
	if !p.IsOpen() {
		return
	}
	p.card.field.SetText(q)
	p.card.field.Select(len(q), len(q))
	p.card.filter(q, u)
}

func (p *Palette) choose(i int, ctrl bool, u *gunim.UI) {
	p.Close(u)
	switch {
	case ctrl && p.CtrlPick != nil:
		p.CtrlPick(i, u)
	case p.Pick != nil:
		p.Pick(i, u)
	}
}

// paletteCard is the palette's popup: the field over the list.
type paletteCard struct {
	anim.Group
	p     *Palette
	field *TextField
	// list builds only the rows in view: a palette holds hundreds of
	// commands, and each letter typed ranks them all again.
	list  *VirtualList
	found []match.Found
	// at is where the query matched each item found, by index, and index
	// each item's index by its key.
	at    map[int][]int
	index map[Key]int
	hot   int
	in    *anim.Float
	// height is the list's height, gliding as the list changes.
	height *anim.Float

	margin, pad, rowH, spacing float32
	fieldH                     float32
	card                       geom.Rect
	status                     shapedText
	transparent                bool
}

func newPaletteCard(p *Palette) *paletteCard {
	c := &paletteCard{p: p, field: NewTextField(), in: anim.NewFloat(0), height: anim.NewFloat(0)}
	c.list = NewVirtualList(func(k Key) gunim.Node { return newPaletteRow(c, k) })
	c.field.Placeholder = p.Placeholder
	c.field.OnEdit = c.filter
	c.Add(c.in, c.height)
	return c
}

// Children implements [gunim.Composite].
func (c *paletteCard) Children() []gunim.Node { return []gunim.Node{c.field, c.list} }

// PopupPadding implements [gunim.PopupPadder].
func (c *paletteCard) PopupPadding() geom.Insets { return geom.Uniform(c.margin) }

// Transition implements [gunim.Transitioner].
func (c *paletteCard) Transition(p gunim.Presence, f gunim.Frame) bool {
	switch p {
	case gunim.Entering:
		c.in.Animate(1, Bounce.Get(f.Theme))
	case gunim.Exiting:
		c.in.Animate(0, Quick.Get(f.Theme))
	case gunim.Present:
	}
	return !c.in.Active()
}

// filter shows the items the query finds, keeping the highlight on the
// best.
func (c *paletteCard) filter(q string, u *gunim.UI) {
	if c.p.Search != nil {
		c.p.Search(q, u)
		return
	}
	items := make([]match.Item, len(c.p.Items))
	for i, it := range c.p.Items {
		items[i] = match.Item{Title: it.Title, Also: it.Also}
	}
	c.found = match.Rank(items, q)
	c.p.typedItems = nil
	if c.p.Typed != nil {
		c.p.typedItems = c.p.Typed(q)
	}
	if n := len(c.p.typedItems); n > 0 {
		first := make([]match.Found, n, n+len(c.found))
		for k := range first {
			first[k] = match.Found{Index: len(c.p.Items) + k}
		}
		c.found = append(first, c.found...)
	}
	c.list.ScrollTo(0, Quick.Get(u.Theme()))
	c.refill(u)
}

// show shows the items as given, for a palette whose Search ranks them.
// The highlight stays on the item under key was while the items hold it.
func (c *paletteCard) show(was Key, u *gunim.UI) {
	c.found = make([]match.Found, len(c.p.Items))
	for i, it := range c.p.Items {
		c.found[i] = match.Found{Index: i, At: it.At}
	}
	for k := range c.found {
		if was != "" && c.p.keyOf(k) == was {
			c.refill(u)
			c.hot = k
			c.light(u)
			return
		}
	}
	c.list.ScrollTo(0, Quick.Get(u.Theme()))
	c.refill(u)
}

// refill puts the items found in the list, the highlight on the first.
func (c *paletteCard) refill(u *gunim.UI) {
	c.hot = 0
	c.at = make(map[int][]int, len(c.found))
	c.index = make(map[Key]int, len(c.found))
	keys := make([]Key, len(c.found))
	for i, f := range c.found {
		c.at[f.Index] = f.At
		keys[i] = c.p.keyOf(f.Index)
		c.index[keys[i]] = f.Index
	}
	c.list.SetKeys(keys, u)
	c.light(u)
	rows := float32(min(len(c.found), paletteRows))
	c.height.Animate(rows*(c.rowOr(u)+ListSpacing.Get(u.Theme())), Settle.Get(u.Theme()))
}

// keyOf is item i's key in the list.
func (p *Palette) keyOf(i int) Key {
	if it := p.item(i); it.Key != "" {
		return it.Key
	}
	return Key(strconv.Itoa(i))
}

// hotIndex is the item highlighted, or -1.
func (c *paletteCard) hotIndex() int {
	if c.hot < 0 || c.hot >= len(c.found) {
		return -1
	}
	return c.found[c.hot].Index
}

func (c *paletteCard) rowOr(u *gunim.UI) float32 {
	if c.rowH > 0 {
		return c.rowH
	}
	return PaletteRowHeight.Get(u.Theme())
}

// light scrolls the highlighted row into view; each row lights itself
// as it is laid out.
func (c *paletteCard) light(u *gunim.UI) {
	if c.hot >= 0 && c.hot < len(c.found) {
		step := c.rowOr(u) + ListSpacing.Get(u.Theme())
		y := float32(c.hot) * step
		c.list.revealContent(geom.Rc(0, y, 1, step), u)
	}
	u.Invalidate()
}

func (c *paletteCard) move(by int, u *gunim.UI) {
	if len(c.found) == 0 {
		return
	}
	c.hot = min(max(c.hot+by, 0), len(c.found)-1)
	c.light(u)
}

func (c *paletteCard) pick(ctrl bool, u *gunim.UI) {
	if c.hot >= 0 && c.hot < len(c.found) {
		c.p.choose(c.found[c.hot].Index, ctrl, u)
	}
}

// Handle implements [gunim.Handler]: the keys the field passes on.
func (c *paletteCard) Handle(e input.Event, u *gunim.UI) bool {
	k, ok := e.(input.KeyPress)
	if !ok {
		return false
	}
	switch k.Key {
	case input.KeyUp:
		c.move(-1, u)
	case input.KeyDown:
		c.move(1, u)
	case input.KeyPageUp:
		c.move(-paletteRows, u)
	case input.KeyPageDown:
		c.move(paletteRows, u)
	case input.KeyEnter, input.KeyKPEnter:
		c.pick(k.Mods.Has(input.ModControl), u)
	case input.KeyEscape:
		c.p.Close(u)
	default:
		return false
	}
	u.Invalidate()
	return true
}

// Layout implements [gunim.Node].
func (c *paletteCard) Layout(cs gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	th := f.Theme
	c.transparent = f.Transparent
	c.margin = 0
	if f.Transparent {
		c.margin = MenuMargin.Get(th)
	}
	c.pad, c.rowH, c.spacing = MenuPadding.Get(th)*2, PaletteRowHeight.Get(th), ListSpacing.Get(th)
	w := PaletteWidth.Get(th)
	inner := w - 2*c.pad
	fh := kids.At(0).Layout(gunim.Tight(geom.Sz(inner, FieldHeight.Get(th)))).H
	kids.At(0).Place(geom.Pt(c.margin+c.pad, c.margin+c.pad))
	c.fieldH = fh
	if c.p.Status != "" {
		// The status line sits under the field.
		fh += paletteStatus(th)
	}
	lh := max(0, c.height.Value())
	kids.At(1).Layout(gunim.Tight(geom.Sz(inner, lh)))
	gap := min(c.pad, lh)
	kids.At(1).Place(geom.Pt(c.margin+c.pad, c.margin+c.pad+fh+gap))
	h := c.pad + fh + gap + lh + c.pad
	c.card = geom.Rc(c.margin, c.margin, w, h)
	// Where the window can show what is behind it, it is kept at the
	// palette's tallest and the card grows and shrinks inside it: a
	// window resized every frame of the card's motion is slow, and on
	// Windows very slow. Below the card, the pointer passes through.
	if c.transparent {
		h = max(h, c.pad+fh+c.pad+paletteRows*(c.rowH+c.spacing)+c.pad+paletteStatus(th))
	}
	return cs.Constrain(geom.Sz(w+2*c.margin, h+2*c.margin))
}

// Covers implements [gunim.Shaped]: the card is the palette's, and the
// rest of a window kept at its tallest is not.
func (c *paletteCard) Covers(p geom.Point) bool {
	return !c.transparent || c.card.Contains(p)
}

// Paint implements [gunim.Node]: a card like a menu's, fading in and
// unfolding from its top edge.
func (c *paletteCard) Paint(p *paint.Painter, f gunim.Frame, _ geom.Size, kids gunim.Children) {
	th := f.Theme
	radius := MenuRadius.Get(th)
	t := c.in.Value()
	if c.transparent {
		defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: c.card.Max.Add(geom.Pt(c.margin, c.margin))}, Opacity: min(1, max(0, t))})()
		defer p.Push(paint.Scale(0.96+0.04*t, geom.Pt(c.card.Center().X, c.card.Min.Y)))()
		p.ShadowRRect(c.card, radius, paint.Solid(MenuFill.Get(th)), paint.Shadow{
			Offset: geom.Pt(0, 3), Blur: c.margin * 0.6, Color: MenuShadow.Get(th),
		})
	} else {
		radius = 0
		p.RRect(c.card, 0, paint.Solid(MenuFill.Get(th)))
	}
	p.RRectStroke(c.card, radius, paint.Fill{}, paint.Stroke{Width: 1, Color: MenuBorder.Get(th)})
	for k := range kids.All {
		k.Paint(p)
	}
	if c.p.Status != "" {
		face := faceIn(Font, th)
		run := c.status.shape(face, c.p.Status, TextSize.Get(th)*0.85)
		room := c.card.Size().W - 2*c.pad - MenuRowPadding.Get(th)
		if run.Advance > room {
			run = cutRun(run, face.Shape("…", run.Size), room)
		}
		y := c.card.Min.Y + c.pad + c.fieldH + (paletteStatus(th)-run.Height())/2
		run.Paint(p, geom.Pt(c.card.Min.X+c.pad+MenuRowPadding.Get(th), y), PaletteHint.Get(th))
	}
}

// paletteStatus is the height of the line under a palette's field.
func paletteStatus(th *theme.Live) float32 { return TextSize.Get(th) * 1.6 }

// paletteRow is one item in a palette's list.
type paletteRow struct {
	anim.Group
	c   *paletteCard
	key Key
	// index is the item's index, and item and at the item and its marks
	// as last laid out, which the row keeps drawing as it leaves.
	index  int
	item   PaletteItem
	at     []int
	hot    *anim.Float
	on     bool
	title  shapedText
	detail shapedText
	hint   shapedText
}

func newPaletteRow(c *paletteCard, key Key) *paletteRow {
	r := &paletteRow{c: c, key: key, index: -1, hot: anim.NewFloat(0)}
	r.Add(r.hot)
	r.refresh()
	return r
}

// refresh takes the row's item afresh, while the palette still holds it.
func (r *paletteRow) refresh() {
	if i, ok := r.c.index[r.key]; ok {
		r.index, r.item, r.at = i, r.c.p.item(i), r.c.at[i]
	}
}

// Layout implements [gunim.Node]. The row lights as it is laid out, when
// it has become the one highlighted, or dims when it has stopped being.
func (r *paletteRow) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	r.refresh()
	if on := r.index >= 0 && r.c.hotIndex() == r.index; on != r.on {
		r.on = on
		to := float32(0)
		if on {
			to = 1
		}
		r.hot.Animate(to, Quick.Get(f.Theme))
	}
	return c.Constrain(geom.Sz(c.Max.W, PaletteRowHeight.Get(f.Theme)))
}

// Paint implements [gunim.Node].
func (r *paletteRow) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	if t := min(max(r.hot.Value(), 0), 1); t > 0.01 {
		hot := MenuHot.Get(th)
		hot.A = uint8(float32(hot.A) * t)
		p.RRect(geom.Rect{Max: box.Point()}, MenuRadius.Get(th)/2, paint.Solid(hot))
	}
	pad := MenuRowPadding.Get(th)
	size := TextSize.Get(th)
	item := r.item
	run := r.title.shape(faceIn(Font, th), item.Title, size)
	top := (box.H - run.Height()) / 2
	// The matched letters sit on marks, joined where they run on.
	mark := PaletteMark.Get(th)
	at := r.at
	for i := 0; i < len(at); {
		j := i
		for j+1 < len(at) && at[j+1] == at[j]+1 {
			j++
		}
		x0, x1 := run.CaretX(at[i]), run.CaretX(at[j]+1)
		p.RRect(geom.Rc(pad+x0-1, top, x1-x0+2, run.Height()), 3, paint.Solid(mark))
		i = j + 1
	}
	ink := Ink.Get(th)
	if item.Problem {
		ink = DialogProblem.Get(th)
	}
	run.Paint(p, geom.Pt(pad, top), ink)
	end := box.W - pad
	if item.Hint != "" {
		hint := r.hint.shape(faceIn(Font, th), item.Hint, size*0.9)
		end -= hint.Advance + pad
		hint.Paint(p, geom.Pt(box.W-pad-hint.Advance, (box.H-hint.Height())/2), PaletteHint.Get(th))
	}
	if x := pad + run.Advance + size*0.8; item.Detail != "" && end-x > size {
		face := faceIn(Font, th)
		detail := r.detail.shape(face, item.Detail, size*0.85)
		if detail.Advance > end-x {
			detail = cutRun(detail, face.Shape("…", detail.Size), end-x)
		}
		detail.Paint(p, geom.Pt(x, (box.H-detail.Height())/2), PaletteHint.Get(th))
	}
}

// Handle implements [gunim.Handler]: the pointer highlights and picks.
func (r *paletteRow) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerMove:
		// A move alone: the palette opening under a pointer at rest, or
		// the rows shifting under it as the query narrows, leaves the
		// highlight on the best match, where Enter picks it.
		for k, f := range r.c.found {
			if f.Index == r.index && r.c.hot != k {
				r.c.hot = k
				r.c.light(u)
			}
		}
		return false
	case input.PointerDown:
		return true
	case input.PointerUp:
		r.c.pick(e.Mods.Has(input.ModControl), u)
		return true
	}
	return false
}

// item returns item i, counting Typed's items after Items.
func (p *Palette) item(i int) PaletteItem {
	if i < len(p.Items) {
		return p.Items[i]
	}
	if k := i - len(p.Items); k < len(p.typedItems) {
		return p.typedItems[k]
	}
	return PaletteItem{}
}
