package main

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/markdown"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// registerViews is the window half: the chat view, its patches and the themes.
func registerViews(w *gunim.Window) {
	w.RegisterTheme(darkTheme())
	w.RegisterTheme(lightTheme())
	gunim.RegisterView(w, "chat", buildChat, (*chatView).set)
	gunim.RegisterPatch(w, "chat", func(v *chatView, t Typing, u *gunim.UI) { v.setTyping(t.Who, u) })
}

// Widths of the rail and the sidebar.
const (
	railW    = 68
	sidebarW = 248
)

// TimelineSpacing is the room between the timeline's rows.
var TimelineSpacing = theme.Length("chat.timeline.spacing", 0)

// chatView is the whole window: the project rail, the sidebar of conversations, and the open conversation.
type chatView struct {
	root *widget.Flex

	rail        *rail
	projectName *widget.Label
	convs       *widget.List

	title    *widget.Label
	link     *widget.Button
	linkBar  *linkBar
	timeline *slot
	list     *widget.VirtualList
	typing   *widget.Label
	reply    *replyBar
	composer *widget.TextArea

	// items holds the timeline's rows by key, for building them.
	items    map[widget.Key]Item
	current  string
	last     widget.Key
	draftSeq int
}

func buildChat(Chat) *chatView {
	v := &chatView{items: map[widget.Key]Item{}, draftSeq: -1}

	v.rail = &rail{hover: -1}
	v.projectName = widget.NewLabel("")
	v.projectName.Face, v.projectName.Size = widget.BoldFont, sidebarTitle
	v.convs = widget.NewList()
	v.convs.OnClick = func(k widget.Key) gunim.Intent { return ConversationChosen{ID: string(k)} }
	side := widget.Column(v.projectName, v.convs)
	side.Cross = widget.CrossStretch
	sideTheme := theme.Make("chat.side", theme.Set(widget.ListSpacing, 2))
	sidebar := &panel{child: widget.NewThemed(widget.NewPad(side), sideTheme), fill: SidebarFill, width: sidebarW}

	v.title = widget.NewLabel("")
	v.title.Face, v.title.Size = widget.BoldFont, sidebarTitle
	hash := widget.NewIcon(icon.Hash, "Conversation")
	v.link = widget.NewButton("Online")
	v.link.Icon, v.link.Ghost, v.link.On = icon.Wifi, true, LinkToggled{}
	themeButton := widget.NewIconButton(icon.SunMoon, "Switch theme")
	themeButton.On = ThemeToggled{}
	spacer := widget.NewSpacer()
	header := widget.Row(hash, v.title, spacer, v.link, themeButton).Grow(spacer, 1)
	header.Cross = widget.CrossCenter

	v.linkBar = &linkBar{open: anim.NewFloat(0)}
	v.linkBar.Add(v.linkBar.open)
	v.list = v.newList()
	v.timeline = &slot{child: v.list}

	v.typing = widget.NewLabel(" ")
	v.typing.Color, v.typing.Size = Faint, SmallText
	v.reply = &replyBar{open: anim.NewFloat(0)}
	v.reply.Add(v.reply.open)
	v.composer = widget.NewTextArea()
	v.composer.Rows = 2
	v.composer.OnSubmit = func(s string) gunim.Intent { return Submitted{Text: s} }
	v.composer.OnChange = func(s string) gunim.Intent { return Drafted{Text: s} }
	send := widget.NewIconButton(icon.SendHorizontal, "Send")
	send.OnActivate(func(u *gunim.UI) { u.Send(send, Submitted{Text: v.composer.Text()}) })
	box := widget.Row(v.composer, send).Grow(v.composer, 1)
	box.Cross = widget.CrossEnd
	composer := &composerBox{child: widget.Column(v.reply, box)}
	composer.child.Cross = widget.CrossStretch

	top := widget.NewPad(header)
	bottom := widget.Column(v.typing, composer)
	bottom.Cross = widget.CrossStretch
	bottomPad := widget.NewPad(bottom)
	bottomPad.Padding = composerPad
	main := widget.Column(top, v.linkBar, v.timeline, bottomPad).Grow(v.timeline, 1)
	main.Cross, main.Gap = widget.CrossStretch, zeroGap
	pane := &panel{child: main, fill: PaneFill}

	v.root = widget.Row(v.rail, sidebar, pane).Grow(pane, 1)
	v.root.Cross, v.root.Gap = widget.CrossStretch, zeroGap
	return v
}

var (
	sidebarTitle = theme.Length("chat.title.size", 16)
	zeroGap      = theme.Length("chat.gap.none", 0)
	composerPad  = theme.Insets("chat.composer.pad", geom.Insets{Top: 0, Right: 16, Bottom: 16, Left: 16})
)

