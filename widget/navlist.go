package widget

import (
	"image/color"
	"strconv"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// NavList tokens.
var (
	// NavRowHeight is a row's height, and NavGap the room between rows.
	NavRowHeight = theme.Length("nav.row", 40)
	NavGap       = theme.Length("nav.gap", 2)
	// NavPadding is the room either end of a row, and NavRadius rounds its pill and hover.
	NavPadding = theme.Length("nav.padding", 12)
	NavRadius  = theme.Length("nav.radius", 6)
	// NavMark is the height of the accent mark at the pill's left.
	NavMark = theme.Length("nav.mark", 16)
	// NavWidth is the list's width where it is given no bound.
	NavWidth = theme.Length("nav.width", 220)
	// NavHover lights the row under the pointer, and NavSelected fills the pill behind the chosen row.
	NavHover    = theme.Color("nav.hover", color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x0e})
	NavSelected = theme.Color("nav.selected", color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x16})
)

// NavItem is one row of a [NavList].
type NavItem struct {
	Label string
	Icon  *icon.Icon
	// Count shows in a badge at the row's end, as the number of problems on a page. Zero shows none.
	Count int
	// CountInk colours the badge, and is the theme's [Accent] when unset.
	CountInk theme.Token[color.NRGBA]
	// Trail is a quiet icon at the row's end, as for a row that opens a window of its own.
	Trail *icon.Icon
	// Foot puts the row, and the rows after it, at the bottom of the list, when the list is taller than its rows.
	Foot bool
}

// NavList is a column of icon-and-label rows for moving between an application's pages, one of them chosen.
//
// A pill glides behind the chosen row, with an accent mark at its left, and the chosen label turns bold. A row's count
// grows in and out in a badge at its end. A click chooses a row. With focus, Up and Down move the focus ring between
// rows, Home and End go to the ends, and Enter or Space chooses the row with the ring. Disabled, it fades faint and
// takes no clicks or keys. A label cut short shows whole in a tooltip.
type NavList struct {
	Control
	// OnSelect runs on the UI goroutine when the user chooses a row; a non-nil result is sent to the application as
	// the list's intent.
	OnSelect func(i int, u *gunim.UI) gunim.Intent

	items    []NavItem
	selected int
	// foot is the first row at the bottom, or -1.
	foot int
	// cursor is the row the focus ring is on, and ringY where the ring is drawn, gliding between rows.
	cursor int
	ringY  *anim.Float
	// lag is how far the pill trails the chosen row, springing to nothing.
	lag *anim.Float
	// hot is the row under the pointer, or -1; hotV lights it, and was is the row left, fading out with wasV.
	hot, was   int
	hotV, wasV *anim.Float
	// badges hold the badges growing in or out, by row, until they settle.
	badges map[int]*navBadge
	click  Clicker
	tip    PartTip
	shaped map[int]*navText
	// size is the list's size from the last layout, and shown the rows painted last.
	size     geom.Size
	from, to int
	// rowH and gap are a row's height and the gap after it, from the last layout.
	rowH, gap float32
}

// navBadge is a badge growing in or shrinking out, keeping its words while it goes.
type navBadge struct {
	shown *anim.Float
	text  string
}

// navText is a row's shaped words.
type navText struct {
	label, count, ell shapedText
	cut               bool
}

// NewNavList returns a list of items, the first chosen.
func NewNavList(items ...NavItem) *NavList {
	n := &NavList{Control: newControl(), hot: -1, was: -1, ringY: anim.NewFloat(0), lag: anim.NewFloat(0),
		hotV: anim.NewFloat(0), wasV: anim.NewFloat(0), badges: map[int]*navBadge{}, shaped: map[int]*navText{}}
	n.Add(n.ringY, n.lag, n.hotV, n.wasV)
	n.SetItems(items, nil)
	return n
}

// Items returns the rows.
func (n *NavList) Items() []NavItem { return n.items }

// SetItems makes items the rows, keeping the chosen row's place, and sends no intent. Once laid out, a count that
// comes or goes grows or shrinks its badge.
func (n *NavList) SetItems(items []NavItem, u *gunim.UI) {
	if n.laid && u != nil {
		for i := range min(len(items), len(n.items)) {
			was, is := n.items[i].Count > 0, items[i].Count > 0
			if was == is {
				continue
			}
			b := n.badges[i]
			if b == nil {
				b = &navBadge{shown: anim.NewFloat(anim.OnOff(was))}
				n.badges[i] = b
				n.Add(b.shown)
			}
			if was {
				b.text = strconv.Itoa(n.items[i].Count)
				b.shown.Animate(0, Settle.Get(u.Theme()))
			} else {
				b.shown.Animate(1, Quick.Get(u.Theme()))
			}
		}
	}
	n.items = items
	n.foot = -1
	for i, it := range items {
		if it.Foot {
			n.foot = i
			break
		}
	}
	n.selected = min(max(n.selected, 0), max(len(items)-1, 0))
	n.cursor = min(max(n.cursor, 0), max(len(items)-1, 0))
	clear(n.shaped)
	u.Invalidate()
}

