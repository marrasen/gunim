package widget

import (
	"strings"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// findAll returns every node under n of role.
func findAll(n *access.Node, role access.Role) []*access.Node {
	var out []*access.Node
	if n.Role == role {
		out = append(out, n)
	}
	for _, k := range n.Children {
		out = append(out, findAll(k, role)...)
	}
	return out
}

// A table reads as a table: a row of titles, and the rows in view with their cells. The cursor's row is active, the
// rows marked are selected, and pressing a row puts the cursor on it.
func TestATableReadsAsATable(t *testing.T) {
	w, tbl, ev, run := newTableStage(t, 50)
	tbl.SetMarked([]Key{"3"})
	w.Offscreen().ListenForAccess()
	run(2)
	tree := w.Offscreen().AccessTree()
	table := find(tree.Root, access.RoleTable, "")
	if table == nil {
		t.Fatal("no table in the tree")
	}
	var b strings.Builder
	dump(table, 0, &b)
	got := b.String()
	for _, want := range []string{"row Name, Size", "column header Name", "cell row 0", "cell 0 KB"} {
		if !strings.Contains(got, want) {
			t.Fatalf("the table lacks %q:\n%s", want, got)
		}
	}
	if find(table, access.RoleRow, "row 0, 0 KB") == nil {
		t.Fatalf("the table lacks its first row:\n%s", got)
	}
	if r := find(table, access.RoleRow, "row 3, 3 KB"); r == nil || !r.State.Has(access.StateSelected) ||
		r.Description != "row 4 of 50" {
		t.Fatalf("the marked row says %+v", r)
	}
	cell := find(table, access.RoleCell, "row 5")
	w.Input(access.Request{ID: cell.ID, Action: access.ActionPress})
	size := find(table, access.RoleColumnHeader, "Size")
	w.Input(access.Request{ID: size.ID, Action: access.ActionPress})
	run(2)
	if k, _ := tbl.Cursor(); k != "5" {
		t.Fatalf("pressing a cell of row 5 put the cursor on row %q", k)
	}
	if len(ev.sorts) != 1 || ev.sorts[0] != [2]int{1, 0} {
		t.Fatalf("pressing the title Size asked for %v, want it sorted by column 1", ev.sorts)
	}
}

// A tile grid reads as a list of its tiles, the selected ones selected, and pressing a tile selects it.
func TestATileGridReadsAsAListOfTiles(t *testing.T) {
	g, w, run := tileStage(t, 10)
	g.runs, g.cursor = [][2]int{{2, 3}}, 2
	w.Offscreen().ListenForAccess()
	run(2)
	tree := w.Offscreen().AccessTree()
	list := find(tree.Root, access.RoleList, "")
	if list == nil || list.Description != "10 tiles" {
		t.Fatalf("the grid reads as %+v, want a list of 10 tiles", list)
	}
	items := findAll(list, access.RoleListItem)
	if len(items) != 10 {
		t.Fatalf("the grid shows %d items, want its 10 tiles", len(items))
	}
	for i, it := range items {
		if sel := it.State.Has(access.StateSelected); sel != (i == 2) {
			t.Fatalf("tile %d (%s) reads selected %v", i, it.Description, sel)
		}
	}
	w.Input(access.Request{ID: items[5].ID, Action: access.ActionPress})
	run(2)
	if sel, cursor := g.Selected(); cursor != 5 || len(sel) != 1 || sel[0] != [2]int{5, 6} {
		t.Fatalf("pressing tile 5 left %v selected and the cursor on %d", sel, cursor)
	}
}

// A tone curve reads as a group of points, each a slider of its height; a screen reader can set one.
func TestAToneCurveReadsAsItsPoints(t *testing.T) {
	c := NewToneCurve()
	c.SetPoints([]geom.Point{{X: 0, Y: 0}, {X: 0.5, Y: 0.4}, {X: 1, Y: 1}}, nil)
	var committed []geom.Point
	c.OnCommit = func(pts []geom.Point, _ *gunim.UI) gunim.Intent { committed = pts; return nil }
	w, run := stage(t, &frame{child: c, size: geom.Sz(300, 300)})
	w.Offscreen().ListenForAccess()
	run(2)
	tree := w.Offscreen().AccessTree()
	group := find(tree.Root, access.RoleGroup, "Tone curve")
	if group == nil {
		t.Fatal("no tone curve in the tree")
	}
	points := findAll(group, access.RoleSlider)
	if len(points) != 3 || points[1].Name != "Point at 50%" || points[1].Value != "40%" {
		t.Fatalf("the curve reads as %d points, the middle %+v", len(points), points[1])
	}
	w.Input(access.Request{ID: points[1].ID, SetValue: true, Value: 0.7})
	run(2)
	if p := c.Points()[1]; p.Y != 0.7 || len(committed) != 3 {
		t.Fatalf("set to 0.7 by a screen reader, the middle point is %v, and %v committed", p, committed)
	}
}

// A text widget hands a screen reader its text without copying it every frame: only an edit makes it again.
func TestATextWidgetSaysItsTextWithoutCopyingIt(t *testing.T) {
	tf, ta, ce := NewTextField(), NewTextArea(), NewCodeEditor()
	long := strings.Repeat("A line of text. ", 200)
	tf.SetText(long, nil)
	ta.SetText(long, nil)
	ce.SetText(long, nil)
	for name, n := range map[string]gunim.Accessible{"text field": tf, "text area": ta, "code editor": ce} {
		if v := n.Access().Value; v != long {
			t.Fatalf("the %s says %d bytes, want its %d", name, len(v), len(long))
		}
		if a := testing.AllocsPerRun(10, func() { n.Access() }); a > 0 {
			t.Fatalf("the %s allocates %v times to say its unchanged text, want its text kept", name, a)
		}
	}
	w, run := stage(t, &frame{child: tf, size: geom.Sz(300, 40)})
	click(w, 10, 10)
	w.Input(input.TextInput{Text: "!"})
	run(1)
	if v := tf.Access().Value; !strings.Contains(v, "!") || len(v) != len(long)+1 {
		t.Fatalf("after typing !, the field says %d bytes, with ! in it %v", len(v), strings.Contains(v, "!"))
	}
}
