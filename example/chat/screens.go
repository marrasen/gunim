package main

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/widget"
)

// narrowW is the width below which the chat shows one screen at a time,
// as on a phone: the projects and the conversations, or the one open.
const narrowW = 700

// screens lays out the rail, the sidebar and the open conversation side
// by side, or below narrowW one screen at a time: the rail and the
// sidebar across the whole width, or the conversation, which slides in
// from the right over them as one is picked, and back out as the back
// button takes the user back to the list.
type screens struct {
	anim.Group
	rail, sidebar, right gunim.Node
	// open slides from 0, the list showing, to 1, the conversation.
	open *anim.Float
	// narrow says the last layout showed one screen at a time.
	narrow bool
}

func newScreens() *screens {
	s := &screens{open: anim.NewFloat(0)}
	s.Add(s.open)
	return s
}

// show slides the conversation in over the list, or back out.
func (s *screens) show(conversation bool, u *gunim.UI) {
	to := float32(0)
	if conversation {
		to = 1
	}
	s.open.Animate(to, widget.Settle.Get(u.Theme()))
	u.Invalidate()
}

// showing reports whether the conversation shows, or is on its way in.
func (s *screens) showing() bool { return s.open.Target() == 1 }

// Children implements [gunim.Composite].
func (s *screens) Children() []gunim.Node { return []gunim.Node{s.rail, s.sidebar, s.right} }

// Layout implements [gunim.Node].
func (s *screens) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	size := c.Max
	rail, sidebar, right := kids.At(0), kids.At(1), kids.At(2)
	s.narrow = size.W < narrowW
	side := s.sidebar.(*panel)
	if !s.narrow {
		side.width = sidebarW
		rail.Layout(gunim.Tight(geom.Sz(railW, size.H)))
		rail.Place(geom.Point{})
		sidebar.Layout(gunim.Tight(geom.Sz(sidebarW, size.H)))
		sidebar.Place(geom.Pt(railW, 0))
		right.Layout(gunim.Tight(geom.Sz(max(0, size.W-railW-sidebarW), size.H)))
		right.Place(geom.Pt(railW+sidebarW, 0))
		return size
	}
	x := -s.open.Value() * size.W
	side.width = max(0, size.W-railW)
	rail.Layout(gunim.Tight(geom.Sz(railW, size.H)))
	rail.Place(geom.Pt(x, 0))
	sidebar.Layout(gunim.Tight(geom.Sz(side.width, size.H)))
	sidebar.Place(geom.Pt(x+railW, 0))
	right.Layout(gunim.Tight(size))
	right.Place(geom.Pt(x+size.W, 0))
	return size
}

// Paint implements [gunim.Node].
func (s *screens) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for k := range kids.All {
		k.Paint(p)
	}
}

// narrowOnly shows its child only while the screens are narrow, when on
// is set, or only while they are wide otherwise: the back button to the
// list for a phone, and the button that pops a conversation out into a
// window of its own for the desktop.
type narrowOnly struct {
	s     *screens
	on    bool
	child gunim.Node
}

// Children implements [gunim.Composite].
func (n *narrowOnly) Children() []gunim.Node { return []gunim.Node{n.child} }

func (n *narrowOnly) shown() bool { return n.s.narrow == n.on }

// Layout implements [gunim.Node].
func (n *narrowOnly) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	k := kids.At(0)
	if !n.shown() {
		k.Layout(gunim.Tight(geom.Size{}))
		k.Place(geom.Point{})
		return geom.Size{}
	}
	s := k.Layout(c)
	k.Place(geom.Point{})
	return s
}

// Paint implements [gunim.Node].
func (n *narrowOnly) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	if n.shown() {
		kids.At(0).Paint(p)
	}
}
