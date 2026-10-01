package filemanager

import (
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"
)

// Commands the context menus do in the window.
const (
	localCopyPath   = "local.copypath"
	localOpenWindow = "local.openwindow"
	localOpenPlace  = "local.openplace"
	localUnpin      = "local.unpin"
	localRenameFav  = "local.renamefav"
)

// rowItems are the context menu of the items selected.
var rowItems = []menuItem{
	{"Open", "Enter", CmdOpen},
	{"Open with system", "", CmdOpenSystem},
	{"Open in new window", "", localOpenWindow},
	{"Show in system file manager", "", CmdReveal},
	{"-", "", ""},
	{"Cut", "Ctrl+X", CmdCut},
	{"Copy", "Ctrl+C", CmdCopy},
	{"Paste", "Ctrl+V", CmdPaste},
	{"-", "", ""},
	{"Rename", "F2", CmdRename},
	{"Duplicate", "", CmdDuplicate},
	{"Move to trash", "Delete", CmdTrash},
	{"Delete permanently", "Shift+Delete", CmdDelete},
	{"-", "", ""},
	{"Pin to favourites", "Ctrl+D", CmdPin},
	{"Copy path", "Ctrl+Shift+C", localCopyPath},
	{"Properties", "Alt+Enter", CmdProperties},
}

// emptyItems are the context menu of the folder showing, for a press
// beside the items.
var emptyItems = []menuItem{
	{"New folder", "Ctrl+Shift+N", CmdNewFolder},
	{"Paste", "Ctrl+V", CmdPaste},
	{"Refresh", "F5", CmdRefresh},
	{"-", "", ""},
	{"Sort by name", "", CmdSortName},
	{"Sort by size", "", CmdSortSize},
	{"Sort by date modified", "", CmdSortTime},
	{"Sort by type", "", CmdSortType},
	{"-", "", ""},
	{"Show hidden files", "Ctrl+H", CmdHidden},
	{"Preview pane", "", CmdPreview},
	{"-", "", ""},
	{"Open in new window", "Ctrl+N", CmdNewWindow},
	{"Copy path", "Ctrl+Shift+C", localCopyPath},
	{"Properties", "Alt+Enter", CmdProperties},
}

// placeItems are the context menu of a place in the sidebar, and
// favItems of a favourite.
var (
	placeItems = []menuItem{
		{"Open", "", localOpenPlace},
		{"Open in new window", "", localOpenWindow},
	}
	favItems = []menuItem{
		{"Open", "", localOpenPlace},
		{"Open in new window", "", localOpenWindow},
		{"-", "", ""},
		{"Rename favourite", "", localRenameFav},
		{"Unpin", "", localUnpin},
	}
)

// menuState is what a context menu offers, worked out as it opens.
type menuState struct {
	cmds []string
	// path is the item or place the menu is about.
	path string
}

// fill sets c's items from items, dimming those off says are off and
// ticking those on says are on, and returns the commands in order.
func fill(c *widget.ContextMenu, items []menuItem, off, on func(cmd string) bool) []string {
	c.Items, c.Hints, c.Disabled, c.Checked, c.Breaks = nil, nil, nil, nil, nil
	var cmds []string
	for _, it := range items {
		if it.label == "-" {
			c.Breaks = append(c.Breaks, len(c.Items))
			continue
		}
		c.Items = append(c.Items, it.label)
		c.Hints = append(c.Hints, it.hint)
		c.Disabled = append(c.Disabled, off(it.cmd))
		c.Checked = append(c.Checked, on(it.cmd))
		cmds = append(cmds, it.cmd)
	}
	return cmds
}

// checked reports whether the menu bar ticks the item that sends cmd.
func (t *titleBar) checked(cmd string) bool {
	for m, cmds := range t.cmds {
		for i, c := range cmds {
			if c == cmd {
				return t.bar.Menus[m].Checked[i]
			}
		}
	}
	return false
}

// contextMenu makes the listing's context menu around the grid: for the
// items selected when pressed on one, and for the folder showing when
// pressed beside them.
func (pg *listingPage) contextMenu(g gunim.Node, rowAt func(geom.Point) int, selected func() [][2]int) *widget.ContextMenu {
	m := widget.NewContextMenu(g)
	var st menuState
	m.Prepare = func(at geom.Point, u *gunim.UI) bool {
		b := pg.b
		row := rowAt(at)
		clipEmpty := b.dnd.clip.Count == 0
		if row < 0 {
			b.listing.selectNone(u)
			st = menuState{path: b.listing.path}
			st.cmds = fill(m, emptyItems, func(cmd string) bool { return cmd == CmdPaste && clipEmpty },
				b.title.checked)
			return true
		}
		sel, dirs := pg.selectedRows(selected())
		one := len(sel) == 1
		st = menuState{}
		if one {
			st.path = b.shell.Paths.Join(b.listing.path, sel[0].Name)
		}
		st.cmds = fill(m, rowItems, func(cmd string) bool {
			switch cmd {
			case CmdPaste:
				return clipEmpty
			case CmdRename:
				return !one
			case localOpenWindow:
				return !one || !sel[0].Dir
			case CmdPin:
				return dirs == 0
			}
			return false
		}, func(string) bool { return false })
		return true
	}
	m.Picked = func(i int, u *gunim.UI) { pg.b.dnd.menuPicked(m, st, i, u) }
	return m
}

