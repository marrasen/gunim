package widget

import (
	"math"
	"strconv"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/input"
)

// NumberField is a [TextField] that holds a number.
//
// It takes typing as a text field does, and adds what a number wants:
// the up and down arrows step the value, Page Up and Page Down step it
// ten times as far, the wheel turns it while the field has the
// keyboard and the pointer is over it, and anything typed is held to Min and Max when the field is left or
// Enter is pressed.
//
// Half-typed text is left alone while the field has focus, so a value
// can be cleared and retyped without the field fighting back. The
// application hears a value only when one can be read, through
// OnChange.
type NumberField struct {
	*TextField

	// Min and Max bound the value. Leave Max at zero for no ceiling.
	Min, Max float64
	// Increment is how far one arrow press moves the value; zero is one.
	Increment float64
	// Decimals is how many places the value is written with. Zero
	// writes whole numbers.
	Decimals int
	// Suffix is written after the number, such as " ms". It is not part
	// of what the user types.
	Suffix string
	// OnChange runs on the UI goroutine with a new value: when the text
	// can be read as a number, and again when the field is left or Enter
	// is pressed and the value had to be held to the bounds. It may act
	// in the window through u, such as a slider beside the field that
	// follows it; a non-nil result is sent to the application as the
	// field's intent. It shadows the text field's own OnChange, which
	// takes a string and which the number field uses itself.
	OnChange func(v float64, u *gunim.UI) gunim.Intent

	value float64
}

// NewNumberField returns a field holding lo, bounded by lo and hi.
func NewNumberField(lo, hi float64) *NumberField {
	n := &NumberField{TextField: NewTextField(), Min: lo, Max: hi, value: lo}
	n.SetText(n.format(lo), nil)
	n.TextField.OnChange = func(s string, u *gunim.UI) gunim.Intent {
		v, ok := parseNumber(s)
		if !ok {
			return nil
		}
		// While typing, the value is taken as it reads. Holding it to
		// the bounds mid-word would fight the fingers: typing 1 on the
		// way to 12 would jump to the minimum.
		n.value = v
		if n.OnChange != nil {
			send(u, n, n.OnChange(v, u))
		}
		return nil
	}
	// The text field's own OnChange returns nothing to send, and its
	// OnCommit stays unset: they would send from a node that is not in
	// the tree. Everything this field reports goes out from the field
	// itself.
	return n
}

// Value returns the number the field holds.
func (n *NumberField) Value() float64 { return n.value }

// SetValue sets the value, held to Min and Max, rewrites the text, and
// sends no intent. u may be nil, as before the field is laid out.
func (n *NumberField) SetValue(v float64, u *gunim.UI) {
	n.value = n.clamp(v)
	n.SetText(n.format(n.value), u)
}

func (n *NumberField) clamp(v float64) float64 {
	if math.IsNaN(v) {
		return n.Min
	}
	v = max(n.Min, v)
	if n.Max > n.Min {
		v = min(n.Max, v)
	}
	return v
}

func (n *NumberField) format(v float64) string {
	return strconv.FormatFloat(v, 'f', n.Decimals, 64) + n.Suffix
}

func (n *NumberField) step() float64 {
	if n.Increment > 0 {
		return n.Increment
	}
	return 1
}

// nudge moves the value by delta steps and rewrites the text.
func (n *NumberField) nudge(delta float64, u *gunim.UI) {
	if n.clamp(n.value+delta) != n.value {
		u.Cue(gunim.CueTick, n)
	}
	n.commit(n.value+delta, u)
}

// settle holds the value to the bounds and rewrites the text, which is
// what leaving the field or pressing Enter does.
func (n *NumberField) settle(u *gunim.UI) {
	v, ok := parseNumber(n.TextField.Text())
	if !ok {
		v = n.value
	}
	n.commit(v, u)
}

func (n *NumberField) commit(v float64, u *gunim.UI) {
	v = n.clamp(v)
	changed := v != n.value
	n.value = v
	n.SetText(n.format(v), u)
	if !changed {
		return
	}
	if n.OnChange != nil {
		send(u, n, n.OnChange(v, u))
	}
}

// Handle implements [gunim.Handler]: the keys and the wheel a number
// wants, with everything else left to the text field underneath.
func (n *NumberField) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.KeyPress:
		if n.Disabled || e.Mods.Has(input.ModControl) || e.Mods.Has(input.ModAlt) {
			break
		}
		switch e.Key {
		case input.KeyUp:
			n.nudge(n.step(), u)
			return true
		case input.KeyDown:
			n.nudge(-n.step(), u)
			return true
		case input.KeyPageUp:
			n.nudge(10*n.step(), u)
			return true
		case input.KeyPageDown:
			n.nudge(-10*n.step(), u)
			return true
		case input.KeyEnter:
			n.settle(u)
			// The field's own Handle never sees Enter, so the
			// submission is sent from here, where the node is the one
			// the tree knows.
			if n.OnCommit != nil {
				send(u, n, n.OnCommit(n.Text(), u))
			}
			return true
		}
	case input.Scroll:
		// Only while the field has the keyboard, so scrolling a panel
		// leaves every number the pointer passes over as it is. The
		// wheel turned up gives a positive Delta.Y, and turns the value
		// up.
		if n.Disabled || e.Delta.Y == 0 || !u.HasFocus(n) {
			break
		}
		n.nudge(math.Copysign(n.step(), float64(e.Delta.Y)), u)
		return true
	case input.FocusLost:
		n.settle(u)
	}
	return n.TextField.Handle(e, u)
}

// parseNumber reads what was typed, ignoring any suffix and taking a
// comma for a decimal point, as a European keyboard gives.
func parseNumber(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	cut := len(s)
	for i, r := range s {
		if !strings.ContainsRune("0123456789+-.,eE", r) {
			cut = i
			break
		}
	}
	s = strings.TrimSpace(strings.Replace(s[:cut], ",", ".", 1))
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
