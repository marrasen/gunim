package filemanager

import (
	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// RegisterViews registers the window half on w: the themes, the browser that holds
// every area, and the dialogs.
func RegisterViews(w *gunim.Window) {
	w.RegisterTheme(darkTheme())
	w.RegisterTheme(lightTheme())
	gunim.RegisterView(w, "browser", func(Shell) *browser { return newBrowser() }, (*browser).setShell)
	registerListing(w)
	registerSidebar(w)
	registerPathBar(w)
	registerPreview(w)
	registerOps(w)
	registerStatus(w)
	registerDialogs(w)
	registerIcons(w)
	registerPalette(w)
	registerSpeed(w)
	registerRowsLeft(w)
	registerDnd(w)
	registerProps(w)
}

// root is the window's root: the background, with the views stacked on
// it. It tells the application when the window has the keyboard back.
type root struct {
	widget.Surface
}

// Handle implements [gunim.Handler].
func (r *root) Handle(e input.Event, u *gunim.UI) bool {
	if _, ok := e.(input.WindowFocusGained); ok {
		u.Send(r, WindowFocused{})
	}
	return false
}

// browser is the whole window: the title bar, the path bar, the sidebar,
// the listing and the preview, the progress panel, the status bar, and
// the toasts over them.
type browser struct {
	shell Shell
	// icons are the icons Windows shows for items, by their keys, as the
	// app half sent them.
	icons   map[string]SystemIcon
	shown   bool
	title   *titleBar
	path    *pathBar
	banner  *bannerView
	side    *sidebar
	listing *listingArea
	preview *previewPane
	ops     *opsPanel
	status  *statusBar
	toasts  *widget.Toasts
	palette *filesPalette
	split   *widget.Split
	main    *widget.Split
	page    *widget.Flex
	dnd     *dndView
}

func newBrowser() *browser {
	b := &browser{toasts: &widget.Toasts{}, icons: map[string]SystemIcon{}}
	b.title = newTitleBar(b)
	b.path = newPathBar(b)
	b.banner = newBannerView()
	b.side = newSidebar()
	b.listing = newListingArea(b)
	b.preview = newPreviewPane()
	b.ops = newOpsPanel()
	b.status = newStatusBar()
	b.palette = newFilesPalette(b)
	b.dnd = newDndView(b)
	b.main = widget.NewSplit(b.dnd.listing, b.preview)
	b.main.SetShare(0.72, nil)
	b.split = widget.NewSplit(b.dnd.side, b.main)
	b.split.Fixed = true
	b.split.SetShare(sidebarWidth, nil)
	b.split.OnMove = func(w float32) gunim.Intent { return SidebarMoved{Width: w} }
	b.page = widget.Column(b.title, b.dnd.crumbs, b.banner.fold, b.split, b.ops.fold, b.status).Grow(b.split, 1)
	b.page.Cross = widget.CrossStretch
	b.page.Gap = noGap
	return b
}

// sidebarWidth is the sidebar's width until the user moves it.
const sidebarWidth = 220

// noGap is a gap of nothing, for rows and columns packed tight.
var noGap = theme.Length("files.nogap", 0)

// setShell takes the state of the window as a whole.
func (b *browser) setShell(s Shell, u *gunim.UI) {
	was := b.shell
	b.shell = s
	if s.Light != was.Light {
		th := darkTheme()
		if s.Light {
			th = lightTheme()
		}
		u.UseTheme(th)
	}
	if s.Sidebar > 0 && !b.shown {
		b.split.SetShare(s.Sidebar, nil)
	}
	share := float32(0.72)
	if !s.ShowPreview {
		share = 1
	}
	if b.shown {
		b.main.SetShare(share, Page.Get(u.Theme()))
	} else {
		b.main.SetShare(share, nil)
	}
	b.title.setShell(s, u)
	b.side.fs, b.side.ps = s.FS, s.Paths
	b.dnd.u = u
	b.shown = true
}

// Children implements [gunim.Composite].
func (b *browser) Children() []gunim.Node { return []gunim.Node{b.page, b.toasts} }

// Layout implements [gunim.Node]: the page fills the window, and the
// toasts sit at the bottom right, over the progress panel and the status
// bar.
func (b *browser) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	page, toasts := kids.At(0), kids.At(1)
	page.Layout(gunim.Tight(c.Max))
	page.Place(geom.Point{})
	ts := toasts.Layout(gunim.Loose(geom.Sz(c.Max.W-32, c.Max.H)))
	toasts.Place(geom.Pt(c.Max.W-ts.W-16, c.Max.H-ts.H-40-b.ops.fold.height))
	return c.Max
}

