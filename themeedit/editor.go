// Package themeedit is an editor for a gunim theme that any program can
// show: in a dialog, on a tab, or anywhere else a node goes.
//
// The editor edits overrides: values laid over a base theme, such as the
// dark or light theme the user started from. Every edit is live: the
// editor hands the program the new theme on each change, and the program
// switches the window to it.
//
//	ed := themeedit.New(themeedit.Options{
//		Base:      widget.Dark(),
//		Overrides: saved,
//		Sections: []themeedit.Section{{
//			Title: "Terminal",
//			Fields: []themeedit.Field{{
//				Key:    widget.Caret.Key(),
//				Label:  "Cursor",
//				Detail: "How the cursor moves along a line.",
//				Presets: []themeedit.Preset{
//					{Label: "Glides", Value: widget.Caret.Default()},
//					{Label: "Jumps", Value: themeedit.Instant},
//				},
//			}},
//		}},
//		OnChange: func(th theme.Theme, u *gunim.UI) gunim.Intent {
//			u.UseTheme(th)
//			return nil
//		},
//		OnCommit: func(over theme.Theme, u *gunim.UI) gunim.Intent {
//			return SaveTheme{Over: over}
//		},
//	})
//
// It has two tabs. Chosen shows the values the program picked out, in
// sections, each with a label and a sentence saying what it does. All
// values lists every token declared, grouped by the first part of its
// key, with a field to search them.
package themeedit

