package widget

import (
	"image/color"
	"math"
	"slices"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// The tile grid's tokens.
var (
	TileGap      = theme.Length("tiles.gap", 8)
	TilePadding  = theme.Length("tiles.padding", 14)
	TileRadius   = theme.Length("tiles.radius", 8)
	TileSelected = theme.Color("tiles.selected", color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0x48})
	TileHover    = theme.Color("tiles.hover", color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0x1a})
	TileCursor   = theme.Color("tiles.cursor", color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0xc0})
	BandFill     = theme.Color("tiles.band", color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0x2a})
	BandEdge     = theme.Color("tiles.band.edge", color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0xd0})
	// TileMotion carries tiles to new places and sizes.
	TileMotion = theme.Spring("motion.tiles", anim.Spring{Response: 0.42, Damping: 0.78})
)

// TileGrid shows a long run of items as tiles of one size in rows, such as pictures in a folder, and scrolls
// through them. It builds only the tiles in view and a row beyond, so a hundred thousand items cost what the few
// dozen on screen do.
//
// It selects as a [DataGrid] with Multi does: a click, Ctrl and Shift with a click, the arrow keys in two
// directions, Page Up and Page Down, Home and End, and Ctrl+A. A drag from empty space draws a band that selects
// the tiles it touches. When the tiles change size or the grid its width, the tiles spring to their new places.
type TileGrid struct {
	scrolling

	// Tile builds the node that draws tile i, laid out at the tile's size.
	Tile func(i int) gunim.Node
	// OnView reports the tiles built, when they change.
	OnView func(first, count int) gunim.Intent
	// OnSelect reports the selection, as runs of tiles, and the tile the keyboard is on.
	OnSelect func(sel [][2]int, cursor int) gunim.Intent
	// OnActivate reports a double click on a tile, or Enter.
	OnActivate func(i int) gunim.Intent
	// OnType, when set, turns the text typed at the grid into an intent, to find the tile it names: text typed close
	// together adds up, as [TypeAhead] gathers it, and [FindTyped] finds it.
	OnType func(text string) gunim.Intent
	typed  TypeAhead
	// OnZoom, when set, takes Ctrl with the wheel over the grid, in notches up, from the window's zoom.
	OnZoom func(notches float32, u *gunim.UI)
	// DragTiles, when set, lets the tiles selected be dragged, as [DataGrid.DragRows] does for rows.
	DragTiles func(sel [][2]int, at geom.Point) (data any, ghost gunim.Node, grab geom.Point)
	// OnDragEnd, when set, hears how a drag of the tiles ended.
	OnDragEnd func(e input.DragEnd) gunim.Intent

	n int
	// Size is the size of each tile. Change it and the tiles spring to their new places and sizes.
	Size geom.Size

	// cols, left and step are the columns, the left edge of the first and the room from one tile to the next, as
	// last laid out.
	cols       int
	left, pad  float32
	step       geom.Size
	live       map[int]*tileCell
	built      [2]int
	runs       [][2]int
	cursor     int
	anchor     int
	focused    bool
	cue        ringCue
	hover      int
	banding    bool
	bandFrom   geom.Point
	bandBase   [][2]int
	band       *anim.Rect
	bandIn     *anim.Float
	arrive     func(i int) (geom.Rect, bool)
	depart     func(i int) (geom.Rect, bool)
	departing  bool
	revealNext int
	moved      bool
	jumpRow    int
	// rebuild drops the tiles built at the next layout.
	rebuild bool
	lift    tileLift
}

// tileLift is a press on a tile that may become a drag of the tiles selected.
type tileLift struct {
	tile  int
	at    geom.Point
	mods  input.Mods
	armed bool
	// later leaves the selection's change to the release, and dragging says the press became a drag.
	later, dragging bool
}

// NewTileGrid returns an empty grid of tiles of size.
func NewTileGrid(size geom.Size) *TileGrid {
	return &TileGrid{scrolling: newScrolling(), Size: size, live: map[int]*tileCell{}, cursor: -1, anchor: -1,
		hover: -1, band: anim.NewRect(geom.Rect{}), bandIn: anim.NewFloat(0), revealNext: -1, jumpRow: -1}
}

// Len returns how many tiles the grid holds.
func (g *TileGrid) Len() int { return g.n }

