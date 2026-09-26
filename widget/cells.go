package widget

import (
	"image/color"
	"math"
	"slices"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
)

// CellGrid shows characters in a grid of equal cells, each with its own
// colours and style, as a terminal does. It fills the space it is given
// and reports, through Fit, how many cells fit there.
//
// The grid keeps its cells; the application sets them a row at a time
// with SetRow, from a view's update function. Each row keeps what it
// drew, and draws again only once it changes, so a screen where one
// line changed costs one line. A screen that changes whole, as a
// scrolling one does, stays cheap: characters take their font's glyphs
// directly, placed at their cells, without the shaping a paragraph
// needs.
//
// The cursor glides to a new cell along its row, and jumps to another
// row.
type CellGrid struct {
	anim.Group
	// Faces are the faces for regular, bold, italic and bold italic
	// text, in that order. They default to Go Mono.
	Faces [4]*text.Face
	// Size is the font size in logical pixels; zero takes the theme's
	// [TextSize].
	Size float32
	// Background fills the grid where a cell's own background is
	// clear. It defaults to the theme's [Background].
	Background theme.Token[color.NRGBA]
	// Foreground colours a cell whose own colour is clear. It defaults
	// to the theme's [Ink].
	Foreground theme.Token[color.NRGBA]

	cols, rows int
	lines      []cellRow
	cursor     Cursor
	at         *anim.Point
	// lit fades the cursor through a blink.
	lit *anim.Float

	// metrics are what the last layout measured the cells by.
	metrics cellMetrics
	// fitCols and fitRows are how many cells fit the last layout's box.
	fitCols, fitRows int
	glyphs           map[glyphKey]cellGlyph
}

// Cell is one character cell of a [CellGrid]. The zero Cell is blank.
type Cell struct {
	// Rune is the character, and Marks the combining marks drawn over
	// it, such as an accent. Zero and a space draw nothing.
	Rune  rune
	Marks string
	// FG and BG are the character's colour and the cell's. A clear FG
	// takes the grid's Foreground, and a clear BG shows the grid's
	// Background.
	FG, BG color.NRGBA
	Style  CellStyle
	// Wide marks a character two cells wide, such as a Chinese one. It
	// covers the cell after it, whose content is left undrawn.
	Wide bool
}

// CellStyle is a set of styles for a cell's character.
type CellStyle uint8

// The styles a cell can have.
const (
	CellBold CellStyle = 1 << iota
	CellItalic
	CellUnderline
	CellStrike
)

// CursorShape is how a [CellGrid] draws its cursor.
type CursorShape uint8

// The cursor's shapes.
const (
	// CursorBlock fills the cell, and shows the character in it in the
	// cell's background colour.
	CursorBlock CursorShape = iota
	// CursorBar is a thin line at the cell's left edge.
	CursorBar
	// CursorUnderline is a thin line along the cell's bottom.
	CursorUnderline
	// CursorOutline is the cell's outline, as terminals draw the cursor
	// while their window is in the background.
	CursorOutline
)

// Cursor is where a [CellGrid] shows its cursor, and how.
type Cursor struct {
	Col, Row int
	Shape    CursorShape
	Visible  bool
	// Blinked marks the off half of a blink: the cursor fades out, and
	// back in once Blinked clears, where Visible hides it at once.
	Blinked bool
	// Color is the cursor's colour; clear takes the colour of the
	// character under it.
	Color color.NRGBA
}

// cellRow is one row of cells, and what it drew.
type cellRow struct {
	cells []Cell
	drawn rowPaint
	stale bool
}

// rowPaint is a row's drawing, in the row's own space: backgrounds,
// glyphs grouped by colour, and lines under and through characters.
type rowPaint struct {
	fills []cellFill
	runs  []glyphRun
}

type cellFill struct {
	r geom.Rect
	c color.NRGBA
}

type glyphRun struct {
	c      color.NRGBA
	glyphs []paint.Glyph
	// lo and hi are the run's left and right edges.
	lo, hi float32
}

type cellMetrics struct {
	size, scale    float32
	faces          [4]*text.Face
	w, h, ascent   float32
	line, strikeAt float32
}

type glyphKey struct {
	r     rune
	style CellStyle
}

