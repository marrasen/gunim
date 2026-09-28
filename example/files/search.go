package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/match"
)

// How often the walk reports, how long an index stays fresh, and how
// many hits the palette shows.
const (
	indexFresh  = 10 * time.Second
	indexReport = 100 * time.Millisecond
	paletteHits = 60
)

// bounds are how many items the palette's walk takes at most, and for
// how long it walks.
type bounds struct {
	items int
	time  time.Duration
}

// indexBounds are the walk's bounds.
var indexBounds = bounds{items: 200_000, time: 5 * time.Second}

// indexed is one item under the folder the palette searches.
type indexed struct {
	// rel is the item's path under the root, and lower it in lower case.
	rel, lower string
	// name starts at nameAt in rel, and lowerName is it in lower case.
	// Lower case can change a string's length in bytes, as a Kelvin sign
	// or a dotted capital I does, so the name's own lower case is kept
	// rather than cut out of lower at nameAt.
	nameAt    int
	lowerName string
	dir       bool
	depth     int
}

func (x indexed) name() string { return x.rel[x.nameAt:] }

// walkError is a folder the walk could not read.
type walkError struct {
	rel string
	err error
}

// searchState is the palette's index of the folder showing, and the
// query it answers.
type searchState struct {
	root  string
	items []indexed
	errs  []walkError
	// walking is set while the walk runs, stopped says why it stopped
	// early, and done when it ended.
	walking bool
	stopped string
	done    time.Time
	gen     int
	cancel  context.CancelFunc
	// seq and text are the query last asked, and ranking cancels its
	// ranking.
	seq     int
	text    string
	ranking context.CancelFunc
	// reranked is when the query was last ranked, as the walk went on.
	reranked time.Time
}

// stop stops the walk and the ranking.
func (s *searchState) stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.ranking != nil {
		s.ranking()
	}
}

// handleSearch takes the palette's intents.
func (a *app) handleSearch(in gunim.Intent) bool {
	switch v := in.(type) {
	case PaletteQuery:
		a.paletteQuery(v)
	case PalettePicked:
		a.palettePicked(v)
	default:
		return false
	}
	return true
}

// paletteQuery answers a query, and starts the index of the folder
// showing when there is none of it or it has gone stale.
func (a *app) paletteQuery(q PaletteQuery) {
	s := &a.search
	s.seq, s.text = q.Seq, q.Text
	here := a.nav.path
	stale := !s.walking && time.Since(s.done) > indexFresh
	if here != "" && (!samePath(s.root, here) || stale) && !strings.HasPrefix(q.Text, ">") {
		a.startIndex(here)
	}
	a.rank()
}

// startIndex walks the tree under root in the background.
func (a *app) startIndex(root string) {
	s := &a.search
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	s.gen++
	s.root, s.items, s.errs, s.walking, s.stopped, s.cancel = root, nil, nil, true, "", cancel
	gen := s.gen
	go walkIndex(ctx, root, indexBounds, os.ReadDir, func(items []indexed, errs []walkError, stopped string, done bool) {
		a.post(func() { a.indexed(gen, items, errs, stopped, done) })
	})
}

// indexed takes a batch of the walk of index gen.
func (a *app) indexed(gen int, items []indexed, errs []walkError, stopped string, done bool) {
	s := &a.search
	if gen != s.gen {
		return
	}
	s.items = append(s.items, items...)
	s.errs = append(s.errs, errs...)
	if done {
		s.walking, s.stopped, s.done = false, stopped, time.Now()
		s.cancel()
	}
	if done || time.Since(s.reranked) > 4*indexReport {
		a.rank()
	}
}

