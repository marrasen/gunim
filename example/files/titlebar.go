package main

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// menuItem is one item of a menu: its label, its keys, and the command
// it sends. A label of "-" puts a line in the menu.
type menuItem struct {
	label, hint, cmd string
}

// Commands the window does itself.
const (
	localEditPath   = "local.editpath"
	localFilter     = "local.filter"
	localSelectAll  = "local.selectall"
	localSelectNone = "local.selectnone"
)

// menus are the menu bar's menus.
var menus = []struct {
	title string
	items []menuItem
}{
	{"File", []menuItem{
		{"New folder", "Ctrl+Shift+N", CmdNewFolder},
		{"Open", "Enter", CmdOpen},
		{"-", "", ""},
		{"Pin to sidebar", "Ctrl+D", CmdPin},
		{"Show in system file manager", "", CmdReveal},
		{"-", "", ""},
		{"Close", "Ctrl+W", CmdCloseApp},
	}},
	{"Edit", []menuItem{
		{"Undo", "Ctrl+Z", CmdUndo},
		{"-", "", ""},
		{"Cut", "Ctrl+X", CmdCut},
		{"Copy", "Ctrl+C", CmdCopy},
		{"Paste", "Ctrl+V", CmdPaste},
		{"-", "", ""},
		{"Rename", "F2", CmdRename},
		{"Move to trash", "Delete", CmdTrash},
		{"Delete permanently", "Shift+Delete", CmdDelete},
		{"-", "", ""},
		{"Select all", "Ctrl+A", localSelectAll},
		{"Select none", "Escape", localSelectNone},
	}},
	{"View", []menuItem{
		{"Show hidden files", "Ctrl+H", CmdHidden},
		{"Preview pane", "", CmdPreview},
		{"-", "", ""},
		{"Sort by name", "", CmdSortName},
		{"Sort by size", "", CmdSortSize},
		{"Sort by date modified", "", CmdSortTime},
		{"Sort by type", "", CmdSortType},
		{"-", "", ""},
		{"Light theme", "", CmdTheme},
		{"Refresh", "F5", CmdRefresh},
	}},
	{"Go", []menuItem{
		{"Back", "Alt+Left", CmdBack},
		{"Forward", "Alt+Right", CmdForward},
		{"Up", "Alt+Up", CmdUp},
		{"Home", "Alt+Home", CmdHome},
		{"-", "", ""},
		{"Edit path", "Ctrl+L", localEditPath},
		{"Filter", "Ctrl+F", localFilter},
	}},
}

// titleBar is the window's own title bar: the menus, the title, and the
// window's buttons.
type titleBar struct {
	b        *browser
	bar      *widget.Menubar
	controls *widget.WindowControls
	// cmds holds each menu's commands, in the order the bar has them.
	cmds [][]string
}

func newTitleBar(b *browser) *titleBar {
	t := &titleBar{b: b, controls: widget.NewWindowControls()}
	bars := make([]widget.BarMenu, 0, len(menus))
	for _, m := range menus {
		bm := widget.BarMenu{Title: m.title}
		var cmds []string
		for _, it := range m.items {
			if it.label == "-" {
				bm.Breaks = append(bm.Breaks, len(bm.Items))
				continue
			}
			bm.Items = append(bm.Items, it.label)
			bm.Hints = append(bm.Hints, it.hint)
			cmds = append(cmds, it.cmd)
		}
		bm.Checked = make([]bool, len(bm.Items))
		bars = append(bars, bm)
		t.cmds = append(t.cmds, cmds)
	}
	t.bar = widget.NewMenubar(bars...)
	t.bar.Title = "Files"
	t.bar.Pick = t.pick
	return t
}

// pick runs the command of item i of menu m.
func (t *titleBar) pick(m, i int, u *gunim.UI) {
	cmd := t.cmds[m][i]
	switch cmd {
	case localEditPath:
		t.b.path.edit(u)
	case localFilter:
		u.Focus(t.b.path.filter)
	case localSelectAll:
		t.b.listing.selectAll(u)
	case localSelectNone:
		t.b.listing.selectNone(u)
	default:
		u.Send(t.bar, Command{Name: cmd})
	}
}

// check ticks the item that sends cmd, or not.
func (t *titleBar) check(cmd string, on bool) {
	for m, cmds := range t.cmds {
		for i, c := range cmds {
			if c == cmd {
				t.bar.Menus[m].Checked[i] = on
			}
		}
	}
}

func (t *titleBar) setShell(s Shell) {
	t.check(CmdHidden, s.ShowHidden)
	t.check(CmdPreview, s.ShowPreview)
	t.check(CmdTheme, s.Light)
}

// setListing names the folder in the title, and ticks the sort.
func (t *titleBar) setListing(l Listing) {
	t.bar.Title = l.Title + " — Files"
	for by, cmd := range []string{CmdSortName, CmdSortSize, CmdSortTime, CmdSortType} {
		t.check(cmd, SortBy(by) == l.Sort)
	}
}

// Children implements [gunim.Composite].
func (t *titleBar) Children() []gunim.Node { return []gunim.Node{t.bar, t.controls} }

// Layout implements [gunim.Node].
func (t *titleBar) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	h := widget.MenubarHeight.Get(f.Theme)
	if f.Chromeless() {
		h = max(h, 34)
	}
	bar, controls := kids.At(0), kids.At(1)
	cs := controls.Layout(gunim.Constraints{Max: geom.Sz(c.Max.W, h)})
	controls.Place(geom.Pt(c.Max.W-cs.W, 0))
	bar.Layout(gunim.Tight(geom.Sz(max(0, c.Max.W-cs.W), h)))
	bar.Place(geom.Point{})
	return geom.Sz(c.Max.W, h)
}

// Paint implements [gunim.Node].
func (t *titleBar) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(widget.MenubarFill.Get(f.Theme)))
	for k := range kids.All {
		k.Paint(p)
	}
}

// Handle implements [gunim.Handler]: F10 opens the first menu.
func (t *titleBar) Handle(e input.Event, u *gunim.UI) bool {
	if k, ok := e.(input.KeyPress); ok && k.Key == input.KeyF10 {
		t.bar.Open(0, u)
		return true
	}
	return false
}
