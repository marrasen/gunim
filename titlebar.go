package gunim

import (
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
)

// A TitleBar is the title bar the engine gives a chromeless window: a node that is the window's [Caption], holds its
// buttons, and shows the window's title.
type TitleBar interface {
	Node
	SetTitle(title string)
}

// newTitleBar makes the title bar a chromeless window gets, or is nil; see RegisterTitleBar.
var newTitleBar func() TitleBar

// RegisterTitleBar sets how the engine makes the title bar it gives a chromeless window. Package widget registers its
// own as it is imported. Without one, windows keep the system's title bar and frame.
func RegisterTitleBar(maker func() TitleBar) { newTitleBar = maker }

// windowFrame is the top of a chromeless window's tree when the engine gives the window a title bar: the bar across
// the top, and the application's root below it, or under it with [WindowOptions.UnderTitleBar]. The bar steps aside
// while a node of the application's is a [MaximizeButton], so the application draws its own title bar, and while the
// window fills its monitor.
type windowFrame struct {
	u   *UI
	bar TitleBar
	// shown says the last layout showed the bar.
	shown bool
	// over is how much of the application's top the bar covers, added to its [Frame.Safe]: the bar's height while
	// the application draws under a bar that shows, and zero otherwise.
	over float32
}

// giveTitleBar puts a window frame at the top of the window's tree, with bar across it. The application's root keeps
// its ID, so views mount into it as before.
func (u *UI) giveTitleBar(bar TitleBar) {
	app := u.root
	fr := &windowFrame{u: u, bar: bar}
	fs := &state{node: fr, presence: Present}
	u.index[fr] = fs
	app.parent = fs
	fs.kids = []*state{app}
	u.root = fs
	u.InsertAt(fr, 0, bar)
	u.titleBar = bar
}

// showsBar reports whether the frame shows its title bar: while no node of the application's is a MaximizeButton, and
// the window does not fill its monitor.
func (fr *windowFrame) showsBar(app *state) bool {
	if fs, ok := fr.u.w.dw.(driver.FullScreener); ok && fs.FullScreen() {
		return false
	}
	return !hasButtons(app)
}

// hasButtons reports whether s or a node beneath it, still there, is a MaximizeButton.
func hasButtons(s *state) bool {
	if s.presence == Exiting {
		return false
	}
	if _, ok := s.node.(MaximizeButton); ok {
		return true
	}
	for _, k := range s.kids {
		if hasButtons(k) {
			return true
		}
	}
	return false
}

// Layout implements [Node]: the bar across the top, as tall as it asks, and the application below it, or the
// application over all of it while the bar steps aside or the application draws under it.
func (fr *windowFrame) Layout(c Constraints, _ Frame, kids Children) geom.Size {
	size := c.Max
	var bar, app Child
	for k := range kids.All {
		if k.Node() == Node(fr.bar) {
			bar = k
		} else if k.n.id == Root {
			app = k
		}
	}
	top := float32(0)
	fr.shown = bar.n != nil && app.n != nil && fr.showsBar(app.n)
	if bar.n != nil {
		if fr.shown {
			top = bar.Layout(Constraints{Min: geom.Sz(size.W, 0), Max: size}).H
		} else {
			bar.Layout(Tight(geom.Size{}))
		}
		bar.Place(geom.Point{})
	}
	fr.over = 0
	if fr.u.underBar {
		fr.over, top = top, 0
	}
	if app.n != nil {
		app = fr.under(app)
		app.Layout(Tight(geom.Sz(size.W, max(0, size.H-top))))
		app.Place(geom.Pt(0, top))
	}
	return size
}

// under is the application's root with the part of it the bar covers added to its frame's Safe.
func (fr *windowFrame) under(app Child) Child {
	app.f.Safe.Top += fr.over
	return app
}

// Paint implements [Node]: the application, then the bar over it.
func (fr *windowFrame) Paint(p *paint.Painter, _ Frame, _ geom.Size, kids Children) {
	var bar Child
	for k := range kids.All {
		if k.Node() == Node(fr.bar) {
			bar = k
			continue
		}
		fr.under(k).Paint(p)
	}
	if fr.shown {
		bar.Paint(p)
	}
}