// walkIndex walks the tree under root with read, a folder at a time, and
// hands tell what it found every indexReport, stopping at the bounds. A
// folder it cannot read goes to tell as an error.
func walkIndex(ctx context.Context, root string, b bounds, read func(string) ([]os.DirEntry, error),
	tell func([]indexed, []walkError, string, bool)) {
	start := time.Now()
	last := start
	var batch []indexed
	var errs []walkError
	seen := 0
	stopped := ""
	// A queue, so the shallow items come first.
	queue := []string{""}
	for len(queue) > 0 && stopped == "" {
		if ctx.Err() != nil {
			return
		}
		rel := queue[0]
		queue = queue[1:]
		es, err := read(filepath.Join(root, rel))
		if err != nil {
			errs = append(errs, walkError{rel: rel, err: err})
		}
		for _, e := range es {
			p := filepath.Join(rel, e.Name())
			dir := e.IsDir()
			batch = append(batch, indexed{rel: p, lower: strings.ToLower(p), nameAt: len(p) - len(e.Name()),
				lowerName: strings.ToLower(e.Name()), dir: dir,
				depth: strings.Count(p, string(filepath.Separator))})
			if dir {
				queue = append(queue, p)
			}
			seen++
			if seen >= b.items {
				stopped = "Stopped at " + plural(seen, "item")
				break
			}
		}
		if stopped == "" && len(queue) > 0 && time.Since(start) > b.time {
			stopped = fmt.Sprintf("Stopped after %s, at %s", b.time.Round(time.Millisecond), plural(seen, "item"))
		}
		if time.Since(last) >= indexReport {
			last = time.Now()
			tell(batch, errs, "", false)
			batch, errs = nil, nil
		}
	}
	tell(batch, errs, stopped, true)
}

