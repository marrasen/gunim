package calendar

import (
	"strconv"
	"strings"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// DateField holds a day. A click, Enter or Space opens a small month under it to pick the day from, and the Up and
// Down keys move the day by one.
type DateField struct {
	anim.Group
	// OnChange runs when the user picks a day.
	OnChange func(day time.Time, u *gunim.UI)

	value time.Time
	popup *gunim.Popup
	focus *anim.Float
	size  geom.Size
}

// NewDateField returns a field holding day.
func NewDateField(day time.Time) *DateField {
	f := &DateField{value: Day(day), focus: anim.NewFloat(0)}
	f.Add(f.focus)
	return f
}

// Value returns the day the field holds.
func (f *DateField) Value() time.Time { return f.value }

// SetValue puts day in the field.
func (f *DateField) SetValue(day time.Time, u *gunim.UI) {
	f.value = Day(day)
	u.Invalidate()
}

// set puts day in the field as the user picked it.
func (f *DateField) set(day time.Time, u *gunim.UI) {
	f.SetValue(day, u)
	if f.OnChange != nil {
		f.OnChange(f.value, u)
	}
}

// open shows the small month under the field.
func (f *DateField) open(u *gunim.UI) {
	if f.popup != nil && f.popup.Open() {
		return
	}
	m := NewMiniMonth(f.value)
	m.Pick = func(day time.Time, u *gunim.UI) {
		f.set(day, u)
		f.close(u)
	}
	f.popup = u.OpenPopup(f, widget.NewCard(widget.NewPad(m)), gunim.PopupOptions{
		Anchor:  geom.Rc(0, 0, f.size.W, f.size.H+4),
		Dismiss: f.close,
	})
	u.Focus(m)
}

// close takes the small month away and gives the field the keyboard back.
func (f *DateField) close(u *gunim.UI) {
	if f.popup != nil {
		f.popup.Close()
		f.popup = nil
		u.Focus(f)
	}
}

// Layout implements [gunim.Node].
func (f *DateField) Layout(c gunim.Constraints, fr gunim.Frame, _ gunim.Children) geom.Size {
	f.size = c.Constrain(geom.Sz(170, widget.FieldHeight.Get(fr.Theme)))
	return f.size
}

// Paint implements [gunim.Node].
func (f *DateField) Paint(p *paint.Painter, fr gunim.Frame, box geom.Size, _ gunim.Children) {
	th := fr.Theme
	focus := min(max(f.focus.Value(), 0), 1)
	border := widget.FieldBorder.Get(th)
	if focus > 0.5 || (f.popup != nil && f.popup.Open()) {
		border = widget.Accent.Get(th)
	}
	p.RRectStroke(geom.Rect{Max: box.Point()}, widget.FieldRadius.Get(th), paint.Solid(widget.FieldFill.Get(th)),
		paint.Stroke{Width: 1 + focus, Color: border})
	t := widget.Font.Get(th).Shape(f.value.Format("Mon 2 Jan 2006"), widget.TextSize.Get(th))
	t.Paint(p, geom.Pt(widget.FieldPadding.Get(th), (box.H-t.Height())/2), widget.Ink.Get(th))
	s := float32(16)
	widget.PaintIcon(p, th, icon.Calendar, geom.Rc(box.W-s-8, (box.H-s)/2, s, s), widget.PaletteHint.Get(th))
}

// Handle implements [gunim.Handler].
func (f *DateField) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.FocusGained:
		f.focus.Animate(1, widget.Quick.Get(u.Theme()))
	case input.FocusLost:
		f.focus.Animate(0, widget.Quick.Get(u.Theme()))
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		if f.popup != nil && f.popup.Open() {
			f.close(u)
		} else {
			f.open(u)
		}
		return true
	case input.KeyPress:
		switch e.Key {
		case input.KeyEnter, input.KeySpace:
			f.open(u)
		case input.KeyUp:
			f.set(AddDays(f.value, -1), u)
		case input.KeyDown:
			f.set(AddDays(f.value, 1), u)
		default:
			return false
		}
		return true
	}
	return false
}