// items returns the node showing the items: the tiles or the grid.
func (pg *listingPage) items() gunim.Node {
	if pg.icons != nil && pg.icons.on {
		return pg.icons.grid
	}
	return pg.grid
}

// itemAt returns the item at p in the space of items, or -1.
func (pg *listingPage) itemAt(p geom.Point) int {
	if pg.icons != nil && pg.icons.on {
		return pg.icons.grid.TileAt(p)
	}
	return pg.grid.RowAt(p)
}

// itemRect returns where item i is in the space of items.
func (pg *listingPage) itemRect(i int) (geom.Rect, bool) {
	if pg.icons != nil && pg.icons.on {
		return pg.icons.grid.TileRect(i), i >= 0 && i < pg.icons.grid.Len()
	}
	return pg.grid.RowRect(i)
}

// selection returns the runs selected in the view showing: the tiles or the grid.
func (pg *listingPage) selection() [][2]int {
	if pg.icons != nil && pg.icons.on {
		sel, _ := pg.icons.grid.Selected()
		return sel
	}
	return pg.grid.SelectedRows()
}

// selectedRows returns the rows of the runs selected, and how many are folders.
func (pg *listingPage) selectedRows(runs [][2]int) (rows []Row, dirs int) {
	for _, r := range runs {
		for i := r[0]; i < r[1]; i++ {
			if row, ok := pg.view(i); ok {
				rows = append(rows, row)
				if row.Dir {
					dirs++
				}
			}
		}
	}
	return rows, dirs
}

// menuPicked does item i of a context menu m, which offered st.
func (v *dndView) menuPicked(m *widget.ContextMenu, st menuState, i int, u *gunim.UI) {
	if i < 0 || i >= len(st.cmds) {
		return
	}
	switch cmd := st.cmds[i]; cmd {
	case localCopyPath:
		v.copyPaths(u)
	case localOpenWindow:
		u.Send(m, OpenWindow{Path: st.path})
	case localOpenPlace:
		u.Send(m, Navigate{Path: st.path})
	case localUnpin:
		u.Send(m, Unpin{Path: st.path})
	case localRenameFav:
		u.Send(m, RenameFavourite{Path: st.path})
	default:
		u.Send(m, Command{Name: cmd})
	}
}

// copyPaths puts the paths of the items selected on the clipboard, a
// line each, or the folder's path when none is selected.
func (v *dndView) copyPaths(u *gunim.UI) {
	l := v.b.listing
	var paths []string
	if l.cur != nil {
		rows, _ := l.cur.selectedRows(l.cur.selection())
		for _, r := range rows {
			paths = append(paths, v.b.shell.Paths.Join(l.path, r.Name))
		}
	}
	if len(paths) == 0 {
		paths = []string{l.path}
	}
	u.SetClipboard(strings.Join(paths, "\n"))
}

// newSideMenu wraps the sidebar in a context menu for its places and
// favourites.
func newSideMenu(b *browser) *widget.ContextMenu {
	m := widget.NewContextMenu(b.side)
	var st menuState
	m.Prepare = func(at geom.Point, u *gunim.UI) bool {
		mr, ok := u.Bounds(m)
		if !ok {
			return false
		}
		p := at.Add(mr.Min)
		for _, l := range []*widget.List{b.side.places, b.side.favs} {
			for _, k := range l.Keys() {
				n, ok := l.Row(k)
				if pr, isPlace := n.(*placeRow); !ok || !isPlace || pr.item.heading || pr.item.away {
					continue
				}
				if r, ok := u.Bounds(n); ok && r.Contains(p) {
					st = menuState{path: string(k)}
					items := placeItems
					if l == b.side.favs {
						items = favItems
					}
					st.cmds = fill(m, items, func(string) bool { return false }, func(string) bool { return false })
					return true
				}
			}
		}
		return false
	}
	m.Picked = func(i int, u *gunim.UI) { b.dnd.menuPicked(m, st, i, u) }
	return m
}

// keys takes the keys of drag and drop, the context menus and the
// windows: the Menu key and Shift+F10 open the listing's menu at the row
// the keyboard is on, Ctrl+N opens a window, Alt+Enter the properties,
// and Ctrl+Shift+C copies the paths.
func (v *dndView) keys(e input.KeyPress, u *gunim.UI) bool {
	ctrl, shift, alt := e.Mods.Has(input.ModControl), e.Mods.Has(input.ModShift), e.Mods.Has(input.ModAlt)
	switch {
	case ctrl && !shift && e.Key == input.KeyN:
		u.Send(v.b, Command{Name: CmdNewWindow})
		return true
	case alt && (e.Key == input.KeyEnter || e.Key == input.KeyKPEnter):
		u.Send(v.b, Command{Name: CmdProperties})
		return true
	case ctrl && shift && e.Key == input.KeyC:
		v.copyPaths(u)
		return true
	case e.Key == input.KeyMenu, shift && e.Key == input.KeyF10:
	default:
		return false
	}
	l := v.b.listing
	if l.cur == nil {
		return false
	}
	at := geom.Pt(40, 40)
	if row, ok := l.cur.grid.Selected(); ok {
		if r, ok := l.cur.grid.RowRect(row); ok {
			at = geom.Pt(r.Min.X+40, r.Max.Y)
		}
	}
	l.cur.menu.Open(at, u)
	return true
}