// rank ranks the index for the query last asked, in the background, and
// sends the hits.
func (a *app) rank() {
	s := &a.search
	if s.ranking != nil {
		s.ranking()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	s.ranking = cancel
	s.reranked = time.Now()
	seq, text := s.seq, s.text
	if strings.HasPrefix(text, ">") {
		hits := rankCommands(strings.TrimSpace(strings.TrimPrefix(text, ">")), a.places, a.prefs.Favourites)
		a.patch(PaletteResults{Seq: seq, Hits: hits, Status: "Commands. Delete the > to look for files."})
		return
	}
	items, errs, root := s.items, s.errs, s.root
	status := a.searchStatus()
	go func() {
		hits := rankIndex(ctx, root, items, errs, text)
		if ctx.Err() != nil {
			return
		}
		a.post(func() {
			if a.search.seq == seq {
				a.patch(PaletteResults{Seq: seq, Hits: hits, Status: status})
			}
		})
	}()
}

// searchStatus says how far the index has got.
func (a *app) searchStatus() string {
	s := &a.search
	var b strings.Builder
	switch {
	case s.walking:
		fmt.Fprintf(&b, "Looking through %s: %s items so far…", placeName(s.root), count(len(s.items)))
	case s.stopped != "":
		fmt.Fprintf(&b, "%s in %s. Type more to narrow it down.", s.stopped, placeName(s.root))
	default:
		fmt.Fprintf(&b, "%s under %s. Type > for commands.", plural(len(s.items), "item"), placeName(s.root))
	}
	if n := len(s.errs); n > 0 {
		fmt.Fprintf(&b, " %s could not be read.", plural(n, "folder"))
	}
	return b.String()
}

// scored is an item the query found, and how well.
type scored struct {
	i int
	// byName is set when the query is in the item's name, and at holds
	// where in it.
	byName bool
	at     []int
	score  int
}

// rankIndex returns the hits for query among items: matches in the name
// first, then in the path, each by how well they match and then by how
// shallow and short the path is. The folders that could not be read
// follow.
func rankIndex(ctx context.Context, root string, items []indexed, errs []walkError, query string) []PaletteHit {
	q := strings.ToLower(strings.TrimSpace(query))
	var found []scored
	for i, it := range items {
		if i%4096 == 0 && ctx.Err() != nil {
			return nil
		}
		if q == "" {
			if it.depth == 0 {
				found = append(found, scored{i: i, byName: true})
			}
			continue
		}
		if !subsequence(q, it.lower) {
			continue
		}
		if subsequence(q, it.lowerName) {
			if at, score, ok := match.Find(q, it.name()); ok {
				found = append(found, scored{i: i, byName: true, at: at, score: score})
				continue
			}
		}
		if _, score, ok := match.Find(q, it.rel); ok {
			found = append(found, scored{i: i, score: score})
		}
	}
	slices.SortFunc(found, func(x, y scored) int {
		if x.byName != y.byName {
			if x.byName {
				return -1
			}
			return 1
		}
		a, b := items[x.i], items[y.i]
		if q == "" && a.dir != b.dir {
			if a.dir {
				return -1
			}
			return 1
		}
		return cmp.Or(cmp.Compare(y.score, x.score), cmp.Compare(a.depth, b.depth), cmp.Compare(len(a.rel), len(b.rel)),
			naturalCompare(a.lower, b.lower))
	})
	hits := make([]PaletteHit, 0, min(len(found), paletteHits)+len(errs))
	for _, f := range found[:min(len(found), paletteHits)] {
		it := items[f.i]
		h := PaletteHit{Title: it.name(), Detail: folderOf(root, it), Key: "file:" + filepath.Join(root, it.rel), At: f.at}
		if it.dir {
			h.Hint = "Folder"
		}
		hits = append(hits, h)
	}
	for _, e := range errs {
		where := filepath.Join(root, e.rel)
		if q != "" && !subsequence(q, strings.ToLower(e.rel)) {
			continue
		}
		hits = append(hits, PaletteHit{Title: "Could not read " + placeName(where), Detail: rootCause(e.err).Error(),
			Key: "go:" + where, Problem: true})
	}
	return hits
}

// folderOf is the folder an item is in, as the palette shows it.
func folderOf(root string, it indexed) string {
	if it.nameAt == 0 {
		return placeName(root)
	}
	return filepath.Join(placeName(root), it.rel[:it.nameAt-1])
}

// subsequence reports whether the runes of q appear in s in order.
func subsequence(q, s string) bool {
	for _, r := range q {
		i := strings.IndexRune(s, r)
		if i < 0 {
			return false
		}
		s = s[i+utf8.RuneLen(r):]
	}
	return true
}

// rankCommands returns the commands of the menus and the places to go
// that query finds, best first.
func rankCommands(query string, places []Place, favourites []string) []PaletteHit {
	var hits []PaletteHit
	var items []match.Item
	for _, m := range menus {
		for _, it := range m.items {
			if it.label == "-" {
				continue
			}
			hits = append(hits, PaletteHit{Title: it.label, Detail: m.title, Hint: it.hint, Key: "cmd:" + it.cmd})
			items = append(items, match.Item{Title: it.label, Also: []string{m.title}})
		}
	}
	for _, p := range places {
		hits = append(hits, PaletteHit{Title: "Go to " + p.Name, Detail: p.Path, Key: "go:" + p.Path})
		items = append(items, match.Item{Title: "Go to " + p.Name, Also: []string{p.Path}})
	}
	for _, f := range favourites {
		hits = append(hits, PaletteHit{Title: "Go to " + placeName(f), Detail: f, Hint: "Favourite", Key: "go:" + f})
		items = append(items, match.Item{Title: "Go to " + placeName(f), Also: []string{f}})
	}
	found := match.Rank(items, query)
	out := make([]PaletteHit, len(found))
	for i, f := range found {
		out[i] = hits[f.Index]
		out[i].At = f.At
	}
	return out
}

// palettePicked does what a hit picked in the palette says.
func (a *app) palettePicked(v PalettePicked) {
	kind, rest, _ := strings.Cut(v.Key, ":")
	switch kind {
	case "cmd":
		a.handle(a.handlers, Command{Name: rest})
	case "go":
		a.navigate(rest, 0, true)
	case "file":
		info, err := os.Lstat(rest)
		if err != nil {
			a.fail(fmt.Sprintf("Opening %s: %v", rest, err))
			return
		}
		if v.Ctrl {
			if info.IsDir() {
				a.navigate(rest, 0, true)
				return
			}
			a.openWith(rest)
			return
		}
		a.navigate(filepath.Dir(rest), 0, true)
		a.nav.pick = filepath.Base(rest)
	}
}

// openWith opens the file at path with its program.
func (a *app) openWith(path string) {
	go func() {
		if err := a.c.Open(path); err != nil {
			a.post(func() { a.fail(fmt.Sprintf("Opening %s: %v", path, err)) })
		}
	}()
}
