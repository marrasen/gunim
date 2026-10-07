package widget

import (
	"math"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// barPanel is a compact menubar's popup: the list of its menus and, beside the list's line for the open menu, that
// menu on a card of the panel's own. The window is kept at the size of the list and its largest menu, so moving from
// menu to menu opens and resizes no window. The card springs to its new line and size while the menus' rows
// cross-fade.
type barPanel struct {
	anim.Group
	b    *Menubar
	list *Menu
	// probes are the bar's menus, laid out to find the largest.
	probes []*Menu

	// cardY, cardW and cardH are the card's top and size, springing to the open menu's; cardOn fades it in and out.
	// placed says the card has a place to spring from, and left that it is on the list's left.
	cardY, cardW, cardH, cardOn *anim.Float
	placed, left                bool
	margin                      float32
	transparent                 bool

	// room is the room the screen leaves round the button: below its bottom, and right and left of its left edge,
	// less a pixel so rounding keeps the window on the screen. told says it was told, and pad is the padding last
	// reported, which it was told from.
	room driver.Room
	told bool
	pad  geom.Insets
	// up says the panel opens above the button, as it does when the list does not fit below. head and lead are how far
	// the window reaches above the list and left of it, for a menu too tall for the room below or too wide for the room
	// right. off is where the list sits in the panel, and limit how far down the cards may reach.
	up         bool
	head, lead float32
	off        geom.Point
	limit      float32
	// sides says which menus open on the list's left.
	sides []bool
}

func newBarPanel(b *Menubar, list *Menu) *barPanel {
	p := &barPanel{b: b, list: list,
		cardY: anim.NewFloat(0), cardW: anim.NewFloat(0), cardH: anim.NewFloat(0), cardOn: anim.NewFloat(0)}
	p.Add(p.cardY, p.cardW, p.cardH, p.cardOn)
	return p
}

// Children implements [gunim.Composite]: the list. The menus shown beside it come and go as its children after it.
func (p *barPanel) Children() []gunim.Node { return []gunim.Node{p.list} }

// PopupPadding implements [gunim.PopupPadder]: the list's, which is the part of the panel lined up with the button,
// and above it and left of it the room the window reaches past the list.
func (p *barPanel) PopupPadding() geom.Insets {
	in := p.list.PopupPadding()
	in.Top += p.head
	in.Left += p.lead
	p.pad = in
	return in
}

// FitPopup implements [gunim.PopupFitter]. The room is told from the window's corner, which is the padding up and left
// of the button, and kept from the button.
func (p *barPanel) FitPopup(r driver.Room) {
	p.room = driver.Room{Below: r.Below - p.pad.Top - 1, Above: r.Above, Left: r.Left + p.pad.Left, Right: r.Right - p.pad.Left - 1}
	p.told = true
}

// listCard is the list's card in the panel's space.
func (p *barPanel) listCard() geom.Rect { return p.list.card.Add(p.off) }

// cardInList is the card beside the list in the list's space.
func (p *barPanel) cardInList() geom.Rect { return p.sideCard().Add(geom.Pt(-p.off.X, -p.off.Y)) }

// sideCard is where the card beside the list is drawn now, in the panel's space: against the list's right edge, or
// its left.
func (p *barPanel) sideCard() geom.Rect {
	lc := p.listCard()
	w, h := max(0, p.cardW.Value()), max(0, p.cardH.Value())
	x := lc.Max.X
	if p.left {
		x = lc.Min.X - w
	}
	return geom.Rc(x, p.cardY.Value(), w, h)
}

// cardAt is where the card for menu i, h tall, goes: its first line level with the list's line i, or as much higher
// as keeps it inside the room the screen leaves.
func (p *barPanel) cardAt(i int, h float32, f gunim.Frame) float32 {
	top := p.off.Y + p.list.RowRect(i).Min.Y - MenuPadding.Get(f.Theme)
	return max(min(top, p.limit-p.margin-h), p.margin)
}

// Covers implements [gunim.Shaped]: the list's card and the card beside it are the panel's, and the rest of a window
// kept at its largest is not.
func (p *barPanel) Covers(at geom.Point) bool {
	if !p.transparent {
		return true
	}
	return p.listCard().Contains(at) || (p.cardOn.Target() > 0 && p.sideCard().Contains(at))
}

// CoverRects implements [gunim.RegionShaped]: the list's card, and the card beside it as it springs to the open
// menu's line and size.
func (p *barPanel) CoverRects() []geom.Rect {
	if !p.transparent {
		return nil
	}
	rects := []geom.Rect{p.listCard()}
	if p.cardOn.Target() > 0 {
		rects = append(rects, p.sideCard())
	}
	return rects
}

// Layout implements [gunim.Node]: the list, and beside it the card, as large as the window needs for the bar's largest
// menu beside its line, within the room the screen leaves. Below the button the list is at the top; above it, at the
// bottom, against the button. A menu too wide for the room right of the list opens on its left.
func (p *barPanel) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	p.transparent = f.Transparent
	p.margin = 0
	if f.Transparent {
		p.margin = MenuMargin.Get(f.Theme)
	}
	lk := kids.At(0)
	list := lk.Layout(gunim.Loose(c.Max))
	lc := p.list.card
	room := driver.NoRoomLimit
	if p.told {
		room = p.room
	}

	// Below the button while the list fits there, as the window is put, else above it.
	below := room.Below + p.margin
	p.up = p.told && list.H > below && room.Above > below
	// A menu beside the list is no taller than the window can be, and scrolls what does not fit: the room above the
	// button when the panel opens above it, and the room above and below it when it opens below.
	side := gunim.Loose(c.Max)
	switch {
	case p.up:
		side.Max.H = min(side.Max.H, room.Above-2*p.margin)
	case p.told:
		side.Max.H = min(side.Max.H, room.Above+below-2*p.margin)
	}
	side.Max.H = max(side.Max.H, 1)

	// Every menu, laid out as it would be beside its line
	if len(p.probes) != len(p.b.Menus) {
		p.probes = make([]*Menu, len(p.b.Menus))
	}
	sizes := make([]geom.Size, len(p.b.Menus))
	tallest := float32(0)
	for i := range p.b.Menus {
		if p.probes[i] == nil {
			p.probes[i] = p.b.newMenu(i)
			p.probes[i].bare = true
		} else {
			p.b.fill(p.probes[i], i)
		}
		sizes[i] = p.probes[i].Layout(side, f, gunim.Children{})
		tallest = max(tallest, sizes[i].H)
	}

	// Right of the list while a menu fits there, else left of it where it fits there, and the window reaches as far left
	// as the menus on the left need. The list's card starts at the button's left edge, its margin into the window.
	p.sides = p.sides[:0]
	p.lead = 0
	for _, s := range sizes {
		left := lc.Max.X-lc.Min.X+s.W > room.Right-p.margin && s.W <= room.Left-p.margin
		p.sides = append(p.sides, left)
		if left {
			p.lead = max(p.lead, p.margin+s.W-lc.Min.X)
		}
	}
	p.lead = min(p.lead, max(0, room.Left-lc.Min.X))
	width := p.lead + list.W
	for i, s := range sizes {
		if !p.sides[i] {
			width = max(width, p.lead+lc.Max.X+s.W+p.margin)
		}
	}

	// The list's card starts at the button's bottom, its margin down the window, and the cards may reach as far down
	// as the room below the button.
	p.head, p.limit = 0, float32(math.Inf(1))
	height := list.H
	switch {
	case p.up:
		height = max(list.H, min(room.Above, tallest+2*p.margin))
		p.off.Y, p.limit = height-list.H, height
	case p.told:
		// A menu too tall for the room below its line rises above the list, as far as the screen allows
		pad := MenuPadding.Get(f.Theme)
		for i, s := range sizes {
			top := min(p.list.RowRect(i).Min.Y-pad, below-p.margin-s.H)
			p.head = max(p.head, p.margin-top)
		}
		p.head = min(p.head, max(0, room.Above))
		p.off.Y, p.limit = p.head, p.head+below
		height = p.off.Y + list.H
	default:
		p.off.Y = 0
	}
	p.off.X = p.lead
	lk.Place(p.off)
	for i, s := range sizes {
		height = max(height, p.cardAt(i, s.H, f)+s.H+p.margin)
	}

	// The menus shown beside the list ride on the card; the open one sets where it goes
	open := p.b.menu
	for i := 1; i < kids.Len(); i++ {
		k := kids.At(i)
		s := k.Layout(side)
		if k.Node() == open && p.b.open >= 0 && p.b.open < len(p.sides) {
			p.aim(p.cardAt(p.b.open, s.H, f), s, p.sides[p.b.open], f)
		}
		k.Place(p.sideCard().Min)
	}
	if open == nil {
		p.cardOn.Animate(0, Quick.Get(f.Theme))
	}
	return c.Constrain(geom.Sz(width, height))
}