// newList makes an empty timeline, which starts at its end and stays there as messages arrive.
func (v *chatView) newList() *widget.VirtualList {
	l := widget.NewVirtualList(func(k widget.Key) gunim.Node { return newMsgRow(v.items[k], v.jump) })
	l.StickToEnd = true
	l.Estimate = 44
	l.Spacing = TimelineSpacing
	return l
}

// jump scrolls the timeline to the message id and flashes it.
func (v *chatView) jump(id string, u *gunim.UI) {
	v.list.ScrollToKey(widget.Key(id), u)
	if n, ok := v.list.Row(widget.Key(id)); ok {
		n.(*msgRow).Flash()
	}
}

// set shows the state s.
func (v *chatView) set(s Chat, u *gunim.UI) {
	v.rail.set(s.Projects, s.Project, u)
	if s.Project < len(s.Projects) {
		v.projectName.SetText(s.Projects[s.Project].Name)
	}
	widget.Sync(v.convs, u, s.Conversations,
		func(c Conversation) widget.Key { return widget.Key(c.ID) },
		func(c Conversation) *convRow { return newConvRow(c, c.ID == s.Current) },
		func(r *convRow, c Conversation, u *gunim.UI) { r.set(c, c.ID == s.Current, u) })
	v.title.SetText(s.Title)
	v.setLink(s.Link, u)
	v.reply.set(s.Replying, s.Editing != "", u)

	if s.Current != v.current {
		// A new conversation gets a new timeline, which opens at its end.
		v.current = s.Current
		v.list = v.newList()
		v.timeline.swap(v.list, u)
		v.items = map[widget.Key]Item{}
		v.last = ""
	}
	keys := make([]widget.Key, len(s.Items))
	items := make(map[widget.Key]Item, len(s.Items))
	for i, it := range s.Items {
		k := widget.Key(it.Key)
		keys[i], items[k] = k, it
		if old, ok := v.items[k]; ok && old != it {
			if n, built := v.list.Row(k); built {
				n.(*msgRow).set(it, u)
			}
		}
	}
	v.items = items
	v.list.SetKeys(keys, u)
	if n := len(keys); n > 0 && keys[n-1] != v.last {
		// The user's own message brings the timeline back to its end.
		if v.last != "" && s.Items[n-1].Mine {
			v.list.ScrollToEnd(widget.Quick.Get(u.Theme()))
		}
		v.last = keys[n-1]
	}

	if s.Draft.Seq != v.draftSeq {
		v.draftSeq = s.Draft.Seq
		v.composer.SetText(s.Draft.Text)
		u.Focus(v.composer)
	}
}

// setTyping shows who is typing, or nobody.
func (v *chatView) setTyping(who string, u *gunim.UI) {
	if who == "" {
		v.typing.SetText(" ")
	} else {
		v.typing.SetText(who + " is typing…")
	}
	u.Invalidate()
}

// setLink shows the state of the connection, in the header's button and the bar under it.
func (v *chatView) setLink(l Link, u *gunim.UI) {
	switch l {
	case Online:
		v.link.SetLabel("Online")
		v.link.Icon = icon.Wifi
	case Offline:
		v.link.SetLabel("Offline")
		v.link.Icon = icon.WifiOff
	case Reconnecting:
		v.link.SetLabel("Connecting")
		v.link.Icon = icon.Wifi
	}
	v.linkBar.set(l, u)
}

// Children implements [gunim.Composite].
func (v *chatView) Children() []gunim.Node { return []gunim.Node{v.root} }

// Layout implements [gunim.Node].
func (v *chatView) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	kid.Layout(gunim.Tight(c.Max))
	kid.Place(geom.Point{})
	return c.Max
}

// Paint implements [gunim.Node].
func (v *chatView) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}

// panel fills its box with a colour behind its child, and is width wide when width is set.
type panel struct {
	child gunim.Node
	fill  theme.Token[color.NRGBA]
	width float32
}

// Children implements [gunim.Composite].
func (p *panel) Children() []gunim.Node { return []gunim.Node{p.child} }

// Layout implements [gunim.Node].
func (p *panel) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	size := c.Max
	if p.width > 0 {
		size.W = p.width
	}
	kid := kids.At(0)
	kid.Layout(gunim.Tight(size))
	kid.Place(geom.Point{})
	return size
}

// Paint implements [gunim.Node].
func (p *panel) Paint(pt *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	pt.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(p.fill.Get(f.Theme)))
	kids.At(0).Paint(pt)
}

// slot holds one child at a time, and swaps it for another at once.
type slot struct{ child gunim.Node }

// Children implements [gunim.Composite].
func (s *slot) Children() []gunim.Node { return []gunim.Node{s.child} }