// SetLen makes the grid hold n tiles, keeping the selection that is still inside.
func (g *TileGrid) SetLen(n int, u *gunim.UI) {
	g.n = max(0, n)
	g.runs = clipRuns(g.runs, g.n)
	if g.cursor >= g.n {
		g.cursor = -1
	}
	if g.anchor >= g.n {
		g.anchor = -1
	}
	u.Invalidate()
}

// Rebuild drops every tile built, for a new set of items in the same grid, such as another folder's pictures: the
// next layout builds those in view anew from Tile, and tells OnView again. Pair it with Arrive for the new tiles to
// come in, and with Depart before it for the old ones to leave first.
func (g *TileGrid) Rebuild(u *gunim.UI) {
	g.rebuild = true
	u.Invalidate()
}

// JumpToTile puts the row of tile i at the top of the view at once, as near as the end allows.
func (g *TileGrid) JumpToTile(i int, u *gunim.UI) {
	g.jumpRow = i
	u.Invalidate()
}

// Built returns how many tiles are built right now.
func (g *TileGrid) Built() int { return len(g.live) }

// Columns returns how many tiles each row held at the last layout.
func (g *TileGrid) Columns() int { return g.cols }

// Selected returns the runs of tiles selected and the tile the keyboard is on, or -1.
func (g *TileGrid) Selected() (sel [][2]int, cursor int) { return slices.Clone(g.runs), g.cursor }

// IsSelected reports whether tile i is selected.
func (g *TileGrid) IsSelected(i int) bool { return hasRun(g.runs, i) }

// SetSelected selects runs, with the keyboard on cursor, without telling OnSelect. Call it from a view's update
// function.
func (g *TileGrid) SetSelected(runs [][2]int, cursor int, u *gunim.UI) {
	g.runs = clipRuns(slices.Clone(runs), g.n)
	g.cursor = cursor
	if cursor >= g.n {
		g.cursor = -1
	}
	g.anchor = g.cursor
	u.Invalidate()
}

// ShowTile scrolls tile i into view, at the next layout.
func (g *TileGrid) ShowTile(i int, u *gunim.UI) {
	g.revealNext = i
	u.Invalidate()
}

// TileRect returns where tile i sits, heading for its place, in the grid's own space.
func (g *TileGrid) TileRect(i int) geom.Rect {
	r := g.target(i)
	return r.Add(geom.Pt(0, -g.offset.Value()))
}

// Arrive has the tiles of the next layout fly in, one after another, each from the place from gives it in the
// grid's space, or grow in where from gives none.
func (g *TileGrid) Arrive(from func(i int) (geom.Rect, bool), u *gunim.UI) {
	g.arrive, g.depart, g.departing = from, nil, false
	u.Invalidate()
}

// Depart has the tiles built fly to the places to gives them in the grid's space, fading as they go; the grid
// builds no more until Arrive. Departed reports when they have got there.
func (g *TileGrid) Depart(to func(i int) (geom.Rect, bool), u *gunim.UI) {
	g.depart, g.departing, g.arrive = to, true, nil
	u.Invalidate()
}

// Departed reports whether the tiles sent off by Depart have all arrived.
func (g *TileGrid) Departed() bool {
	if !g.departing {
		return false
	}
	for _, c := range g.live {
		if c.moving() {
			return false
		}
	}
	return g.depart == nil
}

// Focusable implements [gunim.Focusable].
func (g *TileGrid) Focusable() bool { return !g.departing }

// ZoomsWithWheel implements [gunim.WheelZoomer].
func (g *TileGrid) ZoomsWithWheel() bool { return g.OnZoom != nil }

// DragHeld implements [gunim.DragHolder]: a band held near an edge scrolls the grid.
func (g *TileGrid) DragHeld() bool { return g.banding }

// EdgeScroll implements [gunim.EdgeScroller].
func (g *TileGrid) EdgeScroll(p geom.Point, dt time.Duration, u *gunim.UI) geom.Point {
	return g.edgeScroll(p, dt, u)
}

// Reveal implements [gunim.Revealer].
func (g *TileGrid) Reveal(r geom.Rect, u *gunim.UI) { g.reveal(r, u) }

// Step implements [gunim.Animator].
func (g *TileGrid) Step(dt time.Duration) bool {
	a := g.scrolling.Step(dt)
	b := g.band.Step(dt)
	c := g.bandIn.Step(dt)
	return a || b || c
}

