package widget

import (
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
)

// DataGrid tokens.
var (
	GridRowHeight    = theme.Length("grid.row", 22)
	GridHeaderHeight = theme.Length("grid.header", 26)
	GridTextSize     = theme.Length("grid.text.size", 12.5)
	// GridCellPadding is the room at either side of a cell's text.
	GridCellPadding = theme.Length("grid.cell.padding", 6)
	// GridCursor lights the selected row, and GridRule marks a row that
	// starts something new, such as a session in a log.
	GridCursor = theme.Color("grid.cursor", color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0x38})
	GridRule   = theme.Color("grid.rule", color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0x90})
	// GridPending draws a row that has not arrived yet.
	GridPending = theme.Color("grid.pending", color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0x14})
	// GridChipRadius rounds a span drawn on a fill.
	GridChipRadius = theme.Length("grid.chip.radius", 4)
	// GridBarWidth is the width of the scrollbar's track.
	GridBarWidth = theme.Length("grid.bar.width", 10)
)

// GridColumn is one column of a [DataGrid].
type GridColumn struct {
	Title string
	// Width is the column's width in logical pixels. Zero takes what the
	// other columns leave, and at least GridMinFill.
	Width float32
	// End lines the column's text up at its right edge, as numbers are.
	End bool
	// Face is the face the column is set in, and the theme's [Font] when
	// unset.
	Face theme.Token[*text.Face]
	// Closable shows a cross on the title under the pointer, which sends
	// the grid's OnClose.
	Closable bool
}

// GridMinFill is the narrowest a column with no width gets.
const GridMinFill = 120

// GridSpan is a piece of a cell's text in a colour and face of its own.
type GridSpan struct {
	Text string
	// Ink is the text's colour, and the theme's [Ink] when unset.
	Ink theme.Token[color.NRGBA]
	// Fill, when set, draws the span on a rounded chip of that colour.
	Fill theme.Token[color.NRGBA]
	// Face is the span's face, and the column's when unset.
	Face theme.Token[*text.Face]
	// Faint draws the span at half strength.
	Faint bool
}

// GridRow is what a [DataGrid] shows for one row: the spans of each
// column's cell, in order.
type GridRow struct {
	Cells [][]GridSpan
	// Tint, when set, colours the row's background.
	Tint theme.Token[color.NRGBA]
	// Rule draws a line along the row's top.
	Rule bool
	// Dim draws the whole row at half strength, as for a row about to be
	// replaced.
	Dim bool
}

// Text returns the row as plain text: its cells' spans joined, with a
// tab between cells.
func (r GridRow) Text() string {
	var b strings.Builder
	for i, cell := range r.Cells {
		if i > 0 {
			b.WriteByte('\t')
		}
		for _, s := range cell {
			b.WriteString(s.Text)
		}
	}
	return b.String()
}

// DataGrid shows rows of styled cells by index, drawing only the rows in
// view, so a grid of millions of rows costs what the rows on screen do.
//
// The grid asks Row for each row it draws. A row that has not arrived
// yet draws as a placeholder, and OnView tells the application which
// rows are in view, so it can send them. The grid scrolls on a spring,
// sideways too when the columns are wider than it, and its scrollbar can
// be dragged. Columns resize from the edges of their titles.
//
// One row can be selected: by a click, which on the selected row clears
// it, and by the arrow keys, Page Up and Page Down, Home and End. Escape
// clears it, and Ctrl+C copies it.
type DataGrid struct {
	Columns []GridColumn
	// Row returns row i, and false while it has not arrived.
	Row func(i int) (GridRow, bool)
	// OnView turns the rows in view, first to first+count, into an intent.
	OnView func(first, count int) gunim.Intent
	// OnSelect turns a change of selection into an intent; row is -1
	// when nothing is selected.
	OnSelect func(row int) gunim.Intent
	// OnActivate turns a double click or Enter on a row into an intent.
	OnActivate func(row int) gunim.Intent
	// OnResize turns a column resized by a drag into an intent.
	OnResize func(column int, width float32) gunim.Intent
	// OnClose turns a click on a closable column's cross into an intent.
	OnClose func(column int) gunim.Intent
	// Copy returns the text Ctrl+C copies for row i, and the row's
	// [GridRow.Text] when nil.
	Copy func(i int) string
	// NoHeader hides the column titles, and NoBar the scrollbar, for a
	// grid that shows its position some other way.
	NoHeader bool
	NoBar    bool

	rows     int
	selected int
	focused  bool

	// top is the row at the top of the view, with a fraction for a row
	// partly scrolled off, goal where the spring carries it, and vel its
	// speed in rows a second.
	top, goal, vel float64
	// left is how far the columns are scrolled sideways.
	left float32

	// From the last layout: the view's size, the rows it holds, and each
	// column's left edge and width, before scrolling sideways.
	view      geom.Size
	header    float32
	rowH      float32
	xs        [][2]float32
	contentW  float32
	sentFirst int
	sentCount int

	// drag is what the pointer holds, and grab where it took hold.
	drag     gridDrag
	dragCol  int
	grab     float32
	grabFrom float64
	hoverCol int

	pending bool
	pulse   float64

	// th is the window's live theme, kept from Layout for Step.
	th *theme.Live

	shapes map[spanKey]*spanShape
	frame  uint64
	cut    map[cutKey]text.Run
}

