package filemanager

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
)

// Access implements [gunim.Accessible]: a link to the place, with its
// free space or why it cannot be read.
func (r *placeRow) Access() access.Info {
	if r.item.heading {
		return access.Info{Role: access.RoleHeading, Name: r.item.Name}
	}
	info := access.Info{Role: access.RoleLink, Name: r.item.Name, Description: r.noteText(),
		Actions: []string{access.ActionPress}}
	if r.item.current {
		info.State = access.StateSelected
	}
	return info
}

// AccessAct implements [gunim.AccessActor].
func (r *placeRow) AccessAct(req access.Request, u *gunim.UI) bool {
	switch {
	case req.Action != access.ActionPress || r.item.heading:
		return false
	case r.item.away:
		u.Send(r, Visit{FS: r.item.FS, Path: r.item.Path})
	default:
		u.Send(r, Navigate{Path: r.item.Path})
	}
	return true
}

// Access implements [gunim.Accessible].
func (t *typeTile) Access() access.Info {
	name := t.label + " file"
	switch {
	case t.tint == TintFolder:
		name = "Folder"
	case t.label == "":
		name = "File"
	}
	return access.Info{Role: access.RoleImage, Name: name}
}

// Access implements [gunim.Accessible]: a group named by what the
// operation does.
func (r *opRow) Access() access.Info { return access.Info{Role: access.RoleGroup, Name: r.title.Text} }