// target returns where tile i belongs, in the content's space, as last laid out.
func (g *TileGrid) target(i int) geom.Rect {
	cols := max(g.cols, 1)
	r, c := i/cols, i%cols
	return geom.Rc(g.left+float32(c)*g.step.W, g.pad+float32(r)*g.step.H, g.Size.W, g.Size.H)
}

// TileAt returns the tile at p, in the grid's own space, or -1 for none.
func (g *TileGrid) TileAt(p geom.Point) int { return g.at(p) }

// at returns the tile at p, in the grid's space, or -1 over empty space.
func (g *TileGrid) at(p geom.Point) int {
	if g.cols == 0 || g.step.W <= 0 || g.step.H <= 0 {
		return -1
	}
	x, y := p.X-g.left, p.Y+g.offset.Value()-g.pad
	if x < 0 || y < 0 {
		return -1
	}
	c, r := int(x/g.step.W), int(y/g.step.H)
	if c >= g.cols || x-float32(c)*g.step.W > g.Size.W || y-float32(r)*g.step.H > g.Size.H {
		return -1
	}
	if i := r*g.cols + c; i < g.n {
		return i
	}
	return -1
}

// inBand returns the runs of tiles band touches, in the content's space.
func (g *TileGrid) inBand(band geom.Rect) [][2]int {
	if g.cols == 0 || g.n == 0 {
		return nil
	}
	col := func(x float32, end bool) int {
		c := (x - g.left) / g.step.W
		if end {
			return min(g.cols-1, int(math.Floor(float64(c))))
		}
		// Past a tile's right edge, into the gap, the band has not reached the next.
		k := int(math.Floor(float64(c)))
		if x-g.left-float32(k)*g.step.W > g.Size.W {
			k++
		}
		return max(0, k)
	}
	row := func(y float32, end bool) int {
		r := (y - g.pad) / g.step.H
		if end {
			return int(math.Floor(float64(r)))
		}
		k := int(math.Floor(float64(r)))
		if y-g.pad-float32(k)*g.step.H > g.Size.H {
			k++
		}
		return max(0, k)
	}
	c0, c1 := col(band.Min.X, false), col(band.Max.X, true)
	r0, r1 := row(band.Min.Y, false), row(band.Max.Y, true)
	var runs [][2]int
	for r := r0; r <= r1 && c0 <= c1; r++ {
		a, b := r*g.cols+c0, min(r*g.cols+c1+1, g.n)
		if a >= b {
			break
		}
		runs = addRun(runs, a, b)
	}
	return runs
}

// Handle implements [gunim.Handler].
func (g *TileGrid) Handle(e input.Event, u *gunim.UI) bool {
	if g.barEvent(e, u) {
		return true
	}
	if g.typed.take(e, u, g, g.OnType) {
		return true
	}
	switch e := e.(type) {
	case input.FocusGained:
		g.focused = true
		u.Invalidate()
		return true
	case input.FocusRing:
		g.cue.follow(e)
		u.Invalidate()
		return true
	case input.FocusLost:
		g.focused = false
		u.Invalidate()
		return true
	case input.Scroll:
		if e.Mods.Has(input.ModControl) && g.OnZoom != nil {
			g.OnZoom(e.Notches.Y, u)
			return true
		}
		return g.handle(e, u)
	case input.PointerDown:
		return g.press(e, u)
	case input.PointerMove:
		if g.banding {
			g.drawBand(e.Pos, u)
			return true
		}
		if l := g.lift; l.armed && !l.dragging {
			if d := e.Pos.Sub(l.at); d.X*d.X+d.Y*d.Y >= pickUp*pickUp && g.IsSelected(l.tile) {
				g.startDrag(u)
			}
			return true
		}
		i := g.at(e.Pos)
		if g.onBar {
			i = -1
		}
		if i != g.hover {
			g.hover = i
			u.Invalidate()
		}
		return false
	case input.PointerUp:
		if g.lift.armed {
			l := g.lift
			g.lift = tileLift{}
			if l.later && !l.dragging {
				g.pick(l.tile, l.mods, u)
			}
			return true
		}
		if !g.banding {
			return false
		}
		g.banding = false
		g.bandIn.Animate(0, Settle.Get(u.Theme()))
		u.Invalidate()
		return true
	case input.PointerLeave:
		if g.hover >= 0 && !g.banding {
			g.hover = -1
			u.Invalidate()
		}
		return false
	case input.DragEnd:
		g.lift = tileLift{}
		if g.OnDragEnd != nil {
			if in := g.OnDragEnd(e); in != nil {
				u.Send(g, in)
			}
		}
		return true
	case input.KeyPress:
		return g.key(e, u)
	}
	return false
}

