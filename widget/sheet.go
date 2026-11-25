package widget

import (
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// BottomSheet tokens.
var (
	// SheetWidth is the widest a sheet grows, centred in a wider window, and SheetTop the room it leaves above
	// itself when full.
	SheetWidth = theme.Length("sheet.width", 640)
	SheetTop   = theme.Length("sheet.top", 24)
	// SheetHalf is the share of the window's height a sheet stands at when half open.
	SheetHalf = theme.Number("sheet.half", 0.5)
	// SheetPadding is the room round the handle, the title and the body.
	SheetPadding = theme.Length("sheet.padding", 16)
	// SheetHandleWidth and SheetHandleHeight size the handle along the top.
	SheetHandleWidth  = theme.Length("sheet.handle.width", 32)
	SheetHandleHeight = theme.Length("sheet.handle.height", 4)
	// SheetTitleSize is the title's text.
	SheetTitleSize = theme.FontSize("sheet.title.size", 17)
	// SheetExpand and SheetCollapse name a screen reader's actions that open a sheet full and take it back to half.
	SheetExpand   = theme.Choice("sheet.expand", "Expand")
	SheetCollapse = theme.Choice("sheet.collapse", "Collapse")
)

// sheetFling is the speed, in logical pixels a second, past which a sheet let go goes on the way it was flung.
const sheetFling = 600

// BottomSheet is a modal panel that slides up over the page from the bottom of the window, with a handle and a
// title along its top and a body under them that scrolls. It stands half open, or full where its body is taller
// than that, and as tall as its body where that is shorter. In a window wider than SheetWidth it stands centred.
//
// A drag on the handle or the title moves the sheet with the finger or the mouse, and it springs on, as it is let
// go, to the nearer of half and full, or on the way it was flung. A drag up on the body of a sheet below full opens
// the sheet full first, and the body scrolls with the rest. A drag down scrolls the body back to its top first, and
// the rest takes the sheet down. A fling down closes it from half, and so does a tap on the page above it, Escape and a phone's Back. The
// sheet keeps itself and the field being typed in above the keyboard on the screen.
//
// Mounted, as with [gunim.UI.Insert] over the window's root, it slides up, takes the keyboard and holds it until it
// closes, and then gives it back. With the keyboard on the sheet itself, Up opens it full and Down takes it back to
// half. Tab goes into the body.
type BottomSheet struct {
	anim.Group
	// Title shows along the top, under the handle, and names the sheet for a screen reader.
	Title string
	// OnDismiss runs on the UI goroutine as the user closes the sheet; a non-nil result is sent to the application
	// as the sheet's intent. The sheet closes itself.
	OnDismiss func(u *gunim.UI) gunim.Intent

	body   gunim.Node
	scroll *Scroll
	// pos is how far up the sheet stands, from the bottom of the room it keeps above the keyboard.
	pos *anim.Float
	// full says it rests full rather than half, and arrive that it is to spring up once it knows its height.
	full, arrive, laid bool
	// half and top are the heights it rests at, from the last layout, header the height of its handle and title,
	// and pad the room it keeps under its body.
	half, top, header, pad float32
	// box is the sheet's whole size, panel the panel's place in it, and keyboard how far up the keyboard
	// reaches into it, from the last layout.
	box      geom.Size
	panel    geom.Rect
	keyboard float32
	// drag follows the pointer dragging the sheet by its top, or a finger's scrolls the body left over: whether one
	// is under way, where the sheet and the pointer stood as it began, and the trail for its speed.
	drag struct {
		on, scrolls bool
		from, start float32
		trail       anim.Trail
		moved       float32
		// lift says a finger's drag up on the body opens the sheet full before the body scrolls.
		lift bool
	}
	// place is where the sheet stood in the window as it was last painted: the shade covers the whole window.
	place   windowPlace
	outside Clicker
	ring    *anim.Float
	titled  shapedText
	ell     shapedText
	line    shapedText
	// live is the window's live theme, kept from Layout for Access, which has no frame.
	live *theme.Live
}

// th is the window's live theme, or nil before the first layout.
func (s *BottomSheet) th() *theme.Live { return s.live }

// NewBottomSheet returns a sheet titled title holding body, which may be nil. Mount it, and it slides up half
// open.
func NewBottomSheet(title string, body gunim.Node) *BottomSheet {
	s := &BottomSheet{Title: title, body: body, pos: anim.NewFloat(0), ring: anim.NewFloat(0)}
	s.Add(s.pos, s.ring)
	return s
}

// Body returns what the sheet holds.
func (s *BottomSheet) Body() gunim.Node { return s.body }

// Expanded reports whether the sheet stands full, or is on its way there.
func (s *BottomSheet) Expanded() bool { return s.full }

// SetExpanded opens the sheet full, or takes it back to half, and sends no intent. Once laid out it springs there;
// before, it opens that way. A sheet whose body fits in half stays as tall as its body.
func (s *BottomSheet) SetExpanded(full bool, u *gunim.UI) {
	if s.full == full {
		return
	}
	s.full = full
	if !s.laid || u == nil || s.arrive {
		return
	}
	s.pos.Animate(s.rest(), Settle.Get(u.Theme()))
	u.Invalidate()
}

// Close slides the sheet away, and sends no intent.
func (s *BottomSheet) Close(u *gunim.UI) {
	if u != nil {
		u.Remove(s)
	}
}

// dismiss closes the sheet for the user, sending what OnDismiss makes.
func (s *BottomSheet) dismiss(u *gunim.UI) {
	if u.Presence(s) == gunim.Exiting {
		return
	}
	var what gunim.Intent
	if s.OnDismiss != nil {
		what = s.OnDismiss(u)
	}
	u.Remove(s)
	send(u, s, what)
}

// rest is the height the sheet rests at now.
func (s *BottomSheet) rest() float32 {
	if s.full {
		return s.top
	}
	return s.half
}

// held is the body as the sheet holds it: in a scroll, unless it scrolls itself.
func (s *BottomSheet) held() gunim.Node {
	switch b := s.body.(type) {
	case *Scroll:
		return b
	case interface{ Offset() float32 }:
		return s.body
	}
	if s.scroll == nil {
		s.scroll = NewScroll(s.body)
	}
	return s.scroll
}

// Children implements [gunim.Composite]: the body.
func (s *BottomSheet) Children() []gunim.Node {
	if s.body == nil {
		return nil
	}
	return []gunim.Node{s.held()}
}

// Modal implements [gunim.Modal].
func (s *BottomSheet) Modal() bool { return true }

// Focusable implements [gunim.Focusable]: the sheet takes the keyboard as it opens.
func (s *BottomSheet) Focusable() bool { return true }

// TakesEscape implements [gunim.EscapeTaker]: Escape and Back close the sheet.
func (s *BottomSheet) TakesEscape(*gunim.UI) bool { return true }

// DragsTouch implements [gunim.TouchDragger]: a finger on the handle or the title drags the sheet.
func (s *BottomSheet) DragsTouch() bool { return s.drag.on && !s.drag.scrolls }

// ClaimsPointer implements [gunim.PointerClaimer]: while a finger has taken the sheet down from where it rests, or
// drags it up from half, its scrolls move the sheet before the body scrolls.
func (s *BottomSheet) ClaimsPointer(geom.Point) bool {
	return s.drag.lift || s.drag.on && s.drag.scrolls && s.pos.Value() < s.rest()-0.5
}

// Overhear implements [gunim.Overhearer]. A finger's scroll mostly up on the body of a sheet below full is the
// sheet's before the body's, so the sheet opens full first. A press, by a finger or the mouse, ends a drag whose end
// went unheard.
func (s *BottomSheet) Overhear(e input.Event, u *gunim.UI) {
	if s.drag.lift && !s.drag.on {
		// The scroll the sheet claimed went to a node drawn over it: the claim ends.
		s.drag.lift = false
	}
	switch e := e.(type) {
	case input.PointerDown:
		if s.drag.on && s.drag.scrolls {
			s.letGo(e.Time, true, u)
		}
	case input.Scroll:
		if !e.Touch || e.Fling || -e.Delta.Y <= abs32(e.Delta.X) || s.drag.lift || s.top <= s.half+0.5 ||
			s.pos.Target() >= s.top-0.5 {
			return
		}
		at, ok := u.Bounds(s)
		if !ok {
			return
		}
		p := e.Pos.Sub(at.Min)
		if s.outsideAt(p) < 0 && !s.onTop(p) && !away(p) {
			s.drag.lift = true
		}
	}
}

// Transition implements [gunim.Transitioner]: the sheet slides up as it arrives, with no overshoot, and down as it goes.
func (s *BottomSheet) Transition(p gunim.Presence, f gunim.Frame) bool {
	switch p {
	case gunim.Entering:
		if !s.laid {
			s.arrive = true
		}
		return !s.arrive && !s.pos.Active()
	case gunim.Exiting:
		s.endDrag()
		if s.pos.Target() != 0 {
			s.pos.Retarget(0, Settle.Get(f.Theme))
		}
		return !s.pos.Active() || s.pos.Value() <= 0.5
	case gunim.Present:
	}
	return true
}

// onTop reports whether p lies on the sheet's handle or title.
func (s *BottomSheet) onTop(p geom.Point) bool {
	r := s.panel
	return p.X >= r.Min.X && p.X < r.Max.X && p.Y >= r.Min.Y && p.Y < r.Min.Y+s.header
}

// Handle implements [gunim.Handler].
func (s *BottomSheet) Handle(e input.Event, u *gunim.UI) bool {
	th := u.Theme()
	switch e := e.(type) {
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return true
		}
		switch {
		case s.outsideAt(e.Pos) > 0:
			s.outside.Press(e, 1)
		case s.onTop(e.Pos):
			s.startDrag(e.Pos.Y, e.Time, false)
		}
		return true
	case input.PointerMove:
		if s.drag.on && !s.drag.scrolls {
			s.drag.trail.Add(geom.Pt(0, e.Pos.Y), e.Time)
			s.moveTo(s.drag.from+s.drag.start-e.Pos.Y, u)
		}
		return true
	case input.PointerUp:
		if s.drag.on && !s.drag.scrolls {
			s.letGo(e.Time, away(e.Pos), u)
			return true
		}
		if s.outside.Release(e, s.outsideAt(e.Pos)) {
			s.dismiss(u)
		}
		return true
	case input.Scroll:
		// A finger on the page above the sheet moves nothing.
		if e.Touch && !e.Fling && e.Delta.Y != 0 && (s.drag.on || s.outsideAt(e.Pos) < 0) {
			if !s.drag.on {
				s.startDrag(0, e.Time, true)
			}
			s.drag.moved -= e.Delta.Y
			s.drag.trail.Add(geom.Pt(0, -s.drag.moved), e.Time)
			s.moveTo(s.drag.from+s.drag.moved, u)
			if s.drag.lift && s.pos.Value() >= s.top-0.5 {
				// Full: the body scrolls with the rest of the drag.
				s.drag.lift = false
			}
		}
		return true
	case input.ScrollEnd:
		if s.drag.on && s.drag.scrolls {
			s.letGo(e.Time, false, u)
		}
		return false
	case input.KeyPress:
		switch {
		case e.Key == input.KeyEscape && e.Mods == 0:
			s.dismiss(u)
		case e.Key == input.KeyUp && e.Mods == 0 && u.Focused() == gunim.Node(s):
			s.SetExpanded(true, u)
		case e.Key == input.KeyDown && e.Mods == 0 && u.Focused() == gunim.Node(s):
			s.SetExpanded(false, u)
		default:
			// A modal keeps the keys pressed in it, and the window's menus shut.
			return e.Key == input.KeyF10 || e.Mods.Has(input.ModAlt)
		}
		return true
	case input.AltTapped:
		return true
	case input.FocusRing:
		s.ring.Animate(ringTo(e), RingFade.Get(th))
		u.Invalidate()
		return true
	case input.FocusLost:
		s.ring.Animate(0, RingFade.Get(th))
	}
	return false
}