type gridDrag uint8

const (
	dragNone gridDrag = iota
	dragBar
	dragEdge
)

type spanKey struct {
	row       int
	col, span int32
}

type spanShape struct {
	face *text.Face
	s    string
	size float32
	run  text.Run
	seen uint64
}

type cutKey struct {
	face *text.Face
	size float32
}

// NewDataGrid returns an empty grid of columns.
func NewDataGrid(columns ...GridColumn) *DataGrid {
	return &DataGrid{
		Columns:   columns,
		selected:  -1,
		sentFirst: -1,
		hoverCol:  -1,
		shapes:    map[spanKey]*spanShape{},
		cut:       map[cutKey]text.Run{},
	}
}

// Rows returns how many rows the grid has.
func (g *DataGrid) Rows() int { return g.rows }

// SetRows sets how many rows the grid has. The view stays where it is,
// inside the new rows, and a selection past the end is cleared.
func (g *DataGrid) SetRows(n int, u *gunim.UI) {
	g.rows = max(n, 0)
	if g.selected >= g.rows {
		g.selected = -1
	}
	g.goal = g.clampTop(g.goal)
	g.top = g.clampTop(g.top)
	g.sentFirst = -1
	u.Invalidate()
}

// Selected returns the selected row, and false when there is none.
func (g *DataGrid) Selected() (int, bool) { return g.selected, g.selected >= 0 }

// Select selects row i, or clears the selection for -1, without telling
// OnSelect. With reveal, the view scrolls to show it.
func (g *DataGrid) Select(i int, reveal bool, u *gunim.UI) {
	if i < 0 || i >= g.rows {
		i = -1
	}
	g.selected = i
	if reveal && i >= 0 {
		g.reveal(i)
	}
	u.Invalidate()
}

// Top returns the row at the top of the view, with a fraction for a row
// partly scrolled off.
func (g *DataGrid) Top() float64 { return g.top }

// Visible returns how many rows the view holds.
func (g *DataGrid) Visible() float64 {
	if g.rowH <= 0 {
		return 0
	}
	return float64(g.view.H-g.header) / float64(g.rowH)
}

// ScrollTo carries the view on its spring until row is at the top.
func (g *DataGrid) ScrollTo(row float64, u *gunim.UI) {
	g.goal = g.clampTop(row)
	u.Invalidate()
}

// JumpTo puts row at the top of the view at once, or in the middle with
// center, for rows that are new rather than moved.
func (g *DataGrid) JumpTo(row float64, center bool, u *gunim.UI) {
	if center {
		row -= g.Visible() / 2
	}
	g.goal = g.clampTop(row)
	g.top, g.vel = g.goal, 0
	u.Invalidate()
}

func (g *DataGrid) clampTop(row float64) float64 {
	return max(0, min(row, float64(g.rows)-g.Visible()))
}

