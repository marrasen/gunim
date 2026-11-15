package widget

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// eyeName is what the eye is called when the field gives it no EyeLabel.
const eyeName = "Show text"

// fieldEye is the eye at the end of a Secret field. Pressing it shows the field's text in place of the dots, and
// pressing it again hides it. It is a Tab stop of its own, and a screen reader reads it as a toggle button that is
// pressed while the text shows. A click on it leaves the keyboard in the field.
type fieldEye struct {
	IconButton
	t *TextField
	// has says the eye has the keyboard.
	has bool
}

func newFieldEye(t *TextField) *fieldEye {
	e := &fieldEye{IconButton: IconButton{Button: *NewButton("")}, t: t}
	e.Icon, e.Ghost, e.self = icon.Eye, true, e
	e.KeepFocus = true
	e.IconSize = IconSize
	e.OnClick = func(u *gunim.UI) gunim.Intent {
		t.SetShown(!t.showing, u)
		return nil
	}
	return e
}

// name is what the eye is called, on its tooltip and to a screen reader.
func (e *fieldEye) name() string {
	if e.t.EyeLabel != "" {
		return e.t.EyeLabel
	}
	return eyeName
}

// Focusable implements [gunim.Focusable]: the eye takes the keyboard while it shows and the field is enabled.
func (e *fieldEye) Focusable() bool { return e.t.wantsEye() && !e.t.Disabled }

// Cursor implements [gunim.CursorShaper]: an arrow, as over any button.
func (e *fieldEye) Cursor(geom.Point) input.Cursor { return input.CursorArrow }

// Handle implements [gunim.Handler]. Space and Enter flip the eye. The other keys go on to the nodes round the
// field, past the field, so typing while the eye has the keyboard changes nothing in the text.
func (e *fieldEye) Handle(ev input.Event, u *gunim.UI) bool {
	switch ev.(type) {
	case input.FocusGained:
		e.has = true
	case input.FocusLost:
		e.has = false
		e.t.leaving = true
		u.Invalidate()
	}
	done := e.Button.Handle(ev, u)
	switch ev.(type) {
	case input.FocusGained, input.FocusLost, input.FocusRing, input.TextInput, input.Composing, input.TextEdit:
		// These are the eye's own; the field must not take them as its own.
		return true
	}
	return done
}

// Access implements [gunim.Accessible]: a toggle button, pressed while the text shows.
func (e *fieldEye) Access() access.Info {
	if !e.t.wantsEye() {
		return access.Info{Role: access.RoleGroup}
	}
	return e.accessHeld(access.Info{Role: access.RoleButton, Name: e.name(),
		State:   access.StateCheckable | activeState(e.t.showing) | e.accessState(),
		Actions: []string{access.ActionPress}})
}

// wantsEye reports whether the field shows an eye: it is Secret and keeps its eye.
func (t *TextField) wantsEye() bool { return t.Secret && !t.SkipEye }

// Children implements [gunim.Composite]: the eye, for a Secret field.
func (t *TextField) Children() []gunim.Node {
	if !t.wantsEye() {
		return nil
	}
	return []gunim.Node{t.eyeNode()}
}

func (t *TextField) eyeNode() *fieldEye {
	if t.eye == nil {
		t.eye = newFieldEye(t)
	}
	return t.eye
}

// Shown reports whether a Secret field shows its text in place of the dots.
func (t *TextField) Shown() bool { return t.showing }

// SetShown shows a Secret field's text in place of the dots, or hides it again, as its eye does. It sends no
// intent, and does nothing for a field that is not Secret. The text stays off the clipboard either way. u may be nil.
func (t *TextField) SetShown(on bool, u *gunim.UI) {
	on = on && t.Secret
	if on == t.showing {
		return
	}
	t.showing = on
	if t.eye != nil {
		t.eye.Active = on
		t.eye.Icon = icon.Eye
		if on {
			t.eye.Icon = icon.EyeOff
		}
	}
	if u != nil {
		u.Invalidate()
	}
}

// eyeChild finds the eye among the field's children. A field made Secret after it was put in the tree builds its
// eye here, and lays it out from the next frame.
func (t *TextField) eyeChild(kids gunim.Children, f gunim.Frame) (gunim.Child, bool) {
	if t.eye != nil {
		for k := range kids.All {
			if k.Node() == gunim.Node(t.eye) {
				return k, true
			}
		}
	}
	if t.wantsEye() && f.UI() != nil {
		kids.Build(t.eyeNode())
	}
	return gunim.Child{}, false
}

// layEye lays the eye out at the end of the field, and hides the text again when the keyboard has left the field
// and its eye.
func (t *TextField) layEye(kids gunim.Children, f gunim.Frame, box geom.Size, th *theme.Live) {
	if !t.wantsEye() {
		t.SetShown(false, nil)
	}
	if t.leaving {
		t.leaving = false
		if !t.KeepShown && !t.has && (t.eye == nil || !t.eye.has) {
			t.SetShown(false, nil)
		}
	}
	k, ok := t.eyeChild(kids, f)
	t.eyeOn = ok && t.wantsEye()
	if !t.eyeOn {
		return
	}
	t.eye.Disabled = t.Disabled
	t.eye.Tooltip = t.eye.name()
	s := k.Layout(gunim.Loose(box))
	t.eyeX = box.W - FieldPadding.Get(th) - IconSize.Get(th) - IconPadding.Get(th)
	k.Place(geom.Pt(t.eyeX, (box.H-s.H)/2))
}

// paintEye paints the eye, when it shows.
func (t *TextField) paintEye(p *paint.Painter, kids gunim.Children) {
	if !t.eyeOn {
		return
	}
	for k := range kids.All {
		if k.Node() == gunim.Node(t.eye) {
			k.Paint(p)
		}
	}
}

// parts is a field with Tab stops of its own inside it, such as a secret field's eye.
type parts interface{ tabStops() []gunim.Node }

// tabStops returns the eye, while Tab can stop at it.
func (t *TextField) tabStops() []gunim.Node {
	if t.eye == nil || !t.eyeOn || !t.eye.Focusable() {
		return nil
	}
	return []gunim.Node{t.eye}
}
