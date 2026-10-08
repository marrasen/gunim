package themeedit

import (
	"fmt"
	"image/color"
	"math"
	"reflect"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// A control edits one value in a row.
type control interface {
	// node returns what the row shows.
	node() gunim.Node
	// show shows v, telling no one.
	show(v any, u *gunim.UI)
}

// row is one token's row: its name, its control, whether it is
// overridden, and a button that resets it.
type row struct {
	key     string
	ctl     control
	changed *widget.Label
	reset   *widget.IconButton
	node    gunim.Node
}

// newRow returns a row for the token info describes, as field sets it
// out. A row of All values shows the key; one of Chosen shows the
// field's label and sentence.
func (e *Editor) newRow(info theme.Info, f Field, all bool) *row {
	r := &row{key: info.Key}
	label := f.Label
	if label == "" {
		label = Words(info.Key)
	}
	edit := func(v any, commit bool, src control, u *gunim.UI) { _ = e.edit(info.Key, v, commit, src, u) }
	r.ctl = newControl(info, f, label, e.choices[info.Key], all, edit)
	r.ctl.show(e.value(info.Key), nil)

	var title, detail *widget.Label
	if all {
		title = widget.NewLabel(info.Key)
		title.Face, title.Size, title.Selectable = widget.MonoFont, KeySize, true
		detail = widget.NewLabel(label + " · " + info.Kind.String())
	} else {
		title = widget.NewLabel(label)
		detail = widget.NewLabel(f.Detail)
	}
	detail.Size, detail.Color = DetailSize, widget.Placeholder
	r.changed = widget.NewLabel("")
	r.changed.Size, r.changed.Color = DetailSize, widget.Accent
	head := widget.Row(title, r.changed)
	head.Cross = widget.CrossCenter
	names := widget.Column(head, detail)
	names.Gap = NamesGap

	r.reset = widget.NewIconButton(icon.RotateCcw, "Reset "+label+" to the base theme")
	r.reset.OnClick = func(u *gunim.UI) gunim.Intent {
		e.Reset(info.Key, u)
		return nil
	}
	r.mark(e.over.Has(info.Key), nil)
	line := widget.Row(names, r.ctl.node(), r.reset).Grow(names, 1)
	line.Cross = widget.CrossCenter
	r.node = line
	return r
}

// mark shows whether the row's value is overridden.
func (r *row) mark(on bool, u *gunim.UI) {
	r.reset.Disabled = !on
	r.changed.Text = ""
	if on {
		r.changed.Text = "changed"
	}
	u.Invalidate()
}

// editFunc sets a row's value from the control src, which already
// shows it.
type editFunc func(v any, commit bool, src control, u *gunim.UI)

// newControl returns the control for a value of info's kind, or one of
// presets where there are presets.
func newControl(info theme.Info, f Field, label string, choices []Preset, all bool, edit editFunc) control {
	if len(f.Presets) > 0 && !all {
		return newPresets(f.Presets, label, edit)
	}
	switch info.Kind {
	case theme.KindColor, theme.KindForeground:
		return newColorControl(label, edit)
	case theme.KindLength, theme.KindNumber:
		return newNumberControl(info, f, label, edit)
	case theme.KindInsets:
		return newInsetsControl(label, edit)
	case theme.KindSpring:
		return newSpringControl(label, edit)
	case theme.KindChoice, theme.KindOther:
	}
	if len(choices) > 0 {
		return newChoiceControl(choices, label, edit)
	}
	return newTextControl()
}

// colorControl is a swatch that opens a colour picker, and the colour
// in hex beside it.
type colorControl struct {
	button *widget.ColorButton
	hex    *widget.Label
	row    *widget.Flex
}

func newColorControl(label string, edit editFunc) *colorControl {
	c := &colorControl{button: widget.NewColorButton(color.NRGBA{}), hex: widget.NewLabel("")}
	c.button.Label = label
	c.hex.Face, c.hex.Size, c.hex.Color = widget.MonoFont, KeySize, widget.Placeholder
	c.button.OnChange = func(v color.NRGBA, u *gunim.UI) gunim.Intent {
		c.hex.Text = theme.Hex(v)
		edit(v, false, c, u)
		return nil
	}
	c.button.OnCommit = func(v color.NRGBA, u *gunim.UI) gunim.Intent {
		c.hex.Text = theme.Hex(v)
		edit(v, true, c, u)
		return nil
	}
	c.row = widget.Row(c.hex, c.button)
	c.row.Cross = widget.CrossCenter
	return c
}

func (c *colorControl) node() gunim.Node { return c.row }

func (c *colorControl) show(v any, u *gunim.UI) {
	if col, ok := v.(color.NRGBA); ok {
		c.button.SetValue(col, u)
		c.hex.Text = theme.Hex(col)
	}
}

// numberControl is a field for a length or a number, with a slider
// where the value has a sensible range.
type numberControl struct {
	field  *widget.NumberField
	slider *widget.Slider
	row    gunim.Node
}

func newNumberControl(info theme.Info, f Field, label string, edit editFunc) *numberControl {
	def, _ := info.Default.(float32)
	lo, hi, ok := numberRange(info.Kind, def, f)
	c := &numberControl{field: widget.NewNumberField(-1e6, 1e6)}
	if f.Max > f.Min {
		c.field.Min, c.field.Max = float64(f.Min), float64(f.Max)
	}
	// A whole number steps by ones; any other by hundredths.
	c.field.Decimals, c.field.Increment = 2, 0.01
	if def == float32(math.Round(float64(def))) {
		c.field.Decimals, c.field.Increment = 0, 1
	}
	c.field.Tooltip, c.field.Placeholder = label, label
	c.field.OnChange = func(v float64, u *gunim.UI) gunim.Intent {
		if c.slider != nil {
			c.slider.SetValue(float32(v), u)
		}
		edit(float32(v), true, c, u)
		return nil
	}
	kids := []gunim.Node{&sized{child: c.field, width: NumberWidth}}
	if ok {
		c.slider = widget.NewSlider(lo, hi)
		c.slider.Label = label
		c.slider.OnChange = func(v float32, u *gunim.UI) gunim.Intent {
			c.field.SetValue(float64(v), u)
			edit(v, false, c, u)
			return nil
		}
		c.slider.OnCommit = func(v float32, u *gunim.UI) gunim.Intent {
			edit(v, true, c, u)
			return nil
		}
		kids = append([]gunim.Node{&sized{child: c.slider, width: SliderWidth}}, kids...)
	}
	r := widget.Row(kids...)
	r.Cross = widget.CrossCenter
	c.row = r
	return c
}

// numberRange returns the range a slider for a value of kind k with
// default def covers: the field's, or one that suits def. It reports
// false where none does.
func numberRange(k theme.Kind, def float32, f Field) (lo, hi float32, ok bool) {
	switch {
	case f.Max > f.Min:
		return f.Min, f.Max, true
	case def >= 0 && def <= 1 && k == theme.KindNumber:
		return 0, 1, true
	case def > 0 && def < 1 && k == theme.KindLength:
		return 0, 1, true
	case def >= 1 && k == theme.KindLength:
		// Three times the default, rounded up to a whole ten.
		return 0, float32(math.Ceil(float64(def)*3/10) * 10), true
	default:
		return 0, 0, false
	}
}

func (c *numberControl) node() gunim.Node { return c.row }

func (c *numberControl) show(v any, u *gunim.UI) {
	f, ok := v.(float32)
	if !ok {
		return
	}
	c.field.SetValue(float64(f), u)
	if c.slider != nil {
		c.slider.SetValue(f, u)
	}
}

// insetsControl is four fields: top, right, bottom and left.
type insetsControl struct {
	sides [4]*widget.NumberField
	row   gunim.Node
}

func newInsetsControl(label string, edit editFunc) *insetsControl {
	c := &insetsControl{}
	kids := make([]gunim.Node, 0, 4)
	for i, name := range []string{"Top", "Right", "Bottom", "Left"} {
		n := widget.NewNumberField(-1e4, 1e4)
		n.Decimals = 1
		n.Tooltip, n.Placeholder = label+": "+name, name
		c.sides[i] = n
		n.OnChange = func(_ float64, u *gunim.UI) gunim.Intent {
			edit(c.value(), true, c, u)
			return nil
		}
		kids = append(kids, &sized{child: n, width: SideWidth})
	}
	r := widget.Row(kids...)
	r.Cross = widget.CrossCenter
	c.row = r
	return c
}

// value returns the insets the fields hold.
func (c *insetsControl) value() geom.Insets {
	f := func(i int) float32 { return float32(c.sides[i].Value()) }
	return geom.Insets{Top: f(0), Right: f(1), Bottom: f(2), Left: f(3)}
}

func (c *insetsControl) node() gunim.Node { return c.row }

func (c *insetsControl) show(v any, u *gunim.UI) {
	in, ok := v.(geom.Insets)
	if !ok {
		return
	}
	for i, s := range []float32{in.Top, in.Right, in.Bottom, in.Left} {
		c.sides[i].SetValue(float64(s), u)
	}
}

// presetControl picks one of a field's presets, or shows Custom for a
// value none of them has.
type presetControl struct {
	presets []Preset
	seg     *widget.Segmented
	// custom says the control shows Custom, after the presets.
	custom bool
}

func newPresets(presets []Preset, label string, edit editFunc) *presetControl {
	c := &presetControl{presets: presets, seg: widget.NewSegmented()}
	c.seg.Tooltip = label
	c.seg.OnChange = func(i int, u *gunim.UI) gunim.Intent {
		if i < len(c.presets) {
			edit(c.presets[i].Value, true, c, u)
			c.setCustom(false)
		}
		return nil
	}
	c.setCustom(false)
	return c
}

// setCustom shows Custom after the presets, or not.
func (c *presetControl) setCustom(on bool) {
	c.custom = on
	items := make([]string, 0, len(c.presets)+1)
	for _, p := range c.presets {
		items = append(items, p.Label)
	}
	if on {
		items = append(items, "Custom")
	}
	c.seg.Items = items
}

func (c *presetControl) node() gunim.Node { return c.seg }

func (c *presetControl) show(v any, u *gunim.UI) {
	i := presetOf(c.presets, v)
	c.setCustom(i < 0)
	if i < 0 {
		i = len(c.presets)
	}
	c.seg.SetSelected(i, u)
}

// presetOf returns the preset of value v, or -1.
func presetOf(presets []Preset, v any) int {
	for i, p := range presets {
		if same(p.Value, v) {
			return i
		}
	}
	return -1
}

// springControl edits a spring: presets, a dot that moves with it, and
// sliders for its response and damping.
type springControl struct {
	presets  *presetControl
	preview  *preview
	response *widget.Slider
	damping  *widget.Slider
	col      gunim.Node
}

func newSpringControl(label string, edit editFunc) *springControl {
	c := &springControl{preview: newPreview(label)}
	c.presets = newPresets(springPresets, label, func(v any, commit bool, _ control, u *gunim.UI) {
		if s, ok := v.(anim.Spring); ok {
			c.showSliders(s, u)
			c.preview.play(s, u)
		}
		edit(v, commit, c, u)
	})
	c.response = widget.NewSlider(0, 1)
	c.response.Snap, c.response.Label = 0.01, label+": response"
	c.damping = widget.NewSlider(0.1, 1.5)
	c.damping.Snap, c.damping.Label = 0.01, label+": damping"
	slid := func(commit bool) func(float32, *gunim.UI) gunim.Intent {
		return func(_ float32, u *gunim.UI) gunim.Intent {
			s := c.value()
			c.presets.show(s, u)
			c.preview.play(s, u)
			edit(s, commit, c, u)
			return nil
		}
	}
	c.response.OnChange, c.response.OnCommit = slid(false), slid(true)
	c.damping.OnChange, c.damping.OnCommit = slid(false), slid(true)
	resp := widget.NewSliderRow("Response", c.response)
	resp.Format = func(v float32) string { return fmt.Sprintf("%.2f s", v) }
	damp := widget.NewSliderRow("Damping", c.damping)
	top := widget.Row(c.presets.node(), c.preview)
	top.Cross = widget.CrossCenter
	col := widget.Column(top, resp, damp)
	col.Cross = widget.CrossStretch
	c.col = &sized{child: col, width: SpringWidth}
	return c
}

// value returns the spring the sliders hold.
func (c *springControl) value() anim.Spring {
	return anim.Spring{Response: c.response.Value(), Damping: c.damping.Value()}
}

// showSliders moves the sliders to s.
func (c *springControl) showSliders(s anim.Spring, u *gunim.UI) {
	c.response.SetValue(s.Response, u)
	c.damping.SetValue(s.Damping, u)
}

func (c *springControl) node() gunim.Node { return c.col }

func (c *springControl) show(v any, u *gunim.UI) {
	s, ok := v.(anim.Spring)
	if !ok {
		return
	}
	c.presets.show(s, u)
	c.showSliders(s, u)
	c.preview.spring = s
}

// choiceControl picks one of the values a program offers for a token,
// such as a font.
type choiceControl struct {
	choices []Preset
	drop    *widget.Dropdown
}

func newChoiceControl(choices []Preset, label string, edit editFunc) *choiceControl {
	items := make([]string, 0, len(choices))
	for _, p := range choices {
		items = append(items, p.Label)
	}
	c := &choiceControl{choices: choices, drop: widget.NewDropdown(widget.Labels(items...))}
	c.drop.Label = label
	c.drop.OnChange = func(i int, u *gunim.UI) gunim.Intent {
		edit(c.choices[i].Value, true, c, u)
		return nil
	}
	return c
}

func (c *choiceControl) node() gunim.Node { return c.drop }

func (c *choiceControl) show(v any, u *gunim.UI) { c.drop.SetSelected(presetOf(c.choices, v), u) }

// textControl shows a value the editor cannot change, as text.
type textControl struct{ label *widget.Label }

func newTextControl() *textControl {
	l := widget.NewLabel("")
	l.Size, l.Color, l.MaxLines = DetailSize, widget.Placeholder, 1
	return &textControl{label: l}
}

func (c *textControl) node() gunim.Node { return c.label }

func (c *textControl) show(v any, u *gunim.UI) {
	c.label.Text = describe(v)
	u.Invalidate()
}

// describe writes a value as text: its own words, or its type.
func describe(v any) string {
	switch v := v.(type) {
	case fmt.Stringer:
		return v.String()
	case *text.Face:
		return "a font"
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return "none"
	}
	switch k := rv.Kind(); k {
	case reflect.String, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64:
		return fmt.Sprint(v)
	default:
		return "a " + rv.Type().String()
	}
}

// sized lays its child out at a width from a token.
type sized struct {
	child gunim.Node
	width theme.Token[float32]
}

// Children implements [gunim.Composite].
func (s *sized) Children() []gunim.Node { return []gunim.Node{s.child} }

// Layout implements [gunim.Node].
func (s *sized) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	w := s.width.Get(f.Theme)
	if c.Max.W > 0 {
		w = min(w, c.Max.W)
	}
	cs := gunim.Constraints{Min: geom.Sz(w, c.Min.H), Max: geom.Sz(w, c.Max.H)}
	size := kids.At(0).Layout(cs)
	kids.At(0).Place(geom.Point{})
	return c.Constrain(geom.Sz(w, size.H))
}

// Paint implements [gunim.Node].
func (s *sized) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}