// reveal scrolls just far enough to show row i whole.
func (g *DataGrid) reveal(i int) {
	vis := g.Visible()
	switch {
	case float64(i) < g.goal:
		g.goal = g.clampTop(float64(i))
	case float64(i+1) > g.goal+vis:
		g.goal = g.clampTop(float64(i+1) - vis)
	}
}

func (g *DataGrid) selectAndTell(i int, u *gunim.UI) {
	if g.rows == 0 {
		return
	}
	if i >= 0 {
		i = min(max(i, 0), g.rows-1)
	}
	if i == g.selected {
		if i >= 0 {
			g.reveal(i)
		}
		return
	}
	g.selected = i
	if i >= 0 {
		g.reveal(i)
	}
	if g.OnSelect != nil {
		if v := g.OnSelect(i); v != nil {
			u.Send(g, v)
		}
	}
	u.Invalidate()
}

// Focusable implements [gunim.Focusable].
func (g *DataGrid) Focusable() bool { return true }

// Step implements [gunim.Animator].
func (g *DataGrid) Step(dt time.Duration) bool {
	moving := false
	if g.top != g.goal || g.vel != 0 {
		g.top, g.vel = Quick.Get(g.th).Follow(g.top, g.vel, g.goal, dt)
		// Still once it is a hundredth of a pixel away and all but stopped.
		px := float64(max(g.rowH, 1))
		if math.Abs(g.top-g.goal)*px < 0.01 && math.Abs(g.vel)*px < 1 {
			g.top, g.vel = g.goal, 0
		}
		moving = true
	}
	if g.pending {
		g.pulse += dt.Seconds()
		moving = true
	}
	return moving
}

// Layout implements [gunim.Node]. The grid fills the space it is given.
func (g *DataGrid) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	g.view = c.Max
	g.th = th
	g.rowH = GridRowHeight.Get(th)
	g.header = 0
	if !g.NoHeader {
		g.header = GridHeaderHeight.Get(th)
	}
	viewW := g.bodyWidth(th)
	fixed, fills := float32(0), 0
	for _, col := range g.Columns {
		if col.Width > 0 {
			fixed += col.Width
		} else {
			fills++
		}
	}
	fill := max(GridMinFill, (viewW-fixed)/float32(max(fills, 1)))
	g.xs = g.xs[:0]
	x := float32(0)
	for _, col := range g.Columns {
		w := col.Width
		if w <= 0 {
			w = fill
		}
		g.xs = append(g.xs, [2]float32{x, w})
		x += w
	}
	g.contentW = x
	g.left = max(0, min(g.left, g.contentW-viewW))
	g.goal = g.clampTop(g.goal)
	if !g.dragging() {
		g.top = g.clampTop(g.top)
	}

	if g.OnView != nil && g.rowH > 0 {
		first := int(math.Floor(g.top))
		count := int(math.Ceil(g.Visible())) + 1
		if first != g.sentFirst || count != g.sentCount {
			g.sentFirst, g.sentCount = first, count
			if v := g.OnView(first, count); v != nil {
				f.Send(g, v)
			}
		}
	}
	return c.Max
}

func (g *DataGrid) dragging() bool { return g.drag != dragNone }

// bodyWidth is the width the columns show in, less the scrollbar.
func (g *DataGrid) bodyWidth(th *theme.Live) float32 {
	if g.NoBar {
		return g.view.W
	}
	return max(0, g.view.W-GridBarWidth.Get(th))
}

// Paint implements [gunim.Node].
func (g *DataGrid) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	g.frame++
	g.pending = false
	bodyW := g.bodyWidth(th)
	size := GridTextSize.Get(th)
	pad := GridCellPadding.Get(th)

	if g.rowH > 0 && g.Row != nil {
		func() {
			defer p.Layer(paint.LayerOpts{Bounds: geom.Rc(0, g.header, bodyW, box.H-g.header), Opacity: 1, Clip: true})()
			first := int(math.Floor(g.top))
			last := min(g.rows, int(math.Ceil(g.top+g.Visible()))+1)
			for i := first; i < last; i++ {
				y := g.header + float32((float64(i)-g.top)*float64(g.rowH))
				g.paintRow(p, th, i, y, bodyW, size, pad)
			}
		}()
	}
	if !g.NoHeader {
		g.paintHeader(p, th, bodyW, size, pad)
	}
	if !g.NoBar {
		g.paintBar(p, th)
	}
	// Forget the shapes of rows that left the view, once there are many.
	if len(g.shapes) > 4096 {
		for k, s := range g.shapes {
			if s.seen != g.frame {
				delete(g.shapes, k)
			}
		}
	}
}

