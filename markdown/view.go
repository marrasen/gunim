package markdown

import (
	"image/color"
	"strconv"
	"strings"
	"unicode"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// Link travels when the reader clicks a link, unless the view's OnLink says otherwise.
type Link struct{ URL string }

func init() { gunim.RegisterType[Link]("markdown.link") }

// View shows a Markdown document wrapped to its width. The reader selects its text with the mouse, across
// paragraphs, and copies it with Ctrl+C; a double click selects a word and a triple click a paragraph.
type View struct {
	// Breaks makes a single line break in a paragraph break the line, as it does in a chat.
	Breaks bool
	// Ink colours the text, and the theme's [widget.Ink] when unset.
	Ink theme.Token[color.NRGBA]
	// OnLink turns a click on a link into an intent. Unset, the view sends [Link].
	OnLink func(url string) gunim.Intent

	src string
	// parsed is the source and the breaks the blocks come from.
	parsed struct {
		src    string
		breaks bool
	}
	blocks []block

	// laidFor is what paras and marks were laid out for.
	laidFor struct {
		width, size float32
		parsed      struct {
			src    string
			breaks bool
		}
	}
	paras []laidPara
	marks []mark
	size  geom.Size
	// plain is the text of the paragraphs, one after another with a line break between two.
	plain []rune

	caret, anchor int
	held          bool
	// hover is the link under the pointer, as a paragraph and a span, or -1.
	hover [2]int
}

// laidPara is a paragraph laid out: where it sits, its lines, the spans its pieces come from, and where its text
// starts in the view's.
type laidPara struct {
	at    geom.Point
	p     text.SpanParagraph
	spans []span
	// code marks a code block's text, whose fill is the block's.
	code    bool
	base, n int
}

// mark is what a view draws besides its text.
type mark struct {
	kind markKind
	r    geom.Rect
	run  text.Run
	done bool
}

type markKind uint8

const (
	codeBox markKind = iota
	quoteBar
	bullet
	number
	checkBox
	ruleLine
)

// New returns a view of the Markdown src.
func New(src string) *View { return &View{src: src, hover: [2]int{-1, -1}} }

// SetText replaces the document, and drops the selection. Call it from a view's update function.
func (v *View) SetText(src string) {
	if src == v.src {
		return
	}
	v.src = src
	v.caret, v.anchor = 0, 0
}

// Text returns the document's Markdown.
func (v *View) Text() string { return v.src }

// Selection returns the selected runes' range in the text the view shows, start before end.
func (v *View) Selection() (start, end int) {
	n := len(v.plain)
	return min(v.caret, v.anchor, n), min(max(v.caret, v.anchor), n)
}

// SelectedText returns the selected text, without its Markdown.
func (v *View) SelectedText() string {
	start, end := v.Selection()
	return string(v.plain[start:end])
}

// parse parses the source, unless it already is.
func (v *View) parse() {
	if v.parsed.src == v.src && v.parsed.breaks == v.Breaks && v.blocks != nil {
		return
	}
	v.parsed.src, v.parsed.breaks = v.src, v.Breaks
	v.blocks = parse(v.src, v.Breaks)
}

// lay lays the document out w wide, unless it already is.
func (v *View) lay(th *theme.Live, w float32) {
	v.parse()
	size := widget.TextSize.Get(th)
	if v.laidFor.width == w && v.laidFor.size == size && v.laidFor.parsed == v.parsed && v.paras != nil {
		return
	}
	v.laidFor.width, v.laidFor.size, v.laidFor.parsed = w, size, v.parsed
	v.paras, v.marks, v.plain = v.paras[:0], v.marks[:0], v.plain[:0]
	l := layout{v: v, th: th, size: size}
	h := l.blocks(v.blocks, 0, 0, w)
	v.size = geom.Sz(w, h)
	if v.paras == nil {
		v.paras = []laidPara{}
	}
}

// layout lays a view's blocks out.
type layout struct {
	v    *View
	th   *theme.Live
	size float32
}

// gap is the room between blocks.
func (l layout) gap() float32 { return l.size * 0.6 }

// blocks lays bs out from y, between x and x+w, and returns where they end.
func (l layout) blocks(bs []block, x, y, w float32) float32 {
	for i, b := range bs {
		if i > 0 {
			y += l.gap()
		}
		y = l.block(b, x, y, w)
	}
	return y
}

func (l layout) block(b block, x, y, w float32) float32 {
	switch b.kind {
	case paragraph:
		return l.para(b.spans, x, y, w, l.size, 0, false)
	case heading:
		scale := map[int]float32{1: 1.45, 2: 1.25, 3: 1.1}[b.level]
		if scale == 0 {
			scale = 1
		}
		return l.para(b.spans, x, y, w, l.size*scale, bold, false)
	case code:
		pad := l.size * 0.6
		end := l.para([]span{{text: b.text, style: mono}}, x+pad, y+pad, w-2*pad, l.size, 0, true)
		l.v.marks = append(l.v.marks, mark{kind: codeBox, r: geom.Rc(x, y, w, end-y+pad)})
		return end + pad
	case quote:
		indent := l.size
		end := l.blocks(b.kids, x+indent, y, w-indent)
		l.v.marks = append(l.v.marks, mark{kind: quoteBar, r: geom.Rc(x+2, y, 3, end-y)})
		return end
	case list:
		return l.list(b, x, y, w)
	case rule:
		l.v.marks = append(l.v.marks, mark{kind: ruleLine, r: geom.Rc(x, y+l.size*0.4, w, 1)})
		return y + l.size*0.8
	}
	return y
}

// list lays a list out, with each item's bullet, number or box beside its first line.
func (l layout) list(b block, x, y, w float32) float32 {
	indent := l.size * 1.6
	for i, it := range b.items {
		if i > 0 {
			y += l.size * 0.25
		}
		first := len(l.v.paras)
		end := l.blocks(it.blocks, x+indent, y, w-indent)
		line := l.size * 1.3
		if first < len(l.v.paras) && len(l.v.paras[first].p.Lines) > 0 {
			line = l.v.paras[first].p.Lines[0].Height
		}
		mid := y + line/2
		switch {
		case it.task != noTask:
			s := l.size
			l.v.marks = append(l.v.marks, mark{kind: checkBox, r: geom.Rc(x+indent-s-6, mid-s/2, s, s), done: it.task == doneTask})
		case b.ordered:
			run := widget.Font.Get(l.th).Shape(strconv.Itoa(b.start+i)+".", l.size)
			l.v.marks = append(l.v.marks, mark{kind: number, r: geom.Rc(x+indent-6-run.Advance, mid-run.Height()/2, run.Advance, run.Height()), run: run})
		default:
			d := l.size * 0.36
			l.v.marks = append(l.v.marks, mark{kind: bullet, r: geom.Rc(x+indent-8-d, mid-d/2, d, d)})
		}
		y = max(end, y+line)
	}
	return y
}

// para lays spans out as a paragraph at x, y, w wide, in size and with extra style on top of each span's own.
func (l layout) para(spans []span, x, y, w, size float32, extra style, isCode bool) float32 {
	laid := make([]span, len(spans))
	ts := make([]text.Span, len(spans))
	n := 0
	for i, s := range spans {
		s.style |= extra
		laid[i] = s
		sz := size
		if s.style&mono != 0 {
			sz = size * 0.92
		}
		ts[i] = text.Span{Text: s.text, Face: faceFor(s.style).Get(l.th), Size: sz}
		n += len([]rune(s.text))
	}
	p := text.LayoutSpans(ts, text.Style{}, w)
	v := l.v
	base := len(v.plain)
	if len(v.paras) > 0 {
		base++
		v.plain = append(v.plain, '\n')
	}
	for _, s := range spans {
		v.plain = append(v.plain, []rune(s.text)...)
	}
	v.paras = append(v.paras, laidPara{at: geom.Pt(x, y), p: p, spans: laid, code: isCode, base: base, n: n})
	return y + p.Size.H
}

// faceFor returns the face a style sets text in.
func faceFor(st style) theme.Token[*text.Face] {
	switch {
	case st&mono != 0:
		return widget.MonoFont
	case st&bold != 0 && st&italic != 0:
		return widget.BoldItalicFont
	case st&bold != 0:
		return widget.BoldFont
	case st&italic != 0:
		return widget.ItalicFont
	}
	return widget.Font
}

// Layout implements [gunim.Node].
func (v *View) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	v.lay(f.Theme, c.Max.W)
	return c.Constrain(v.size)
}

