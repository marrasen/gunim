package widget

import (
	"image/color"
	"math"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
)

// Table tokens.
var (
	TableRowHeight = theme.Length("table.row", 26)
	TableHeader    = theme.Color("table.header", color.NRGBA{R: 0x8a, G: 0x93, B: 0xa6, A: 0xff})
	// TableCursor lights the row the keys are on, and TableMark the rows
	// marked.
	TableCursor = theme.Color("table.cursor", color.NRGBA{R: 0x5e, G: 0x9c, B: 0xff, A: 0x40})
	TableMark   = theme.Color("table.mark", color.NRGBA{R: 0xe8, G: 0xb3, B: 0x4a, A: 0x30})
	// TableStrong colours a strong row's text, such as a folder's.
	TableStrong = theme.Color("table.strong", color.NRGBA{R: 0x8c, G: 0xb8, B: 0xff, A: 0xff})
	tableNoGap  = theme.Length("table.gap", 0)
)

// TableColumn is one column of a [Table].
type TableColumn struct {
	Title string
	// Width is the column's width in logical pixels. Zero shares out
	// what the other columns leave.
	Width float32
	// End lines the column's text up at its right edge, as numbers are.
	End bool
	// Face is the face the column's cells are set in, and the theme's [Font] when unset.
	Face theme.Token[*text.Face]
}

// TableRow is what a [Table] shows for one row: a line of text for each
// column. A strong row is set apart, as a folder is among files; a
// faint row is dimmed, as a hidden file is.
type TableRow struct {
	Cells  []string
	Strong bool
	Faint  bool
	// Accent draws the row in the theme's accent colour, as for a link.
	Accent bool
}

// Table shows rows of cells under a header of column titles, and
// scrolls through them, building only the rows in view, so a table of
// a hundred thousand rows costs what the rows on screen do.
//
// The rows ask Row for their cells as they draw, so the data can change
// under the same keys. A cursor row, lit, follows the arrow keys, Page
// Up and Page Down, Home and End, and the pointer; Enter or a double
// click activates it. Space marks it and moves on, and the marked rows
// are tinted. A click on a column's title asks for the rows sorted by
// it, and again for the other way round.
type Table struct {
	Columns []TableColumn
	Row     func(Key) TableRow
	// OnActivate runs with the row Enter or a double click activates.
	OnActivate func(key Key, u *gunim.UI)
	// OnSort runs with the column clicked and the direction asked for.
	OnSort func(column int, descending bool, u *gunim.UI)

	list   *VirtualList
	header *tableHeader
	keys   []Key
	index  map[Key]int
	cursor int
	marked map[Key]bool
	// sorted is the column sorted by, or -1, and descending its
	// direction, as the header shows them.
	sorted     int
	descending bool
	// xs holds each column's left edge and width, from the last layout.
	xs      [][2]float32
	focused bool
	// typed is what has been typed to find a row, and typedAt when the
	// last of it was.
	typed   string
	typedAt time.Time
}

// NewTable returns an empty table of columns.
func NewTable(columns ...TableColumn) *Table {
	t := &Table{Columns: columns, index: map[Key]int{}, marked: map[Key]bool{}, sorted: -1}
	t.list = NewVirtualList(func(k Key) gunim.Node { return newTableRow(t, k) })
	t.list.Spacing = tableNoGap
	t.header = &tableHeader{t: t}
	return t
}

// SetKeys makes keys the table's rows, in order. The cursor stays on
// its row while the row stays, and marks on theirs.
func (t *Table) SetKeys(keys []Key, u *gunim.UI) {
	var at Key
	if t.cursor >= 0 && t.cursor < len(t.keys) {
		at = t.keys[t.cursor]
	}
	t.keys = keys
	clear(t.index)
	for i, k := range keys {
		t.index[k] = i
	}
	for k := range t.marked {
		if _, ok := t.index[k]; !ok {
			delete(t.marked, k)
		}
	}
	if i, ok := t.index[at]; ok {
		t.cursor = i
	} else {
		t.cursor = min(t.cursor, len(keys)-1)
	}
	t.cursor = max(t.cursor, 0)
	t.list.SetKeys(keys, u)
}

// SetSorted shows the header's arrow on column, pointing the way the
// rows run; -1 shows none.
func (t *Table) SetSorted(column int, descending bool) {
	t.sorted, t.descending = column, descending
}

// Cursor returns the cursor's row, and false for an empty table.
func (t *Table) Cursor() (Key, bool) {
	if t.cursor < 0 || t.cursor >= len(t.keys) {
		return "", false
	}
	return t.keys[t.cursor], true
}