// aim sends the card to top at size, on the list's left or its right, springing there from where it was, or there at
// once when it had nowhere to spring from or moves to the list's other side.
func (p *barPanel) aim(top float32, s geom.Size, left bool, f gunim.Frame) {
	if !p.placed || p.cardOn.Value() < 0.05 || left != p.left {
		if p.placed && left != p.left {
			// Across the list, the card fades in again rather than jumping
			p.cardOn.Jump(0)
		}
		p.cardY.Jump(top)
		p.cardW.Jump(s.W)
		p.cardH.Jump(s.H)
		p.placed, p.left = true, left
	} else if p.cardY.Target() != top || p.cardW.Target() != s.W || p.cardH.Target() != s.H {
		p.cardY.Animate(top, anim.Snappy)
		p.cardW.Animate(s.W, anim.Snappy)
		p.cardH.Animate(s.H, anim.Snappy)
	}
	p.cardOn.Animate(1, Quick.Get(f.Theme))
}

// Paint implements [gunim.Node]: the list, then the card beside it, with the menus on it clipped to it.
func (p *barPanel) Paint(pt *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	th := f.Theme
	if !p.transparent {
		// The window cannot show what is behind it: it is all the menus' colour
		pt.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(MenuFill.Get(th)))
	}
	kids.At(0).Paint(pt)
	// The card comes and goes with the list, and fades as the last menu leaves it
	on := min(1, max(0, p.cardOn.Value())) * min(1, max(0, p.list.in.Value()))
	if on < 0.005 || kids.Len() < 2 {
		return
	}
	card := p.sideCard()
	radius := MenuRadius.Get(th)
	if !p.transparent {
		radius = 0
	}
	defer pt.Layer(paint.LayerOpts{Bounds: card.Inset(geom.Uniform(-p.margin)), Opacity: on})()
	if p.transparent {
		pt.ShadowRRect(card, radius, paint.Solid(MenuFill.Get(th)), paint.Shadow{
			Offset: geom.Pt(0, 3), Blur: p.margin * 0.6, Color: MenuShadow.Get(th),
		})
	} else {
		pt.RRect(card, 0, paint.Solid(MenuFill.Get(th)))
	}
	pt.RRectStroke(card, radius, paint.Fill{}, paint.Stroke{Width: 1, Color: MenuBorder.Get(th)})
	func() {
		defer pt.Layer(paint.LayerOpts{Bounds: card, Opacity: 1, Clip: true, Radius: radius})()
		for i := 1; i < kids.Len(); i++ {
			kids.At(i).Paint(pt)
		}
	}()
}