import (
	"reflect"
	"slices"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// Instant is a spring that moves at once: the value is where it is going
// on the next frame, with nothing in between. A field of a motion, such
// as a cursor's, offers it as "Jumps".
var Instant = anim.Spring{Response: 0, Damping: 1}

// The presets every spring offers in All values.
var springPresets = []Preset{
	{Label: "Instant", Value: Instant},
	{Label: "Snappy", Value: anim.Snappy},
	{Label: "Gentle", Value: anim.Gentle},
	{Label: "Bouncy", Value: anim.Bouncy},
}

// Options sets up an editor.
type Options struct {
	// Base is the theme the edits are laid over. A token the overrides
	// leave out has Base's value, or its default where Base leaves it
	// out too.
	Base theme.Theme
	// Overrides are the values the user has changed, as the program
	// saved them: [theme.UnmarshalValues] reads them from a file.
	Overrides theme.Theme
	// Sections are the values the program picks out for the Chosen tab.
	// With none, the editor shows All values alone.
	Sections []Section
	// Choices offers values to pick from for tokens the editor has no
	// control of its own for, such as a font, by key. A token without
	// choices shows its value as text.
	Choices map[string][]Preset
	// OnChange runs on the UI goroutine with the theme as edited, Base
	// with the overrides over it, on every change: every step of a drag
	// too. It usually switches the window to it with [gunim.UI.UseTheme].
	// A non-nil result is sent to the application as the editor's
	// intent.
	OnChange func(th theme.Theme, u *gunim.UI) gunim.Intent
	// OnCommit runs, in the same way, with the overrides alone, as each
	// edit ends: a drag let go, a value typed, a preset picked, a value
	// reset. It is the time to save them.
	OnCommit func(over theme.Theme, u *gunim.UI) gunim.Intent
	// OnExport, when set, adds an Export button, which runs it with the
	// overrides written as JSON, for the program to save where the user
	// says.
	OnExport func(data []byte, u *gunim.UI) gunim.Intent
	// OnImport, when set, adds an Import button, which runs it. The
	// program reads a file the user picks and hands it to
	// [Editor.Import].
	OnImport func(u *gunim.UI) gunim.Intent
}

// A Section is a group of fields on the Chosen tab, under a title.
type Section struct {
	Title  string
	Fields []Field
}

// A Field is one value on the Chosen tab.
type Field struct {
	// Key is the token's key, such as widget.Caret.Key().
	Key string
	// Label names the value, such as "Cursor", and Detail says in one
	// plain sentence what it changes.
	Label  string
	Detail string
	// Presets, when set, are the values to pick from, shown as a
	// segmented control in place of the token's own control. A value
	// none of them has shows as Custom.
	Presets []Preset
	// Min and Max, when Max is above Min, bound a length or a number, and
	// give it a slider over that range.
	Min, Max float32
}

// A Preset is a value with a name, such as "Jumps" for an instant
// spring.
type Preset struct {
	Label string
	Value any
}

// Editor edits a theme as overrides on a base theme. Make one with
// [New] and mount it as any node; it fills the room it is given, or
// [Width] by [Height] where it is given none.
//
// Every row shows a token's value with a control for its kind: a swatch
// that opens a colour picker, a number and a slider, four numbers for
// insets, or two sliders, presets and a moving dot for a spring. A row
// whose value is overridden says so, and its reset button takes it back
// to the base theme. Reset all takes every value back.
//
// Everything is reached by Tab, and every control is named for a screen
// reader.
type Editor struct {
	// OnChange, OnCommit, OnExport and OnImport are as in [Options].
	OnChange func(th theme.Theme, u *gunim.UI) gunim.Intent
	OnCommit func(over theme.Theme, u *gunim.UI) gunim.Intent
	OnExport func(data []byte, u *gunim.UI) gunim.Intent
	OnImport func(u *gunim.UI) gunim.Intent

	base, over theme.Theme
	sections   []Section
	choices    map[string][]Preset
	// tokens are every token declared, by key, and labels the labels the
	// sections give some of them.
	tokens map[string]theme.Info
	labels map[string]string

	root     gunim.Node
	tabs     *widget.Tabs
	search   *widget.TextField
	list     *widget.VirtualList
	resetAll *widget.Button
	// chosen are the rows of the Chosen tab, by key, and all the rows of
	// All values built now, by key.
	chosen map[string][]*row
	all    map[string]*row
	// query is the search, in lower case, and keys the list's keys for
	// it, headings among them.
	query string
	keys  []widget.Key
}

// headingKey starts the keys of the headings in All values, which no
// token key starts with.
const headingKey = "\x00"

// New returns an editor set up by o.
func New(o Options) *Editor {
	e := &Editor{
		OnChange: o.OnChange, OnCommit: o.OnCommit, OnExport: o.OnExport, OnImport: o.OnImport,
		base: o.Base, over: o.Overrides, sections: o.Sections, choices: o.Choices,
		tokens: map[string]theme.Info{}, labels: map[string]string{},
		chosen: map[string][]*row{}, all: map[string]*row{},
	}
	for _, info := range theme.Tokens() {
		e.tokens[info.Key] = info
	}
	for _, s := range o.Sections {
		for _, f := range s.Fields {
			if _, ok := e.labels[f.Key]; !ok && f.Label != "" {
				e.labels[f.Key] = f.Label
			}
		}
	}
	e.build()
	return e
}

// build makes the editor's nodes.
func (e *Editor) build() {
	e.resetAll = widget.NewButton("Reset all")
	e.resetAll.Tooltip = "Takes every value back to the base theme"
	e.resetAll.OnClick = func(u *gunim.UI) gunim.Intent {
		e.ResetAll(u)
		return nil
	}
	e.resetAll.Disabled = e.over.Len() == 0
	bar := []gunim.Node{widget.NewSpacer()}
	if e.OnImport != nil {
		b := widget.NewButton("Import…")
		b.Tooltip = "Reads values from a file"
		b.OnClick = func(u *gunim.UI) gunim.Intent { return e.OnImport(u) }
		bar = append(bar, b)
	}
	if e.OnExport != nil {
		b := widget.NewButton("Export…")
		b.Tooltip = "Writes the changed values to a file"
		b.OnClick = func(u *gunim.UI) gunim.Intent {
			data, err := theme.MarshalValues(e.over)
			if err != nil {
				return nil
			}
			return e.OnExport(data, u)
		}
		bar = append(bar, b)
	}
	bar = append(bar, e.resetAll)
	tools := widget.Row(bar...).Grow(bar[0], 1)
	tools.Cross = widget.CrossCenter

	e.search = widget.NewTextField()
	e.search.Placeholder = "Search keys and names"
	e.search.Icon = icon.Search
	e.search.Clearable = true
	e.search.OnChange = func(s string, u *gunim.UI) gunim.Intent {
		e.filter(s, u)
		return nil
	}
	e.list = widget.NewVirtualList(e.buildListRow)
	e.list.Estimate = 44
	e.list.Spacing = RowGap
	e.filter("", nil)
	all := widget.Column(e.search, e.list).Grow(e.list, 1)
	all.Cross = widget.CrossStretch

	if len(e.sections) == 0 {
		e.tabs = widget.NewTabs([]string{"All values"}, all)
	} else {
		e.tabs = widget.NewTabs([]string{"Chosen", "All values"}, widget.NewScroll(e.buildChosen()), all)
	}
	col := widget.Column(tools, e.tabs).Grow(e.tabs, 1)
	col.Cross = widget.CrossStretch
	e.root = col
}

// buildChosen makes the Chosen tab's sections.
func (e *Editor) buildChosen() gunim.Node {
	blocks := make([]gunim.Node, 0, len(e.sections))
	for _, s := range e.sections {
		kids := make([]gunim.Node, 0, len(s.Fields)+1)
		if s.Title != "" {
			kids = append(kids, heading(s.Title))
		}
		for _, f := range s.Fields {
			info, ok := e.tokens[f.Key]
			if !ok {
				continue
			}
			r := e.newRow(info, f, false)
			e.chosen[f.Key] = append(e.chosen[f.Key], r)
			kids = append(kids, r.node)
		}
		block := widget.Column(kids...)
		block.Cross, block.Gap = widget.CrossStretch, RowGap
		blocks = append(blocks, block)
	}
	col := widget.Column(blocks...)
	col.Cross, col.Gap = widget.CrossStretch, SectionGap
	pad := widget.NewPad(col)
	pad.Padding = widget.CardPadding
	return pad
}

// heading returns a section's or a group's title.
func heading(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Size, l.Face = TitleSize, widget.BoldFont
	return l
}

// buildListRow makes All values' row for key k: a group's heading, or a
// token's row.
func (e *Editor) buildListRow(k widget.Key) gunim.Node {
	key := string(k)
	if group, ok := strings.CutPrefix(key, headingKey); ok {
		return heading(group)
	}
	info := e.tokens[key]
	r := e.newRow(info, Field{Key: key, Label: e.label(key)}, true)
	e.all[key] = r
	return r.node
}

// label returns the name a token shows under: the one a section gives
// it, or its key in words.
func (e *Editor) label(key string) string {
	if l, ok := e.labels[key]; ok {
		return l
	}
	return Words(key)
}

// Words writes a key as words, such as "Button primary hover" for
// "button.primary.hover".
func Words(key string) string {
	s := strings.NewReplacer(".", " ", "-", " ", "_", " ").Replace(key)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// group returns the group a key lists under: its part before the first
// dot.
func group(key string) string {
	g, _, _ := strings.Cut(key, ".")
	return g
}

// filter shows in All values the tokens whose key or name holds every
// word of q.
func (e *Editor) filter(q string, u *gunim.UI) {
	e.query = strings.ToLower(strings.TrimSpace(q))
	words := strings.Fields(e.query)
	keys := make([]string, 0, len(e.tokens))
	for k := range e.tokens {
		hay := strings.ToLower(k + " " + e.label(k))
		if !slices.ContainsFunc(words, func(w string) bool { return !strings.Contains(hay, w) }) {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	out := make([]widget.Key, 0, len(keys)+16)
	last := ""
	for i, k := range keys {
		if g := group(k); i == 0 || g != last {
			out = append(out, widget.Key(headingKey+g))
			last = g
		}
		out = append(out, widget.Key(k))
	}
	e.keys = out
	e.list.SetKeys(out, u)
}

// Listed returns the keys All values lists now, under the search, in
// order, headings left out.
func (e *Editor) Listed() []string {
	out := make([]string, 0, len(e.keys))
	for _, k := range e.keys {
		if !strings.HasPrefix(string(k), headingKey) {
			out = append(out, string(k))
		}
	}
	return out
}

// Chosen returns the keys of the Chosen tab's fields, in order.
func (e *Editor) Chosen() []string {
	var out []string
	for _, s := range e.sections {
		for _, f := range s.Fields {
			if _, ok := e.tokens[f.Key]; ok {
				out = append(out, f.Key)
			}
		}
	}
	return out
}

// Search sets the search of All values, as if typed, and sends no
// intent.
func (e *Editor) Search(q string, u *gunim.UI) {
	e.search.SetText(q, u)
	e.filter(q, u)
}

// Theme returns the theme as edited: the base with the overrides over
// it.
func (e *Editor) Theme() theme.Theme { return e.over.Over(e.base) }

// Base returns the theme the edits are laid over.
func (e *Editor) Base() theme.Theme { return e.base }

// Overrides returns the values the user has changed.
func (e *Editor) Overrides() theme.Theme { return e.over }

// SetBase lays the edits over th from now on, as when the user switches
// from the dark theme to the light, and sends no intent. Rows the
// overrides leave out show th's values.
func (e *Editor) SetBase(th theme.Theme, u *gunim.UI) {
	e.base = th
	e.refreshAll(u)
}

// SetOverrides makes over the values the user has changed, and sends no
// intent.
func (e *Editor) SetOverrides(over theme.Theme, u *gunim.UI) {
	e.over = over
	e.refreshAll(u)
}

// Export returns the overrides written as JSON, as [theme.MarshalValues]
// writes them.
func (e *Editor) Export() ([]byte, error) { return theme.MarshalValues(e.over) }

// Import makes the values in data, written as [Editor.Export] writes
// them, the overrides, as the user asked by Import. It runs OnChange and
// OnCommit, as an edit does. Values it cannot read are left out, and the
// error lists them; the rest are used.
func (e *Editor) Import(data []byte, u *gunim.UI) error {
	over, err := theme.UnmarshalValues(theme.Make(e.over.Name), data)
	if over.Len() == 0 && err != nil {
		return err
	}
	e.over = over
	e.refreshAll(u)
	e.changed(true, u)
	return err
}

// ResetAll takes every value back to the base theme, as the Reset all
// button does, running OnChange and OnCommit.
func (e *Editor) ResetAll(u *gunim.UI) {
	if e.over.Len() == 0 {
		return
	}
	e.over = e.over.Without(e.over.Keys()...)
	e.refreshAll(u)
	e.changed(true, u)
}

// Reset takes the token named key back to the base theme, as its row's
// reset button does, running OnChange and OnCommit.
func (e *Editor) Reset(key string, u *gunim.UI) {
	if !e.over.Has(key) {
		return
	}
	e.over = e.over.Without(key)
	e.refresh(key, nil, u)
	e.changed(true, u)
}

// Set gives the token named key the value v, as an edit in its row
// does, running OnChange, and OnCommit when commit is set. A value that
// does not suit the token gives an error and changes nothing.
func (e *Editor) Set(key string, v any, commit bool, u *gunim.UI) error {
	return e.edit(key, v, commit, nil, u)
}

// edit sets key to v from the control src, which already shows it.
func (e *Editor) edit(key string, v any, commit bool, src control, u *gunim.UI) error {
	over, err := e.over.WithValue(key, v)
	if err != nil {
		return err
	}
	e.over = over
	e.refresh(key, src, u)
	e.changed(commit, u)
	return nil
}

// changed tells the program of an edit.
func (e *Editor) changed(commit bool, u *gunim.UI) {
	if e.OnChange != nil {
		send(u, e, e.OnChange(e.Theme(), u))
	}
	if commit && e.OnCommit != nil {
		send(u, e, e.OnCommit(e.over, u))
	}
}

// send sends in from n, when there is one.
func send(u *gunim.UI, n gunim.Node, in gunim.Intent) {
	if in != nil && u != nil {
		u.Send(n, in)
	}
}

// value returns the value the token named key has as edited.
func (e *Editor) value(key string) any {
	if v, ok := e.over.Value(key); ok {
		return v
	}
	if v, ok := e.base.Value(key); ok {
		return v
	}
	return e.tokens[key].Default
}

// refresh shows key's value in every row of it but the one of src.
func (e *Editor) refresh(key string, src control, u *gunim.UI) {
	v, on := e.value(key), e.over.Has(key)
	rows := e.chosen[key]
	if r, ok := e.all[key]; ok {
		rows = append(slices.Clip(rows), r)
	}
	for _, r := range rows {
		if r.ctl != src {
			r.ctl.show(v, u)
		}
		r.mark(on, u)
	}
	e.resetAll.Disabled = e.over.Len() == 0
	u.Invalidate()
}

// refreshAll shows every row's value.
func (e *Editor) refreshAll(u *gunim.UI) {
	for k := range e.chosen {
		e.refresh(k, nil, u)
	}
	for k := range e.all {
		e.refresh(k, nil, u)
	}
	e.resetAll.Disabled = e.over.Len() == 0
}

// Children implements [gunim.Composite].
func (e *Editor) Children() []gunim.Node { return []gunim.Node{e.root} }

// Layout implements [gunim.Node]. The editor fills the room it is given,
// or [Width] by [Height] where that is unbounded.
func (e *Editor) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	w, h := c.Max.W, c.Max.H
	if w <= 0 {
		w = Width.Get(f.Theme)
	}
	if h <= 0 {
		h = Height.Get(f.Theme)
	}
	size := c.Constrain(geom.Sz(w, h))
	kids.At(0).Layout(gunim.Tight(size))
	kids.At(0).Place(geom.Point{})
	return size
}

// Paint implements [gunim.Node].
func (e *Editor) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}

// same reports whether two values are equal.
func same(a, b any) bool { return reflect.DeepEqual(a, b) }
