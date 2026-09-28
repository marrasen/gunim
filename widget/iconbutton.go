package widget

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
)

// IconButton is a square button showing only an icon, such as a toolbar's Refresh. Its fill stays clear until the
// pointer comes over it; it squashes when pressed and grows a ring when focused, as [Button] does.
//
// Its Tooltip says what it does: a popup shows it once the pointer rests on the button, and a screen reader reads
// it as the button's name.
type IconButton struct {
	Button
	Tooltip string

	tip tipper
}

// NewIconButton returns a button showing ic, with tooltip saying what it does.
func NewIconButton(ic *icon.Icon, tooltip string) *IconButton {
	b := &IconButton{Button: *NewButton(""), Tooltip: tooltip}
	b.Icon, b.ghost, b.self = ic, true, b
	return b
}

// Handle implements [gunim.Handler].
func (b *IconButton) Handle(e input.Event, u *gunim.UI) bool {
	b.tip.handle(e, u, b, b.Tooltip, tipDelay)
	return b.Button.Handle(e, u)
}

// Access implements [gunim.Accessible].
func (b *IconButton) Access() access.Info {
	name := b.Tooltip
	if name == "" {
		name = iconName(b.Icon)
	}
	return access.Info{Role: access.RoleButton, Name: name, State: activeState(b.Active),
		Actions: []string{access.ActionPress}}
}