// startDrag drags the tiles selected, for a press that moved far enough.
func (g *TileGrid) startDrag(u *gunim.UI) {
	data, ghost, grab := g.DragTiles(slices.Clone(g.runs), g.lift.at)
	if data == nil {
		g.lift = tileLift{}
		return
	}
	g.lift.dragging, g.lift.later = true, false
	u.StartDrag(g, data, ghost, grab)
}

func (g *TileGrid) press(e input.PointerDown, u *gunim.UI) bool {
	i := g.at(e.Pos)
	if e.Button == input.ButtonSecondary {
		// A context menu around the grid acts on the tile pressed.
		if i >= 0 && !g.IsSelected(i) {
			g.pick(i, 0, u)
		}
		return false
	}
	if e.Button != input.ButtonPrimary || g.departing {
		return false
	}
	if i < 0 {
		ctrl := e.Mods.Has(input.ModControl) || e.Mods.Has(input.ModShift)
		g.bandBase = nil
		if ctrl {
			g.bandBase = slices.Clone(g.runs)
		} else {
			g.setCursor(-1, u)
			g.setRuns(nil, u)
		}
		g.banding = true
		g.bandFrom = e.Pos.Add(geom.Pt(0, g.offset.Value()))
		g.band.Jump(geom.Rect{Min: g.bandFrom, Max: g.bandFrom})
		g.bandIn.Animate(1, Quick.Get(u.Theme()))
		u.Invalidate()
		return true
	}
	if e.Clicks == 2 {
		if g.OnActivate != nil {
			u.Send(g, g.OnActivate(i))
		}
		return true
	}
	if g.DragTiles != nil {
		// A press on a tile selected may start a drag of the selection, so the release changes the selection instead
		g.lift = tileLift{tile: i, at: e.Pos, mods: e.Mods, armed: true}
		if g.IsSelected(i) && !e.Mods.Has(input.ModShift) {
			g.lift.later = true
			return true
		}
	}
	g.pick(i, e.Mods, u)
	return true
}

// drawBand stretches the band to p, in the grid's space, and selects what it touches.
func (g *TileGrid) drawBand(p geom.Point, u *gunim.UI) {
	to := p.Add(geom.Pt(0, g.offset.Value()))
	r := geom.Rect{Min: g.bandFrom, Max: to}.Normalized()
	g.band.Animate(r, Caret.Get(u.Theme()))
	runs := slices.Clone(g.bandBase)
	for _, run := range g.inBand(r) {
		runs = addRun(runs, run[0], run[1])
	}
	if len(runs) == 0 {
		runs = nil
	}
	g.setRuns(runs, u)
	u.Invalidate()
}

func (g *TileGrid) key(e input.KeyPress, u *gunim.UI) bool {
	if e.Mods.Has(input.ModAlt) {
		return false
	}
	if e.Mods.Has(input.ModControl) {
		if e.Key == input.KeyA && e.Mods == input.ModControl && g.n > 0 {
			g.setRuns([][2]int{{0, g.n}}, u)
			return true
		}
		return false
	}
	cols := max(g.cols, 1)
	page := cols * max(1, int(g.viewport/max(g.step.H, 1)))
	at := g.cursor
	var to int
	switch e.Key {
	case input.KeyLeft:
		to = at - 1
	case input.KeyRight:
		to = at + 1
	case input.KeyUp:
		to = at - cols
	case input.KeyDown:
		to = at + cols
	case input.KeyPageUp:
		to = at - page
	case input.KeyPageDown:
		to = at + page
	case input.KeyHome:
		to = 0
	case input.KeyEnd:
		to = g.n - 1
	case input.KeyEnter, input.KeyKPEnter:
		if g.cursor < 0 || g.OnActivate == nil {
			return false
		}
		u.Send(g, g.OnActivate(g.cursor))
		return true
	case input.KeyEscape:
		if g.cursor < 0 && len(g.runs) == 0 {
			return false
		}
		g.setCursor(-1, u)
		g.setRuns(nil, u)
		return true
	default:
		return false
	}
	if g.n == 0 {
		return true
	}
	if at < 0 {
		to = 0
	}
	// Up from the first row and down from the last stay in the column, and go no further.
	if (e.Key == input.KeyUp || e.Key == input.KeyDown) && (to < 0 || to >= g.n) && at >= 0 {
		to = at
		if e.Key == input.KeyDown && at/cols < (g.n-1)/cols {
			to = g.n - 1
		}
	}
	to = min(max(to, 0), g.n-1)
	g.setCursor(to, u)
	if e.Mods.Has(input.ModShift) {
		if g.anchor < 0 {
			g.anchor = max(at, 0)
		}
		g.setRuns([][2]int{{min(g.anchor, to), max(g.anchor, to) + 1}}, u)
	} else {
		g.anchor = to
		g.setRuns([][2]int{{to, to + 1}}, u)
	}
	g.revealNext = to
	u.Invalidate()
	return true
}

