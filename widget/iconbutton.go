package widget

import (
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/icon"
)

// IconButton is a square button showing only an icon, such as a toolbar's Refresh. Its fill stays clear until the
// pointer comes over it; it squashes when pressed and grows a ring when focused, as [Button] does.
//
// Its Tooltip says what it does: a popup shows it once the pointer rests on the button, and a screen reader reads
// it as the button's name.
type IconButton struct {
	Button
}

// NewIconButton returns a button showing ic, with tooltip saying what it does.
func NewIconButton(ic *icon.Icon, tooltip string) *IconButton {
	b := &IconButton{Button: *NewButton("")}
	b.Tooltip = tooltip
	b.Icon, b.Ghost, b.self = ic, true, b
	return b
}

// Access implements [gunim.Accessible].
func (b *IconButton) Access() access.Info {
	name := b.Tooltip
	if name == "" {
		name = iconName(b.Icon)
	}
	return access.Info{Role: access.RoleButton, Name: name, State: b.state(), Actions: []string{access.ActionPress}}
}