// swap puts n in place of the child.
func (s *slot) swap(n gunim.Node, u *gunim.UI) {
	u.Remove(s.child)
	s.child = n
	u.Insert(s, n)
}

// Layout implements [gunim.Node]: the child fills the slot, and a child leaving is left out.
func (s *slot) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	for kid := range kids.All {
		kid.Layout(gunim.Tight(c.Max))
		kid.Place(geom.Point{})
	}
	return c.Max
}

// Paint implements [gunim.Node].
func (s *slot) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	for kid := range kids.All {
		if kid.Node() == s.child {
			kid.Paint(p)
		}
	}
}

// composerBox is the message box with the reply bar over it. Escape in it drops a reply or an edit.
type composerBox struct {
	child *widget.Flex
}

// Children implements [gunim.Composite].
func (b *composerBox) Children() []gunim.Node { return []gunim.Node{b.child} }

// Layout implements [gunim.Node].
func (b *composerBox) Layout(c gunim.Constraints, _ gunim.Frame, kids gunim.Children) geom.Size {
	kid := kids.At(0)
	size := kid.Layout(gunim.Constraints{Min: geom.Sz(c.Max.W, 0), Max: geom.Sz(c.Max.W, c.Max.H)})
	kid.Place(geom.Point{})
	return size
}

// Paint implements [gunim.Node].
func (b *composerBox) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	kids.At(0).Paint(p)
}

// Handle implements [gunim.Handler].
func (b *composerBox) Handle(e input.Event, u *gunim.UI) bool {
	if k, ok := e.(input.KeyPress); ok && k.Key == input.KeyEscape {
		u.Send(b, Cancelled{})
		return true
	}
	return false
}

// linkBar is the strip that opens under the header while the connection is down.
type linkBar struct {
	anim.Group
	open *anim.Float
	link Link
	run  text.Run
	msg  string
}

func (b *linkBar) set(l Link, u *gunim.UI) {
	b.link = l
	switch l {
	case Offline:
		b.msg = "You are offline. Messages you send wait here and go when the connection is back."
	case Reconnecting:
		b.msg = "Connecting…"
	}
	b.open.Animate(map[bool]float32{false: 0, true: 1}[l != Online], widget.Settle.Get(u.Theme()))
	u.Invalidate()
}

// Layout implements [gunim.Node].
func (b *linkBar) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	b.run = widget.Font.Get(f.Theme).Shape(b.msg, SmallText.Get(f.Theme))
	return geom.Sz(c.Max.W, 30*max(0, b.open.Value()))
}

// Paint implements [gunim.Node].
func (b *linkBar) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	if box.H < 0.5 {
		return
	}
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: min(1, b.open.Value()), Clip: true})()
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(WarnFill.Get(f.Theme)))
	b.run.Paint(p, geom.Pt(16, (box.H-b.run.Height())/2), widget.Ink.Get(f.Theme))
}

// replyBar is the strip over the message box that says what the next message replies to, or that it edits one.
type replyBar struct {
	anim.Group
	open    *anim.Float
	msg     string
	run     text.Run
	editing bool
}

func (b *replyBar) set(q Quote, editing bool, u *gunim.UI) {
	b.editing = editing
	switch {
	case editing:
		b.msg = "Editing a message. Escape stops."
	case q.ID != "":
		b.msg = "Replying to " + q.Author + ": " + firstLine(markdown.Plain(q.Text))
	}
	b.open.Animate(map[bool]float32{false: 0, true: 1}[editing || q.ID != ""], widget.Quick.Get(u.Theme()))
	u.Invalidate()
}

// Layout implements [gunim.Node].
func (b *replyBar) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	b.run = widget.Font.Get(f.Theme).Shape(b.msg, SmallText.Get(f.Theme))
	return geom.Sz(c.Max.W, 30*max(0, b.open.Value()))
}

// Paint implements [gunim.Node].
func (b *replyBar) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	if box.H < 0.5 {
		return
	}
	th := f.Theme
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: min(1, b.open.Value()), Clip: true})()
	r := geom.Rc(0, 2, box.W, box.H-6)
	p.RRect(r, 6, paint.Solid(QuoteFill.Get(th)))
	p.RRect(geom.Rc(0, 2, 3, box.H-6), 1.5, paint.Solid(widget.Accent.Get(th)))
	b.run.Paint(p, geom.Pt(12, r.Min.Y+(r.Size().H-b.run.Height())/2), Faint.Get(th))
}

// Handle implements [gunim.Handler]: a click on the bar drops the reply or the edit.
func (b *replyBar) Handle(e input.Event, u *gunim.UI) bool {
	if d, ok := e.(input.PointerDown); ok && d.Button == input.ButtonPrimary {
		u.Send(b, Cancelled{})
		return true
	}
	return false
}
