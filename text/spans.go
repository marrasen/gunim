package text

import (
	"strings"
	"unicode"

	"github.com/go-text/typesetting/segmenter"

	"github.com/marrasen/gunim/geom"
)

// A Span is a piece of text in a face and size of its own, for
// [LayoutSpans].
type Span struct {
	Text string
	Face *Face
	// Size is the font size in logical pixels.
	Size float32
	// Box, when above zero, makes the span a box that many pixels wide in place of its text, such as an icon. It
	// is as tall as the span's text, and breaks with the text beside it as a letter would.
	Box float32
}

// A Piece is the part of one span that sits on one line of a
// [SpanParagraph].
type Piece struct {
	// Span is the index of the span the piece comes from.
	Span int
	Run  Run
	// At is the top-left of the piece's line box, where Run.Paint puts it.
	At geom.Point
	// Start is the rune the piece starts at, counted through the spans' texts one after another, with a box as one
	// rune.
	Start int
}

// A SpanLine is one line of a [SpanParagraph].
type SpanLine struct {
	Pieces []Piece
	// Top is the line's top within the paragraph, and Height the room it
	// takes to the next line's top.
	Top, Height float32
	// Width is how far the line's text reaches, without trailing spaces.
	Width float32
}

// A SpanParagraph is spans laid out in lines.
type SpanParagraph struct {
	Lines []SpanLine
	// Size is the box the lines fill.
	Size geom.Size
}

// boxRune stands for a box when finding where lines may break, so a box breaks as a letter does.
const boxRune = 'x'

// unit is the text between two places a line may break: pieces of one or
// more spans, laid out one after another.
type unit struct {
	pieces []Piece
	// texts are the pieces' texts, in the same order.
	texts []string
	// width is how far the unit reaches, and ink how far without the
	// spaces at its end.
	width, ink float32
	// mandatory says a line must end after the unit.
	mandatory bool
}

// LayoutSpans sets spans as one paragraph, each in its own face and size,
// wrapped at width where Unicode's line breaking rules allow, and within a
// word only when the word alone is wider than width. A newline starts a
// new line. A width of zero or less sets each line unbroken.
//
// It sets text left to right; a right-to-left run keeps its own order
// within a piece. st's Size and MaxLines are not used.
func LayoutSpans(spans []Span, st Style, width float32) SpanParagraph {
	if st.LineHeight <= 0 {
		st.LineHeight = 1
	}
	var all []rune
	var owner []int
	for i, s := range spans {
		if s.Box > 0 {
			all = append(all, boxRune)
			owner = append(owner, i)
			continue
		}
		for _, r := range s.Text {
			all = append(all, r)
			owner = append(owner, i)
		}
	}
	var units []unit
	if len(all) > 0 {
		var seg segmenter.Segmenter
		seg.Init(all)
		lines := seg.LineIterator()
		for lines.Next() {
			l := lines.Line()
			units = append(units, makeUnit(spans, all, owner, l.Offset, l.Offset+len(l.Text), l.IsMandatoryBreak))
		}
	}

	var out SpanParagraph
	var line []Piece
	var pen, ink float32
	flush := func() {
		out.Lines = append(out.Lines, finishLine(spans, line, ink, st))
		line, pen, ink = nil, 0, 0
	}
	place := func(u unit) {
		for _, p := range u.pieces {
			p.At.X += pen
			line = append(line, p)
		}
		if u.ink > 0 {
			ink = pen + u.ink
		}
		pen += u.width
	}
	for _, u := range units {
		if width > 0 && len(line) > 0 && pen+u.ink > width {
			flush()
		}
		for width > 0 && len(line) == 0 && u.ink > width {
			head, rest, ok := splitUnit(spans, u, width)
			if !ok {
				break
			}
			place(head)
			flush()
			u = rest
		}
		place(u)
		if u.mandatory {
			flush()
		}
	}
	if len(line) > 0 || len(out.Lines) == 0 {
		flush()
	}

	var top float32
	for i := range out.Lines {
		l := &out.Lines[i]
		l.Top = top
		for k := range l.Pieces {
			l.Pieces[k].At.Y += top
		}
		top += l.Height
		out.Size.W = max(out.Size.W, l.Width)
	}
	out.Size.H = top
	for i := range out.Lines {
		l := &out.Lines[i]
		var shift float32
		switch st.Align {
		case AlignCenter:
			shift = (out.Size.W - l.Width) / 2
		case AlignEnd:
			shift = out.Size.W - l.Width
		case AlignStart:
		}
		for k := range l.Pieces {
			l.Pieces[k].At.X += shift
		}
	}
	return out
}

