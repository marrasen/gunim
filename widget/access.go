package widget

import (
	"strconv"
	"strings"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
)

// What each widget tells a screen reader, and what it does when a
// screen reader asks. See [gunim.Accessible].

// Access implements [gunim.Accessible].
func (l *Label) Access() access.Info {
	return access.Info{Role: access.RoleLabel, Name: l.Text}
}

// Access implements [gunim.Accessible]. A button showing an icon alone is named by the icon; an active one reads as
// checked, as a toggle that is on.
func (b *Button) Access() access.Info {
	name := b.Label
	if name == "" {
		name = iconName(b.Icon)
	}
	return access.Info{Role: access.RoleButton, Name: name, State: activeState(b.Active),
		Actions: []string{access.ActionPress}}
}

// activeState is the state of a button that is active, as a toggle that is on, or none.
func activeState(active bool) access.State {
	if active {
		return access.StateCheckable | access.StateChecked
	}
	return 0
}

// iconName is what a screen reader calls an icon shown without words: its name in Lucide, in words.
func iconName(ic *icon.Icon) string {
	if ic == nil {
		return ""
	}
	return strings.ReplaceAll(ic.Name, "-", " ")
}

// AccessAct implements [gunim.AccessActor].
func (b *Button) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress {
		return false
	}
	b.fire(u)
	return true
}

// Access implements [gunim.Accessible].
func (t *TextField) Access() access.Info {
	value := t.Text()
	if t.Secret {
		value = ""
	}
	return access.Info{Role: access.RoleTextField, Name: t.Placeholder, Value: value, State: access.StateEditable}
}

// Access implements [gunim.Accessible].
func (a *TextArea) Access() access.Info {
	return access.Info{
		Role: access.RoleTextField, Name: a.Placeholder, Value: string(a.text),
		State: access.StateEditable | access.StateMultiline,
	}
}

// access is what a checkbox or a switch says.
func (t *toggle) access(role access.Role) access.Info {
	s := access.StateCheckable
	if t.On {
		s |= access.StateChecked
	}
	return access.Info{Role: role, Name: t.Label, State: s, Actions: []string{access.ActionPress}}
}

// Access implements [gunim.Accessible].
func (c *Checkbox) Access() access.Info { return c.access(access.RoleCheckbox) }

// AccessAct implements [gunim.AccessActor].
func (c *Checkbox) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress {
		return false
	}
	c.flip(c, u)
	return true
}

// Access implements [gunim.Accessible].
func (s *Switch) Access() access.Info { return s.access(access.RoleSwitch) }

// AccessAct implements [gunim.AccessActor].
func (s *Switch) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress {
		return false
	}
	s.flip(s, u)
	return true
}

// Access implements [gunim.Accessible].
func (s *Slider) Access() access.Info {
	return access.Info{
		Role:  access.RoleSlider,
		Value: strconv.FormatFloat(float64(s.value), 'g', 4, 32),
		Range: &access.Range{Min: float64(s.Min), Max: float64(s.Max), Value: float64(s.value), Step: float64(s.Snap)},
	}
}

// AccessAct implements [gunim.AccessActor]: a new value.
func (s *Slider) AccessAct(r access.Request, u *gunim.UI) bool {
	if !r.SetValue {
		return false
	}
	s.set(float32(r.Value), Quick.Get(u.Theme()), u)
	return true
}

// Access implements [gunim.Accessible]: a tab list with a tab for each
// title.
func (b *tabBar) Access() access.Info {
	t := b.t
	info := access.Info{Role: access.RoleTabList}
	for i, title := range t.Titles {
		part := access.Info{Role: access.RoleTab, Name: title, Actions: []string{access.ActionPress}}
		if i == t.selected {
			part.State = access.StateSelected
		}
		if i < len(t.spans) {
			part.Bounds = geom.Rc(t.spans[i][0], 0, t.spans[i][1]-t.spans[i][0], t.head)
		}
		info.Parts = append(info.Parts, part)
	}
	// The chosen tab is where the titles' focus is.
	info.Active = t.selected + 1
	return info
}

// AccessAct implements [gunim.AccessActor]: pressing a tab chooses it.
func (b *tabBar) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress || r.Part < 0 {
		return false
	}
	b.t.choose(r.Part, u)
	return true
}

// Access implements [gunim.Accessible].
func (d *Dropdown) Access() access.Info {
	info := access.Info{
		Role:    access.RoleComboBox,
		Name:    d.Label,
		State:   access.StateExpandable | access.StateHasPopup,
		Actions: []string{access.ActionPress},
	}
	if d.Selected >= 0 && d.Selected < len(d.Items) {
		info.Value = d.Items[d.Selected]
	}
	if d.IsOpen() {
		info.State |= access.StateExpanded
		// While the list is open, its highlighted item is what the
		// keys move through.
		if h := d.menu.Highlighted(); h >= 0 && h < len(d.Items) {
			info.Value = d.Items[h]
		}
	}
	return info
}

// AccessAct implements [gunim.AccessActor]: press opens the list, or
// closes it.
func (d *Dropdown) AccessAct(r access.Request, u *gunim.UI) bool {
	switch r.Action {
	case access.ActionPress, access.ActionOpen, access.ActionClose:
	default:
		return false
	}
	if d.IsOpen() {
		d.close(u)
	} else if r.Action != access.ActionClose {
		d.open(u)
	}
	return true
}

// Access implements [gunim.Accessible]: a menu with an item for each
// entry, the highlighted one active.
func (m *Menu) Access() access.Info {
	info := access.Info{Role: access.RoleMenu, Active: m.hot + 1}
	for i, s := range m.Items {
		part := access.Info{Role: access.RoleMenuItem, Name: s, Actions: []string{access.ActionPress}}
		if i == m.hot {
			part.State = access.StateSelected
		}
		part.Bounds = geom.Rc(m.card.Min.X, m.rowY(i), m.card.Size().W, m.row)
		info.Parts = append(info.Parts, part)
	}
	return info
}

// AccessAct implements [gunim.AccessActor]: pressing an item picks it.
func (m *Menu) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress || r.Part < 0 || r.Part >= len(m.Items) || m.Pick == nil {
		return false
	}
	m.Pick(r.Part, u)
	return true
}

// Access implements [gunim.Accessible].
func (t *tip) Access() access.Info { return access.Info{Role: access.RoleTooltip, Name: t.text} }

// Access implements [gunim.Accessible].
func (i *Image) Access() access.Info { return access.Info{Role: access.RoleImage, Name: i.Alt} }

// Access implements [gunim.Accessible].
func (d *Dialog) Access() access.Info {
	return access.Info{Role: access.RoleDialog, Name: d.Title, State: access.StateModal}
}

// Access implements [gunim.Accessible].
func (l *List) Access() access.Info { return access.Info{Role: access.RoleList} }

// Access implements [gunim.Accessible].
func (l *VirtualList) Access() access.Info { return access.Info{Role: access.RoleList} }

// Access implements [gunim.Accessible]: each of a list's rows is an
// item, holding what its node says.
func (r *row) Access() access.Info { return access.Info{Role: access.RoleListItem} }

// Access implements [gunim.Accessible].
func (s *Scroll) Access() access.Info { return access.Info{Role: access.RoleScrollArea} }
