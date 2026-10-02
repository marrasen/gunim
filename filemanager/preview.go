package filemanager

import (
	"cmp"
	"path/filepath"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/widget"
)

func registerPreview(w *gunim.Window) {
	gunim.RegisterPatch(w, "browser", func(b *browser, p Preview, u *gunim.UI) { b.preview.show(p, u) })
	gunim.RegisterPatch(w, "browser", func(b *browser, c Counted, u *gunim.UI) { b.preview.counted(c, u) })
}

// previewPane shows the item selected: a picture's thumbnail or the start
// of a text file, and what there is to know about it. A new item
// crossfades in over the one before.
type previewPane struct {
	deck *deck
	cur  *previewPage
	seq  int
}

func newPreviewPane() *previewPane { return &previewPane{deck: &deck{}} }

func (p *previewPane) show(s Preview, u *gunim.UI) {
	if s.Seq < p.seq {
		return
	}
	p.seq = s.Seq
	p.cur = newPreviewPage(s)
	p.deck.show(p.cur, 0, u)
}

func (p *previewPane) counted(c Counted, u *gunim.UI) {
	if p.cur == nil || c.Seq != p.seq {
		return
	}
	p.cur.counted(c)
	u.Invalidate()
}

// Children implements [gunim.Composite].
func (p *previewPane) Children() []gunim.Node { return []gunim.Node{p.deck} }

// Layout implements [gunim.Node].
func (p *previewPane) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	kid.Layout(gunim.Tight(c.Max))
	kid.Place(geom.Point{})
	return c.Max
}

// Paint implements [gunim.Node].
func (p *previewPane) Paint(pt *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	pt.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(PaneFill.Get(f.Theme)))
	kids.At(0).Paint(pt)
}

// previewPage is the preview of one subject.
type previewPage struct {
	scroll *widget.Scroll
	holds  *widget.Label
	size   *widget.Label
	// path shows the item's path, and links are the links under it.
	path  *widget.Label
	links []*widget.Link
}

func newPreviewPage(s Preview) *previewPage {
	pg := &previewPage{}
	var kids []gunim.Node
	if s.Title == "" && s.Seq > 0 && len(s.Facts) == 0 {
		empty := widget.NewLabel("Nothing to preview.")
		empty.Color = Faint
		pg.scroll = widget.NewScroll(&previewPad{child: empty})
		return pg
	}
	if s.Image != nil {
		img := widget.NewImage(s.Image)
		img.Fit, img.Radius, img.Alt = widget.FitContain, 8, s.Title
		kids = append(kids, &thumbBox{img: img})
	} else if s.Title != "" {
		kids = append(kids, &typeTile{tint: s.Tint, label: tileLabel(s)})
	}
	title := widget.NewLabel(s.Title)
	title.Size, title.Face, title.MaxLines, title.Selectable = TitleText, widget.BoldFont, 3, true
	kind := widget.NewLabel(s.Type)
	kind.Color, kind.Size = Faint, SmallText
	kids = append(kids, title, kind)
	for _, f := range s.Facts {
		kids = append(kids, factRow(f.Label, f.Value))
	}
	if s.Counting {
		pg.holds, pg.size = widget.NewLabel("Counting…"), widget.NewLabel("…")
		kids = append(kids, factRowOf("Holds", pg.holds), factRowOf("Total size", pg.size))
	}
	if s.Path != "" {
		shown := cmp.Or(s.Shown, s.Path)
		pg.path = widget.NewLabel(shown)
		pg.path.Size, pg.path.Color, pg.path.Selectable = SmallText, Caption, true
		kids = append(kids, pg.path)
		if !s.NoReveal {
			reveal := widget.NewLink("Show in system file manager")
			reveal.Size = SmallText
			reveal.On = RevealPath{Path: s.Path}
			pg.links = append(pg.links, reveal)
		}
		copyPath := widget.NewLink("Copy path")
		copyPath.Size = SmallText
		copyPath.OnActivate(func(u *gunim.UI) { u.SetClipboard(shown) })
		pg.links = append(pg.links, copyPath)
		nodes := make([]gunim.Node, len(pg.links))
		for i, l := range pg.links {
			nodes[i] = l
		}
		links := &linkWrap{Wrap: widget.NewWrap(), kids: nodes}
		links.Gap = sideGap
		kids = append(kids, links)
	}
	if s.Online {
		note := widget.NewLabel("This file is kept online only. Download it to preview it.")
		note.Color, note.Size = Faint, SmallText
		fetch := widget.NewButton("Download")
		fetch.Icon = icon.CloudDownload
		fetch.On = FetchPreview{Path: s.Path}
		row := widget.Row(fetch, widget.NewSpacer())
		kids = append(kids, note, row)
	}
	if s.Err != "" {
		e := widget.NewLabel(s.Err)
		e.Color, e.Size, e.Selectable = ErrorInk, SmallText, true
		kids = append(kids, e)
	}
	if s.Text != "" {
		body := s.Text
		if s.Cut {
			body = strings.TrimRight(body, "\n") + "\n…"
		}
		t := widget.NewLabel(body)
		t.Face, t.Size, t.Selectable = widget.MonoFont, SmallText, true
		kids = append(kids, &textBox{child: t})
	}
	col := widget.Column(kids...)
	col.Cross = widget.CrossStretch
	pg.scroll = widget.NewScroll(&previewPad{child: col})
	return pg
}

// sideGap is the room between links in a row.
var sideGap = widget.Gap

