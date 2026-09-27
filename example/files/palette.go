package main

import (
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/widget"
)

func registerPalette(w *gunim.Window) {
	gunim.RegisterPatch(w, "browser", func(b *browser, r PaletteResults, u *gunim.UI) { b.palette.results(r, u) })
	gunim.RegisterPatch(w, "browser", func(b *browser, o OpenPalette, u *gunim.UI) { b.palette.open(o.Query, u) })
}

// filesPalette is the palette Ctrl+P opens: it finds a file or a folder
// under the folder showing as the user types, or with ">" a command.
type filesPalette struct {
	b    *browser
	p    *widget.Palette
	seq  int
	hits []PaletteHit
}

func newFilesPalette(b *browser) *filesPalette {
	fp := &filesPalette{b: b}
	fp.p = &widget.Palette{
		Placeholder: "Go to a file or a folder, or type > for commands",
		Search:      fp.search,
		Pick:        func(i int, u *gunim.UI) { fp.pick(i, false, u) },
		CtrlPick:    func(i int, u *gunim.UI) { fp.pick(i, true, u) },
	}
	return fp
}

// open opens the palette over the listing, with query typed.
func (fp *filesPalette) open(query string, u *gunim.UI) {
	if !fp.p.IsOpen() {
		r, ok := u.Bounds(fp.b.listing)
		if !ok {
			return
		}
		fp.p.Status = ""
		fp.p.Items = nil
		fp.p.Open(fp.b.listing, geom.Rc(0, 12, r.Size().W, 0), u)
	}
	if query != "" {
		fp.p.SetQuery(query, u)
	}
}

// search asks the application for what the query finds.
func (fp *filesPalette) search(q string, u *gunim.UI) {
	fp.seq++
	u.Send(fp.b.listing, PaletteQuery{Seq: fp.seq, Text: q})
}

// results shows the application's answer to the query last asked.
func (fp *filesPalette) results(r PaletteResults, u *gunim.UI) {
	if r.Seq != fp.seq || !fp.p.IsOpen() {
		return
	}
	fp.hits = r.Hits
	items := make([]widget.PaletteItem, len(r.Hits))
	for i, h := range r.Hits {
		items[i] = widget.PaletteItem{Title: h.Title, Detail: h.Detail, Hint: h.Hint, Key: widget.Key(h.Key), At: h.At,
			Problem: h.Problem}
	}
	fp.p.SetItems(items, u)
	fp.p.SetStatus(r.Status, u)
}

// pick does what hit i says: a command the window runs itself here, and
// the rest in the application.
func (fp *filesPalette) pick(i int, ctrl bool, u *gunim.UI) {
	if i < 0 || i >= len(fp.hits) {
		return
	}
	key := fp.hits[i].Key
	if cmd, ok := strings.CutPrefix(key, "cmd:"); ok && fp.b.runLocal(cmd, u) {
		return
	}
	u.Send(fp.b.listing, PalettePicked{Key: key, Ctrl: ctrl})
}

// runLocal runs a command the window does itself, and reports whether cmd
// was one.
func (b *browser) runLocal(cmd string, u *gunim.UI) bool {
	switch cmd {
	case localEditPath:
		b.path.edit(u)
	case localFilter:
		u.Focus(b.path.filter)
	case localSelectAll:
		b.listing.selectAll(u)
	case localSelectNone:
		b.listing.selectNone(u)
	default:
		return false
	}
	return true
}
