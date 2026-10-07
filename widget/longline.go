package widget

import (
	"image/color"
	"slices"
	"sort"
	"unicode"

	"golang.org/x/text/unicode/bidi"

	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
)

// longLine is one line of text shaped in pieces, so an edit shapes
// again only the pieces it touches and a frame paints only the pieces in
// view. It answers as a [text.Run] of the whole line does, in runes of
// the whole line.
//
// A piece ends after a run of spaces, where the word before the spaces
// picks a break, or where the piece has grown long. So the breaks
// depend on the text near them, and stay put around an edit. A line with
// any text that runs right to left is one piece: the order of its runs
// on screen depends on the whole line.
type longLine struct {
	pieces []linePiece
	face   *text.Face
	size   float32
	// Advance is the line's width.
	Advance float32
	// shapes holds pieces shaped lately, by their text.
	shapes map[string]text.Run
}

// linePiece is a stretch of the line, shaped on its own: runes start to
// end, set from x on.
type linePiece struct {
	start, end int
	x          float32
	run        text.Run
	// rtl says the piece holds text that runs right to left, or a
	// direction mark.
	rtl bool
}

// Piece limits: a piece breaks where the hash of the word before the
// break is a multiple of pieceEvery, or once it is maxPiece runes long,
// and the cache of shapes is cleared past maxShapes.
const (
	pieceEvery = 8
	maxPiece   = 256
	maxShapes  = 4096
)

// update brings the line up to rs, shaped in face at size, after a
// change that kept head runes at the start and tail at the end of a
// line was runes long. Changed false says rs is as it was.
func (l *longLine) update(face *text.Face, size float32, rs []rune, head, tail, was int, changed bool) {
	if l.face != face || l.size != size || len(l.pieces) == 0 {
		l.face, l.size = face, size
		clear(l.shapes)
		l.pieces = l.pieces[:0]
		head, tail, changed = 0, 0, true
	}
	if !changed {
		return
	}
	if l.shapes == nil {
		l.shapes = map[string]text.Run{}
	}
	if len(l.pieces) == 1 && l.pieces[0].rtl {
		// One piece, for text right to left: while some is left, the
		// whole line shapes again.
		if slices.ContainsFunc(rs, rightToLeft) {
			l.pieces[0] = l.piece(rs, 0, len(rs))
			l.Advance = l.pieces[0].run.Advance
			return
		}
		l.pieces = l.pieces[:0]
	}
	if len(l.pieces) == 0 {
		head, tail, was = 0, 0, 0
	}
	// Start at the piece before the change: a break depends on the runes
	// before it and the one after.
	k := l.pieceAt(max(head-1, 0))
	from := 0
	if k < len(l.pieces) {
		from = l.pieces[k].start
	}
	delta := len(rs) - was
	// From kept on, the text is as it was, moved by delta.
	kept := len(rs) - tail
	var fresh []linePiece
	j := len(l.pieces) // the first old piece kept after the fresh ones
	for s := from; ; {
		e := pieceEnd(rs, s)
		p := l.piece(rs, s, e)
		if p.rtl {
			l.pieces = append(l.pieces[:0], l.piece(rs, 0, len(rs)))
			l.Advance = l.pieces[0].run.Advance
			return
		}
		fresh = append(fresh, p)
		s = e
		if s >= len(rs) {
			break
		}
		if s >= kept {
			// A break the text had before too: from here on the pieces
			// are as they were.
			if o := l.pieceStarting(s - delta); o >= k {
				j = o
				break
			}
		}
	}
	l.pieces = slices.Replace(l.pieces, min(k, len(l.pieces)), j, fresh...)
	x := float32(0)
	for i := range l.pieces {
		if i >= k+len(fresh) {
			l.pieces[i].start += delta
			l.pieces[i].end += delta
		}
		l.pieces[i].x = x
		x += l.pieces[i].run.Advance
	}
	l.Advance = x
}

// piece shapes runes start to end of rs.
func (l *longLine) piece(rs []rune, start, end int) linePiece {
	s := string(rs[start:end])
	run, ok := l.shapes[s]
	if !ok {
		if len(l.shapes) >= maxShapes {
			clear(l.shapes)
		}
		run = l.face.Shape(s, l.size)
		l.shapes[s] = run
	}
	p := linePiece{start: start, end: end, run: run}
	for _, r := range rs[start:end] {
		if rightToLeft(r) {
			p.rtl = true
			break
		}
	}
	return p
}

// pieceEnd returns where the piece that starts at rune start of rs ends.
func pieceEnd(rs []rune, start int) int {
	// hash is the hash of the word before the spaces, FNV-1a; 1 before
	// any word.
	hash, word := uint32(1), false
	for i := start; i < len(rs); i++ {
		r := rs[i]
		space := unicode.IsSpace(r)
		if !space && i > start && unicode.IsSpace(rs[i-1]) && !unicode.Is(unicode.M, r) &&
			(hash%pieceEvery == 0 || i-start >= maxPiece) {
			return i
		}
		switch {
		case space:
			word = false
		case !word:
			hash, word = 2166136261, true
			fallthrough
		default:
			hash = (hash ^ uint32(r)) * 16777619
		}
	}
	return len(rs)
}

