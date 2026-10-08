package themeedit

import (
	"image/color"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/widget"
)

// specimen is the preview's default content: a few of every kind of
// widget, as a small notes program might show them, and a terminal
// whose cursor types along its last line. It lays them out in one
// column, or in two where it is wide enough, as above the controls.
type specimen struct {
	tabs         *widget.Tabs
	name, search *widget.TextField
	sync         *widget.Switch
	pinned       *widget.Checkbox
	when         *widget.Dropdown
	list         *miniList
	del, cancel  *widget.Button
	save         *widget.Button
	term         *terminal
	// twoColumns says the last layout had room for two columns.
	twoColumns bool
}

// twoColumnWidth is the least width the specimen lays out in two
// columns at, as the preview above the controls is.
const twoColumnWidth = 560

func newSpecimen() *specimen {
	s := &specimen{}
	s.tabs = widget.NewTabs([]string{"Notes", "Files", "Shared"}, widget.NewSpacer(), widget.NewSpacer(), widget.NewSpacer())
	s.name = widget.NewTextField()
	s.name.SetText("Groceries for Saturday", nil)
	s.search = widget.NewTextField()
	s.search.Placeholder, s.search.Icon = "Search notes", icon.Search
	s.sync = widget.NewSwitch("Sync")
	s.sync.SetChecked(true, nil)
	s.pinned = widget.NewCheckbox("Pinned")
	s.when = widget.NewDropdown(widget.Labels("This week", "This month", "Any time"))
	s.when.Label = "When"
	s.list = &miniList{items: []string{"Groceries", "Ideas", "Holiday plans"}}
	s.del = widget.NewButton("Delete")
	s.del.Kind = widget.ButtonDanger
	s.cancel = widget.NewButton("Cancel")
	s.save = widget.NewButton("Save")
	s.save.Kind = widget.ButtonPrimary
	s.term = newTerminal()
	return s
}

// Children implements [gunim.Composite].
func (s *specimen) Children() []gunim.Node {
	return []gunim.Node{s.tabs, s.name, s.search, s.sync, s.pinned, s.when, s.list, s.del, s.cancel, s.save, s.term}
}

// Layout implements [gunim.Node].
func (s *specimen) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	th := f.Theme
	w := c.Max.W
	gap := widget.Gap.Get(th)
	m := widget.Margin.Get(th)
	at := func(i int, x, y, width float32) geom.Size {
		k := kids.At(i)
		var cs gunim.Constraints
		if width > 0 {
			cs = gunim.Constraints{Min: geom.Sz(width, 0), Max: geom.Sz(width, 0)}
		} else {
			cs = gunim.Constraints{Max: geom.Sz(w, 0)}
		}
		size := k.Layout(cs)
		k.Place(geom.Pt(x, y))
		return size
	}
	ts := at(0, 0, 0, w)
	inner := max(0, w-m.Left-m.Right)
	s.twoColumns = inner >= twoColumnWidth
	left, colW := m.Left, inner
	if s.twoColumns {
		colW = (inner - 2*gap) / 2
	}
	// The left column: the fields, the switch and the check, the
	// drop-down, and in two columns the buttons.
	y := ts.H
	y += at(1, left, y, colW).H + gap
	y += at(2, left, y, colW).H + gap
	sw := at(3, left, y, 0)
	pc := at(4, left+sw.W+2*gap, y, 0)
	rowH := max(sw.H, pc.H)
	y += rowH + gap
	y += at(5, left, y, colW).H
	buttons := func(x, y, width float32) float32 {
		d := at(7, x, y, 0)
		sv := at(9, 0, y, 0)
		cn := at(8, 0, y, 0)
		kids.At(9).Place(geom.Pt(x+width-sv.W, y))
		kids.At(8).Place(geom.Pt(x+width-sv.W-gap-cn.W, y))
		return max(d.H, sv.H, cn.H)
	}
	var h float32
	if s.twoColumns {
		y += 2 * gap
		y += buttons(left, y, colW)
		right := left + colW + 2*gap
		ry := ts.H
		ry += at(6, right, ry, colW).H + 2*gap
		ry += at(10, right, ry, colW).H
		h = max(y, ry)
	} else {
		y += 2 * gap
		y += at(6, left, y, colW).H + 2*gap
		y += at(10, left, y, colW).H + 2*gap
		y += buttons(left, y, colW)
		h = y
	}
	return c.Constrain(geom.Sz(w, h+m.Bottom))
}

// Paint implements [gunim.Node].
func (s *specimen) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for i := range kids.Len() {
		kids.At(i).Paint(p)
	}
}

// pulse toggles the switch and the check, for a quick motion played.
func (s *specimen) pulse(u *gunim.UI) {
	s.sync.SetChecked(!s.sync.Checked(), u)
	s.pinned.SetChecked(!s.pinned.Checked(), u)
}

// miniList is a short list as a sidebar shows one: a row chosen, and
// another lit as if the pointer were over it.
type miniList struct {
	items  []string
	shaped []text.Run
	face   *text.Face
	size   float32
}

// listRow is the height of a row of a [miniList], as a menu's.
var listRow = widget.MenuRowHeight

// Layout implements [gunim.Node].
func (l *miniList) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	face, size := widget.Font.Get(th), widget.TextSize.Get(th)
	if face != l.face || size != l.size || len(l.shaped) != len(l.items) {
		l.face, l.size = face, size
		l.shaped = l.shaped[:0]
		for _, it := range l.items {
			l.shaped = append(l.shaped, face.Shape(it, size))
		}
	}
	h := listRow.Get(th)
	return c.Constrain(geom.Sz(c.Max.W, h*float32(len(l.items))+widget.ListSpacing.Get(th)*float32(len(l.items)-1)))
}