func (g *DataGrid) paintRow(p *paint.Painter, th *theme.Live, i int, y, bodyW, size, pad float32) {
	row, ok := g.Row(i)
	band := geom.Rc(0, y, bodyW, g.rowH)
	if !ok {
		g.pending = true
		c := GridPending.Get(th)
		c.A = uint8(float32(c.A) * float32(0.65+0.35*math.Sin(g.pulse*4)))
		for _, x := range g.xs {
			left := x[0] - g.left + pad
			if left > bodyW || left+x[1] < 0 {
				continue
			}
			p.RRect(geom.Rc(left, y+g.rowH*0.3, max(0, x[1]*0.6-pad), g.rowH*0.4), g.rowH*0.2, paint.Solid(c))
		}
		return
	}
	if row.Tint.Key() != "" {
		p.RRect(band, 0, paint.Solid(row.Tint.Get(th)))
	}
	if i == g.selected {
		c := GridCursor.Get(th)
		if !g.focused {
			c.A = c.A * 2 / 3
		}
		p.RRect(band, 0, paint.Solid(c))
	}
	if row.Rule {
		p.RRect(geom.Rc(0, y, bodyW, 1), 0, paint.Solid(GridRule.Get(th)))
	}
	for c, spans := range row.Cells {
		if c >= len(g.xs) {
			break
		}
		x, w := g.xs[c][0]-g.left, g.xs[c][1]
		if x > bodyW || x+w < 0 {
			continue
		}
		g.paintCell(p, th, i, c, spans, row.Dim, x, y, w, size, pad)
	}
}

// paintCell sets a cell's spans one after another, cutting the last that
// fits short with an ellipsis.
func (g *DataGrid) paintCell(p *paint.Painter, th *theme.Live, i, c int, spans []GridSpan, dim bool,
	x, y, w, size, pad float32) {
	colFace := faceIn(g.Columns[c].Face, th)
	chipPad := float32(4)
	runs := make([]text.Run, len(spans))
	total := float32(0)
	for k, s := range spans {
		face := colFace
		if s.Face.Key() != "" {
			face = faceIn(s.Face, th)
		}
		runs[k] = g.shape(spanKey{row: i, col: int32(c), span: int32(k)}, face, s.Text, size)
		total += runs[k].Advance
		if s.Fill.Key() != "" {
			total += 2 * chipPad
		}
	}
	pen := x + pad
	limit := x + w - pad
	if g.Columns[c].End && total < limit-pen {
		pen = limit - total
	}
	for k, s := range spans {
		run := runs[k]
		ink := Ink.Get(th)
		if s.Ink.Key() != "" {
			ink = s.Ink.Get(th)
		}
		if s.Faint || dim {
			ink.A /= 2
		}
		chip := s.Fill.Key() != ""
		room := limit - pen
		if chip {
			room -= 2 * chipPad
		}
		cut := run.Advance > room
		// A gap that does not fit ends the cell rather than showing a lone ellipsis
		if cut && strings.TrimSpace(s.Text) == "" {
			return
		}
		if cut {
			run = g.cutRun(run, room)
		}
		ty := y + (g.rowH-run.Height())/2
		if chip {
			fill := s.Fill.Get(th)
			if s.Faint || dim {
				fill.A /= 2
			}
			p.RRect(geom.Rc(pen, y+3, run.Advance+2*chipPad, g.rowH-6), GridChipRadius.Get(th), paint.Solid(fill))
			pen += chipPad
		}
		run.Paint(p, geom.Pt(pen, ty), ink)
		pen += run.Advance
		if chip {
			pen += chipPad
		}
		if cut || pen >= limit {
			return
		}
	}
}