// SetCursor puts the cursor on key's row, scrolling it into view.
func (t *Table) SetCursor(key Key, u *gunim.UI) {
	if i, ok := t.index[key]; ok {
		t.move(i, u)
	}
}

// JumpTo puts the cursor on key and the view on it at once, at the top
// when it is in the first screenful. It is for a table showing rows that
// are new rather than moved, as a file list does for a folder just
// opened: its rows appear in place instead of gliding in from wherever
// the rows before were scrolled to.
func (t *Table) JumpTo(key Key, u *gunim.UI) {
	i, ok := t.index[key]
	if !ok {
		i = 0
	}
	t.cursor = i
	if len(t.keys) == 0 {
		t.cursor = 0
	}
	h := TableRowHeight.Get(u.Theme())
	y := float32(t.cursor) * h
	page := t.list.viewport
	if y+h <= page || page <= 0 {
		y = 0
	} else {
		y -= (page - h) / 2
	}
	t.list.jumpTo(y)
	u.Invalidate()
}

// Offset is how far the table is scrolled, for putting it back later
// with ShowAt.
func (t *Table) Offset() float32 { return t.list.Offset() }

// ShowAt is JumpTo with the view put back where it was, offset down,
// as a file list does going back to a folder: the rows are where the
// user left them. The cursor's row is kept in view.
func (t *Table) ShowAt(key Key, offset float32, u *gunim.UI) {
	t.JumpTo(key, u)
	h := TableRowHeight.Get(u.Theme())
	y := float32(t.cursor) * h
	page := t.list.viewport
	if page > 0 {
		offset = min(max(offset, y+h-page), y)
	}
	t.list.jumpTo(max(0, offset))
	// Put back as it was, the rows come at once, rather than growing
	// in as a folder new to the list does.
	t.list.still = true
}

// Marked returns the marked rows, in the table's order.
func (t *Table) Marked() []Key {
	var out []Key
	for _, k := range t.keys {
		if t.marked[k] {
			out = append(out, k)
		}
	}
	return out
}

// ClearMarks unmarks every row.
func (t *Table) ClearMarks() { clear(t.marked) }

func (t *Table) move(i int, u *gunim.UI) {
	if len(t.keys) == 0 {
		return
	}
	t.cursor = min(max(i, 0), len(t.keys)-1)
	h := TableRowHeight.Get(u.Theme())
	t.list.revealContent(geom.Rc(0, float32(t.cursor)*h, 1, h), u)
	u.Invalidate()
}

// Children implements [gunim.Composite].
func (t *Table) Children() []gunim.Node { return []gunim.Node{t.header, t.list} }

// Focusable implements [gunim.Focusable].
func (t *Table) Focusable() bool { return true }

// Handle implements [gunim.Handler]: the keys that move the cursor,
// mark and activate. Other keys go on, to the application's shortcuts.
func (t *Table) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.FocusGained:
		t.focused = true
		u.Invalidate()
		return true
	case input.FocusLost:
		t.focused = false
		u.Invalidate()
		return true
	case input.TextInput:
		t.find(e, u)
		return true
	case input.KeyPress:
		if e.Mods.Has(input.ModControl) || e.Mods.Has(input.ModAlt) {
			return false
		}
		// A typed press leaves it to the text, which finds a row; Space
		// while finding goes into the name.
		if e.Typed && (e.Key != input.KeySpace || t.finding(e.Time)) {
			return true
		}
		page := max(1, int(t.pageRows(u)))
		switch e.Key {
		case input.KeyUp:
			t.move(t.cursor-1, u)
		case input.KeyDown:
			t.move(t.cursor+1, u)
		case input.KeyPageUp:
			t.move(t.cursor-page, u)
		case input.KeyPageDown:
			t.move(t.cursor+page, u)
		case input.KeyHome:
			t.move(0, u)
		case input.KeyEnd:
			t.move(len(t.keys)-1, u)
		case input.KeyEnter, input.KeyKPEnter:
			if k, ok := t.Cursor(); ok && t.OnActivate != nil {
				t.OnActivate(k, u)
			}
		case input.KeyBackspace:
			// Takes back a letter of a name being found, and is left to
			// the application otherwise.
			if !t.finding(when(e.Time)) {
				return false
			}
			typed := []rune(t.typed)
			t.typed = string(typed[:len(typed)-1])
			t.typedAt = when(e.Time)
			t.findTyped(u)
		case input.KeyEscape:
			// Gives up finding, and is left to the application when
			// nothing is being found.
			if !t.finding(when(e.Time)) {
				return false
			}
			t.typed = ""
		case input.KeySpace, input.KeyInsert:
			if k, ok := t.Cursor(); ok {
				t.marked[k] = !t.marked[k]
				if !t.marked[k] {
					delete(t.marked, k)
				}
				t.move(t.cursor+1, u)
			}
		default:
			return false
		}
		return true
	}
	return false
}