// linkWrap sets links in a row, and moves a link that would pass the
// pane's edge to a line of its own.
type linkWrap struct {
	*widget.Wrap
	kids []gunim.Node
}

// Children implements [gunim.Composite].
func (l *linkWrap) Children() []gunim.Node { return l.kids }

// tileLabel is what the tile for an item without a picture says: its
// extension, or a word for a folder.
func tileLabel(s Preview) string {
	if s.Tint == TintFolder {
		return "Folder"
	}
	ext := strings.TrimPrefix(filepath.Ext(s.Title), ".")
	if ext == "" || len(ext) > 5 || s.Path == "" {
		return ""
	}
	return strings.ToUpper(ext)
}

// factRow is a label and its value, side by side.
func factRow(label, value string) gunim.Node {
	v := widget.NewLabel(value)
	return factRowOf(label, v)
}

func factRowOf(label string, v *widget.Label) gunim.Node {
	l := widget.NewLabel(label)
	l.Color, l.Size = Faint, SmallText
	v.Size, v.Selectable = SmallText, true
	row := widget.Row(widget.NewSized(l, 84, 0), v).Grow(v, 1)
	return row
}

func (pg *previewPage) counted(c Counted) {
	if pg.holds == nil {
		return
	}
	suffix := ""
	if c.Counting {
		suffix = " so far"
	}
	pg.holds.SetText(c.Items + suffix)
	pg.size.SetText(c.Size + suffix)
	if c.Err != "" {
		pg.holds.Color = ErrorInk
		pg.holds.SetText(c.Items + " counted, then: " + c.Err)
	}
}

// Children implements [gunim.Composite].
func (pg *previewPage) Children() []gunim.Node { return []gunim.Node{pg.scroll} }

// Layout implements [gunim.Node].
func (pg *previewPage) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	kid.Layout(gunim.Tight(c.Max))
	kid.Place(geom.Point{})
	return c.Max
}

// Paint implements [gunim.Node].
func (pg *previewPage) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}

// previewPad leaves room around the preview.
type previewPad struct {
	child gunim.Node
}

// Children implements [gunim.Composite].
func (p *previewPad) Children() []gunim.Node { return []gunim.Node{p.child} }

// Layout implements [gunim.Node].
func (p *previewPad) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	w := max(0, c.Max.W-32)
	s := kid.Layout(gunim.Constraints{Min: geom.Sz(w, 0), Max: geom.Sz(w, 0)})
	kid.Place(geom.Pt(16, 16))
	return geom.Sz(c.Max.W, s.H+32)
}

// Paint implements [gunim.Node].
func (p *previewPad) Paint(pt *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(pt)
}

// thumbBox shows a thumbnail as wide as the pane, and no taller than
// thumbHeight.
type thumbBox struct {
	img *widget.Image
}

const thumbHeight = 260

// Children implements [gunim.Composite].
func (t *thumbBox) Children() []gunim.Node { return []gunim.Node{t.img} }

// Layout implements [gunim.Node].
func (t *thumbBox) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	w, h := t.img.Source().Size()
	width := c.Max.W
	height := min(thumbHeight, width*float32(h)/float32(max(w, 1)))
	kid := kids.At(0)
	kid.Layout(gunim.Tight(geom.Sz(width, height)))
	kid.Place(geom.Point{})
	return geom.Sz(width, height+8)
}

// Paint implements [gunim.Node].
func (t *thumbBox) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}

// typeTile is a large rounded square in an item's colour, with its
// extension on it, for an item with no picture.
type typeTile struct {
	tint  Tint
	label string
	run   text.Run
	shown bool
}

// Layout implements [gunim.Node].
func (t *typeTile) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return geom.Sz(c.Max.W, 96)
}

// Paint implements [gunim.Node].
func (t *typeTile) Paint(p *paint.Painter, f gunim.Frame, _ geom.Size, _ gunim.Children) {
	tile := geom.Rc(0, 4, 80, 80)
	c := tintToken(t.tint).Get(f.Theme)
	p.ShadowRRect(tile, 18, paint.Solid(c), paint.Shadow{Offset: geom.Pt(0, 4), Blur: 14, Color: widget.DialogShadow.Get(f.Theme)})
	if t.label == "" {
		return
	}
	if !t.shown {
		t.run = widget.BoldFont.Default().Shape(t.label, 15)
		t.shown = true
	}
	t.run.Paint(p, geom.Pt(tile.Min.X+(80-t.run.Advance)/2, tile.Min.Y+(80-t.run.Height())/2),
		widget.ButtonStrongInk.Get(f.Theme))
}

// textBox frames the start of a text file.
type textBox struct {
	child gunim.Node
}

// Children implements [gunim.Composite].
func (t *textBox) Children() []gunim.Node { return []gunim.Node{t.child} }

// Layout implements [gunim.Node].
func (t *textBox) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	w := max(0, c.Max.W-20)
	s := kid.Layout(gunim.Constraints{Min: geom.Sz(w, 0), Max: geom.Sz(w, 0)})
	kid.Place(geom.Pt(10, 18))
	return geom.Sz(c.Max.W, s.H+28)
}

// Paint implements [gunim.Node].
func (t *textBox) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	r := geom.Rc(0, 8, box.W, box.H-8)
	p.RRect(r, 8, paint.Solid(widget.FieldFill.Get(f.Theme)))
	defer p.Layer(paint.LayerOpts{Bounds: r, Opacity: 1, Clip: true, Radius: 8})()
	kids.At(0).Paint(p)
}
