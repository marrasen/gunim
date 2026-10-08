package themeedit

import (
	"hash/fnv"
	"slices"
	"strconv"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// Filter narrows All values to the values changed, or to a kind.
type Filter int

// The filters, in the order All values offers them.
const (
	// FilterAll lists every value.
	FilterAll Filter = iota
	// FilterChanged lists the values the overrides set.
	FilterChanged
	// FilterColours lists colours, of text or of anything else.
	FilterColours
	// FilterSizes lists lengths, numbers and insets.
	FilterSizes
	// FilterMotion lists springs.
	FilterMotion
	// FilterOther lists the rest, such as fonts.
	FilterOther
)

// filterNames are the filters' names, in order.
var filterNames = []string{"All", "Changed", "Colours", "Sizes", "Motion", "Other"}

// String returns the filter's name, such as "Changed".
func (f Filter) String() string {
	if f < 0 || int(f) >= len(filterNames) {
		return "Filter(" + strconv.Itoa(int(f)) + ")"
	}
	return filterNames[f]
}

// kindFilter returns the filter of a token of kind k.
func kindFilter(k theme.Kind) Filter {
	switch k {
	case theme.KindColor, theme.KindForeground:
		return FilterColours
	case theme.KindLength, theme.KindNumber, theme.KindInsets:
		return FilterSizes
	case theme.KindSpring:
		return FilterMotion
	case theme.KindChoice, theme.KindOther:
	}
	return FilterOther
}

// otherGroup is the group the keys of groups of one list under.
const otherGroup = "Other"

// allValues is the All values tab: a search, the filters, and every
// token in groups by the first part of its key, each a heading over a
// card of rows. The groups are a list that builds only those in view;
// Tab still reaches every row, bringing the next group in as it goes.
type allValues struct {
	e       *Editor
	search  *widget.TextField
	filters *widget.Segmented
	list    *widget.VirtualList
	top     *widget.Pad
	// groupOf is each key's group, and order every key in the order
	// listed: by group, the groups by name, Other last.
	groupOf map[string]string
	order   []string
	// query is the search, in lower case, and filter the filter.
	query  string
	filter Filter
	// keys are the list's keys, one a group, and items what each lists.
	keys  []widget.Key
	items map[widget.Key]*groupItem
	// built are the rows of each node the list built for a group, which
	// the list may build again as the group comes back into view.
	built map[gunim.Node][]*row
	// next is the group Tab brings in, whose first row takes the
	// keyboard once built, or its last where back says so, and shown
	// the node that took it, to scroll into view once laid out.
	next  widget.Key
	back  bool
	shown gunim.Node
}

// groupItem is one group as the list holds it.
type groupItem struct {
	title string
	keys  []string
}

func newAllValues(e *Editor) *allValues {
	a := &allValues{e: e, groupOf: map[string]string{}, items: map[widget.Key]*groupItem{}, built: map[gunim.Node][]*row{}}
	sizes := map[string]int{}
	for k := range e.tokens {
		sizes[group(k)]++
	}
	for k := range e.tokens {
		g := Words(group(k))
		if sizes[group(k)] == 1 {
			g = otherGroup
		}
		a.groupOf[k] = g
		a.order = append(a.order, k)
	}
	slices.SortFunc(a.order, func(x, y string) int {
		gx, gy := a.groupOf[x], a.groupOf[y]
		if gx != gy {
			switch {
			case gx == otherGroup:
				return 1
			case gy == otherGroup:
				return -1
			}
			return strings.Compare(gx, gy)
		}
		return strings.Compare(x, y)
	})

	a.search = widget.NewTextField()
	a.search.Placeholder = "Search names and keys"
	a.search.Icon = icon.Search
	a.search.Clearable = true
	a.search.OnChange = func(s string, u *gunim.UI) gunim.Intent {
		a.apply(s, a.filter, u)
		return nil
	}
	a.filters = widget.NewSegmented(filterNames...)
	a.filters.Tooltip, a.filters.Fit = "Show", true
	a.filters.OnChange = func(i int, u *gunim.UI) gunim.Intent {
		a.apply(a.query, Filter(i), u)
		return nil
	}
	tools := widget.Column(a.search, widget.Row(a.filters))
	tools.Cross, tools.Gap = widget.CrossStretch, HeadingGap
	a.top = widget.NewPad(tools)
	a.top.Padding = ToolsPadding
	a.list = widget.NewVirtualList(a.build)
	a.list.Estimate = 240
	a.list.Spacing = SectionGap
	a.apply("", FilterAll, nil)
	return a
}

// group returns the first part of a key: its part before the first dot.
func group(key string) string {
	g, _, _ := strings.Cut(key, ".")
	return g
}

// title returns the name a token shows under in its group: the one a
// section gives it, or its key in words, the group's own word left out.
func (a *allValues) title(key string) string {
	if l, ok := a.e.labels[key]; ok {
		return l
	}
	if a.groupOf[key] == otherGroup {
		return Words(key)
	}
	if _, rest, ok := strings.Cut(key, "."); ok {
		return Words(rest)
	}
	return Words(key)
}

// matches reports whether the token named key holds every word of the
// search, in its key, its name or its group's.
func (a *allValues) matches(key string, words []string) bool {
	hay := strings.ToLower(key + " " + a.e.label(key) + " " + a.title(key) + " " + a.groupOf[key])
	for _, w := range words {
		if !strings.Contains(hay, w) {
			return false
		}
	}
	return true
}

// holds reports whether filter f lets the token named key through.
func (a *allValues) holds(f Filter, key string) bool {
	switch f {
	case FilterAll:
		return true
	case FilterChanged:
		return a.e.over.Has(key)
	case FilterColours, FilterSizes, FilterMotion, FilterOther:
	}
	return kindFilter(a.e.tokens[key].Kind) == f
}

// apply lists the tokens the search q and the filter f let through.
func (a *allValues) apply(q string, f Filter, u *gunim.UI) {
	a.query = strings.ToLower(strings.TrimSpace(q))
	a.filter = f
	a.filters.SetSelected(int(f), u)
	words := strings.Fields(a.query)
	var keys []widget.Key
	var cur *groupItem
	flush := func() {
		if cur == nil {
			return
		}
		h := fnv.New64a()
		for _, k := range cur.keys {
			h.Write([]byte(k + "\x00"))
		}
		key := widget.Key(cur.title + "\x00" + strconv.FormatUint(h.Sum64(), 36))
		if old, ok := a.items[key]; ok {
			cur = old
		}
		a.items[key] = cur
		keys = append(keys, key)
		cur = nil
	}
	for _, k := range a.order {
		if !a.matches(k, words) || !a.holds(f, k) {
			continue
		}
		if cur != nil && cur.title != a.groupOf[k] {
			flush()
		}
		if cur == nil {
			cur = &groupItem{title: a.groupOf[k]}
		}
		cur.keys = append(cur.keys, k)
	}
	flush()
	// Groups no longer listed are forgotten once the list lets them go.
	listed := map[widget.Key]bool{}
	for _, k := range keys {
		listed[k] = true
	}
	for k := range a.items {
		if !listed[k] {
			delete(a.items, k)
		}
	}
	a.keys = keys
	a.list.SetKeys(keys, u)
	a.showCounts(u)
}

// showCounts writes on each filter how many tokens the search lets
// through it.
func (a *allValues) showCounts(u *gunim.UI) {
	if a.filters == nil {
		return
	}
	words := strings.Fields(a.query)
	counts := make([]int, len(filterNames))
	for _, k := range a.order {
		if !a.matches(k, words) {
			continue
		}
		counts[FilterAll]++
		counts[kindFilter(a.e.tokens[k].Kind)]++
		if a.e.over.Has(k) {
			counts[FilterChanged]++
		}
	}
	items := make([]string, len(filterNames))
	for i, n := range filterNames {
		items[i] = n + " " + strconv.Itoa(counts[i])
	}
	a.filters.Items = items
	u.Invalidate()
}

// listedKeys returns the keys listed now, in order.
func (a *allValues) listedKeys() []string {
	var out []string
	for _, k := range a.keys {
		out = append(out, a.items[k].keys...)
	}
	return out
}

// build makes the list's node for group k: its heading over a card of
// its rows. It forgets the rows of nodes the list has let go.
func (a *allValues) build(k widget.Key) gunim.Node {
	for n, rows := range a.built {
		if len(rows) == 0 || !a.live(n) {
			delete(a.built, n)
		}
	}
	g := a.items[k]
	if g == nil {
		return widget.NewSpacer()
	}
	rows := make([]*row, 0, len(g.keys))
	nodes := make([]gunim.Node, 0, len(g.keys))
	for _, key := range g.keys {
		r := a.e.newRow(a.e.tokens[key], Field{Key: key, Label: a.title(key)}, true)
		rows = append(rows, r)
		nodes = append(nodes, r.line)
	}
	n := &page{child: section(g.title, DenseRowGap, nodes...), sides: true}
	a.built[n] = rows
	return n
}

// live reports whether n is a node the list shows now.
func (a *allValues) live(n gunim.Node) bool {
	for _, k := range a.keys {
		if shown, ok := a.list.Row(k); ok && shown == n {
			return true
		}
	}
	return false
}

// groupRows returns the rows of group k as the list shows it now, or
// nil where it has not built it.
func (a *allValues) groupRows(k widget.Key) []*row {
	n, ok := a.list.Row(k)
	if !ok {
		return nil
	}
	return a.built[n]
}

// rowsOf returns every row built now of the token named key.
func (a *allValues) rowsOf(key string) []*row {
	var out []*row
	for _, rows := range a.built {
		for _, r := range rows {
			if r.key == key {
				out = append(out, r)
			}
		}
	}
	return out
}

// row returns the row of the token named key the list shows now, or
// nil.
func (a *allValues) row(key string) *row {
	for _, k := range a.keys {
		for _, r := range a.groupRows(k) {
			if r.key == key {
				return r
			}
		}
	}
	return nil
}

// stops returns the nodes Tab stops at in group k, in order.
func (a *allValues) stops(k widget.Key) []gunim.Node {
	var out []gunim.Node
	for _, r := range a.groupRows(k) {
		out = append(out, r.stops()...)
	}
	return out
}

// Children implements [gunim.Composite].
func (a *allValues) Children() []gunim.Node { return []gunim.Node{a.top, a.list} }

// Handle implements [gunim.Handler]: Tab at the last stop of a group
// goes on to the first of the next, and Shift+Tab at the first back to
// the last of the one before, bringing that group in where the list
// has not built it.
func (a *allValues) Handle(e input.Event, u *gunim.UI) bool {
	k, ok := e.(input.KeyPress)
	if !ok || k.Key != input.KeyTab || k.Mods.Has(input.ModControl) || k.Mods.Has(input.ModAlt) {
		return false
	}
	back := k.Mods.Has(input.ModShift)
	for i, key := range a.keys {
		stops := a.stops(key)
		at := slices.IndexFunc(stops, u.HasFocus)
		if at < 0 {
			continue
		}
		if (!back && at < len(stops)-1) || (back && at > 0) {
			return false
		}
		j := i + 1
		if back {
			j = i - 1
		}
		if j < 0 || j >= len(a.keys) {
			return false
		}
		a.next, a.back = a.keys[j], back
		if _, built := a.list.Row(a.next); !built {
			a.list.ScrollToKey(a.next, u)
		}
		a.focusNext(u)
		return true
	}
	return false
}

// focusNext gives the keyboard to the group Tab brought in, once it is
// built: to its first stop, or its last going back.
func (a *allValues) focusNext(u *gunim.UI) {
	if a.next == "" {
		return
	}
	if _, built := a.list.Row(a.next); !built {
		u.Invalidate()
		return
	}
	stops := a.stops(a.next)
	a.next = ""
	if len(stops) == 0 {
		return
	}
	n := stops[0]
	if a.back {
		n = stops[len(stops)-1]
	}
	u.ShowFocusRing()
	u.Focus(n)
	a.shown = n
	u.Invalidate()
}

// show scrolls the list just far enough to show the node Tab brought
// the keyboard to, with a heading's room round it.
func (a *allValues) show(u *gunim.UI) {
	n := a.shown
	a.shown = nil
	nb, ok1 := u.Bounds(n)
	lb, ok2 := u.Bounds(a.list)
	if !ok1 || !ok2 {
		return
	}
	room := SectionGap.Get(u.Theme())
	switch {
	case nb.Min.Y < lb.Min.Y+room:
		a.list.ScrollTo(a.list.Offset()-(lb.Min.Y+room-nb.Min.Y), widget.Quick.Get(u.Theme()))
	case nb.Max.Y > lb.Max.Y-room:
		a.list.ScrollTo(a.list.Offset()+(nb.Max.Y-lb.Max.Y+room), widget.Quick.Get(u.Theme()))
	}
}

// Layout implements [gunim.Node].
func (a *allValues) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	th := f.Theme
	w, h := c.Max.W, c.Max.H
	pad := PagePadding.Get(th)
	inner := min(max(0, w-pad.Left-pad.Right), PageWidth.Get(th))
	x := (w - inner) / 2
	// The tools keep to the page's width, as the groups under them do.
	ts := kids.At(0).Layout(gunim.Constraints{Min: geom.Sz(inner+pad.Left+pad.Right, 0), Max: geom.Sz(inner+pad.Left+pad.Right, 0)})
	kids.At(0).Place(geom.Pt(x-pad.Left, 0))
	kids.At(1).Layout(gunim.Tight(geom.Sz(w, max(0, h-ts.H))))
	kids.At(1).Place(geom.Pt(0, ts.H))
	if u := f.UI(); u != nil {
		if a.next != "" {
			u.After(0, a.focusNext)
		}
		if a.shown != nil {
			u.After(0, a.show)
		}
	}
	return c.Constrain(geom.Sz(w, h))
}

// Paint implements [gunim.Node].
func (a *allValues) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
	kids.At(1).Paint(p)
}
