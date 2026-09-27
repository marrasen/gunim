package main

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
)

// navNames name the path bar's buttons by their commands.
var navNames = map[string]string{CmdBack: "Back", CmdForward: "Forward", CmdUp: "Up"}

// Access implements [gunim.Accessible].
func (b *navButton) Access() access.Info {
	info := access.Info{Role: access.RoleButton, Name: navNames[b.cmd], Actions: []string{access.ActionPress}}
	if !b.on {
		info.State = access.StateDisabled
	}
	return info
}

// AccessAct implements [gunim.AccessActor].
func (b *navButton) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress || !b.on {
		return false
	}
	u.Send(b, Command{Name: b.cmd})
	return true
}

// Access implements [gunim.Accessible]: the folders of the path.
func (c *crumbBar) Access() access.Info { return access.Info{Role: access.RoleGroup, Name: "Path"} }

// Access implements [gunim.Accessible]: a link to the folder, or for the
// folder showing, its name.
func (n *crumb) Access() access.Info {
	if n.last {
		return access.Info{Role: access.RoleLabel, Name: n.name}
	}
	return access.Info{Role: access.RoleLink, Name: n.name, Actions: []string{access.ActionPress}}
}

// AccessAct implements [gunim.AccessActor].
func (n *crumb) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress || n.last {
		return false
	}
	u.Send(n, Navigate{Path: n.path})
	return true
}

// Access implements [gunim.Accessible]: a link to the place, with its
// free space or why it cannot be read.
func (r *placeRow) Access() access.Info {
	info := access.Info{Role: access.RoleLink, Name: r.item.Name, Description: r.noteText(),
		Actions: []string{access.ActionPress}}
	if r.item.current {
		info.State = access.StateSelected
	}
	return info
}

// AccessAct implements [gunim.AccessActor].
func (r *placeRow) AccessAct(req access.Request, u *gunim.UI) bool {
	if req.Action != access.ActionPress {
		return false
	}
	u.Send(r, Navigate{Path: r.item.Path})
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