// pick changes the selection for a click on tile i.
func (g *TileGrid) pick(i int, mods input.Mods, u *gunim.UI) {
	g.setCursor(i, u)
	switch {
	case mods.Has(input.ModShift):
		if g.anchor < 0 {
			g.anchor = i
		}
		from, to := min(g.anchor, i), max(g.anchor, i)+1
		if mods.Has(input.ModControl) {
			g.setRuns(addRun(slices.Clone(g.runs), from, to), u)
		} else {
			g.setRuns([][2]int{{from, to}}, u)
		}
	case mods.Has(input.ModControl):
		g.anchor = i
		if hasRun(g.runs, i) {
			g.setRuns(removeRun(g.runs, i), u)
		} else {
			g.setRuns(addRun(slices.Clone(g.runs), i, i+1), u)
		}
	default:
		g.anchor = i
		g.setRuns([][2]int{{i, i + 1}}, u)
	}
	u.Invalidate()
}

// setCursor puts the keyboard on tile i; the setRuns that follows tells OnSelect.
func (g *TileGrid) setCursor(i int, _ *gunim.UI) {
	if i != g.cursor {
		g.cursor, g.moved = i, true
	}
}

// setRuns selects runs, and tells OnSelect when that or the cursor changed.
func (g *TileGrid) setRuns(runs [][2]int, u *gunim.UI) {
	if slices.Equal(runs, g.runs) && !g.moved {
		return
	}
	g.runs, g.moved = runs, false
	if g.OnSelect != nil {
		u.Send(g, g.OnSelect(slices.Clone(g.runs), g.cursor))
	}
	u.Invalidate()
}