// Paint implements [gunim.Node].
func (b *browser) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for k := range kids.All {
		k.Paint(p)
	}
}

// Handle implements [gunim.Handler]: going back and forward, and the keys that work anywhere in the
// window.
func (b *browser) Handle(e input.Event, u *gunim.UI) bool {
	// The mouse's side buttons, and a keyboard's Browser Back and Forward keys
	if h, ok := e.(input.HistoryStep); ok {
		cmd := CmdBack
		if h.Forward {
			cmd = CmdForward
		}
		u.Send(b, Command{Name: cmd})
		return true
	}
	k, ok := e.(input.KeyPress)
	if !ok {
		return false
	}
	if b.dnd.keys(k, u) {
		return true
	}
	ctrl, shift, alt := k.Mods.Has(input.ModControl), k.Mods.Has(input.ModShift), k.Mods.Has(input.ModAlt)
	var cmd string
	switch {
	case ctrl && k.Key == input.KeyL:
		b.path.edit(u)
		return true
	case ctrl && k.Key == input.KeyF:
		u.Focus(b.path.filter)
		return true
	case ctrl && k.Key == input.KeyP:
		b.palette.open("", u)
		return true
	case ctrl && shift && k.Key == input.KeyN:
		cmd = CmdNewFolder
	case ctrl && k.Key == input.KeyC:
		cmd = CmdCopy
	case ctrl && k.Key == input.KeyX:
		cmd = CmdCut
	case ctrl && k.Key == input.KeyV:
		cmd = CmdPaste
	case ctrl && k.Key == input.KeyZ:
		cmd = CmdUndo
	case ctrl && k.Key == input.KeyH:
		cmd = CmdHidden
	case ctrl && k.Key == input.KeyD:
		cmd = CmdPin
	case ctrl && k.Key == input.KeyW:
		cmd = CmdCloseApp
	case ctrl && k.Key == input.Key1:
		cmd = CmdViewDetails
	case ctrl && k.Key == input.Key2:
		cmd = CmdViewIcons
	case ctrl:
		return false
	case alt && k.Key == input.KeyLeft, !alt && k.Key == input.KeyBackspace:
		cmd = CmdBack
	case alt && k.Key == input.KeyRight:
		cmd = CmdForward
	case alt && k.Key == input.KeyUp:
		cmd = CmdUp
	case alt && k.Key == input.KeyHome:
		cmd = CmdHome
	case k.Key == input.KeyF5:
		cmd = CmdRefresh
	case k.Key == input.KeySpace && !shift:
		cmd = CmdViewer
	case k.Key == input.KeyF2:
		if in := b.side.editFocused(u); in != nil {
			u.Send(b, in)
			return true
		}
		cmd = CmdRename
	case shift && k.Key == input.KeyDelete:
		cmd = CmdDelete
	case k.Key == input.KeyDelete:
		cmd = CmdTrash
	default:
		return false
	}
	u.Send(b, Command{Name: cmd})
	return true
}

// focusListing gives the keyboard to the grid showing.
func (b *browser) focusListing(u *gunim.UI) {
	if b.listing.cur != nil {
		u.Focus(b.listing.cur.focusNode())
	}
}