// outsideAt is 1 where p lies off the panel, and -1 on it, or at [input.Away], where a finger that went on to
// scroll lets go.
func (s *BottomSheet) outsideAt(p geom.Point) int {
	if away(p) {
		return -1
	}
	if p.Y < s.panel.Min.Y || p.X < s.panel.Min.X || p.X >= s.panel.Max.X {
		return 1
	}
	return -1
}

// startDrag starts a drag of the sheet from where it stands, by the pointer at y, or by a finger's scrolls.
func (s *BottomSheet) startDrag(y float32, at time.Time, scrolls bool) {
	s.drag.on, s.drag.scrolls = true, scrolls
	s.drag.from, s.drag.start, s.drag.moved = s.pos.Value(), y, 0
	s.drag.trail = anim.Trail{}
	s.drag.trail.Add(geom.Pt(0, y), at)
	s.pos.Jump(s.drag.from)
}

// endDrag forgets a drag under way.
func (s *BottomSheet) endDrag() { s.drag.on, s.drag.scrolls, s.drag.lift = false, false, false }

// moveTo puts the sheet at height h, as dragged: past full it gives less the further it goes.
func (s *BottomSheet) moveTo(h float32, u *gunim.UI) {
	if over := h - s.top; over > 0 {
		h = s.top + over/(1+over/40)
	}
	s.pos.Jump(max(h, 0))
	u.Invalidate()
}