// Paint implements [gunim.Node].
func (v *View) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	v.lay(th, box.W)
	ink := widget.Ink.Get(th)
	if v.Ink.Key() != "" {
		ink = v.Ink.Get(th)
	}
	for _, m := range v.marks {
		if m.kind == codeBox {
			p.RRect(m.r, 6, paint.Solid(widget.CodeFill.Get(th)))
		}
	}
	if start, end := v.Selection(); start != end {
		sel := paint.Solid(widget.Selection.Get(th))
		for _, lp := range v.paras {
			if end < lp.base || start > lp.base+lp.n {
				continue
			}
			lp.p.Select(start-lp.base, end-lp.base, func(r geom.Rect) { p.RRect(r.Add(lp.at), 3, sel) })
		}
	}
	for i, lp := range v.paras {
		for _, l := range lp.p.Lines {
			for _, pc := range l.Pieces {
				v.paintPiece(p, th, lp, i, pc, ink)
			}
		}
	}
	for _, m := range v.marks {
		v.paintMark(p, th, m, ink)
	}
}

// paintPiece draws one piece of paragraph i's text.
func (v *View) paintPiece(p *paint.Painter, th *theme.Live, lp laidPara, i int, pc text.Piece, ink color.NRGBA) {
	s := lp.spans[pc.Span]
	at := pc.At.Add(lp.at)
	box := geom.Rc(at.X, at.Y, pc.Run.Advance, pc.Run.Height())
	if s.style&mono != 0 && !lp.code {
		p.RRect(geom.Rc(box.Min.X-2, box.Min.Y, box.Size().W+4, box.Size().H), 3, paint.Solid(widget.CodeFill.Get(th)))
	}
	c := ink
	if s.url != "" {
		c = widget.LinkInk.Get(th)
	}
	pc.Run.Paint(p, at, c)
	if s.style&strike != 0 {
		p.RRect(geom.Rc(box.Min.X, at.Y+pc.Run.Ascent*0.62, box.Size().W, 1), 0, paint.Solid(c))
	}
	if s.url != "" && v.hover == [2]int{i, pc.Span} {
		p.RRect(geom.Rc(box.Min.X, at.Y+pc.Run.Ascent+1.5, box.Size().W, 1), 0, paint.Solid(c))
	}
}

