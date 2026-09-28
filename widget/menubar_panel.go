package widget

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
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

	// cardX is the card's left edge, against the list's card, and cardY, cardW and cardH its top and size, springing
	// to the open menu's; cardOn fades it in and out. placed says the card has a place to spring from.
	cardX                       float32
	cardY, cardW, cardH, cardOn *anim.Float
	placed                      bool
	margin                      float32
	transparent                 bool
}

func newBarPanel(b *Menubar, list *Menu) *barPanel {
	p := &barPanel{b: b, list: list,
		cardY: anim.NewFloat(0), cardW: anim.NewFloat(0), cardH: anim.NewFloat(0), cardOn: anim.NewFloat(0)}
	p.Add(p.cardY, p.cardW, p.cardH, p.cardOn)
	return p
}

// Children implements [gunim.Composite]: the list. The menus shown beside it come and go as its children after it.
func (p *barPanel) Children() []gunim.Node { return []gunim.Node{p.list} }

// PopupPadding implements [gunim.PopupPadder]: the list's, which is the part of the panel lined up with the button.
func (p *barPanel) PopupPadding() geom.Insets { return p.list.PopupPadding() }

// sideCard is where the card beside the list is drawn now, in the panel's space.
func (p *barPanel) sideCard() geom.Rect {
	return geom.Rc(p.cardX, p.cardY.Value(), max(0, p.cardW.Value()), max(0, p.cardH.Value()))
}

// cardTop is where the card for menu i goes: its first line level with the list's line i.
func (p *barPanel) cardTop(i int, f gunim.Frame) float32 {
	return p.list.RowRect(i).Min.Y - MenuPadding.Get(f.Theme)
}

// Covers implements [gunim.Shaped]: the list's card and the card beside it are the panel's, and the rest of a window
// kept at its largest is not.
func (p *barPanel) Covers(at geom.Point) bool {
	if !p.transparent {
		return true
	}
	return p.list.card.Contains(at) || (p.cardOn.Target() > 0 && p.sideCard().Contains(at))
}

// Layout implements [gunim.Node]: the list at the top left, and beside it the card, as large as the window needs
// for the bar's largest menu beside its line.
func (p *barPanel) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	p.transparent = f.Transparent
	p.margin = 0
	if f.Transparent {
		p.margin = MenuMargin.Get(f.Theme)
	}
	lk := kids.At(0)
	size := lk.Layout(gunim.Loose(c.Max))
	lk.Place(geom.Point{})
	p.cardX = p.list.card.Max.X

	// Every menu, laid out as it would be beside its line
	if len(p.probes) != len(p.b.Menus) {
		p.probes = make([]*Menu, len(p.b.Menus))
	}
	for i := range p.b.Menus {
		if p.probes[i] == nil {
			p.probes[i] = p.b.newMenu(i)
			p.probes[i].bare = true
		} else {
			p.b.fill(p.probes[i], i)
		}
		s := p.probes[i].Layout(gunim.Loose(c.Max), f, gunim.Children{})
		size.W = max(size.W, p.cardX+s.W+p.margin)
		size.H = max(size.H, p.cardTop(i, f)+s.H+p.margin)
	}

	// The menus shown beside the list ride on the card; the open one sets where it goes
	open := p.b.menu
	for i := 1; i < kids.Len(); i++ {
		k := kids.At(i)
		s := k.Layout(gunim.Loose(c.Max))
		k.Place(geom.Pt(p.cardX, p.cardY.Value()))
		if k.Node() == open && p.b.open >= 0 {
			p.aim(p.cardTop(p.b.open, f), s, f)
		}
	}
	if open == nil {
		p.cardOn.Animate(0, Quick.Get(f.Theme))
	}
	return c.Constrain(size)
}

// aim sends the card to top at size, springing there from where it was, or there at once when it had nowhere to
// spring from.
func (p *barPanel) aim(top float32, s geom.Size, f gunim.Frame) {
	if !p.placed || p.cardOn.Value() < 0.05 {
		p.cardY.Jump(top)
		p.cardW.Jump(s.W)
		p.cardH.Jump(s.H)
		p.placed = true
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