// letGo ends a drag let go at t: the sheet springs on with the pointer's speed to the rest it is nearer, or on the
// way it was flung, or closes. A drag called off springs back.
func (s *BottomSheet) letGo(t time.Time, off bool, u *gunim.UI) {
	th := u.Theme()
	// Down the screen is positive, for the pointer and for a finger's scrolls alike.
	v := s.drag.trail.Velocity(t).Y
	s.endDrag()
	at := s.pos.Value()
	to := s.rest()
	switch {
	case off:
	case v > sheetFling && at <= s.half+1, at < s.half/2 && v >= -sheetFling:
		anim.Fling(s.pos, -v, anim.Decay{})
		s.dismiss(u)
		return
	case v > sheetFling:
		s.full = false
	case v < -sheetFling:
		s.full = s.top > s.half
	default:
		s.full = s.top > s.half && at > (s.half+s.top)/2
	}
	if !off {
		to = s.rest()
	}
	anim.Fling(s.pos, -v, anim.Decay{})
	s.pos.Retarget(to, Settle.Get(th))
	u.Invalidate()
}

// headerHeight is the height of the handle and the title.
func (s *BottomSheet) headerHeight(th *theme.Live) float32 {
	pad := SheetPadding.Get(th)
	h := pad + SheetHandleHeight.Get(th) + pad
	if s.Title != "" {
		h += s.line.shape(faceIn(BoldFont, th), "Ag", SheetTitleSize.Get(th)).Height() + pad
	}
	return h
}