// when is an event's time, or now for one that carries none.
func when(at time.Time) time.Time {
	if at.IsZero() {
		return time.Now()
	}
	return at
}

// findPause is how long typing rests before a new name starts.
const findPause = time.Second

func (t *Table) finding(now time.Time) bool {
	return t.typed != "" && now.Sub(t.typedAt) < findPause
}

// find moves the cursor to the first row whose first cell starts with
// what has been typed, whatever its case. Typing after a pause starts
// a new name.
func (t *Table) find(e input.TextInput, u *gunim.UI) {
	now := e.Time
	if now.IsZero() {
		now = time.Now()
	}
	if !t.finding(now) {
		t.typed = ""
		// A name never starts with a space: that one marked a row.
		if strings.TrimSpace(e.Text) == "" {
			return
		}
	}
	t.typed += strings.ToLower(e.Text)
	t.typedAt = now
	t.findTyped(u)
}

// findTyped moves the cursor to the first row whose first cell starts
// with what has been typed.
func (t *Table) findTyped(u *gunim.UI) {
	if t.Row == nil || t.typed == "" {
		return
	}
	for i, k := range t.keys {
		row := t.Row(k)
		if len(row.Cells) > 0 && strings.HasPrefix(strings.ToLower(row.Cells[0]), t.typed) {
			t.move(i, u)
			return
		}
	}
}

// pageRows is how many rows the table shows at once.
func (t *Table) pageRows(u *gunim.UI) float32 {
	h := TableRowHeight.Get(u.Theme())
	if b, ok := u.Bounds(t.list); ok && h > 0 {
		return float32(math.Floor(float64(b.Size().H / h)))
	}
	return 10
}

// Layout implements [gunim.Node]. The table fills the space it is
// given: the header along the top, the rows under it.
func (t *Table) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	own := c.Max
	// Every row is as tall, so a row not laid out yet is where it will
	// be: a view put back far down a table stays on the rows it showed.
	t.list.Estimate = TableRowHeight.Get(f.Theme)
	pad := MenuRowPadding.Get(f.Theme)
	// Fixed columns take their widths; the rest share what is left.
	fixed, shared := float32(0), 0
	for _, col := range t.Columns {
		if col.Width > 0 {
			fixed += col.Width
		} else {
			shared++
		}
	}
	rest := max(0, own.W-fixed-2*pad)
	t.xs = t.xs[:0]
	x := pad
	for _, col := range t.Columns {
		w := col.Width
		if w <= 0 {
			w = rest / float32(max(shared, 1))
		}
		t.xs = append(t.xs, [2]float32{x, w})
		x += w
	}
	hh := TableRowHeight.Get(f.Theme)
	kids.At(0).Layout(gunim.Tight(geom.Sz(own.W, hh)))
	kids.At(0).Place(geom.Point{})
	kids.At(1).Layout(gunim.Tight(geom.Sz(own.W, max(0, own.H-hh))))
	kids.At(1).Place(geom.Pt(0, hh))
	return own
}

// Paint implements [gunim.Node].
func (t *Table) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for k := range kids.All {
		k.Paint(p)
	}
}

// cellText lays out s for a cell width wide, cut short with an ellipsis
// when it will not fit.
func cellText(cache *laidText, face *text.Face, s string, size, width float32) text.Paragraph {
	return cache.layout(face, s, text.Style{Size: size, MaxLines: 1}, max(width, 1))
}

// tableHeader is a table's row of column titles.
type tableHeader struct {
	t      *Table
	titles []laidText
}

// Layout implements [gunim.Node].
func (h *tableHeader) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Max
}