// makeUnit shapes the runes of all from start up to end, one piece for each
// span they touch.
func makeUnit(spans []Span, all []rune, owner []int, start, end int, mandatory bool) unit {
	u := unit{mandatory: mandatory}
	for a := start; a < end; {
		b := a
		for b < end && owner[b] == owner[a] {
			b++
		}
		s := spans[owner[a]]
		if s.Box > 0 {
			run := s.Face.Shape("", s.Size)
			run.Advance = s.Box
			u.pieces = append(u.pieces, Piece{Span: owner[a], Run: run, At: geom.Pt(u.width, 0), Start: a})
			u.texts = append(u.texts, "")
			u.width += s.Box
			u.ink = u.width
			a = b
			continue
		}
		text := strings.TrimRight(string(all[a:b]), "\r\n")
		run := s.Face.Shape(text, s.Size)
		u.pieces = append(u.pieces, Piece{Span: owner[a], Run: run, At: geom.Pt(u.width, 0), Start: a})
		u.texts = append(u.texts, text)
		// The ink ends before the spaces the unit ends in.
		ink := u.width + run.CaretX(len([]rune(strings.TrimRightFunc(text, unicode.IsSpace))))
		if strings.TrimSpace(text) != "" {
			u.ink = ink
		}
		u.width += run.Advance
		a = b
	}
	return u
}

// splitUnit cuts u after as many runes as fit in width, at least one, and
// reports false when there is nothing to cut. A box, such as an icon, that
// runs past width moves to the rest with what follows it.
func splitUnit(spans []Span, u unit, width float32) (head, rest unit, ok bool) {
	for i, p := range u.pieces {
		runes := []rune(u.texts[i])
		if p.At.X+p.Run.Advance <= width {
			continue
		}
		if len(runes) == 0 {
			if i == 0 {
				continue
			}
			head, rest = cutBefore(u, i)
			return head, rest, true
		}
		k := 0
		for k < len(runes) && p.At.X+p.Run.CaretX(k+1) <= width {
			k++
		}
		if k == 0 {
			// With text before it, the cut falls before this piece. After
			// boxes alone, the head keeps a rune, as the unit's first
			// piece does.
			for j := range i {
				if u.texts[j] != "" {
					head, rest = cutBefore(u, i)
					return head, rest, true
				}
			}
			k = 1
		}
		if k >= len(runes) && i == len(u.pieces)-1 {
			return unit{}, unit{}, false
		}
		s := spans[p.Span]
		left := s.Face.Shape(string(runes[:k]), s.Size)
		right := s.Face.Shape(string(runes[k:]), s.Size)
		head = unit{
			pieces: append(append([]Piece(nil), u.pieces[:i]...), Piece{Span: p.Span, Run: left, At: p.At, Start: p.Start}),
			texts:  append(append([]string(nil), u.texts[:i]...), string(runes[:k])),
		}
		head.width = p.At.X + left.Advance
		head.ink = head.width
		rest = unit{pieces: []Piece{{Span: p.Span, Run: right, Start: p.Start + k}}, texts: []string{string(runes[k:])}, mandatory: u.mandatory}
		x := right.Advance
		for j, q := range u.pieces[i+1:] {
			q.At.X = x
			rest.pieces = append(rest.pieces, q)
			rest.texts = append(rest.texts, u.texts[i+1+j])
			x += q.Run.Advance
		}
		rest.width = x
		rest.ink = x - (u.width - u.ink)
		return head, rest, true
	}
	return unit{}, unit{}, false
}