// Layout implements [gunim.Node]: the sheet fills the room it is given, for the shade over the page, and stands its
// panel at the bottom, above the keyboard.
func (s *BottomSheet) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	th := f.Theme
	size := fillOr(c, geom.Sz(390, 700))
	safe, kb := s.place.under(f)
	s.box, s.keyboard, s.live = size, kb, th
	room := max(size.H-kb, 0)
	w := min(size.W, SheetWidth.Get(th))
	x := (size.W - w) / 2
	s.header = s.headerHeight(th)
	pad := SheetPadding.Get(th)
	// Under the keyboard the system's bars are covered already.
	s.pad = pad
	if kb <= 0 {
		s.pad += safe
	}
	var natural float32
	if kids.Len() > 0 {
		natural = kids.At(0).Layout(gunim.Constraints{Max: geom.Sz(w, 0)}).H
	}
	natural += s.header + s.pad
	s.top = max(min(natural, room-SheetTop.Get(th)), min(natural, s.header))
	s.half = min(s.top, max(room*SheetHalf.Get(th), s.header))
	rest := s.rest()
	switch {
	case s.arrive:
		s.arrive = false
		s.pos.Jump(0)
		s.pos.Animate(rest, Settle.Get(th))
	case !s.drag.on && f.UI() != nil && f.UI().Presence(s) != gunim.Exiting && s.pos.Target() != rest && s.laid:
		// The window, the keyboard or the body changed what it rests at.
		s.pos.Animate(rest, Settle.Get(th))
	case !s.laid:
		s.pos.Jump(rest)
	}
	s.laid = true
	h := max(s.pos.Value(), 0)
	s.panel = geom.Rc(x, room-h, w, h+(size.H-room))
	if kids.Len() > 0 {
		view := max(max(h, rest)-s.header-s.pad, 0)
		body := kids.At(0)
		body.Layout(gunim.Tight(geom.Sz(w, view)))
		body.Place(geom.Pt(x, room-h+s.header))
	}
	return size
}