// shape returns s shaped in face at size, from the grid's cache when it
// was shaped for the same span before.
func (g *DataGrid) shape(k spanKey, face *text.Face, s string, size float32) text.Run {
	sh, ok := g.shapes[k]
	if !ok {
		sh = &spanShape{}
		g.shapes[k] = sh
	}
	if sh.face != face || sh.s != s || sh.size != size || !ok {
		sh.face, sh.s, sh.size, sh.run = face, s, size, face.Shape(s, size)
	}
	sh.seen = g.frame
	return sh.run
}

// cutRun returns run cut to the glyphs that fit in room with an ellipsis
// after them.
func (g *DataGrid) cutRun(run text.Run, room float32) text.Run {
	key := cutKey{run.Face, run.Size}
	ell, ok := g.cut[key]
	if !ok {
		ell = run.Face.Shape("…", run.Size)
		g.cut[key] = ell
	}
	avail := room - ell.Advance
	n := 0
	for n < len(run.Glyphs) {
		end := run.Advance
		if n+1 < len(run.Glyphs) {
			end = run.Glyphs[n+1].At.X
		}
		if end > avail {
			break
		}
		n++
	}
	out := run
	pen := float32(0)
	if n > 0 {
		pen = run.Glyphs[n].At.X
	}
	out.Glyphs = make([]paint.Glyph, 0, n+len(ell.Glyphs))
	out.Glyphs = append(out.Glyphs, run.Glyphs[:n]...)
	if avail < 0 {
		out.Glyphs, pen = out.Glyphs[:0], 0
	}
	for _, gl := range ell.Glyphs {
		gl.At.X += pen
		out.Glyphs = append(out.Glyphs, gl)
	}
	out.Advance = pen + ell.Advance
	return out
}

func (g *DataGrid) paintHeader(p *paint.Painter, th *theme.Live, bodyW, size, pad float32) {
	h := g.header
	p.RRect(geom.Rc(0, 0, g.view.W, h), 0, paint.Solid(DialogFill.Get(th)))
	ink := TableHeader.Get(th)
	face := faceIn(Font, th)
	for c, col := range g.Columns {
		if c >= len(g.xs) {
			break
		}
		x, w := g.xs[c][0]-g.left, g.xs[c][1]
		if x > bodyW || x+w < 0 {
			continue
		}
		run := g.shape(spanKey{row: -1, col: int32(c)}, face, col.Title, size)
		room := w - 2*pad
		closing := col.Closable && c == g.hoverCol
		if closing {
			room -= h / 2
		}
		if run.Advance > room {
			run = g.cutRun(run, room)
		}
		at := x + pad
		if col.End {
			at = x + w - pad - run.Advance
		}
		run.Paint(p, geom.Pt(at, (h-run.Height())/2), ink)
		if closing {
			g.paintCross(p, geom.Pt(x+w-pad-h/4, h/2), h/4, ink)
		}
		if c < len(g.Columns)-1 {
			p.RRect(geom.Rc(x+w-1, h*0.25, 1, h*0.5), 0, paint.Solid(MenuBorder.Get(th)))
		}
	}
	p.RRect(geom.Rc(0, h-1, g.view.W, 1), 0, paint.Solid(MenuBorder.Get(th)))
}

// paintCross draws a small ×, r across, centred on at.
func (g *DataGrid) paintCross(p *paint.Painter, at geom.Point, r float32, c color.NRGBA) {
	for _, turn := range [2]float32{math.Pi / 4, -math.Pi / 4} {
		func() {
			defer p.Push(paint.Rotate(turn, at))()
			p.RRect(geom.Rc(at.X-r/2, at.Y-0.75, r, 1.5), 0.75, paint.Solid(c))
		}()
	}
}

// barRect returns the scrollbar's track, and its thumb within it.
func (g *DataGrid) barRect(th *theme.Live) (track, thumb geom.Rect, ok bool) {
	vis := g.Visible()
	if g.NoBar || g.rows == 0 || float64(g.rows) <= vis {
		return geom.Rect{}, geom.Rect{}, false
	}
	bw := GridBarWidth.Get(th)
	track = geom.Rc(g.view.W-bw, g.header, bw, g.view.H-g.header)
	length := max(24, track.Size().H*float32(vis/float64(g.rows)))
	span := track.Size().H - length
	at := float32(g.top / (float64(g.rows) - vis))
	thumb = geom.Rc(track.Min.X+2, track.Min.Y+span*max(0, min(at, 1)), bw-4, length)
	return track, thumb, true
}