// Selected returns the chosen row.
func (n *NavList) Selected() int { return n.selected }

// SetSelected chooses row i and sends no intent. Once laid out the pill glides to it; before that, or with a nil u, it
// jumps.
func (n *NavList) SetSelected(i int, u *gunim.UI) {
	if i < 0 || i >= len(n.items) || i == n.selected {
		return
	}
	from := n.top(n.selected) + n.lag.Value()
	n.selected = i
	if !n.laid || u == nil {
		n.lag.Jump(0)
		return
	}
	n.lag.Jump(from - n.top(i))
	n.lag.Animate(0, Settle.Get(u.Theme()))
	u.Invalidate()
}

// choose chooses row i and tells the application.
func (n *NavList) choose(i int, u *gunim.UI) {
	n.moveCursor(i, u)
	if i < 0 || i >= len(n.items) || i == n.selected {
		return
	}
	n.SetSelected(i, u)
	act(u, n, gunim.CueSelect, n.OnSelect, i)
}

// moveCursor puts the focus ring on row i.
func (n *NavList) moveCursor(i int, u *gunim.UI) {
	if len(n.items) == 0 {
		return
	}
	n.cursor = min(max(i, 0), len(n.items)-1)
	n.ringY.Animate(n.top(n.cursor), Quick.Get(u.Theme()))
	u.Invalidate()
}

// pitch is a row's height and the gap after it.
func (n *NavList) pitch() float32 { return n.rowH + n.gap }

// rows is the height the rows take, without the room the foot rows are pushed down by.
func (n *NavList) rows() float32 {
	if len(n.items) == 0 {
		return 0
	}
	return float32(len(n.items))*n.pitch() - n.gap
}

// spare is how far the foot rows sit below the rows before them.
func (n *NavList) spare() float32 {
	if n.foot < 0 {
		return 0
	}
	return max(0, n.size.H-n.rows())
}

// top is where row i starts.
func (n *NavList) top(i int) float32 {
	y := float32(i) * n.pitch()
	if n.foot >= 0 && i >= n.foot {
		y += n.spare()
	}
	return y
}

// at returns the row at y, or -1 for none, as in a gap or past the ends.
func (n *NavList) at(y float32) int {
	pitch := n.pitch()
	if pitch <= 0 || y < 0 {
		return -1
	}
	if n.foot >= 0 && y >= n.top(n.foot) {
		y -= n.spare()
	} else if n.foot >= 0 && y >= float32(n.foot)*pitch {
		return -1
	}
	i := int(y / pitch)
	if i >= len(n.items) || y-float32(i)*pitch >= n.rowH {
		return -1
	}
	return i
}

// near returns the row at or just before y, kept to the rows there are.
func (n *NavList) near(y float32) int {
	pitch := n.pitch()
	if len(n.items) == 0 || pitch <= 0 {
		return 0
	}
	var i int
	switch {
	case n.foot >= 0 && y >= n.top(n.foot):
		i = n.foot + int((y-n.top(n.foot))/pitch)
	case n.foot >= 0:
		i = min(int(max(y, 0)/pitch), n.foot-1)
	default:
		i = int(max(y, 0) / pitch)
	}
	return min(max(i, 0), len(n.items)-1)
}