// Layout implements [gunim.Node].
func (g *TileGrid) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	own := c.Max
	th := f.Theme
	g.th, g.viewport = th, own.H
	gap, pad := TileGap.Get(th), TilePadding.Get(th)
	motion := TileMotion.Get(th)

	oldStep, oldPad, oldCols := g.step, g.pad, g.cols
	g.pad = pad
	g.step = geom.Sz(g.Size.W+gap, g.Size.H+gap)
	g.cols = max(1, int((own.W-2*pad+gap)/g.step.W))
	g.left = max(pad, (own.W-float32(g.cols)*g.step.W+gap)/2)
	rows := (g.n + g.cols - 1) / g.cols
	content := 2*pad + float32(rows)*g.step.H - gap
	relaid := oldCols != 0 && (oldStep != g.step || oldCols != g.cols || oldPad != g.pad)
	if relaid {
		g.hold(oldCols, oldStep, oldPad)
	}
	if i := min(g.jumpRow, g.n-1); i >= 0 {
		g.jumpTo(g.target(i).Min.Y - pad)
	}
	g.jumpRow = -1
	g.fit(max(content, 0), own, th)
	if i := g.revealNext; i >= 0 && i < g.n {
		g.revealNext = -1
		r := g.target(i)
		switch at := g.offset.Value(); {
		case r.Min.Y-gap < at:
			g.ScrollTo(r.Min.Y-gap, Quick.Get(th))
		case r.Max.Y+gap > at+own.H:
			g.ScrollTo(r.Max.Y+gap-own.H, Quick.Get(th))
		}
	}

	children := make(map[*tileCell]gunim.Child, kids.Len())
	for kid := range kids.All {
		if tc, ok := kid.Node().(*tileCell); ok {
			children[tc] = kid
		}
	}
	for i, tc := range g.live {
		if _, ok := children[tc]; !ok {
			delete(g.live, i)
		}
	}
	if g.rebuild {
		g.rebuild = false
		for i, tc := range g.live {
			kids.Drop(tc)
			delete(g.live, i)
			delete(children, tc)
		}
		g.built = [2]int{}
		g.depart, g.departing = nil, false
	}
	offset := g.offset.Value()
	first, last := 0, -1
	if g.n > 0 {
		first = max(0, int((offset-pad)/g.step.H)-1) * g.cols
		last = min(g.n-1, (int((offset+own.H-pad)/g.step.H)+2)*g.cols-1)
	}
	if !g.departing {
		for i, tc := range g.live {
			if i < first || i > last {
				kids.Drop(tc)
				delete(g.live, i)
			}
		}
	}
	arrive := g.arrive
	g.arrive = nil
	var order int
	place := func(i int, tc *tileCell) {
		kid := children[tc]
		to := g.target(i)
		switch {
		case g.departing:
			if g.depart != nil {
				tc.fade.Animate(0, Settle.Get(th))
				if r, ok := g.depart(i); ok {
					tc.moveTo(r.Add(geom.Pt(0, offset)), motion)
				}
			}
		case tc.wait <= 0:
			tc.moveTo(to, motion)
			tc.fade.Animate(1, Settle.Get(th))
		}
		tc.selected = hasRun(g.runs, i)
		tc.sel.Animate(map[bool]float32{false: 0, true: 1}[tc.selected], Quick.Get(th))
		tc.hot.Animate(map[bool]float32{false: 0, true: 1}[i == g.hover && !g.banding], Quick.Get(th))
		tc.cursor = g.cue.on && i == g.cursor
		r := tc.rect()
		kid.Layout(gunim.Tight(geom.Sz(max(r.Size().W, 0), max(r.Size().H, 0))))
		kid.Place(geom.Pt(r.Min.X, r.Min.Y-offset))
		order++
	}
	if g.departing {
		for i, tc := range g.live {
			place(i, tc)
		}
		if g.depart != nil {
			// The flight is set once; the tiles carry on to where they were sent.
			g.depart = nil
		}
	} else {
		for i := first; i <= last; i++ {
			tc, ok := g.live[i]
			if !ok {
				if g.Tile == nil {
					continue
				}
				tc = newTileCell(g.Tile(i))
				tc.jump(g.target(i))
				if relaid || arrive != nil {
					tc.growIn()
				}
				g.live[i] = tc
				children[tc] = kids.Build(tc)
			}
			if arrive != nil {
				if r, ok := arrive(i); ok {
					tc.jump(r.Add(geom.Pt(0, offset)))
					tc.fade.Jump(1)
				} else {
					tc.growIn()
				}
				tc.wait = time.Duration(min(i-first, 24)) * 14 * time.Millisecond
			}
			place(i, tc)
		}
	}
	if span := [2]int{first, last - first + 1}; span != g.built && !g.departing && g.OnView != nil && g.n > 0 {
		g.built = span
		if v := g.OnView(first, last-first+1); v != nil {
			f.Send(g, v)
		}
	}
	return own
}

// hold keeps the tile the keyboard is on, or the first in view, where it is on screen as the tiles move to a new
// layout, and moves the tiles built by as much as the view moves, so each springs from where it was.
func (g *TileGrid) hold(oldCols int, oldStep geom.Size, oldPad float32) {
	offset := g.offset.Value()
	i := g.cursor
	oldY := func(i int) float32 { return oldPad + float32(i/oldCols)*oldStep.H }
	if i < 0 || oldY(i)+oldStep.H < offset || oldY(i) > offset+g.viewport {
		i = max(0, int((offset-oldPad)/oldStep.H)) * oldCols
	}
	if i >= g.n {
		return
	}
	d := g.target(i).Min.Y - oldY(i)
	if d == 0 {
		return
	}
	g.shift(d)
	for _, tc := range g.live {
		anim.Shift(tc.y, d)
	}
}

// Paint implements [gunim.Node].
func (g *TileGrid) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	th := f.Theme
	// Drawn last, over the rest
	defer func() {
		if g.cue.whole {
			groupRing(p, geom.Rect{Max: box.Point()}, 0, 1, th)
		}
	}()
	func() {
		defer g.layer(p, geom.Rect{Max: box.Point()}, f.Theme)()
		offset := g.offset.Value()
		for kid := range kids.All {
			tc, ok := kid.Node().(*tileCell)
			if !ok || kid.Presence() == gunim.Exiting {
				continue
			}
			r := tc.rect()
			if r.Max.Y-offset < 0 || r.Min.Y-offset > box.H {
				continue
			}
			kid.Paint(p)
		}
		if t := g.bandIn.Value(); t > 0.01 {
			b := g.band.Value().Add(geom.Pt(0, -offset))
			fill, edge := BandFill.Get(th), BandEdge.Get(th)
			p.RRectStroke(b, 3, paint.Solid(scaleAlpha(fill, t)), paint.Stroke{Width: 1, Color: scaleAlpha(edge, t)})
		}
	}()
	g.paintBar(p, f)
}