// paintMark draws what goes beside the text: bullets, numbers, boxes, the bars beside quotes and rules.
func (v *View) paintMark(p *paint.Painter, th *theme.Live, m mark, ink color.NRGBA) {
	switch m.kind {
	case quoteBar:
		p.RRect(m.r, 1.5, paint.Solid(widget.QuoteBar.Get(th)))
	case ruleLine:
		p.RRect(m.r, 0, paint.Solid(widget.QuoteBar.Get(th)))
	case bullet:
		p.RRect(m.r, m.r.Size().W/2, paint.Solid(ink))
	case number:
		m.run.Paint(p, m.r.Min, ink)
	case checkBox:
		radius := m.r.Size().W * 0.25
		if !m.done {
			p.RRectStroke(m.r.Inset(geom.Uniform(0.75)), radius, paint.Fill{}, paint.Stroke{Width: 1.5, Color: widget.FieldBorder.Get(th)})
			return
		}
		p.RRect(m.r, radius, paint.Solid(widget.Accent.Get(th)))
		tick := widget.BoldFont.Get(th).Shape("✓", m.r.Size().H*0.85)
		c := m.r.Center()
		tick.Paint(p, geom.Pt(c.X-tick.Advance/2, c.Y-tick.Height()/2), widget.ButtonStrongInk.Get(th))
	case codeBox:
	}
}

// index returns the rune of the view's text a caret put at pt sits before.
func (v *View) index(pt geom.Point) int {
	for _, lp := range v.paras {
		if pt.Y < lp.at.Y {
			return lp.base
		}
		if pt.Y < lp.at.Y+lp.p.Size.H {
			return lp.base + lp.p.Index(pt.Sub(lp.at))
		}
	}
	return len(v.plain)
}

