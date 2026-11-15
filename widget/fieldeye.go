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

// eyeName is what the eye is called when the field gives it no EyeLabel, and hideName what its tooltip says while
// the text shows when the field gives it no EyeHideLabel.
const (
	eyeName  = "Show text"
	hideName = "Hide text"
)

// fieldEye is the eye at the end of a Secret field. Pressing it shows the field's text in place of the dots, and
// pressing it again hides it. Like the reveal button in Windows' own password boxes, it is not a Tab stop: from the
// keyboard, Alt+F8 in the field does the same. A screen reader reads it as a toggle button that is pressed while the
// text shows. A press on it gives the keyboard to the field, and leaves the caret and the selection where they are.
type fieldEye struct {
	IconButton
	t *TextField
}

func newFieldEye(t *TextField) *fieldEye {
	e := &fieldEye{IconButton: IconButton{Button: *NewButton("")}, t: t}
	e.Icon, e.Ghost, e.self = icon.Eye, true, e
	e.IconSize = IconSize
	e.OnClick = func(u *gunim.UI) gunim.Intent {
		// A screen reader presses the eye without a pointer, so the field may not have the keyboard yet. The text
		// shows only while it does.
		if u != nil && !u.HasFocus(t) {
			u.Focus(t)
		}
		t.SetShown(!t.showing, u)
		return nil
	}
	return e
}

// name is what the eye is called to a screen reader, which also hears whether it is pressed.
func (e *fieldEye) name() string {
	if e.t.EyeLabel != "" {
		return e.t.EyeLabel
	}
	return eyeName
}

// tipText is the eye's tooltip: what a press on it does now.
func (e *fieldEye) tipText() string {
	if !e.t.showing {
		return e.name() + " (Alt+F8)"
	}
	if e.t.EyeHideLabel != "" {
		return e.t.EyeHideLabel + " (Alt+F8)"
	}
	return hideName + " (Alt+F8)"
}

// Focusable implements [gunim.Focusable]: never, so Tab passes the eye, and a press on it focuses the field.
func (e *fieldEye) Focusable() bool { return false }

// Cursor implements [gunim.CursorShaper]: an arrow, as over any button.
func (e *fieldEye) Cursor(geom.Point) input.Cursor { return input.CursorArrow }

// Access implements [gunim.Accessible]: a toggle button, pressed while the text shows.
func (e *fieldEye) Access() access.Info {
	if !e.t.eyeOn {
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
// intent, and does nothing for a field that is not Secret. The text stays off the clipboard either way. Without
// KeepShown, the text shows only while the field has the keyboard: shown while it lacks it, the text hides again at
// the next layout. u may be nil.
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
// eye here, and lays it out from the next frame. A type that embeds TextField and has children of its own gets no
// eye, so its own children stay as they are; Alt+F8 still shows its text.
func (t *TextField) eyeChild(kids gunim.Children, f gunim.Frame) (gunim.Child, bool) {
	others := false
	for k := range kids.All {
		if t.eye != nil && k.Node() == gunim.Node(t.eye) {
			return k, true
		}
		others = true
	}
	if t.wantsEye() && !others && f.UI() != nil {
		kids.Build(t.eyeNode())
	}
	return gunim.Child{}, false
}

// layEye lays the eye out at the end of the field when it fits, and hides the text again when the field lacks the
// keyboard.
func (t *TextField) layEye(kids gunim.Children, f gunim.Frame, box geom.Size, th *theme.Live) {
	if !t.wantsEye() || !t.KeepShown && (f.UI() == nil || !f.UI().HasFocus(t)) {
		t.SetShown(false, nil)
	}
	k, ok := t.eyeChild(kids, f)
	t.eyeOn = ok && t.wantsEye() && t.fits.eye
	if !ok {
		return
	}
	if !t.eyeOn {
		// Out of room, or no longer Secret: the eye takes no space, so no press lands on it.
		k.Layout(gunim.Tight(geom.Size{}))
		k.Place(geom.Pt(box.W, box.H/2))
		return
	}
	t.eye.Disabled = t.Disabled
	t.eye.Tooltip = t.eye.tipText()
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
