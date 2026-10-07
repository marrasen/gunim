package widget

import (
	"fmt"
	"math"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
)

// Access implements [gunim.Accessible]: a table of the rows in view, each
// a row of cells, under a row of the columns' titles. The row the
// keyboard is on is the active one. Rows and cells are keyed by the row
// and column they show, so a screen reader holding one keeps it as the
// grid scrolls.
func (g *DataGrid) Access() access.Info {
	info := access.Info{Role: access.RoleTable}
	g.partRow, g.partCol = g.partRow[:0], g.partCol[:0]
	part := func(row, col int) int {
		g.partRow, g.partCol = append(g.partRow, row), append(g.partCol, col)
		return len(g.partRow)
	}
	bodyW := g.view.W
	if !g.NoHeader {
		head := access.Info{Role: access.RoleRow, Bounds: geom.Rc(0, 0, bodyW, g.header)}
		part(-1, -1)
		var titles []string
		for c, col := range g.Columns {
			if c >= len(g.xs) {
				break
			}
			h := access.Info{Role: access.RoleColumnHeader, Name: col.Title,
				Bounds: geom.Rc(g.xs[c][0]-g.left, 0, g.xs[c][1], g.header)}
			if g.OnHeader != nil {
				h.Actions = []string{access.ActionPress}
			}
			part(-1, c)
			head.Parts = append(head.Parts, h)
			titles = append(titles, col.Title)
		}
		head.Name = strings.Join(titles, ", ")
		info.Parts = append(info.Parts, head)
	}
	if g.Row == nil || g.rowH <= 0 {
		return info
	}
	first := max(0, int(math.Floor(g.top)))
	last := min(g.rows, int(math.Ceil(g.top+g.Visible())))
	for i := first; i < last; i++ {
		r := access.Info{Role: access.RoleRow, Key: cellKey(i, -1), Bounds: geom.Rc(0, g.rowY(i), bodyW, g.rowH),
			Description: fmt.Sprintf("row %d of %d", i+1, g.rows), Actions: []string{access.ActionPress}}
		if g.IsSelected(i) {
			r.State = access.StateSelected
		}
		if k := part(i, -1); i == g.selected {
			info.Active = k
		}
		row, ok := g.Row(i)
		if !ok {
			r.Name = "not read yet"
			info.Parts = append(info.Parts, r)
			continue
		}
		var names []string
		for c, spans := range row.Cells {
			if c >= len(g.xs) {
				break
			}
			var b strings.Builder
			for _, s := range spans {
				b.WriteString(s.Text)
			}
			text := strings.TrimSpace(b.String())
			if text != "" {
				names = append(names, text)
			}
			part(i, c)
			r.Parts = append(r.Parts, access.Info{Role: access.RoleCell, Name: text, Key: cellKey(i, c),
				Bounds: geom.Rc(g.xs[c][0]-g.left, g.rowY(i), g.xs[c][1], g.rowH)})
		}
		r.Name = strings.Join(names, ", ")
		info.Parts = append(info.Parts, r)
	}
	return info
}

// AccessAct implements [gunim.AccessActor]: pressing a row selects it,
// and pressing a column's title does what a click on it does.
func (g *DataGrid) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress || r.Part < 0 || r.Part >= len(g.partRow) {
		return false
	}
	row, col := g.partRow[r.Part], g.partCol[r.Part]
	switch {
	case row < 0 && col >= 0 && g.OnHeader != nil:
		g.send(g.OnHeader(col, u), u)
	case row < 0 || row >= g.rows:
		return false
	case g.Multi:
		g.pick(row, 0, u)
	default:
		g.selectAndTell(row, u)
	}
	return true
}

// cellKey is the access Key of row i's cell in column c, or of the row
// itself with c -1.
func cellKey(i, c int) uint64 { return uint64(i+1)<<16 | uint64(c+1) }