// cutBefore cuts u before its piece i, which moves to the start of the rest.
func cutBefore(u unit, i int) (head, rest unit) {
	at := u.pieces[i].At.X
	head = unit{pieces: append([]Piece(nil), u.pieces[:i]...), texts: append([]string(nil), u.texts[:i]...), width: at, ink: at}
	rest = unit{texts: append([]string(nil), u.texts[i:]...), mandatory: u.mandatory, width: u.width - at, ink: u.ink - at}
	for _, q := range u.pieces[i:] {
		q.At.X -= at
		rest.pieces = append(rest.pieces, q)
	}
	return head, rest
}

// finishLine sets the line's height from its tallest piece, and puts every
// piece's line box at the same baseline.
func finishLine(spans []Span, pieces []Piece, ink float32, st Style) SpanLine {
	var ascent, descent, gap float32
	measure := func(s Span) {
		a, d, g := s.Face.Metrics(s.Size)
		ascent, descent, gap = max(ascent, a), max(descent, d), max(gap, g)
	}
	for _, p := range pieces {
		measure(spans[p.Span])
	}
	if len(pieces) == 0 && len(spans) > 0 {
		measure(spans[0])
	}
	height := (ascent + descent + gap) * st.LineHeight
	halfLeading := (height - ascent - descent) / 2
	for i := range pieces {
		p := &pieces[i]
		// A piece's box starts its own ascent above the shared baseline.
		p.At.Y = halfLeading + ascent - p.Run.Ascent
	}
	return SpanLine{Pieces: pieces, Height: height, Width: ink}
}

// Index returns the rune a caret put at pt, in the paragraph's space, sits before: on the line pt is level with, or
// the nearest line, at the place on it nearest pt.
func (p SpanParagraph) Index(pt geom.Point) int {
	if len(p.Lines) == 0 {
		return 0
	}
	line := p.Lines[len(p.Lines)-1]
	for _, l := range p.Lines {
		if pt.Y < l.Top+l.Height {
			line = l
			break
		}
	}
	if len(line.Pieces) == 0 {
		return 0
	}
	pc := line.Pieces[len(line.Pieces)-1]
	for _, q := range line.Pieces {
		if pt.X < q.At.X+q.Run.Advance {
			pc = q
			break
		}
	}
	return pc.Start + pc.Run.Index(pt.X-pc.At.X)
}

// Select calls fn with the box, in the paragraph's space, that runes start to end cover on each line they touch. A
// selection that carries on past a line's end covers a little more, standing for the line break.
func (p SpanParagraph) Select(start, end int, fn func(geom.Rect)) {
	const lineBreak = 6
	for _, l := range p.Lines {
		if len(l.Pieces) == 0 {
			continue
		}
		first, last := l.Pieces[0], l.Pieces[len(l.Pieces)-1]
		from, to := first.Start, last.Start+last.Run.End
		if end < from || start > to || start == end {
			continue
		}
		x0, x1 := float32(-1), float32(0)
		for _, pc := range l.Pieces {
			a, b := max(start, pc.Start), min(end, pc.Start+pc.Run.End)
			if a > b {
				continue
			}
			l0, l1 := pc.At.X+pc.Run.CaretX(a-pc.Start), pc.At.X+pc.Run.CaretX(b-pc.Start)
			if x0 < 0 || l0 < x0 {
				x0 = l0
			}
			x1 = max(x1, l1)
		}
		if x0 < 0 {
			continue
		}
		if end > to {
			x1 += lineBreak
		}
		fn(geom.Rc(x0, l.Top, x1-x0, l.Height))
	}
}
