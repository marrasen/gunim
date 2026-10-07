package widget

import (
	"image/color"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// Segmented tokens.
var (
	// SegmentedHeight is a segmented control's height.
	SegmentedHeight = theme.Length("segmented.height", 28)
	// SegmentedPadding is the room either side of an option's icon and label.
	SegmentedPadding = theme.Length("segmented.padding", 12)
)

// Segmented is a row of options in a rounded track, one of them chosen, with a pill that springs to the chosen one.
//
// A click on an option chooses it. With focus, Left and Right choose the option beside the chosen one, and Home and
// End the first and the last. Every option is as wide as the widest.
type Segmented struct {
	anim.Group
	// Labels and Icons are the options, in order: an icon, a label or both. The longer of the two sets how many
	// options there are.
	Labels []string
	Icons  []*icon.Icon
	// IconSize, when set, is the icons' size in place of [IconSize].
	IconSize theme.Token[float32]
	// Track, when set, fills the track in place of [FieldFill].
	Track theme.Token[color.NRGBA]
	// OnChange turns the option chosen into an intent for the application.
	OnChange func(i int) gunim.Intent
	// KeepFocus leaves the keyboard where it is when the control is clicked; Tab still reaches it.
	KeepFocus bool

	selected int
	// pill is the pill's place, in options from the first.
	pill *anim.Float
	ring *anim.Float
	// hot is the option under the pointer, or -1.
	hot    int
	laid   bool
	shaped []shapedText
	// width is each option's width, and size the control's, from the last layout.
	width float32
	size  geom.Size
}

// NewSegmented returns a segmented control of labels, the first chosen.
func NewSegmented(labels ...string) *Segmented {
	s := &Segmented{Labels: labels, pill: anim.NewFloat(0), ring: anim.NewFloat(0), hot: -1}
	s.Add(s.pill, s.ring)
	return s
}

// Len returns how many options there are.
func (s *Segmented) Len() int { return max(len(s.Labels), len(s.Icons)) }

// Selected returns the chosen option.
func (s *Segmented) Selected() int { return s.selected }

// SetSelected chooses option i without an intent. Call it from a view's update function; the pill springs to it. Before
// the control is mounted, as a view builds it, u may be nil: the pill starts on i.
func (s *Segmented) SetSelected(i int, u *gunim.UI) {
	if i < 0 || i >= s.Len() || i == s.selected {
		return
	}
	s.selected = i
	if u == nil {
		return
	}
	if s.laid {
		s.pill.Animate(float32(i), Bounce.Get(u.Theme()))
	}
	u.Invalidate()
}

// choose chooses option i and tells the application.
func (s *Segmented) choose(i int, u *gunim.UI) {
	if i < 0 || i >= s.Len() || i == s.selected {
		return
	}
	s.SetSelected(i, u)
	u.Cue(gunim.CueSelect, s)
	if s.OnChange != nil {
		u.Send(s, s.OnChange(i))
	}
}

// Focusable implements [gunim.Focusable].
func (s *Segmented) Focusable() bool { return true }

// FocusOnPress implements [gunim.PressFocuser].
func (s *Segmented) FocusOnPress() bool { return !s.KeepFocus }

// at returns the option at x in the control's space, or -1.
func (s *Segmented) at(x float32) int {
	if s.width <= 0 || x < 0 {
		return -1
	}
	if i := int(x / s.width); i < s.Len() {
		return i
	}
	return -1
}

// Handle implements [gunim.Handler].
func (s *Segmented) Handle(e input.Event, u *gunim.UI) bool {
	th := u.Theme()
	switch e := e.(type) {
	case input.PointerEnter:
		s.hot = s.at(e.Pos.X)
	case input.PointerMove:
		s.hot = s.at(e.Pos.X)
	case input.PointerLeave:
		s.hot = -1
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		s.choose(s.at(e.Pos.X), u)
	case input.KeyPress:
		if e.Mods.Has(input.ModControl) || e.Mods.Has(input.ModAlt) {
			return false
		}
		switch e.Key {
		case input.KeyLeft:
			s.choose(s.selected-1, u)
		case input.KeyRight:
			s.choose(s.selected+1, u)
		case input.KeyHome:
			s.choose(0, u)
		case input.KeyEnd:
			s.choose(s.Len()-1, u)
		default:
			return false
		}
	case input.FocusRing:
		s.ring.Animate(ringTo(e), Quick.Get(th))
	case input.FocusLost:
		s.ring.Animate(0, Settle.Get(th))
	default:
		return false
	}
	u.Invalidate()
	return true
}

// icon returns option i's icon, or nil.
func (s *Segmented) icon(i int) *icon.Icon {
	if i < 0 || i >= len(s.Icons) {
		return nil
	}
	return s.Icons[i]
}

// label returns option i's label, or "".
func (s *Segmented) label(i int) string {
	if i < 0 || i >= len(s.Labels) {
		return ""
	}
	return s.Labels[i]
}

// iconSize is the size the control draws its icons at.
func (s *Segmented) iconSize(th *theme.Live) float32 {
	if s.IconSize.Key() != "" {
		return s.IconSize.Get(th)
	}
	return IconSize.Get(th)
}

// content returns the width of option i's icon and label, side by side.
func (s *Segmented) content(i int, th *theme.Live) float32 {
	w := float32(0)
	if l := s.label(i); l != "" {
		w = s.shaped[i].shape(faceIn(Font, th), l, TextSize.Get(th)).Advance
	}
	if s.icon(i) != nil {
		if w > 0 {
			w += IconGap.Get(th)
		}
		w += s.iconSize(th)
	}
	return w
}

// Layout implements [gunim.Node].
func (s *Segmented) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	n := s.Len()
	if len(s.shaped) != n {
		s.shaped = make([]shapedText, n)
	}
	widest := float32(0)
	for i := range n {
		widest = max(widest, s.content(i, th))
	}
	s.width = widest + 2*SegmentedPadding.Get(th)
	if !s.laid {
		s.pill.Jump(float32(s.selected))
		s.laid = true
	}
	s.size = c.Constrain(geom.Sz(s.width*float32(n), SegmentedHeight.Get(th)))
	// Given another width, the options share it: squeezed into less room,
	// or spread across more, never past the track.
	if n > 0 {
		s.width = s.size.W / float32(n)
	}
	return s.size
}

