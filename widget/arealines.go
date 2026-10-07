package widget

import (
	"image/color"
	"slices"
	"sort"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

// areaText is text laid out a paragraph at a time, each wrapped on its
// own, so an edit lays out again only the paragraphs it touches. It
// answers as the [text.Paragraph] of the whole text does: a line counts
// down the whole text, and a rune across it.
type areaText struct {
	paras []areaPara
	face  *text.Face
	style text.Style
	width float32
	// LineHeight is the distance from one line's top to the next's, and
	// Size the box the lines fill: the widest line by every line.
	LineHeight float32
	Size       geom.Size
	// lines counts the lines.
	lines int
}

// areaPara is a paragraph: runes start to end, and the line break after
// it sep runes long. It is laid out alone, and its first line is line
// number line of the text.
type areaPara struct {
	start, end, sep int
	line            int
	p               text.Paragraph
}

// update brings the text up to rs, laid out in face with st at width,
// after a change that kept head runes at the start and tail at the end
// of a text was runes long. Changed false says rs is as it was.
func (t *areaText) update(face *text.Face, st text.Style, width float32, rs []rune, head, tail, was int, changed bool) {
	switch {
	case t.face != face || t.style != st || len(t.paras) == 0:
		t.face, t.style, t.width = face, st, width
		t.paras = t.paras[:0]
		head, tail, was, changed = 0, 0, 0, true
	case t.width != width && changed:
		t.width = width
		t.paras = t.paras[:0]
		head, tail, was = 0, 0, 0
	case t.width != width:
		// A paragraph laid out on one line that fits the new width stays
		// as it is; the rest wrap again.
		t.width = width
		for i := range t.paras {
			if p := &t.paras[i]; len(p.p.Lines) > 1 || p.p.Size.W > width {
				p.p = face.Layout(string(rs[p.start:p.end]), st, width)
			}
		}
		if !changed {
			t.measure()
			return
		}
	}
	if !changed {
		return
	}

	// Start at the paragraph before the change: a change just after a
	// carriage return can join it to a newline.
	k := t.paraAt(max(head-1, 0))
	from := 0
	if k < len(t.paras) {
		from = t.paras[k].start
	}
	delta := len(rs) - was
	kept := len(rs) - tail
	var fresh []areaPara
	j := len(t.paras)
	for s := from; ; {
		e, sep := s, 0
		for e < len(rs) {
			if sep = lineBreak(rs, e); sep > 0 {
				break
			}
			e++
		}
		fresh = append(fresh, areaPara{start: s, end: e, sep: sep, p: face.Layout(string(rs[s:e]), st, width)})
		s = e + sep
		if sep == 0 {
			break
		}
		if s >= kept {
			// A paragraph the text had before too: from here on they are
			// as they were.
			if o := t.paraStarting(s - delta); o >= k {
				j = o
				break
			}
		}
	}
	t.paras = slices.Replace(t.paras, min(k, len(t.paras)), j, fresh...)
	for i := k + len(fresh); i < len(t.paras); i++ {
		t.paras[i].start += delta
		t.paras[i].end += delta
	}
	t.measure()
}

// measure counts the lines down to each paragraph, and the size of
// them all.
func (t *areaText) measure() {
	t.lines, t.Size.W = 0, 0
	for i := range t.paras {
		p := &t.paras[i]
		p.line = t.lines
		t.lines += len(p.p.Lines)
		t.Size.W = max(t.Size.W, p.p.Size.W)
		t.LineHeight = p.p.LineHeight
	}
	t.Size.H = float32(t.lines) * t.LineHeight
}

// lineBreak returns how many runes the line break at rune i of rs
// takes, as [text.Face.Layout] reads them: two for \r\n, one for the
// others, and none for a rune that breaks no line.
func lineBreak(rs []rune, i int) int {
	switch rs[i] {
	case '\r':
		if i+1 < len(rs) && rs[i+1] == '\n' {
			return 2
		}
		return 1
	case '\n', '\u0085', ' ', ' ':
		return 1
	}
	return 0
}

// paraAt returns the paragraph holding rune i: the last that starts at
// or before it.
func (t *areaText) paraAt(i int) int {
	k := sort.Search(len(t.paras), func(k int) bool { return t.paras[k].start > i }) - 1
	return max(0, k)
}

// paraStarting returns the paragraph that starts at rune i, or -1.
func (t *areaText) paraStarting(i int) int {
	k := sort.Search(len(t.paras), func(k int) bool { return t.paras[k].start >= i })
	if k < len(t.paras) && t.paras[k].start == i {
		return k
	}
	return -1
}

// paraOfLine returns the paragraph holding line n.
func (t *areaText) paraOfLine(n int) int {
	k := sort.Search(len(t.paras), func(k int) bool { return t.paras[k].line > n }) - 1
	return max(0, k)
}

// count returns how many lines the text takes.
func (t *areaText) count() int { return t.lines }

// line returns line n: its run, whose rune indices count from base, and
// the top-left of its box.
func (t *areaText) line(n int) (run text.Run, base int, at geom.Point) {
	p := t.paras[t.paraOfLine(n)]
	l := p.p.Lines[n-p.line]
	at = geom.Pt(0, float32(p.line)*t.LineHeight+l.At.Y)
	if l.RightToLeft {
		// Right to left lines line up on the widest line's right.
		at.X = t.Size.W - l.Run.Advance
	}
	return l.Run, p.start, at
}

// lineHeight returns how tall a line's text is.
func (t *areaText) lineHeight() float32 {
	if len(t.paras) == 0 || len(t.paras[0].p.Lines) == 0 {
		return t.LineHeight
	}
	return t.paras[0].p.Lines[0].Run.Height()
}

// Caret returns where a caret before rune i goes, as
// [text.Paragraph.Caret] does.
func (t *areaText) Caret(i int) (line int, at geom.Point) {
	if len(t.paras) == 0 {
		return 0, geom.Point{}
	}
	p := t.paras[t.paraAt(i)]
	n, _ := p.p.Caret(i - p.start)
	run, base, at := t.line(p.line + n)
	return p.line + n, geom.Pt(at.X+run.CaretX(i-base), at.Y)
}

// Index returns the rune whose caret is nearest pt, as
// [text.Paragraph.Index] does.
func (t *areaText) Index(pt geom.Point) int {
	if t.lines == 0 {
		return 0
	}
	n := 0
	if t.LineHeight > 0 {
		n = int(pt.Y / t.LineHeight)
	}
	run, base, at := t.line(max(0, min(n, t.lines-1)))
	return base + run.Index(pt.X-at.X)
}

// Paint draws the lines p shows, with the text's top-left at topLeft.
func (t *areaText) Paint(p *paint.Painter, topLeft geom.Point, c color.NRGBA) {
	seen, cull := p.Visible()
	from, to := t.shown(p, topLeft)
	for n := from; n < to; n++ {
		run, _, at := t.line(n)
		at = topLeft.Add(at)
		if h := run.Height(); cull && (at.Y+2*h < seen.Min.Y || at.Y-h > seen.Max.Y) {
			continue
		}
		run.Paint(p, at, c)
	}
}

// shown returns the lines p shows, a line's height of room kept above
// and below for the strokes that reach past a line's box, as
// [text.Paragraph.Paint] keeps.
func (t *areaText) shown(p *paint.Painter, topLeft geom.Point) (from, to int) {
	seen, cull := p.Visible()
	if !cull || t.LineHeight <= 0 {
		return 0, t.lines
	}
	h := t.lineHeight()
	from = int((seen.Min.Y-topLeft.Y-2*h)/t.LineHeight) - 1
	to = int((seen.Max.Y-topLeft.Y+h)/t.LineHeight) + 2
	return max(0, from), min(t.lines, to)
}

// spans calls fn with the rectangle, in the text's space, that runes
// start to end cover on each line from line from to line to that they
// touch, as [paraSpans] does.
func (t *areaText) spans(start, end, from, to int, fn func(geom.Rect)) {
	const lineBreak = 6
	first, _ := t.Caret(start)
	last, _ := t.Caret(end)
	for n := max(first-1, from); n <= min(last+1, to-1); n++ {
		run, base, at := t.line(n)
		rs, re := run.Start+base, run.End+base
		s0, s1 := max(start, rs), min(end, re)
		if s0 > s1 || (s0 == s1 && end <= re) {
			continue
		}
		x0, x1 := run.CaretX(s0-base), run.CaretX(s1-base)
		if x0 > x1 {
			x0, x1 = x1, x0
		}
		if end > re {
			x1 += lineBreak
		}
		fn(geom.Rc(at.X+x0, at.Y, x1-x0, run.Height()))
	}
}