// Handle implements [gunim.Handler].
func (n *NavList) Handle(e input.Event, u *gunim.UI) bool {
	n.showTip(e, u, n)
	th := u.Theme()
	if n.Disabled {
		n.setHot(-1, th)
		return n.handleDisabled(e, u, nil)
	}
	switch e := e.(type) {
	case input.PointerEnter:
		n.setHot(n.at(e.Pos.Y), th)
		n.tip.Handle(e, u, n, n.tipText(n.hot))
		return false
	case input.PointerMove:
		n.setHot(n.at(e.Pos.Y), th)
		n.tip.Handle(e, u, n, n.tipText(n.hot))
		return false
	case input.PointerLeave:
		n.setHot(-1, th)
		n.tip.Handle(e, u, n, "")
		return false
	case input.PointerDown:
		n.tip.Handle(e, u, n, "")
		if e.Button != input.ButtonPrimary {
			return false
		}
		n.click.Press(e, n.at(e.Pos.Y))
	case input.PointerUp:
		if i := n.at(e.Pos.Y); n.click.Release(e, i) && i >= 0 && e.Pos.X >= 0 && e.Pos.X < n.size.W {
			n.choose(i, u)
		}
	case input.FocusGained:
		if e.Keyed {
			n.cursor = n.selected
			n.ringY.Jump(n.top(n.cursor))
		}
	case input.KeyPress:
		if e.Mods.Has(input.ModControl) || e.Mods.Has(input.ModAlt) || e.Mods.Has(input.ModSuper) {
			return false
		}
		switch e.Key {
		case input.KeyUp:
			n.moveCursor(n.cursor-1, u)
		case input.KeyDown:
			n.moveCursor(n.cursor+1, u)
		case input.KeyHome:
			n.moveCursor(0, u)
		case input.KeyEnd:
			n.moveCursor(len(n.items)-1, u)
		case input.KeyEnter, input.KeyKPEnter, input.KeySpace:
			n.choose(n.cursor, u)
		default:
			return false
		}
	default:
		if !n.ringFollows(e, th) {
			return false
		}
	}
	u.Invalidate()
	return true
}

// setHot lights row i, under the pointer, or none for -1: it lights quickly, and the row left fades out more slowly.
func (n *NavList) setHot(i int, th *theme.Live) {
	if i == n.hot {
		return
	}
	if n.hot >= 0 {
		n.was = n.hot
		n.wasV.Jump(n.hotV.Value())
		n.wasV.Animate(0, Settle.Get(th))
	}
	n.hot = i
	n.hotV.Jump(0)
	if i >= 0 {
		n.hotV.Animate(1, Quick.Get(th))
	}
}

// tipText is row i's whole label where it was cut short.
func (n *NavList) tipText(i int) string {
	if t := n.shaped[i]; t != nil && t.cut && i < len(n.items) {
		return n.items[i].Label
	}
	return ""
}

// Step implements [anim.Stepper]: it drops the badges that have settled.
func (n *NavList) Step(dt time.Duration) bool {
	moving := n.Group.Step(dt)
	for i, b := range n.badges {
		if !b.shown.Active() {
			n.Remove(b.shown)
			delete(n.badges, i)
		}
	}
	return moving
}

// Layout implements [gunim.Node]: as tall as its rows, or as tall as it is given with foot rows, and as wide as it
// is given.
func (n *NavList) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	n.rowH, n.gap = NavRowHeight.Get(th), NavGap.Get(th)
	h := n.rows()
	if n.foot >= 0 && c.Max.H > 0 {
		h = max(h, c.Max.H)
	}
	w := c.Max.W
	if w <= 0 {
		w = NavWidth.Get(th)
	}
	if !n.laid {
		n.lag.Jump(0)
	}
	n.follow(th)
	n.size = c.Constrain(geom.Sz(w, h))
	if !n.ringY.Active() {
		n.ringY.Jump(n.top(n.cursor))
	}
	return n.size
}

// Paint implements [gunim.Node]: the pill, then the rows in view over it.
func (n *NavList) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	defer n.faint(p, box)()
	th := f.Theme
	rowH := NavRowHeight.Get(th)
	radius := NavRadius.Get(th)
	view := geom.Rect{Max: box.Point()}
	if v, ok := p.Visible(); ok {
		view.Min.Y, view.Max.Y = max(view.Min.Y, v.Min.Y), min(view.Max.Y, v.Max.Y)
	}
	if len(n.items) > 0 {
		y := n.top(n.selected) + n.lag.Value()
		p.RRect(geom.Rc(0, y, box.W, rowH), radius, paint.Solid(NavSelected.Get(th)))
		mark := min(NavMark.Get(th), rowH-8)
		p.RRect(geom.Rc(0, y+(rowH-mark)/2, 3, mark), 1.5, paint.Solid(Accent.Get(th)))
	}
	n.from, n.to = n.near(view.Min.Y), n.near(view.Max.Y)
	for i := range n.shaped {
		if i < n.from || i > n.to {
			delete(n.shaped, i)
		}
	}
	for i := n.from; i <= n.to && i < len(n.items); i++ {
		top := n.top(i)
		if top >= view.Max.Y || top+rowH <= view.Min.Y {
			continue
		}
		n.paintRow(p, th, i, geom.Rc(0, top, box.W, rowH))
	}
	if len(n.items) > 0 {
		n.paintRing(p, geom.Rc(0, n.ringY.Value(), box.W, rowH), radius, th)
	}
}