// linkAt returns the link under pt, as a paragraph and a span, or -1s.
func (v *View) linkAt(pt geom.Point) [2]int {
	for i, lp := range v.paras {
		for _, l := range lp.p.Lines {
			for _, pc := range l.Pieces {
				if lp.spans[pc.Span].url == "" {
					continue
				}
				at := pc.At.Add(lp.at)
				if geom.Rc(at.X, at.Y, pc.Run.Advance, pc.Run.Height()).Contains(pt) {
					return [2]int{i, pc.Span}
				}
			}
		}
	}
	return [2]int{-1, -1}
}

// Focusable implements [gunim.Focusable]: a click gives the view the keyboard, so Ctrl+C reaches it.
func (v *View) Focusable() bool { return true }

// SkipsTab implements [gunim.TabSkipper].
func (v *View) SkipsTab() {}

// DragHeld implements [gunim.DragHolder], so a scroll view scrolls while a selection is dragged past its edge.
func (v *View) DragHeld() bool { return v.held }

// Cursor implements [gunim.CursorShaper]: a hand over a link, and a text cursor over text.
func (v *View) Cursor(pt geom.Point) input.Cursor {
	if v.linkAt(pt)[0] >= 0 {
		return input.CursorHand
	}
	return input.CursorText
}

// Handle implements [gunim.Handler]: a click on a link sends it, the mouse selects, Ctrl+C copies and Ctrl+A
// selects everything.
func (v *View) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.FocusLost:
		v.anchor, v.held = v.caret, false
	case input.PointerMove:
		if at := v.linkAt(e.Pos); at != v.hover {
			v.hover = at
			u.Invalidate()
		}
		if !v.held {
			return false
		}
		v.caret = v.index(e.Pos)
	case input.PointerLeave:
		if v.hover[0] >= 0 {
			v.hover = [2]int{-1, -1}
			u.Invalidate()
		}
		return false
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		if at := v.linkAt(e.Pos); at[0] >= 0 && e.Clicks < 2 && !e.Mods.Has(input.ModShift) {
			url := v.paras[at[0]].spans[at[1]].url
			var in gunim.Intent = Link{URL: url}
			if v.OnLink != nil {
				in = v.OnLink(url)
			}
			u.Send(v, in)
			return true
		}
		i := v.index(e.Pos)
		switch {
		case e.Clicks < 2 && e.Mods.Has(input.ModShift):
			v.caret = i
		case e.Clicks == 2:
			v.anchor, v.caret = wordAround(v.plain, i)
		case e.Clicks >= 3:
			v.anchor, v.caret = lineAround(v.plain, i)
		default:
			v.anchor, v.caret = i, i
		}
		v.held = true
	case input.PointerUp:
		if !v.held {
			return false
		}
		v.held = false
	case input.KeyPress:
		if e.Typed || !e.Mods.Has(input.ModControl) && !e.Mods.Has(input.ModSuper) {
			return false
		}
		switch e.Key {
		case input.KeyC:
			if start, end := v.Selection(); start != end {
				u.SetClipboard(v.SelectedText())
			}
		case input.KeyA:
			v.anchor, v.caret = 0, len(v.plain)
		default:
			return false
		}
	default:
		return false
	}
	u.Invalidate()
	return true
}

// wordAround returns the word at rune i of rs.
func wordAround(rs []rune, i int) (start, end int) {
	i = max(0, min(i, len(rs)))
	start, end = i, i
	for start > 0 && !unicode.IsSpace(rs[start-1]) {
		start--
	}
	for end < len(rs) && !unicode.IsSpace(rs[end]) {
		end++
	}
	return start, end
}

// lineAround returns the line, which is a paragraph or a line of one broken by hand, that holds rune i of rs.
func lineAround(rs []rune, i int) (start, end int) {
	i = max(0, min(i, len(rs)))
	start, end = i, i
	for start > 0 && rs[start-1] != '\n' {
		start--
	}
	for end < len(rs) && rs[end] != '\n' {
		end++
	}
	return start, end
}

// Access implements [gunim.Accessible]: the text, without its Markdown, read as a label.
func (v *View) Access() access.Info {
	return access.Info{Role: access.RoleLabel, Name: strings.TrimSpace(Plain(v.src))}
}