// Paint implements [gunim.Node].
func (l *miniList) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	h, gap := listRow.Get(th), widget.ListSpacing.Get(th)
	pad := widget.MenuRowPadding.Get(th)
	radius := widget.RowRadius.Get(th)
	for i, run := range l.shaped {
		r := geom.Rc(0, float32(i)*(h+gap), box.W, h)
		switch i {
		case 0:
			p.RRect(r, radius, paint.Solid(widget.Selection.Get(th)))
		case 1:
			p.RRect(r, radius, paint.Solid(widget.ButtonFill.Get(th)))
		}
		size := widget.IconSize.Get(th)
		widget.PaintIcon(p, th, icon.FileText, geom.Rc(pad, r.Min.Y+(h-size)/2, size, size), widget.Placeholder.Get(th))
		run.Paint(p, geom.Pt(pad+size+widget.IconGap.Get(th), r.Min.Y+(h-run.Height())/2), widget.Ink.Get(th))
	}
}

// Access implements [gunim.Accessible].
func (l *miniList) Access() access.Info {
	return access.Info{Role: access.RoleList, Name: "A list, for the preview"}
}

// terminal is a grid of cells as a terminal shows them, whose cursor
// types along its last line, over and over, so the cursor's motion
// shows. Asked to, it jumps the cursor back and forth along a line.
type terminal struct {
	grid  *widget.CellGrid
	ink   color.NRGBA
	typed int
	// wait is how long until the next step, and jumps how many jumps of
	// the cursor are left to show.
	wait  time.Duration
	jumps int
}

// The terminal's lines: a prompt, what was typed after it, and output.
// The last line is typed out a letter at a time.
var (
	termLines = []string{"~/notes $ ls", "groceries.md  ideas.md  todo.md", "~/notes $ "}
	termTyped = "git commit -m \"Saturday\""
	prompt    = len("~/notes $ ")
)

// The terminal's pace: a letter typed, the rest at the end of a line,
// and a jump of the cursor when one plays.
const (
	typeEvery = 120 * time.Millisecond
	typeRest  = 1200 * time.Millisecond
	jumpEvery = 650 * time.Millisecond
)

func newTerminal() *terminal {
	g := widget.NewCellGrid()
	g.Resize(44, len(termLines))
	g.Background = widget.FieldFill
	return &terminal{grid: g}
}

// fill writes the lines, the prompts in colour.
func (t *terminal) fill() {
	for y, s := range termLines {
		if y == len(termLines)-1 {
			s += termTyped[:t.typed]
		}
		cells := make([]widget.Cell, 0, len(s))
		for x, r := range s {
			c := widget.Cell{Rune: r}
			if y != 1 && x < prompt {
				c.FG = t.ink
			}
			cells = append(cells, c)
		}
		t.grid.SetRow(y, cells)
	}
}

// place puts the cursor after what is typed.
func (t *terminal) place() {
	t.grid.SetCursor(widget.Cursor{Col: prompt + t.typed, Row: len(termLines) - 1, Visible: true})
}

// jump has the cursor jump to the start of the line and back a few
// times, for the cursor's motion played.
func (t *terminal) jump() {
	t.jumps, t.wait = 4, 0
}

// Children implements [gunim.Composite].
func (t *terminal) Children() []gunim.Node { return []gunim.Node{t.grid} }

// Step implements [gunim.Animator].
func (t *terminal) Step(dt time.Duration) bool {
	t.wait -= dt
	if t.wait > 0 {
		return false
	}
	last := len(termLines) - 1
	switch {
	case t.jumps > 0:
		t.jumps--
		col := prompt + t.typed
		if t.jumps%2 == 1 {
			col = prompt
		}
		t.grid.SetCursor(widget.Cursor{Col: col, Row: last, Visible: true})
		t.wait = jumpEvery
	case t.typed < len(termTyped):
		t.typed++
		t.fill()
		t.place()
		t.wait = typeEvery
		if t.typed == len(termTyped) {
			t.wait = typeRest
		}
	default:
		t.typed = 0
		t.fill()
		t.place()
		t.wait = typeRest / 2
	}
	return true
}

// WakeIn implements [gunim.Waker].
func (t *terminal) WakeIn() time.Duration { return max(time.Millisecond, t.wait) }

// Layout implements [gunim.Node]. The grid takes the room it is given,
// inside a field's padding, in the terminal's text size.
func (t *terminal) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	th := f.Theme
	t.grid.Size = TerminalSize.Get(th)
	if ink := widget.Accent.Get(th); ink != t.ink {
		t.ink = ink
		t.fill()
		t.place()
	}
	pad := widget.FieldPadding.Get(th)
	s := kids.At(0).Layout(gunim.Constraints{Max: geom.Sz(max(0, c.Max.W-2*pad), 0)})
	kids.At(0).Place(geom.Pt(pad, pad))
	return c.Constrain(geom.Sz(c.Max.W, s.H+2*pad))
}

// Paint implements [gunim.Node]: a field's ground with the grid on it,
// clipped to round corners.
func (t *terminal) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	th := f.Theme
	r := geom.Rect{Max: box.Point()}
	radius := widget.FieldRadius.Get(th)
	p.RRectStroke(r, radius, paint.Solid(widget.FieldFill.Get(th)), paint.Stroke{Width: 1, Color: widget.FieldBorder.Get(th)})
	defer p.Layer(paint.LayerOpts{Bounds: r.Inset(geom.Uniform(1)), Opacity: 1, Clip: true, Radius: radius})()
	kids.At(0).Paint(p)
}