// rightToLeft reports whether r runs right to left or sets a direction.
func rightToLeft(r rune) bool {
	if r < 0x0590 {
		return false
	}
	p, _ := bidi.LookupRune(r)
	switch p.Class() {
	case bidi.R, bidi.AL, bidi.AN, bidi.LRO, bidi.RLO, bidi.LRE, bidi.RLE, bidi.PDF, bidi.LRI, bidi.RLI, bidi.FSI, bidi.PDI:
		return true
	default:
		return false
	}
}

// pieceAt returns the piece holding rune i, the last for the line's end.
func (l *longLine) pieceAt(i int) int {
	k := sort.Search(len(l.pieces), func(k int) bool { return l.pieces[k].start > i }) - 1
	return max(0, k)
}

// pieceStarting returns the piece that starts at rune i, or -1.
func (l *longLine) pieceStarting(i int) int {
	k := sort.Search(len(l.pieces), func(k int) bool { return l.pieces[k].start >= i })
	if k < len(l.pieces) && l.pieces[k].start == i {
		return k
	}
	return -1
}

// pieceAtX returns the piece across x.
func (l *longLine) pieceAtX(x float32) int {
	k := sort.Search(len(l.pieces), func(k int) bool { return l.pieces[k].x > x }) - 1
	return max(0, min(k, len(l.pieces)-1))
}

// CaretX returns the x of a caret before rune i, as [text.Run.CaretX].
func (l *longLine) CaretX(i int) float32 {
	if len(l.pieces) == 0 {
		return 0
	}
	p := l.pieces[l.pieceAt(i)]
	return p.x + p.run.CaretX(i-p.start)
}

// Index returns the rune whose caret is nearest x, as [text.Run.Index].
func (l *longLine) Index(x float32) int {
	if len(l.pieces) == 0 {
		return 0
	}
	p := l.pieces[l.pieceAtX(x)]
	return p.start + p.run.Index(x-p.x)
}

// Beside returns the caret place next on screen, as [text.Run.Beside].
func (l *longLine) Beside(i int, x float32, right bool) (next int, nextX float32) {
	if len(l.pieces) == 0 {
		return i, x
	}
	k, step := l.pieceAtX(x), -1
	if right {
		step = 1
	}
	for _, k := range []int{k, k + step} {
		if k < 0 || k >= len(l.pieces) {
			continue
		}
		p := l.pieces[k]
		j, jx := p.run.Beside(i-p.start, x-p.x, right)
		if j != i-p.start || jx != x-p.x {
			return p.start + j, p.x + jx
		}
	}
	return i, x
}

// Places reports whether a caret before rune i can sit at x, as
// [text.Run.Places].
func (l *longLine) Places(i int, x float32) bool {
	if len(l.pieces) == 0 {
		return false
	}
	k := l.pieceAtX(x)
	for _, k := range []int{k, k - 1} {
		if k >= 0 && l.pieces[k].run.Places(i-l.pieces[k].start, x-l.pieces[k].x) {
			return true
		}
	}
	return false
}

// end returns how many runes the line holds.
func (l *longLine) end() int {
	if len(l.pieces) == 0 {
		return 0
	}
	return l.pieces[len(l.pieces)-1].end
}

// Height returns the line's height, as [text.Run.Height].
func (l *longLine) Height() float32 {
	if len(l.pieces) == 0 {
		return 0
	}
	return l.pieces[0].run.Height()
}

// Ascent returns how far the line reaches above its baseline.
func (l *longLine) Ascent() float32 {
	if len(l.pieces) == 0 {
		return 0
	}
	return l.pieces[0].run.Ascent
}

// Paint draws the glyphs of the line from x0 to x1, in the line's space,
// with the top-left of its box at topLeft.
func (l *longLine) Paint(p *paint.Painter, topLeft geom.Point, c color.NRGBA, x0, x1 float32) {
	// A glyph's ink can reach a little past its pen position either
	// way.
	reach := 2 * l.size
	for k := l.pieceAtX(x0 - reach); k < len(l.pieces) && l.pieces[k].x <= x1+reach; k++ {
		piece := l.pieces[k]
		run := piece.run
		gs := run.Glyphs
		lo := sort.Search(len(gs), func(g int) bool { return piece.x+gs[g].At.X >= x0-reach })
		hi := sort.Search(len(gs), func(g int) bool { return piece.x+gs[g].At.X > x1+reach })
		if lo == 0 && hi == len(gs) {
			run.Paint(p, topLeft.Add(geom.Pt(piece.x, 0)), c)
			continue
		}
		run.Glyphs = gs[lo:hi]
		run.Paint(p, topLeft.Add(geom.Pt(piece.x, 0)), c)
	}
}