type cellGlyph struct {
	g       paint.Glyph
	advance float32
	ok      bool
}

// NewCellGrid returns an empty grid.
func NewCellGrid() *CellGrid {
	g := &CellGrid{Background: Background, Foreground: Ink, at: anim.NewPoint(geom.Point{})}
	g.lit = anim.NewFloat(1)
	g.Add(g.at, g.lit)
	return g
}

// Resize sets how many columns and rows the grid holds, keeping the
// cells that still fit.
func (g *CellGrid) Resize(cols, rows int) {
	if cols == g.cols && rows == g.rows {
		return
	}
	lines := make([]cellRow, rows)
	for y := range lines {
		lines[y].cells = make([]Cell, cols)
		lines[y].stale = true
		if y < len(g.lines) {
			copy(lines[y].cells, g.lines[y].cells)
		}
	}
	g.cols, g.rows, g.lines = cols, rows, lines
}

// GridSize returns how many columns and rows the grid holds.
func (g *CellGrid) GridSize() (cols, rows int) { return g.cols, g.rows }

// SetRow sets row y's cells from cells, as far as the row reaches, and
// clears the rest of it. A row whose cells stay the same keeps what it
// drew.
func (g *CellGrid) SetRow(y int, cells []Cell) {
	if y < 0 || y >= g.rows {
		return
	}
	line := &g.lines[y]
	n := min(len(cells), g.cols)
	if slices.Equal(line.cells[:n], cells[:n]) && isBlank(line.cells[n:]) {
		return
	}
	copy(line.cells, cells[:n])
	clear(line.cells[n:])
	line.stale = true
}

func isBlank(cells []Cell) bool {
	for _, c := range cells {
		if c != (Cell{}) {
			return false
		}
	}
	return true
}

// SetCursor places the cursor.
func (g *CellGrid) SetCursor(c Cursor) { g.cursor = c }

// Fit returns how many columns and rows fit the space the grid was last
// laid out in.
func (g *CellGrid) Fit() (cols, rows int) { return g.fitCols, g.fitRows }

// CellSize returns the size of one cell, as last laid out.
func (g *CellGrid) CellSize() geom.Size { return geom.Sz(g.metrics.w, g.metrics.h) }

// CellAt returns the cell under p, in the grid's space, clamped to the
// grid.
func (g *CellGrid) CellAt(p geom.Point) (col, row int) {
	if g.metrics.w <= 0 || g.metrics.h <= 0 {
		return 0, 0
	}
	col = max(0, min(int(p.X/g.metrics.w), g.cols-1))
	row = max(0, min(int(p.Y/g.metrics.h), g.rows-1))
	return col, row
}

func (g *CellGrid) face(s CellStyle) *text.Face {
	i := 0
	if s&CellBold != 0 {
		i |= 1
	}
	if s&CellItalic != 0 {
		i |= 2
	}
	if g.Faces[i] != nil {
		return g.Faces[i]
	}
	if g.Faces[0] != nil {
		return g.Faces[0]
	}
	return text.GoMono(i&1 != 0, i&2 != 0)
}

// measure sets the cells' size for a font size and a display scale.
// Sizes snap to whole device pixels, so every cell's edge falls on a
// pixel and each glyph lands on the same place in its cell.
func (g *CellGrid) measure(size, scale float32) {
	if g.metrics.size == size && g.metrics.scale == scale && g.metrics.faces == g.Faces {
		return
	}
	snap := func(v float32) float32 { return float32(math.Round(float64(v*scale))) / scale }
	face := g.face(0)
	ascent, descent, gap := face.Metrics(size)
	_, advance, ok := face.Glyph('M', size)
	if !ok {
		advance = size * 0.6
	}
	m := cellMetrics{size: size, scale: scale, faces: g.Faces}
	m.w = max(snap(advance), 1/scale)
	m.ascent = snap(ascent + gap/2)
	m.h = max(snap(ascent+descent+gap), 1/scale)
	m.line = max(snap(size/14), 1/scale)
	m.strikeAt = snap(m.ascent - ascent*0.3)
	g.metrics = m
	g.glyphs = map[glyphKey]cellGlyph{}
	for y := range g.lines {
		g.lines[y].stale = true
	}
}