func (g *DataGrid) paintBar(p *paint.Painter, th *theme.Live) {
	_, thumb, ok := g.barRect(th)
	if !ok {
		return
	}
	col := ScrollbarColor.Get(th)
	if g.drag == dragBar {
		col.A = uint8(min(255, int(col.A)*2))
	}
	p.RRect(thumb, thumb.Size().W/2, paint.Solid(col))
}

// Cursor implements [gunim.CursorShaper]: resize arrows over a column's
// edge.
func (g *DataGrid) Cursor(pt geom.Point) input.Cursor {
	if g.drag == dragEdge || g.edgeAt(pt) >= 0 {
		return input.CursorResizeH
	}
	return input.CursorArrow
}

// edgeAt returns the column whose right edge in the header is under pt,
// or -1.
func (g *DataGrid) edgeAt(pt geom.Point) int {
	if g.NoHeader || pt.Y < 0 || pt.Y >= g.header {
		return -1
	}
	for c, x := range g.xs {
		if g.Columns[c].Width <= 0 {
			continue
		}
		edge := x[0] + x[1] - g.left
		if pt.X >= edge-4 && pt.X <= edge+4 {
			return c
		}
	}
	return -1
}

// columnAt returns the column under x, or -1.
func (g *DataGrid) columnAt(x float32) int {
	for c, xs := range g.xs {
		if x >= xs[0]-g.left && x < xs[0]+xs[1]-g.left {
			return c
		}
	}
	return -1
}

func (g *DataGrid) rowAt(y float32) int {
	if y < g.header || g.rowH <= 0 {
		return -1
	}
	i := int(math.Floor(g.top + float64((y-g.header)/g.rowH)))
	if i < 0 || i >= g.rows {
		return -1
	}
	return i
}

func (g *DataGrid) send(v gunim.Intent, u *gunim.UI) {
	if v != nil {
		u.Send(g, v)
	}
}

// Handle implements [gunim.Handler].
func (g *DataGrid) Handle(e input.Event, u *gunim.UI) bool {
	th := u.Theme()
	switch e := e.(type) {
	case input.FocusGained:
		g.focused = true
		u.Invalidate()
		return true
	case input.FocusLost:
		g.focused = false
		u.Invalidate()
		return true
	case input.Scroll:
		dx, dy := e.Delta.X, e.Delta.Y
		if e.Mods.Has(input.ModShift) && dx == 0 {
			dx, dy = dy, 0
		}
		if dy != 0 && g.rowH > 0 {
			g.goal = g.clampTop(g.goal - float64(dy/g.rowH))
		}
		if dx != 0 {
			g.left = max(0, min(g.left-dx, g.contentW-g.bodyWidth(th)))
		}
		u.Invalidate()
		return true
	case input.PointerDown:
		return g.press(e, u)
	case input.PointerMove:
		return g.move(e, u)
	case input.PointerUp:
		if g.drag == dragNone {
			return false
		}
		if g.drag == dragEdge && g.OnResize != nil {
			g.send(g.OnResize(g.dragCol, g.Columns[g.dragCol].Width), u)
		}
		g.drag = dragNone
		u.Invalidate()
		return true
	case input.PointerLeave:
		if g.hoverCol >= 0 {
			g.hoverCol = -1
			u.Invalidate()
		}
	case input.KeyPress:
		return g.key(e, u)
	}
	return false
}

