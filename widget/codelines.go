package widget

import (
	"reflect"
	"slices"
	"sort"

	"github.com/marrasen/gunim/syntax"
	"github.com/marrasen/gunim/text"
)

// splitLines brings the code's lines up to rs after a change that kept
// head runes at the start and tail at the end of code was runes long.
// It splits again only the lines the change touched, moves the lines
// after them, and returns the new lines, from to to, which have their
// text and nothing else.
func (c *CodeEditor) splitLines(rs []rune, head, tail, was int) (from, to int) {
	k := 0
	if len(c.lines) > 0 {
		k = c.lineOf(head)
	}
	s := 0
	if k < len(c.lines) {
		s = c.lines[k].start
	}
	delta := len(rs) - was
	kept := len(rs) - tail
	var fresh []codeLine
	j := len(c.lines)
	for {
		e := s
		for e < len(rs) && rs[e] != '\n' {
			e++
		}
		fresh = append(fresh, codeLine{start: s, end: e, text: string(rs[s:e])})
		if e == len(rs) {
			break
		}
		s = e + 1
		if s >= kept {
			// A line the code had before too: from here on the lines are
			// as they were.
			if o := c.lineStarting(s - delta); o >= k {
				j = o
				break
			}
		}
	}
	c.lines = slices.Replace(c.lines, min(k, len(c.lines)), j, fresh...)
	for i := k + len(fresh); i < len(c.lines); i++ {
		c.lines[i].start += delta
		c.lines[i].end += delta
	}
	return k, k + len(fresh)
}

// lineStarting returns the line that starts at rune i, or -1.
func (c *CodeEditor) lineStarting(i int) int {
	k := sort.Search(len(c.lines), func(k int) bool { return c.lines[k].start >= i })
	if k < len(c.lines) && c.lines[k].start == i {
		return k
	}
	return -1
}

// colour finds the tokens of lines from to to, which are new, and of
// the lines a change of colour carries on to, such as the lines after a
// comment opened. It reads the code a stretch at a time around the new
// lines: from two lines before them, so a line sees the line before it,
// and on until a line after them comes out as it was, so the highlighter
// is where it was there. It returns the lines whose tokens it set.
func (c *CodeEditor) colour(rs []rune, from, to int) (first, end int) {
	if c.Highlight == nil {
		for i := from; i < to; i++ {
			c.lines[i].toks, c.lines[i].cont = nil, false
		}
		return from, to
	}
	// Start on a line no token runs into. The first line read only gives
	// the next its context, unless it starts the code.
	w := max(0, from-2)
	for w > 0 && c.lines[w].cont {
		w--
	}
	first = w + 1
	if w == 0 {
		first = 0
	}
	for more := 16; ; more *= 2 {
		e := min(len(c.lines), to+more)
		lines := c.cut(c.Highlight(string(rs[c.lines[w].start:c.lines[e-1].end])), w, e)
		// The last line read may lack what follows it, unless it ends
		// the code.
		last := e - 2
		if e == len(c.lines) {
			last = e - 1
		}
		end = -1
		for i := to; i <= last; i++ {
			if l := lines[i-w]; l.cont == c.lines[i].cont && slices.Equal(l.toks, c.lines[i].toks) {
				end = i
				break
			}
		}
		if end < 0 && e < len(c.lines) {
			continue
		}
		if end < 0 {
			end = last
		}
		for i := first; i <= end; i++ {
			c.lines[i].toks, c.lines[i].cont = lines[i-w].toks, lines[i-w].cont
		}
		return first, end + 1
	}
}

// cut shares toks, found in the code from line w's start, out among
// lines w to e, each cut to its line and counted from its start.
func (c *CodeEditor) cut(toks []syntax.Token, w, e int) []codeLine {
	out := make([]codeLine, e-w)
	base := c.lines[w].start
	ti := 0
	for i := range out {
		start, end := c.lines[w+i].start-base, c.lines[w+i].end-base
		for ti < len(toks) && toks[ti].End <= start {
			ti++
		}
		for j := ti; j < len(toks) && toks[j].Start < end; j++ {
			tk := toks[j]
			if tk.Start < start {
				out[i].cont = true
			}
			s, e := max(tk.Start, start)-start, min(tk.End, end)-start
			if s < e {
				out[i].toks = append(out[i].toks, syntax.Token{Start: s, End: e, Kind: tk.Kind})
			}
		}
	}
	return out
}

// highlighterID tells highlighters apart, so the code colours again
// when its highlighter changes.
func highlighterID(h syntax.Highlighter) uintptr {
	if h == nil {
		return 0
	}
	return reflect.ValueOf(h).Pointer()
}

// shapeCache holds strings shaped lately, and lets go of the one used
// longest ago once it holds maxCodeShapes.
type shapeCache struct {
	runs map[string]*shapeEntry
	// newest and oldest end the list of entries, newest used first.
	newest, oldest *shapeEntry
}

// shapeEntry is a string in a shapeCache, its shape, and its neighbours
// in the order of use.
type shapeEntry struct {
	s            string
	run          text.Run
	newer, older *shapeEntry
}

const maxCodeShapes = 20000

// shape returns s shaped in face at size, from the cache.
func (c *shapeCache) shape(face *text.Face, s string, size float32) text.Run {
	if c.runs == nil {
		c.runs = map[string]*shapeEntry{}
	}
	e, ok := c.runs[s]
	if ok {
		c.unlink(e)
	} else {
		e = &shapeEntry{s: s, run: face.Shape(s, size)}
		c.runs[s] = e
		if len(c.runs) > maxCodeShapes {
			old := c.oldest
			c.unlink(old)
			delete(c.runs, old.s)
		}
	}
	e.older, e.newer = c.newest, nil
	if c.newest != nil {
		c.newest.newer = e
	}
	c.newest = e
	if c.oldest == nil {
		c.oldest = e
	}
	return e.run
}

// unlink takes e out of the order of use.
func (c *shapeCache) unlink(e *shapeEntry) {
	if e.newer != nil {
		e.newer.older = e.older
	} else {
		c.newest = e.older
	}
	if e.older != nil {
		e.older.newer = e.newer
	} else {
		c.oldest = e.newer
	}
	e.newer, e.older = nil, nil
}

// clear empties the cache.
func (c *shapeCache) clear() {
	clear(c.runs)
	c.newest, c.oldest = nil, nil
}