// Layout implements [gunim.Node]. The grid fills the space it is given,
// or, given none, the space its cells need.
func (g *CellGrid) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	size := g.Size
	if size <= 0 {
		size = TextSize.Get(f.Theme)
	}
	g.measure(size, max(f.Scale, 1))
	m := g.metrics
	own := c.Max
	if own.W <= 0 {
		own.W = float32(g.cols) * m.w
	}
	if own.H <= 0 {
		own.H = float32(g.rows) * m.h
	}
	own = c.Constrain(own)
	g.fitCols, g.fitRows = max(1, int(own.W/m.w)), max(1, int(own.H/m.h))

	// A blink fades the cursor out and back.
	lit := float32(1)
	if g.cursor.Blinked {
		lit = 0
	}
	// A tween, which never overshoots, so the fade never flickers back.
	g.lit.Animate(lit, anim.Tween{Duration: 150 * time.Millisecond})

	// The cursor glides along its row and jumps to another.
	to := geom.Pt(float32(g.cursor.Col)*m.w, float32(g.cursor.Row)*m.h)
	if to.Y == g.at.Target().Y && to != g.at.Target() {
		g.at.Animate(to, Caret.Get(f.Theme))
	} else if to.Y != g.at.Target().Y {
		g.at.Jump(to)
	}
	return own
}

// Paint implements [gunim.Node].
func (g *CellGrid) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	m := g.metrics
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(g.Background.Get(f.Theme)))
	ink := g.Foreground.Get(f.Theme)
	for y := range g.lines {
		top := float32(y) * m.h
		if top >= box.H {
			break
		}
		line := &g.lines[y]
		if line.stale {
			line.drawn = g.drawRow(line.cells, ink)
			line.stale = false
		}
		g.paintRow(p, line.drawn, top)
	}
	g.paintCursor(p, ink, g.Background.Get(f.Theme))
}

// drawRow works out what a row of cells draws.
func (g *CellGrid) drawRow(cells []Cell, ink color.NRGBA) rowPaint {
	m := g.metrics
	var out rowPaint
	put := func(x0, x1, y0, y1 float32, c color.NRGBA) {
		if n := len(out.fills); n > 0 {
			last := &out.fills[n-1]
			if last.c == c && last.r.Max.X == x0 && last.r.Min.Y == y0 && last.r.Max.Y == y1 {
				last.r.Max.X = x1
				return
			}
		}
		out.fills = append(out.fills, cellFill{r: geom.Rect{Min: geom.Pt(x0, y0), Max: geom.Pt(x1, y1)}, c: c})
	}
	var lines []cellFill
	for x := 0; x < len(cells); x++ {
		c := cells[x]
		span := 1
		if c.Wide {
			span = 2
		}
		x0 := float32(x) * m.w
		x1 := x0 + float32(span)*m.w
		if c.BG.A != 0 {
			put(x0, x1, 0, m.h, c.BG)
		}
		fg := c.FG
		if fg.A == 0 {
			fg = ink
		}
		if c.Rune != 0 && c.Rune != ' ' {
			g.place(&out, c.Rune, c.Style, fg, x0, x1)
			for _, mark := range c.Marks {
				g.place(&out, mark, c.Style, fg, x0, x1)
			}
		}
		if c.Style&CellUnderline != 0 {
			lines = append(lines, cellFill{r: geom.Rect{Min: geom.Pt(x0, m.ascent+m.line), Max: geom.Pt(x1, m.ascent+2*m.line)}, c: fg})
		}
		if c.Style&CellStrike != 0 {
			lines = append(lines, cellFill{r: geom.Rect{Min: geom.Pt(x0, m.strikeAt), Max: geom.Pt(x1, m.strikeAt+m.line)}, c: fg})
		}
		x += span - 1
	}
	for _, l := range lines {
		put(l.r.Min.X, l.r.Max.X, l.r.Min.Y, l.r.Max.Y, l.c)
	}
	return out
}