func scaleAlpha(c color.NRGBA, t float32) color.NRGBA {
	c.A = uint8(float32(c.A) * min(max(t, 0), 1))
	return c
}

// tileCell holds one tile in a grid, springing to its place and size, with its selection drawn behind it.
type tileCell struct {
	anim.Group
	child          gunim.Node
	x, y, w, h     *anim.Float
	fade, sel, hot *anim.Float
	selected       bool
	cursor         bool
	// wait holds the tile where it is for a moment, so tiles arriving together go one after another.
	wait time.Duration
}

func newTileCell(child gunim.Node) *tileCell {
	c := &tileCell{child: child, x: anim.NewFloat(0), y: anim.NewFloat(0), w: anim.NewFloat(0), h: anim.NewFloat(0),
		fade: anim.NewFloat(1), sel: anim.NewFloat(0), hot: anim.NewFloat(0)}
	c.Add(c.x, c.y, c.w, c.h, c.fade, c.sel, c.hot)
	return c
}

// Step implements [gunim.Animator].
func (c *tileCell) Step(dt time.Duration) bool {
	if c.wait > 0 {
		c.wait -= dt
		return true
	}
	return c.Group.Step(dt)
}

func (c *tileCell) jump(r geom.Rect) {
	c.x.Jump(r.Min.X)
	c.y.Jump(r.Min.Y)
	c.w.Jump(r.Size().W)
	c.h.Jump(r.Size().H)
}

// growIn has the tile grow and fade in from a little smaller, about its middle.
func (c *tileCell) growIn() {
	w, h := c.w.Target(), c.h.Target()
	c.x.Jump(c.x.Target() + w*0.15)
	c.y.Jump(c.y.Target() + h*0.15)
	c.w.Jump(w * 0.7)
	c.h.Jump(h * 0.7)
	c.fade.Jump(0)
}

func (c *tileCell) moveTo(r geom.Rect, m anim.Motion) {
	c.x.Animate(r.Min.X, m)
	c.y.Animate(r.Min.Y, m)
	c.w.Animate(r.Size().W, m)
	c.h.Animate(r.Size().H, m)
}

func (c *tileCell) moving() bool {
	return c.wait > 0 || c.x.Active() || c.y.Active() || c.w.Active() || c.h.Active() || c.fade.Active()
}

// rect returns where the tile is now, in the content's space.
func (c *tileCell) rect() geom.Rect {
	return geom.Rc(c.x.Value(), c.y.Value(), c.w.Value(), c.h.Value())
}

// Children implements [gunim.Composite].
func (c *tileCell) Children() []gunim.Node { return []gunim.Node{c.child} }

// Layout implements [gunim.Node].
func (c *tileCell) Layout(cs gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	kid.Layout(cs)
	kid.Place(geom.Point{})
	return cs.Max
}

// Paint implements [gunim.Node].
func (c *tileCell) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	fade := min(max(c.fade.Value(), 0), 1)
	if fade <= 0.001 {
		return
	}
	if fade < 0.999 {
		defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}.Inset(geom.Uniform(-8)), Opacity: fade})()
	}
	th := f.Theme
	r := geom.Rect{Max: box.Point()}
	radius := TileRadius.Get(th)
	if hot := c.hot.Value() * (1 - c.sel.Value()); hot > 0.01 {
		p.RRect(r, radius, paint.Solid(scaleAlpha(TileHover.Get(th), hot)))
	}
	if sel := c.sel.Value(); sel > 0.01 {
		p.RRect(r, radius, paint.Solid(scaleAlpha(TileSelected.Get(th), sel)))
	}
	if c.cursor {
		p.RRectStroke(r.Inset(geom.Uniform(0.75)), radius, paint.Solid(color.NRGBA{}), paint.Stroke{Width: 1.5, Color: TileCursor.Get(th)})
	}
	kids.At(0).Paint(p)
}