// Paint implements [gunim.Node].
func (h *tableHeader) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	t := h.t
	th := f.Theme
	if len(h.titles) != len(t.Columns) {
		h.titles = make([]laidText, len(t.Columns))
	}
	size := TextSize.Get(th) * 0.9
	col := TableHeader.Get(th)
	for i, c := range t.Columns {
		if i >= len(t.xs) {
			break
		}
		x, w := t.xs[i][0], t.xs[i][1]
		para := cellText(&h.titles[i], faceIn(Font, th), c.Title, size, w-12)
		at := x
		if c.End {
			at = x + w - 12 - para.Size.W
		}
		para.Paint(p, geom.Pt(at, (box.H-para.Size.H)/2), col)
		if i == t.sorted {
			// A small arrow beside the title, pointing the way the rows
			// run.
			ax := at + para.Size.W + 8
			if c.End {
				ax = at - 8
			}
			ay := box.H / 2
			turn := float32(0)
			if !t.descending {
				turn = math.Pi
			}
			func() {
				defer p.Push(paint.Rotate(turn, geom.Pt(ax, ay)))()
				defer p.Push(paint.Scale(0.7, geom.Pt(ax, ay)))()
				drawChevron(p, geom.Pt(ax, ay), col)
			}()
		}
	}
	line := MenuBorder.Get(th)
	p.RRect(geom.Rc(0, box.H-1, box.W, 1), 0, paint.Solid(line))
}

// Handle implements [gunim.Handler]: a click on a title sorts by it.
func (h *tableHeader) Handle(e input.Event, u *gunim.UI) bool {
	d, ok := e.(input.PointerDown)
	if !ok || d.Button != input.ButtonPrimary {
		return false
	}
	t := h.t
	for i, x := range t.xs {
		if d.Pos.X < x[0] || d.Pos.X >= x[0]+x[1] {
			continue
		}
		desc := false
		if i == t.sorted {
			desc = !t.descending
		}
		t.SetSorted(i, desc)
		if t.OnSort != nil {
			t.OnSort(i, desc, u)
		}
		u.Invalidate()
		return true
	}
	return false
}

// tableRow is one row of a table, drawing its cells from the table's
// Row.
type tableRow struct {
	anim.Group
	t     *Table
	key   Key
	lit   *anim.Float
	on    bool
	cells []laidText
}

func newTableRow(t *Table, k Key) *tableRow {
	r := &tableRow{t: t, key: k, lit: anim.NewFloat(0)}
	r.Add(r.lit)
	return r
}

// Layout implements [gunim.Node]. The cursor's light fades to it.
func (r *tableRow) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	i, ok := r.t.index[r.key]
	on := ok && i == r.t.cursor
	if on != r.on {
		r.on = on
		to := float32(0)
		if on {
			to = 1
		}
		r.lit.Animate(to, Quick.Get(f.Theme))
	}
	return c.Constrain(geom.Sz(c.Max.W, TableRowHeight.Get(f.Theme)))
}

// Paint implements [gunim.Node].
func (r *tableRow) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	t := r.t
	th := f.Theme
	band := geom.Rc(4, 1, box.W-8, box.H-2)
	if t.marked[r.key] {
		p.RRect(band, 5, paint.Solid(TableMark.Get(th)))
	}
	if v := min(max(r.lit.Value(), 0), 1); v > 0.01 {
		c := TableCursor.Get(th)
		if !t.focused {
			c.A /= 2
		}
		c.A = uint8(float32(c.A) * v)
		p.RRect(band, 5, paint.Solid(c))
	}
	if t.Row == nil {
		return
	}
	row := t.Row(r.key)
	if len(r.cells) != len(t.Columns) {
		r.cells = make([]laidText, len(t.Columns))
	}
	ink := Ink.Get(th)
	switch {
	case row.Accent:
		ink = Accent.Get(th)
	case row.Strong:
		ink = TableStrong.Get(th)
	case row.Faint:
		ink.A /= 2
	}
	size := TextSize.Get(th)
	for i, s := range row.Cells {
		if i >= len(t.xs) {
			break
		}
		x, w := t.xs[i][0], t.xs[i][1]
		para := cellText(&r.cells[i], faceIn(t.Columns[i].Face, th), s, size, w-12)
		at := x
		if t.Columns[i].End {
			at = x + w - 12 - para.Size.W
		}
		para.Paint(p, geom.Pt(at, (box.H-para.Size.H)/2), ink)
	}
}

// Handle implements [gunim.Handler]: a press puts the cursor on the
// row, and a double click activates it.
func (r *tableRow) Handle(e input.Event, u *gunim.UI) bool {
	d, ok := e.(input.PointerDown)
	if !ok || d.Button != input.ButtonPrimary {
		return false
	}
	t := r.t
	if i, ok := t.index[r.key]; ok {
		t.cursor = i
		u.Invalidate()
	}
	if d.Clicks == 2 && t.OnActivate != nil {
		t.OnActivate(r.key, u)
	}
	return true
}