// place adds r's glyph to the run of its colour, centred in the cells
// from x0 to x1 when it is narrower, as a fallback font's may be.
func (g *CellGrid) place(out *rowPaint, r rune, style CellStyle, c color.NRGBA, x0, x1 float32) {
	key := glyphKey{r, style & (CellBold | CellItalic)}
	cg, seen := g.glyphs[key]
	if !seen {
		cg.g, cg.advance, cg.ok = g.face(key.style).Glyph(r, g.metrics.size)
		g.glyphs[key] = cg
	}
	if !cg.ok {
		return
	}
	at := x0
	if cg.advance > 0 && cg.advance < x1-x0 {
		at += (x1 - x0 - cg.advance) / 2
	}
	glyph := cg.g
	glyph.At = geom.Pt(at, 0)
	i := slices.IndexFunc(out.runs, func(run glyphRun) bool { return run.c == c })
	if i < 0 {
		out.runs = append(out.runs, glyphRun{c: c, lo: x0, hi: x1})
		i = len(out.runs) - 1
	}
	run := &out.runs[i]
	run.glyphs = append(run.glyphs, glyph)
	run.lo, run.hi = min(run.lo, x0), max(run.hi, x1)
}

// paintRow draws a row's drawing with its top at top.
func (g *CellGrid) paintRow(p *paint.Painter, d rowPaint, top float32) {
	m := g.metrics
	defer p.Push(paint.Translate(geom.Pt(0, top)))()
	for _, fill := range d.fills {
		p.RRect(fill.r, 0, paint.Solid(fill.c))
	}
	if len(d.runs) == 0 {
		return
	}
	defer p.Push(paint.Translate(geom.Pt(0, m.ascent)))()
	for _, run := range d.runs {
		p.Text(run.glyphs, m.size, run.c, geom.Rect{
			Min: geom.Pt(run.lo, -m.ascent),
			Max: geom.Pt(run.hi, m.h-m.ascent),
		})
	}
}

// paintCursor draws the cursor where it has glided to.
func (g *CellGrid) paintCursor(p *paint.Painter, ink, background color.NRGBA) {
	cur := g.cursor
	if !cur.Visible || cur.Row < 0 || cur.Row >= g.rows || cur.Col < 0 || cur.Col >= g.cols {
		return
	}
	lit := min(max(g.lit.Value(), 0), 1)
	if lit < 0.01 {
		return
	}
	m := g.metrics
	under := g.lines[cur.Row].cells[cur.Col]
	col := cur.Color
	if col.A == 0 {
		col = under.FG
		if col.A == 0 {
			col = ink
		}
	}
	w := m.w
	if under.Wide {
		w *= 2
	}
	at := g.at.Value()
	box := geom.Rect{Min: at, Max: geom.Pt(at.X+w, at.Y+m.h)}
	if lit < 1 {
		defer p.Layer(paint.LayerOpts{Bounds: box, Opacity: lit})()
	}
	switch cur.Shape {
	case CursorBar:
		box.Max.X = box.Min.X + 2*m.line
		p.RRect(box, 0, paint.Solid(col))
	case CursorUnderline:
		box.Min.Y = box.Max.Y - 2*m.line
		p.RRect(box, 0, paint.Solid(col))
	case CursorOutline:
		// The outline runs inside the cell, half a line in from its edge.
		half := m.line / 2
		box = geom.Rect{Min: geom.Pt(box.Min.X+half, box.Min.Y+half), Max: geom.Pt(box.Max.X-half, box.Max.Y-half)}
		p.RRectStroke(box, 0, paint.Fill{}, paint.Stroke{Width: m.line, Color: col})
	case CursorBlock:
		p.RRect(box, 0, paint.Solid(col))
		// The character under a block shows in the cell's background.
		if under.Rune == 0 || under.Rune == ' ' {
			return
		}
		bg := under.BG
		if bg.A == 0 {
			bg = background
		}
		var d rowPaint
		g.place(&d, under.Rune, under.Style, bg, 0, w)
		defer p.Push(paint.Translate(geom.Pt(at.X, at.Y+m.ascent)))()
		for _, run := range d.runs {
			p.Text(run.glyphs, m.size, run.c, geom.Rect{Min: geom.Pt(0, -m.ascent), Max: geom.Pt(w, m.h-m.ascent)})
		}
	}
}
