package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// filesPane is a project's files: the folders along the way to the one open, and what that folder holds.
type filesPane struct {
	root    *panel
	address *widget.AddressBar
	grid    *widget.DataGrid
	files   Files
	project string
}

func newFilesPane() *filesPane {
	f := &filesPane{address: widget.NewAddressBar()}
	f.address.OnGo = func(path string) gunim.Intent { return FolderOpened{Path: strings.Trim(path, "/")} }
	f.grid = widget.NewDataGrid(
		widget.GridColumn{Title: "Name", Width: 320},
		widget.GridColumn{Title: "Size", Width: 90, End: true},
		widget.GridColumn{Title: "Changed", Width: 150},
		widget.GridColumn{Title: "By", Width: 140},
	)
	f.grid.Row = f.row
	f.grid.OnActivate = func(i int) gunim.Intent {
		if i < 0 || i >= len(f.files.Entries) {
			return nil
		}
		e := f.files.Entries[i]
		if !e.Folder {
			return nil
		}
		return FolderOpened{Path: joinPath(f.files.Path, e.Name)}
	}
	folderIcon := widget.NewIcon(icon.FolderOpen, "Files")
	header := widget.Row(folderIcon, f.address).Grow(f.address, 1)
	header.Cross = widget.CrossCenter
	body := widget.Column(widget.NewPad(header), f.grid).Grow(f.grid, 1)
	body.Cross, body.Gap = widget.CrossStretch, zeroGap
	f.root = &panel{child: body, fill: PaneFill}
	return f
}

// set shows files, the files of the project named project.
func (f *filesPane) set(project string, files Files, u *gunim.UI) {
	moved := files.Path != f.files.Path || project != f.project
	f.files, f.project = files, project
	crumbs := []widget.Crumb{{Name: project, Path: "/"}}
	if files.Path != "" {
		at := ""
		for _, name := range strings.Split(files.Path, "/") {
			at = joinPath(at, name)
			crumbs = append(crumbs, widget.Crumb{Name: name, Path: "/" + at})
		}
	}
	f.address.SetPath("/"+files.Path, crumbs, u)
	f.grid.SetRows(len(files.Entries), u)
	if moved {
		f.grid.JumpTo(0, false, u)
	}
	u.Invalidate()
}

// row returns entry i as the grid shows it.
func (f *filesPane) row(i int) (widget.GridRow, bool) {
	if i < 0 || i >= len(f.files.Entries) {
		return widget.GridRow{}, false
	}
	e := f.files.Entries[i]
	name := widget.GridSpan{Text: e.Name, Icon: fileIcon(e)}
	size := widget.GridSpan{Text: sizeText(e.Size), Faint: true}
	if e.Folder {
		size.Text = itemsText(e.Items)
	}
	changed := widget.GridSpan{Faint: true}
	if !e.At.IsZero() {
		changed.Text = e.At.Format("2 Jan 2006 15:04")
	}
	return widget.GridRow{Cells: [][]widget.GridSpan{{name}, {size}, {changed}, {{Text: e.By, Faint: true}}}}, true
}

// fileIcon returns the icon for an entry: a folder, a picture, a text, or any other file.
func fileIcon(e FileEntry) *icon.Icon {
	if e.Folder {
		return icon.Folder
	}
	switch ext := strings.ToLower(e.Name[strings.LastIndexByte(e.Name, '.')+1:]); ext {
	case "png", "jpg", "jpeg", "gif", "svg", "webp":
		return icon.FileImage
	case "md", "txt", "pdf":
		return icon.FileText
	}
	return icon.File
}

// sizeText writes a size in bytes the way people read it.
func sizeText(n int64) string {
	switch {
	case n < 1000:
		return strconv.FormatInt(n, 10) + " B"
	case n < 1000*1000:
		return fmt.Sprintf("%.0f kB", float64(n)/1000)
	case n < 1000*1000*1000:
		return fmt.Sprintf("%.1f MB", float64(n)/1e6)
	}
	return fmt.Sprintf("%.1f GB", float64(n)/1e9)
}

// itemsText says how many things a folder holds.
func itemsText(n int) string {
	if n == 1 {
		return "1 item"
	}
	return strconv.Itoa(n) + " items"
}

// joinPath adds name to a folder path.
func joinPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "/" + name
}

// areas shows one of its children, the area of the project chosen, crossfading as the choice changes. The others
// stay built, so they keep what they hold, such as a message half written.
type areas struct {
	anim.Group
	kids  []gunim.Node
	shown int
	vis   []*anim.Float
}

func newAreas(kids ...gunim.Node) *areas {
	a := &areas{kids: kids}
	for i := range kids {
		v := anim.NewFloat(0)
		if i == 0 {
			v.Jump(1)
		}
		a.vis = append(a.vis, v)
		a.Add(v)
	}
	return a
}

// show crossfades to child i.
func (a *areas) show(i int, u *gunim.UI) {
	if i == a.shown {
		return
	}
	a.shown = i
	for k, v := range a.vis {
		v.Animate(map[bool]float32{false: 0, true: 1}[k == i], widget.Quick.Get(u.Theme()))
	}
	u.Invalidate()
}

// Children implements [gunim.Composite].
func (a *areas) Children() []gunim.Node { return a.kids }

// Layout implements [gunim.Node]: every child fills the room.
func (a *areas) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	for kid := range kids.All {
		kid.Layout(gunim.Tight(c.Max))
		kid.Place(geom.Point{})
	}
	return c.Max
}

// Paint implements [gunim.Node]: a child out of sight is not drawn, so it takes no clicks.
func (a *areas) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, kids gunim.Children) {
	for i := range a.kids {
		o := min(max(a.vis[i].Value(), 0), 1)
		switch {
		case o <= 0.01:
		case o >= 0.99:
			kids.At(i).Paint(p)
		default:
			func() {
				defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: o})()
				kids.At(i).Paint(p)
			}()
		}
	}
}