// Paint implements [gunim.Node].
func (s *Segmented) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	h := box.H
	track := geom.Rect{Max: box.Point()}
	focusRing(p, track, h/2, s.ring.Value(), th)
	fill := FieldFill.Get(th)
	if s.Track.Key() != "" {
		fill = s.Track.Get(th)
	}
	p.RRect(track, h/2, paint.Solid(fill))
	at := s.pill.Value()
	p.RRect(geom.Rc(at*s.width+2, 2, s.width-4, h-4), (h-4)/2, paint.Solid(Accent.Get(th)))

	faint, strong := Placeholder.Get(th), ButtonStrongInk.Get(th)
	for i := range s.Len() {
		// The option the pill is on reads on the pill; the one under the pointer lights up.
		near := 1 - min(max(at-float32(i), float32(i)-at), 1)
		ink := faint
		if i == s.hot {
			ink = Ink.Get(th)
		}
		ink = anim.Mix(anim.ColorCodec, ink, strong, near)
		x := float32(i)*s.width + (s.width-s.content(i, th))/2
		if ic := s.icon(i); ic != nil {
			size := s.iconSize(th)
			paintIcon(p, th, ic, geom.Rc(x, (h-size)/2, size, size), ink, 1)
			x += size + IconGap.Get(th)
		}
		if l := s.label(i); l != "" {
			run := s.shaped[i].run
			run.Paint(p, geom.Pt(x, (h-run.Height())/2), ink)
		}
	}
}

// Access implements [gunim.Accessible]: a group with a button for each option, the chosen one checked.
func (s *Segmented) Access() access.Info {
	info := access.Info{Role: access.RoleGroup, Active: s.selected + 1}
	for i := range s.Len() {
		name := s.label(i)
		if name == "" {
			name = iconName(s.icon(i))
		}
		part := access.Info{Role: access.RoleButton, Name: name, State: access.StateCheckable,
			Actions: []string{access.ActionPress}, Bounds: geom.Rc(float32(i)*s.width, 0, s.width, s.size.H)}
		if i == s.selected {
			part.State |= access.StateChecked
		}
		info.Parts = append(info.Parts, part)
	}
	return info
}

// AccessAct implements [gunim.AccessActor]: pressing an option chooses it.
func (s *Segmented) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress || r.Part < 0 || r.Part >= s.Len() {
		return false
	}
	s.choose(r.Part, u)
	return true
}
