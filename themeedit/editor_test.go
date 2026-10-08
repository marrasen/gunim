package themeedit

import (
	"fmt"
	"image/color"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// probe is a patch that hands a test the window's UI.
type probe struct{}

// heard is what the editor told the program.
type heard struct {
	changes []theme.Theme
	commits []theme.Theme
}

// last returns the theme the last change sent.
func (h *heard) last(t *testing.T) theme.Theme {
	t.Helper()
	if len(h.changes) == 0 {
		t.Fatal("OnChange never ran")
	}
	return h.changes[len(h.changes)-1]
}

// cursorSection is the Chosen tab of a terminal: its cursor, its accent
// and the gap between its panes.
var cursorSection = Section{Title: "Terminal", Fields: []Field{
	{Key: widget.Caret.Key(), Label: "Cursor", Detail: "How the cursor moves along a line.",
		Presets: []Preset{{Label: "Glides", Value: widget.Caret.Default()}, {Label: "Jumps", Value: Instant}}},
	{Key: widget.Accent.Key(), Label: "Accent", Detail: "The colour of focus and selection."},
	{Key: widget.Gap.Key(), Label: "Gap", Detail: "The room between things in a row."},
}}

// edStage mounts an editor of o and returns it, the window, its UI, a
// way to run frames, and what the editor tells the program.
func edStage(t *testing.T, o Options) (*Editor, *gunim.Window, *gunim.UI, func(int), *heard) {
	t.Helper()
	h := &heard{}
	o.OnChange = func(th theme.Theme, _ *gunim.UI) gunim.Intent { h.changes = append(h.changes, th); return nil }
	o.OnCommit = func(over theme.Theme, _ *gunim.UI) gunim.Intent { h.commits = append(h.commits, over); return nil }
	e := New(o)
	w := gunimtest.New(t, geom.Sz(800, 600), nil)
	gunim.RegisterView(w, "ed", func(struct{}) gunim.Node { return e }, nil)
	if err := w.Client().Mount(gunim.Root, "ed", "ed", nil); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	var u *gunim.UI
	gunim.RegisterPatch(w, "ed", func(_ gunim.Node, _ probe, ui *gunim.UI) { u = ui })
	if err := w.Client().Patch("ed", probe{}); err != nil {
		t.Fatal(err)
	}
	run(2)
	if u == nil {
		t.Fatal("no UI")
	}
	return e, w, u, run, h
}

// key presses k with mods, lets it go, and runs a frame.
func key(w *gunim.Window, run func(int), k input.Key, mods input.Mods) {
	w.Input(input.KeyPress{Key: k, Mods: mods})
	w.Input(input.KeyRelease{Key: k, Mods: mods})
	run(1)
}

// valueIn returns key's value in th, read as a window would.
func valueIn[T any](th theme.Theme, tok theme.Token[T]) T { return tok.Get(theme.NewLive(th)) }

func TestTheEditorListsTheChosenFieldsAndEveryToken(t *testing.T) {
	e, _, u, run, _ := edStage(t, Options{Base: widget.Dark(), Sections: []Section{cursorSection}})
	if got, want := e.Chosen(), []string{"motion.caret", "accent", "layout.gap"}; !slices.Equal(got, want) {
		t.Fatalf("Chosen lists %v, want %v", got, want)
	}
	for _, k := range e.Chosen() {
		if len(e.chosen[k]) != 1 {
			t.Fatalf("%q has %d rows on the Chosen tab", k, len(e.chosen[k]))
		}
	}
	tokens := theme.Tokens()
	listed := e.Listed()
	if len(listed) != len(tokens) || len(listed) < 200 {
		t.Fatalf("All values lists %d keys, want all %d tokens", len(listed), len(tokens))
	}
	for i, info := range tokens {
		if listed[i] != info.Key {
			t.Fatalf("All values lists %q at %d, want %q", listed[i], i, info.Key)
		}
	}
	// Each group has a heading before its first key.
	headings := 0
	for _, k := range e.keys {
		if strings.HasPrefix(string(k), headingKey) {
			headings++
		}
	}
	if headings < 10 {
		t.Fatalf("All values has %d headings", headings)
	}
	e.tabs.SetSelected(1, u)
	run(5)
	if len(e.all) == 0 || len(e.all) > 60 {
		t.Fatalf("All values built %d rows, want those in view alone", len(e.all))
	}
}

func TestSearchFiltersByKeyAndName(t *testing.T) {
	e, _, u, run, _ := edStage(t, Options{Base: widget.Dark(), Sections: []Section{cursorSection}})
	e.Search("caret", u)
	run(2)
	got := e.Listed()
	if !slices.Contains(got, "motion.caret") {
		t.Fatalf("a search for caret lists %v", got)
	}
	for _, k := range got {
		if !strings.Contains(strings.ToLower(k+" "+e.label(k)), "caret") {
			t.Fatalf("a search for caret lists %q", k)
		}
	}
	// A section's label finds its token, and every word must match.
	e.Search("CURSOR motion", u)
	if got := e.Listed(); !slices.Equal(got, []string{"motion.caret"}) {
		t.Fatalf("a search for the label lists %v", got)
	}
	e.Search("no such token anywhere", u)
	if got := e.Listed(); len(got) != 0 {
		t.Fatalf("a search for nothing lists %v", got)
	}
	e.Search("", u)
	if len(e.Listed()) != len(theme.Tokens()) {
		t.Fatal("an empty search does not list every token")
	}
}

func TestPickingAPresetSetsTheSpring(t *testing.T) {
	e, w, u, run, h := edStage(t, Options{Base: widget.Dark(), Sections: []Section{cursorSection}})
	ctl, ok := e.chosen["motion.caret"][0].ctl.(*presetControl)
	if !ok {
		t.Fatalf("the cursor's control is a %T", e.chosen["motion.caret"][0].ctl)
	}
	if ctl.seg.Selected() != 0 {
		t.Fatalf("the cursor shows preset %d, want Glides", ctl.seg.Selected())
	}
	u.Focus(ctl.seg)
	key(w, run, input.KeyRight, 0)
	if got := valueIn(h.last(t), widget.Caret); got != Instant {
		t.Fatalf("Jumps gives the theme a caret of %+v, want instant", got)
	}
	if len(h.commits) != 1 || !h.commits[0].Has("motion.caret") {
		t.Fatalf("OnCommit heard %v", h.commits)
	}
	if !e.Overrides().Has("motion.caret") || e.chosen["motion.caret"][0].reset.Disabled {
		t.Fatal("the cursor does not show as overridden")
	}
	// A value none of the presets has shows as Custom.
	if err := e.Set("motion.caret", anim.Spring{Response: 0.3, Damping: 0.5}, true, u); err != nil {
		t.Fatal(err)
	}
	if !ctl.custom || ctl.seg.Selected() != 2 || ctl.seg.Items[2] != "Custom" {
		t.Fatalf("a spring of its own shows %v, item %d", ctl.seg.Items, ctl.seg.Selected())
	}
}

func TestEditingAColourANumberAndASpringChangesTheTheme(t *testing.T) {
	e, w, u, run, h := edStage(t, Options{Base: widget.Dark(), Sections: []Section{cursorSection}})

	// A colour, through its button's picker.
	colour, ok := e.chosen["accent"][0].ctl.(*colorControl)
	if !ok {
		t.Fatalf("the accent's control is a %T", e.chosen["accent"][0].ctl)
	}
	u.Focus(colour.button)
	key(w, run, input.KeyEnter, 0)
	run(5)
	if !colour.button.IsOpen() {
		t.Fatal("Enter opened no picker")
	}
	key(w, run, input.KeyLeft, input.ModShift)
	got := valueIn(h.last(t), widget.Accent)
	if got == widget.Accent.Default() || got != colour.button.Value() {
		t.Fatalf("an edit in the picker gives the theme an accent of %v; the button shows %v", got, colour.button.Value())
	}
	if colour.hex.Text != theme.Hex(got) {
		t.Fatalf("the row says %q for %v", colour.hex.Text, got)
	}
	key(w, run, input.KeyEscape, 0)
	run(30)

	// A number, by its field's arrow key.
	num, ok := e.chosen["layout.gap"][0].ctl.(*numberControl)
	if !ok || num.slider == nil {
		t.Fatalf("the gap's control is a %T with no slider", e.chosen["layout.gap"][0].ctl)
	}
	u.Focus(num.field)
	key(w, run, input.KeyUp, 0)
	if got := valueIn(h.last(t), widget.Gap); got != widget.Gap.Default()+1 {
		t.Fatalf("Up in the gap's field gives the theme a gap of %v", got)
	}
	if num.slider.Value() != widget.Gap.Default()+1 {
		t.Fatalf("the slider shows %v", num.slider.Value())
	}

	// A spring, by the damping slider of its row in All values.
	e.tabs.SetSelected(1, u)
	e.Search("motion.quick", u)
	run(5)
	r, ok := e.all["motion.quick"]
	if !ok {
		t.Fatal("All values built no row for motion.quick")
	}
	spring, ok := r.ctl.(*springControl)
	if !ok {
		t.Fatalf("motion.quick's control is a %T", r.ctl)
	}
	u.Focus(spring.damping)
	key(w, run, input.KeyRight, 0)
	want := anim.Spring{Response: anim.Snappy.Response, Damping: anim.Snappy.Damping + 0.01}
	if got := valueIn(h.last(t), widget.Quick); abs(got.Damping-want.Damping) > 1e-4 || got.Response != want.Response {
		t.Fatalf("Right on the damping gives the theme %+v, want %+v", got, want)
	}
	if spring.preview.left == 0 {
		t.Fatal("the preview does not play the new spring")
	}
	// The presets show it as one of their own no longer.
	if !spring.presets.custom {
		t.Fatal("the spring's presets do not show Custom")
	}
	if got := e.Overrides().Keys(); !slices.Equal(got, []string{"accent", "layout.gap", "motion.quick"}) {
		t.Fatalf("the overrides are %v", got)
	}
}

func TestResetTakesAValueBackToTheBase(t *testing.T) {
	base := widget.Light()
	over := theme.Make("mine", theme.Set(widget.Gap, 20), theme.Set(widget.Accent, color.NRGBA{R: 0xff, A: 0xff}))
	e, w, u, run, h := edStage(t, Options{Base: base, Overrides: over, Sections: []Section{cursorSection}})
	r := e.chosen["layout.gap"][0]
	if r.reset.Disabled || r.changed.Text == "" {
		t.Fatal("an overridden row does not say so")
	}
	if num := ctlOf[*numberControl](t, r); num.field.Value() != 20 {
		t.Fatalf("the gap shows %v, want the override's 20", num.field.Value())
	}
	u.Focus(r.reset)
	key(w, run, input.KeyEnter, 0)
	if e.Overrides().Has("layout.gap") {
		t.Fatal("Reset left the override")
	}
	if got := valueIn(h.last(t), widget.Gap); got != valueIn(base, widget.Gap) {
		t.Fatalf("after Reset the theme's gap is %v, want the base's %v", got, valueIn(base, widget.Gap))
	}
	if len(h.commits) != 1 || h.commits[0].Has("layout.gap") || !h.commits[0].Has("accent") {
		t.Fatalf("OnCommit heard %v", h.commits)
	}
	if !r.reset.Disabled || r.changed.Text != "" {
		t.Fatal("a row back at the base still shows as overridden")
	}
	if num := ctlOf[*numberControl](t, r); num.field.Value() != float64(valueIn(base, widget.Gap)) {
		t.Fatalf("the gap shows %v after Reset", num.field.Value())
	}

	// Reset all takes the rest.
	u.Focus(e.resetAll)
	key(w, run, input.KeyEnter, 0)
	if e.Overrides().Len() != 0 || !e.resetAll.Disabled {
		t.Fatalf("Reset all leaves %v", e.Overrides().Keys())
	}
	if got := valueIn(h.last(t), widget.Accent); got != valueIn(base, widget.Accent) {
		t.Fatalf("after Reset all the accent is %v", got)
	}

	// A new base shows in the rows it does not override.
	e.SetBase(widget.Dark(), u)
	if c := ctlOf[*colorControl](t, e.chosen["accent"][0]); c.button.Value() != widget.Accent.Default() {
		t.Fatalf("after the base changed the accent shows %v", c.button.Value())
	}
}

func TestOverridesGoToJSONAndBack(t *testing.T) {
	e, _, u, _, h := edStage(t, Options{Base: widget.Dark(), Sections: []Section{cursorSection}})
	_ = e.Set("motion.caret", Instant, true, u)
	_ = e.Set("accent", color.NRGBA{R: 1, G: 2, B: 3, A: 0x80}, true, u)
	_ = e.Set("card.padding", geom.Insets{Top: 1, Right: 2, Bottom: 3, Left: 4}, true, u)
	data, err := e.Export()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"accent": "#01020380"`) {
		t.Fatalf("the export reads\n%s", data)
	}

	other, _, u2, _, h2 := edStage(t, Options{Base: widget.Dark()})
	if err = other.Import(data, u2); err != nil {
		t.Fatal(err)
	}
	for _, k := range e.Overrides().Keys() {
		a, _ := e.Overrides().Value(k)
		b, ok := other.Overrides().Value(k)
		if !ok || !same(a, b) {
			t.Fatalf("%q came back as %v, want %v", k, b, a)
		}
	}
	if len(h2.changes) != 1 || len(h2.commits) != 1 {
		t.Fatal("Import does not tell the program")
	}

	// A file in between.
	path := filepath.Join(t.TempDir(), "theme.json")
	if err = WriteFile(path, e.Overrides()); err != nil {
		t.Fatal(err)
	}
	back, err := ReadFile(path)
	if err != nil || back.Len() != 3 {
		t.Fatalf("ReadFile gives %v, %v", back.Keys(), err)
	}

	// Values it cannot read are left out; the rest come in.
	err = other.Import([]byte(`{"accent": "#ff0000", "no.such.key": 1}`), u2)
	if err == nil || !strings.Contains(err.Error(), "no.such.key") {
		t.Fatalf("a bad key gives %v", err)
	}
	if got := valueIn(other.Theme(), widget.Accent); got != (color.NRGBA{R: 0xff, A: 0xff}) {
		t.Fatalf("the good value came in as %v", got)
	}
	_ = h
}

func TestEveryKindOfRowBuilds(t *testing.T) {
	e, _, u, run, _ := edStage(t, Options{Base: widget.Dark(), Choices: map[string][]Preset{
		widget.Font.Key(): {{Label: "Sans", Value: widget.Font.Default()}},
	}})
	for _, k := range []string{"card.padding", "text.font", "text.font.mono", "button.fill", "ink", "motion.bounce", "text.size"} {
		e.Search(k, u)
		run(3)
		r, ok := e.all[k]
		if !ok {
			t.Fatalf("no row for %q", k)
		}
		var want string
		switch k {
		case "card.padding":
			want = "*themeedit.insetsControl"
		case "text.font":
			want = "*themeedit.choiceControl"
		case "text.font.mono":
			want = "*themeedit.textControl"
		case "button.fill", "ink":
			want = "*themeedit.colorControl"
		case "motion.bounce":
			want = "*themeedit.springControl"
		default:
			want = "*themeedit.numberControl"
		}
		if got := typeName(r.ctl); got != want {
			t.Fatalf("%q has a %s, want a %s", k, got, want)
		}
	}
	if d := ctlOf[*choiceControl](t, e.all["text.font"]).drop; d.Selected() != 0 {
		t.Fatalf("the font's drop-down shows %d", d.Selected())
	}
	if l := ctlOf[*textControl](t, e.all["text.font.mono"]).label.Text; l == "" {
		t.Fatal("a font with no choices shows nothing")
	}
}

// ctlOf returns r's control as a T, failing t where it is another.
func ctlOf[T control](t *testing.T, r *row) T {
	t.Helper()
	c, ok := r.ctl.(T)
	if !ok {
		t.Fatalf("%q has a %T", r.key, r.ctl)
	}
	return c
}

func typeName(v any) string { return fmt.Sprintf("%T", v) }

func abs(v float32) float32 { return max(v, -v) }