// Focusable implements [gunim.Focusable].
func (f *DateField) Focusable() bool { return true }

// Cursor implements [gunim.CursorShaper].
func (f *DateField) Cursor(geom.Point) input.Cursor { return input.CursorHand }

// TimeField holds a time of day, typed as 9:30, 0930 or 9. The Up and Down keys move it by Step, and leaving the
// field writes what it holds out in full.
type TimeField struct {
	*widget.TextField
	// Step is how far Up and Down move the time; zero is 15 minutes.
	Step time.Duration
	// OnChange runs when the time changes by the keys, or as the field is left.
	OnChange func(t time.Duration, u *gunim.UI)
}

// NewTimeField returns a field holding the time of day t, as a span from midnight.
func NewTimeField(t time.Duration) *TimeField {
	f := &TimeField{TextField: widget.NewTextField()}
	f.TextField.SetText(clockOf(t))
	return f
}

// Value returns the time of day the field holds, and false when what it holds is not one.
func (f *TimeField) Value() (time.Duration, bool) { return ParseClock(f.Text()) }

// SetValue puts the time of day t in the field.
func (f *TimeField) SetValue(t time.Duration, u *gunim.UI) {
	f.TextField.SetText(clockOf(t))
	u.Invalidate()
}

// Layout implements [gunim.Node].
func (f *TimeField) Layout(c gunim.Constraints, fr gunim.Frame, kids gunim.Children) geom.Size {
	// No limit, or a wide one, still gets a field as wide as a time.
	if c.Max.W <= 0 || c.Max.W > 90 {
		c.Max.W = 90
	}
	c.Min.W = min(c.Min.W, c.Max.W)
	return f.TextField.Layout(c, fr, kids)
}

// Handle implements [gunim.Handler].
func (f *TimeField) Handle(e input.Event, u *gunim.UI) bool {
	step := f.Step
	if step <= 0 {
		step = 15 * time.Minute
	}
	switch e := e.(type) {
	case input.KeyPress:
		by := map[input.Key]time.Duration{input.KeyUp: -step, input.KeyDown: step}[e.Key]
		if by == 0 {
			break
		}
		t, ok := f.Value()
		if !ok {
			return true
		}
		t = (t + by + 24*time.Hour) % (24 * time.Hour)
		f.SetValue(t.Truncate(step), u)
		if f.OnChange != nil {
			f.OnChange(t.Truncate(step), u)
		}
		return true
	case input.FocusLost:
		if t, ok := f.Value(); ok {
			f.TextField.SetText(clockOf(t))
			if f.OnChange != nil {
				f.OnChange(t, u)
			}
		}
	}
	return f.TextField.Handle(e, u)
}

// clockOf writes a time of day as 09:30.
func clockOf(t time.Duration) string {
	return clock(int(t/time.Hour)%24, int(t%time.Hour/time.Minute))
}

// ParseClock reads a time of day written as 9, 930, 0930, 9:30 or 9.30, and reports false for anything else.
func ParseClock(s string) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	h, m, sep := s, "", false
	if i := strings.IndexAny(s, ":."); i >= 0 {
		h, m, sep = s[:i], s[i+1:], true
	} else if len(s) > 2 {
		h, m = s[:len(s)-2], s[len(s)-2:]
	}
	if h == "" || len(h) > 2 || (sep && len(m) != 2) || (m != "" && len(m) != 2) {
		return 0, false
	}
	hh, err := strconv.Atoi(h)
	if err != nil || hh < 0 || hh > 23 {
		return 0, false
	}
	mm := 0
	if m != "" {
		if mm, err = strconv.Atoi(m); err != nil || mm < 0 || mm > 59 {
			return 0, false
		}
	}
	return time.Duration(hh)*time.Hour + time.Duration(mm)*time.Minute, true
}