// paintRow paints row i in r: its hover, icon, label and what sits at its end.
func (n *NavList) paintRow(p *paint.Painter, th *theme.Live, i int, r geom.Rect) {
	it := n.items[i]
	radius := NavRadius.Get(th)
	lit := float32(0)
	switch i {
	case n.hot:
		lit = n.hotV.Value()
	case n.was:
		lit = n.wasV.Value()
	}
	if i == n.selected {
		lit = 0
	}
	if lit > 0.01 {
		p.RRect(r, radius, paint.Solid(paint.Faded(NavHover.Get(th), min(lit, 1))))
	}
	t := n.shaped[i]
	if t == nil {
		t = &navText{}
		n.shaped[i] = t
	}
	pad := NavPadding.Get(th)
	ink := Ink.Get(th)
	x, right := r.Min.X+pad, r.Max.X-pad
	mid := r.Min.Y + r.Size().H/2
	size := IconSize.Get(th)
	if it.Icon != nil {
		PaintIcon(p, th, it.Icon, geom.Rc(x, mid-size/2, size, size), ink)
		x += size + ControlGap.Get(th)
	}
	shown, words := anim.OnOff(it.Count > 0), strconv.Itoa(it.Count)
	if b := n.badges[i]; b != nil {
		shown = min(max(b.shown.Value(), 0), 1)
		if it.Count <= 0 {
			words = b.text
		}
	}
	switch {
	case shown > 0.001:
		bt := BadgeSize.Get(th)
		run := t.count.shape(faceIn(Font, th), words, bt)
		bh := BadgeHeight.Get(th)
		w := (run.Advance + 2*BadgePadding.Get(th)) * shown
		badge := geom.Rc(right-w, mid-bh/2, w, bh)
		bink := Accent.Get(th)
		if it.CountInk.Key() != "" {
			bink = it.CountInk.Get(th)
		}
		func() {
			defer p.Layer(paint.LayerOpts{Bounds: badge, Opacity: shown})()
			fill := bink
			fill.A = uint8(float32(fill.A) * min(max(BadgeTint.Get(th), 0), 1))
			p.RRect(badge, bh/2, paint.Solid(fill))
			run.PaintFaded(p, geom.Pt(badge.Min.X+BadgePadding.Get(th), mid-run.Height()/2), bink,
				max(w-BadgePadding.Get(th), 0), TextFade.Get(th))
		}()
		right -= w + ControlGap.Get(th)
	case it.Trail != nil:
		PaintIcon(p, th, it.Trail, geom.Rc(right-size, mid-size/2, size, size), Placeholder.Get(th))
		right -= size + ControlGap.Get(th)
	}
	face := faceIn(Font, th)
	if i == n.selected {
		face = faceIn(BoldFont, th)
	}
	run := t.label.shape(face, it.Label, TextSize.Get(th))
	t.cut = run.Advance > right-x
	if right > x {
		run = fitRun(run, &t.ell, right-x)
		run.Paint(p, geom.Pt(x, mid-run.Height()/2), ink)
	}
}

// Access implements [gunim.Accessible]: a list with an item for each row in view, the chosen one selected.
func (n *NavList) Access() access.Info {
	info := access.Info{Role: access.RoleList, Name: n.Tooltip, State: n.accessState(), Active: n.cursor + 1}
	for i := n.from; i <= n.to && i < len(n.items); i++ {
		it := n.items[i]
		part := access.Info{Role: access.RoleListItem, Name: it.Label, State: n.accessState(), Key: uint64(i) + 1,
			Actions: []string{access.ActionPress}}
		part.Bounds = geom.Rc(0, n.top(i), n.size.W, n.rowH)
		if it.Count > 0 {
			part.Description = strconv.Itoa(it.Count)
		}
		if i == n.selected {
			part.State |= access.StateSelected
		}
		info.Parts = append(info.Parts, part)
	}
	return info
}

// AccessAct implements [gunim.AccessActor]: pressing a row chooses it.
func (n *NavList) AccessAct(r access.Request, u *gunim.UI) bool {
	i := n.from + r.Part
	if r.Action != access.ActionPress || r.Part < 0 || i >= len(n.items) || i > n.to || n.Disabled {
		return false
	}
	n.choose(i, u)
	return true
}