// Paint implements [gunim.Node]: the shade over the page, as deep as the sheet has come up, then the panel.
func (s *BottomSheet) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	th := f.Theme
	s.place.note(f, s)
	h := s.pos.Value()
	if h <= 0.5 {
		return
	}
	shade := min(h/max(s.half, 1), 1)
	dim := Scrim.Get(th)
	// The shade covers the system's bars too, which the root keeps the sheet clear of.
	p.RRect(geom.Rect{Max: box.Point()}.Union(s.place.window(box)), 0, paint.Solid(paint.Faded(dim, shade)))
	radius := DialogRadius.Get(th)
	panel := s.panel
	// The ground runs on under the system's bars and past the window's edge, so only the top corners round.
	ground := panel
	ground.Max.Y = box.H + s.place.runsUnder(f) + radius
	p.RRect(ground, radius, paint.Solid(DialogFill.Get(th)))
	p.RRectStroke(ground, radius, paint.Fill{}, paint.Stroke{Width: 1, Color: DialogBorder.Get(th)})
	pad := SheetPadding.Get(th)
	hw, hh := SheetHandleWidth.Get(th), SheetHandleHeight.Get(th)
	handle := geom.Rc(panel.Min.X+(panel.Size().W-hw)/2, panel.Min.Y+pad, hw, hh)
	p.RRect(handle, hh/2, paint.Solid(Placeholder.Get(th)))
	FocusRing(p, handle.Inset(geom.Uniform(-4)), hh/2+4, s.ring.Value(), th)
	if s.Title != "" {
		run := s.titled.shape(faceIn(BoldFont, th), s.Title, SheetTitleSize.Get(th))
		room := panel.Size().W - 2*pad
		if room > 0 {
			run = fitRun(run, &s.ell, room)
			run.Paint(p, geom.Pt(panel.Min.X+pad, handle.Max.Y+pad), Ink.Get(th))
		}
	}
	if kids.Len() > 0 {
		kids.At(0).Paint(p)
	}
}

// Access implements [gunim.Accessible]: a modal dialog named by its title, expanded while full.
func (s *BottomSheet) Access() access.Info {
	info := access.Info{Role: access.RoleDialog, Name: s.Title, State: access.StateModal,
		Actions: []string{access.ActionClose}}
	if s.top > s.half+0.5 && s.th() != nil {
		info.State |= access.StateExpandable
		if s.full {
			info.State |= access.StateExpanded
			info.Actions = append(info.Actions, SheetCollapse.Get(s.th()))
		} else {
			info.Actions = append(info.Actions, access.ActionOpen, SheetExpand.Get(s.th()))
		}
	}
	return info
}

// AccessAct implements [gunim.AccessActor]: Close closes the sheet, and Open and Expand open it full, and Collapse
// takes it back to half.
func (s *BottomSheet) AccessAct(r access.Request, u *gunim.UI) bool {
	th := u.Theme()
	switch r.Action {
	case access.ActionClose:
		s.dismiss(u)
	case access.ActionOpen, SheetExpand.Get(th):
		s.SetExpanded(true, u)
	case SheetCollapse.Get(th):
		s.SetExpanded(false, u)
	default:
		return false
	}
	return true
}
