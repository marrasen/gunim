package widget

import (
	"fmt"
	"hash/fnv"
	"math"
	"strconv"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
)

// Access implements [gunim.Accessible]: a table of the rows in view, each a row of cells, under a row of the columns'
// titles, as a [DataGrid] reads. The cursor's row is the active one, and the rows marked read as selected. Rows and
// cells are keyed by the row they show, so a screen reader holding one keeps it as the table scrolls.
func (t *Table) Access() access.Info {
	info := access.Info{Role: access.RoleTable, Description: fmt.Sprintf("%d rows", len(t.keys))}
	t.parts = t.parts[:0]
	part := func(row, col int) int {
		t.parts = append(t.parts, [2]int{row, col})
		return len(t.parts)
	}
	head := access.Info{Role: access.RoleRow, Bounds: geom.Rc(0, 0, t.width, t.headH)}
	part(-1, -1)
	var titles []string
	for c, col := range t.Columns {
		if c >= len(t.xs) {
			break
		}
		h := access.Info{Role: access.RoleColumnHeader, Name: col.Title,
			Bounds: geom.Rc(t.xs[c][0]-t.left, 0, t.xs[c][1], t.headH)}
		if t.OnSort != nil {
			h.Actions = []string{access.ActionPress}
		}
		part(-1, c)
		head.Parts = append(head.Parts, h)
		titles = append(titles, col.Title)
	}
	head.Name = strings.Join(titles, ", ")
	info.Parts = append(info.Parts, head)
	if t.Row == nil || t.rowH <= 0 {
		return info
	}
	top := t.list.Offset()
	first := max(0, int(math.Floor(float64(top/t.rowH))))
	last := min(len(t.keys), int(math.Ceil(float64((top+t.viewH)/t.rowH))))
	for i := first; i < last; i++ {
		k := t.keys[i]
		rect, _ := t.RowRect(k)
		key := keyPart(k)
		r := access.Info{Role: access.RoleRow, Key: key, Bounds: rect,
			Description: fmt.Sprintf("row %d of %d", i+1, len(t.keys)), Actions: []string{access.ActionPress}}
		if t.marked[k] {
			r.State = access.StateSelected
		}
		if n := part(i, -1); i == t.cursor {
			info.Active = n
		}
		row := t.Row(k)
		var names []string
		for c, s := range row.Cells {
			if c >= len(t.xs) {
				break
			}
			if s = strings.TrimSpace(s); s != "" {
				names = append(names, s)
			}
			part(i, c)
			r.Parts = append(r.Parts, access.Info{Role: access.RoleCell, Name: s, Key: key + uint64(c+1),
				Bounds: geom.Rc(t.xs[c][0]-t.left, rect.Min.Y, t.xs[c][1], rect.Size().H)})
		}
		r.Name = strings.Join(names, ", ")
		info.Parts = append(info.Parts, r)
	}
	return info
}

// AccessAct implements [gunim.AccessActor]: pressing a row puts the cursor on it, and pressing a column's title sorts
// by it, as a click does.
func (t *Table) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress || r.Part < 0 || r.Part >= len(t.parts) {
		return false
	}
	row, col := t.parts[r.Part][0], t.parts[r.Part][1]
	switch {
	case row < 0 && col >= 0:
		t.header.sort(col, u)
	case row < 0 || row >= len(t.keys):
		return false
	default:
		t.move(row, u)
	}
	return true
}

// keyPart is the access Key of a part showing the row of key k: a hash of k, with room below it for the row's cells.
func keyPart(k Key) uint64 {
	h := fnv.New64a()
	h.Write([]byte(k))
	return h.Sum64()&^0xffff | 1<<16
}

// Access implements [gunim.Accessible]: a list of tiles, each an item of what its own node says.
func (g *TileGrid) Access() access.Info {
	return access.Info{Role: access.RoleList, Description: fmt.Sprintf("%d tiles", g.n)}
}

// Access implements [gunim.Accessible]: a tile is an item of the grid's list, selected or not.
func (c *tileCell) Access() access.Info {
	info := access.Info{Role: access.RoleListItem, Actions: []string{access.ActionPress}}
	if c.g != nil {
		info.Description = fmt.Sprintf("tile %d of %d", c.i+1, c.g.n)
	}
	if c.selected {
		info.State = access.StateSelected
	}
	return info
}

// AccessAct implements [gunim.AccessActor]: pressing a tile selects it, as a click does.
func (c *tileCell) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress || c.g == nil || c.i < 0 || c.i >= c.g.n {
		return false
	}
	c.g.pick(c.i, 0, u)
	return true
}

// Access implements [gunim.Accessible]: a group of the curve's points, each a slider of its height, the chosen one
// active.
func (c *ToneCurve) Access() access.Info {
	info := access.Info{Role: access.RoleGroup, Name: "Tone curve", Active: c.chosen + 1}
	for _, q := range c.pts {
		at := c.toBox(q)
		d := CurvePoint.Default()
		info.Parts = append(info.Parts, access.Info{
			Role:   access.RoleSlider,
			Name:   "Point at " + strconv.Itoa(int(q.X*100+0.5)) + "%",
			Value:  strconv.Itoa(int(q.Y*100+0.5)) + "%",
			Range:  &access.Range{Max: 1, Value: float64(q.Y), Step: keyStep},
			Bounds: geom.Rc(at.X-d, at.Y-d, 2*d, 2*d),
		})
	}
	if c.chosen >= len(c.pts) {
		info.Active = 0
	}
	return info
}

// AccessAct implements [gunim.AccessActor]: a new value for a point sets its height, and commits.
func (c *ToneCurve) AccessAct(r access.Request, u *gunim.UI) bool {
	if !r.SetValue || r.Part < 0 || r.Part >= len(c.pts) || c.held >= 0 {
		return false
	}
	c.chosen = r.Part
	q := c.pts[r.Part]
	c.held = r.Part
	c.moveHeld(geom.Pt(q.X, clamp01(float32(r.Value))))
	c.held = -1
	if c.OnChange != nil {
		send(u, c, c.OnChange(c.Points(), u))
	}
	c.commitNow(u)
	u.Invalidate()
	return true
}