func (g *DataGrid) press(e input.PointerDown, u *gunim.UI) bool {
	if e.Button != input.ButtonPrimary {
		return false
	}
	th := u.Theme()
	if c := g.edgeAt(e.Pos); c >= 0 {
		g.drag, g.dragCol, g.grab = dragEdge, c, e.Pos.X-g.Columns[c].Width
		return true
	}
	if track, thumb, ok := g.barRect(th); ok && track.Contains(e.Pos) {
		if !thumb.Contains(e.Pos) {
			// A press on the track brings the thumb's middle there.
			span := track.Size().H - thumb.Size().H
			at := (e.Pos.Y - track.Min.Y - thumb.Size().H/2) / max(span, 1)
			g.JumpTo(float64(at)*(float64(g.rows)-g.Visible()), false, u)
		}
		g.drag, g.grab, g.grabFrom = dragBar, e.Pos.Y, g.top
		u.Invalidate()
		return true
	}
	if !g.NoHeader && e.Pos.Y < g.header {
		c := g.columnAt(e.Pos.X)
		if c >= 0 && g.Columns[c].Closable && g.OnClose != nil {
			x := g.xs[c][0] + g.xs[c][1] - g.left
			if e.Pos.X >= x-GridCellPadding.Get(th)-g.header/2 {
				g.send(g.OnClose(c), u)
			}
		}
		return true
	}
	i := g.rowAt(e.Pos.Y)
	if i < 0 {
		return true
	}
	if e.Clicks == 2 {
		if g.OnActivate != nil {
			g.send(g.OnActivate(i), u)
		}
		return true
	}
	if i == g.selected && !e.Focusing {
		g.selectAndTell(-1, u)
	} else {
		g.selectAndTell(i, u)
	}
	return true
}

func (g *DataGrid) move(e input.PointerMove, u *gunim.UI) bool {
	switch g.drag {
	case dragEdge:
		g.Columns[g.dragCol].Width = max(24, e.Pos.X-g.grab)
		u.Invalidate()
		return true
	case dragBar:
		track, thumb, ok := g.barRect(u.Theme())
		if ok {
			span := float64(max(track.Size().H-thumb.Size().H, 1))
			rows := float64(g.rows) - g.Visible()
			g.goal = g.clampTop(g.grabFrom + float64(e.Pos.Y-g.grab)/span*rows)
			g.top, g.vel = g.goal, 0
		}
		u.Invalidate()
		return true
	case dragNone:
	}
	hover := -1
	if !g.NoHeader && e.Pos.Y < g.header {
		hover = g.columnAt(e.Pos.X)
	}
	if hover != g.hoverCol {
		g.hoverCol = hover
		u.Invalidate()
	}
	return false
}

func (g *DataGrid) key(e input.KeyPress, u *gunim.UI) bool {
	if e.Mods.Has(input.ModControl) {
		if e.Key == input.KeyC && g.selected >= 0 {
			u.SetClipboard(g.copyText(g.selected))
			return true
		}
		return false
	}
	if e.Mods.Has(input.ModAlt) {
		return false
	}
	page := max(1, int(g.Visible())-1)
	at := g.selected
	if at < 0 {
		at = int(math.Ceil(g.top)) - 1
	}
	switch e.Key {
	case input.KeyUp:
		g.selectAndTell(max(at-1, 0), u)
	case input.KeyDown:
		g.selectAndTell(at+1, u)
	case input.KeyPageUp:
		g.selectAndTell(max(at-page, 0), u)
	case input.KeyPageDown:
		g.selectAndTell(at+page, u)
	case input.KeyHome:
		g.selectAndTell(0, u)
	case input.KeyEnd:
		g.selectAndTell(g.rows-1, u)
	case input.KeyLeft:
		g.left = max(0, g.left-ScrollLine.Get(u.Theme()))
	case input.KeyRight:
		g.left = max(0, min(g.left+ScrollLine.Get(u.Theme()), g.contentW-g.bodyWidth(u.Theme())))
	case input.KeyEnter, input.KeyKPEnter:
		if g.selected < 0 || g.OnActivate == nil {
			return false
		}
		g.send(g.OnActivate(g.selected), u)
	case input.KeyEscape:
		if g.selected < 0 {
			return false
		}
		g.selectAndTell(-1, u)
	default:
		return false
	}
	u.Invalidate()
	return true
}

func (g *DataGrid) copyText(i int) string {
	if g.Copy != nil {
		return g.Copy(i)
	}
	if g.Row == nil {
		return ""
	}
	row, _ := g.Row(i)
	return row.Text()
}
