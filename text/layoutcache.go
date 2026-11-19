package text

import (
	"math"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"
)

// Shaping and laying out text are the most costly steps of a frame, and
// they run under mu, so one window's text waits on another's. A widget
// asks for the same text at the same size and width frame after frame,
// so [Face.Layout] and [Face.Shape] keep what they made in a cache shared
// by every face. A call the cache can answer copies the answer out and
// never takes mu.
//
// The cache holds up to cacheBudget bytes, by an estimate of each
// answer's size, and drops what was used longest ago to make room. An
// answer bigger than a sixteenth of that, such as a long document, is
// made again each time rather than push the rest out.
const cacheBudget = 4 << 20

// generation counts the changes to what a layout reads besides its
// arguments: a face's fallbacks, and whether the system's fonts fill in.
// It is part of every key, so an answer from before a change is never
// given after it; such answers fall out of the cache in time. It changes
// with mu held, so a layout made with mu held belongs to the generation
// read then.
var generation atomic.Uint64

// cacheKey holds every argument a layout depends on. Sizes and widths
// are kept as their bits, so a NaN finds itself and 0 and -0 stay apart.
type cacheKey struct {
	face *Face
	s    string
	// shaped says the answer is a [Face.Shape] run rather than a
	// [Face.Layout] paragraph.
	shaped                  bool
	size, lineHeight, width uint32
	align                   Align
	maxLines                int
	gen                     uint64
}

// layoutKey is the key for a [Face.Layout]. Arguments the layout treats
// alike share a key: every width but one above zero sets lines unbroken,
// a line height of zero or less is 1, and a line limit of zero or less
// is none.
func layoutKey(f *Face, s string, st Style, width float32) cacheKey {
	if !(width > 0) {
		width = 0
	}
	if st.LineHeight <= 0 {
		st.LineHeight = 1
	}
	st.MaxLines = max(st.MaxLines, 0)
	return cacheKey{
		face: f, s: s, size: math.Float32bits(st.Size), lineHeight: math.Float32bits(st.LineHeight),
		width: math.Float32bits(width), align: st.Align, maxLines: st.MaxLines, gen: generation.Load(),
	}
}

func shapeKey(f *Face, s string, size float32) cacheKey {
	return cacheKey{face: f, s: s, shaped: true, size: math.Float32bits(size), gen: generation.Load()}
}

// cacheEntry is one answer, in a list from the most recently used to the
// least. Its run or paragraph never changes once it is in the cache: the
// cache keeps a copy of its own, and gives each caller another.
type cacheEntry struct {
	key        cacheKey
	run        Run
	para       Paragraph
	cost       int
	prev, next *cacheEntry
}

// layoutCache is a map of answers with a list of them by when they were
// last used. Its lock is held only to find, add or drop an entry.
type layoutCache struct {
	mu      sync.Mutex
	entries map[cacheKey]*cacheEntry
	// head is a ring's sentinel: head.next is the newest entry and
	// head.prev the oldest.
	head cacheEntry
	// cost is the estimated size of every entry together, which stays
	// within budget.
	cost, budget int
	// hits and misses count lookups, for the tests.
	hits, misses int
}

var cache = newLayoutCache(cacheBudget)

func newLayoutCache(budget int) *layoutCache {
	c := &layoutCache{entries: map[cacheKey]*cacheEntry{}, budget: budget}
	c.head.prev, c.head.next = &c.head, &c.head
	return c
}

// get returns the entry for k, or nil, and makes it the newest.
func (c *layoutCache) get(k cacheKey) *cacheEntry {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.entries[k]
	if e == nil {
		c.misses++
		return nil
	}
	c.hits++
	c.unlink(e)
	c.pushFront(e)
	return e
}

// put adds e as the newest entry, dropping the oldest until the cache
// fits its budget. An entry too big for the cache is left out.
func (c *layoutCache) put(e *cacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e.cost > c.budget/16 {
		return
	}
	if old := c.entries[e.key]; old != nil {
		// Two callers made the same answer at once; the first one stays.
		return
	}
	// The text may be a slice of a much longer string; the key keeps a
	// copy of its own, so the cache holds no more than it counts.
	e.key.s = strings.Clone(e.key.s)
	c.entries[e.key] = e
	c.pushFront(e)
	c.cost += e.cost
	for c.cost > c.budget {
		old := c.head.prev
		c.unlink(old)
		delete(c.entries, old.key)
		c.cost -= old.cost
	}
}

func (c *layoutCache) unlink(e *cacheEntry) {
	e.prev.next, e.next.prev = e.next, e.prev
	e.prev, e.next = nil, nil
}

func (c *layoutCache) pushFront(e *cacheEntry) {
	e.prev, e.next = &c.head, c.head.next
	c.head.next.prev = e
	c.head.next = e
}

// reset empties the cache and sets its budget. The tests use it to start
// from nothing.
func (c *layoutCache) reset(budget int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.budget = budget
	clear(c.entries)
	c.head.prev, c.head.next = &c.head, &c.head
	c.cost, c.hits, c.misses = 0, 0, 0
}

// entryOverhead is a rough size for an entry and its place in the map,
// beyond its text and slices.
const entryOverhead = int(unsafe.Sizeof(cacheEntry{})) + 64

// runCost is a rough size of what r's slices hold.
func runCost(r *Run) int {
	return len(r.Glyphs)*int(unsafe.Sizeof(r.Glyphs[0])) +
		len(r.carets)*int(unsafe.Sizeof(float32(0))) +
		len(r.stops)*int(unsafe.Sizeof(stop{}))
}

func paragraphCost(p *Paragraph) int {
	n := len(p.Lines) * int(unsafe.Sizeof(Line{}))
	for i := range p.Lines {
		n += runCost(&p.Lines[i].Run)
	}
	return n
}

// clone returns r with slices of its own, so a caller that changes them
// cannot change the cache's copy.
func (r Run) clone() Run {
	r.Glyphs = slices.Clone(r.Glyphs)
	r.carets = slices.Clone(r.carets)
	r.stops = slices.Clone(r.stops)
	return r
}

func (p Paragraph) clone() Paragraph {
	p.Lines = slices.Clone(p.Lines)
	for i := range p.Lines {
		p.Lines[i].Run = p.Lines[i].Run.clone()
	}
	return p
}
