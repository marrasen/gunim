package widget

import (
	"image/color"
	"math"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// StatusSwitch tokens.
var (
	// StatusSwitchHeight is its height, and StatusSwitchPadding the room
	// at its ends.
	StatusSwitchHeight  = theme.Length("statusswitch.height", 28)
	StatusSwitchPadding = theme.Length("statusswitch.padding", 10)
	// StatusSwitchTrack is its switch's track: its width and height.
	StatusSwitchTrackW = theme.Length("statusswitch.track.width", 24)
	StatusSwitchTrackH = theme.Length("statusswitch.track.height", 14)
	// StatusSwitchFill is how strongly the state's colour fills the
	// chip, as alpha.
	StatusSwitchFill = theme.Length("statusswitch.fill", 0x1c)
)

// StatusSwitch says how something is, in words in a rounded chip of the
// state's colour, with a switch at its end that changes it: a
// connection, a sync, a server. Its words say only the state, the same
// whatever a click will do; a click, or Space or Enter, flips the
// switch and runs OnToggle with the state asked for. While Busy, as for
// something on its way on, the knob breathes.
//
// The knob slides as Checked changes, and the chip's colour fades from one
// state's to the next. A key held down does not flip it again and again.
type StatusSwitch struct {
	Control
	// Label is the state in words, as "connected" or "23 ms".
	Label string
	// Ink is the state's colour: of the words, the chip's fill and the
	// switch. Unset, it is the theme's [Ink].
	Ink theme.Token[color.NRGBA]
	// Checked says the switch is on, and Busy that its knob breathes.
	Checked, Busy bool
	// Bare leaves the words out, and the chip's fill shows only under
	// the pointer: the switch alone, its colour saying the state, for
	// where room is short.
	Bare bool
	// Extra, when set, is drawn between the words and the switch, such
	// as a compact [LiveGraph] of what goes over a connection.
	Extra gunim.Node
	// OnToggle runs on the UI goroutine when the switch is flipped, with
	// the state asked for; a non-nil result is sent to the application
	// as the switch's intent.
	OnToggle func(on bool, u *gunim.UI) gunim.Intent

	knob, tone *anim.Float
	// now is the state's colour, and was the one before, which tone
	// fades from.
	now, was color.NRGBA
	inked    bool
	size     geom.Size
	text     shapedText
	laid     bool
}

// NewStatusSwitch returns a status switch saying label.
func NewStatusSwitch(label string) *StatusSwitch {
	s := &StatusSwitch{Control: newControl(), Label: label, knob: anim.NewFloat(0), tone: anim.NewFloat(1)}
	s.Add(s.knob, s.tone)
	return s
}

// Children implements [gunim.Composite]: Extra, when set.
func (s *StatusSwitch) Children() []gunim.Node {
	if s.Extra == nil {
		return nil
	}
	return []gunim.Node{s.Extra}
}

// target is the state's colour now.
func (s *StatusSwitch) target(th *theme.Live) color.NRGBA {
	if s.Ink.Key() != "" {
		return s.Ink.Get(th)
	}
	return Ink.Get(th)
}

// Layout implements [gunim.Node]: as wide as its words, Extra and
// switch, or as it is made wider; the switch at the end.
func (s *StatusSwitch) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	th := f.Theme
	s.follow(th)
	to := float32(0)
	if s.Checked {
		to = 1
	}
	if !s.laid {
		s.knob.Jump(to)
	} else {
		s.knob.Animate(to, Settle.Get(th))
	}
	// The colour fades from the last state's to this one's.
	if ink := s.target(th); !s.inked || ink != s.now {
		s.was = ink
		if s.inked {
			s.was = s.colour()
			s.tone.Jump(0)
			s.tone.Animate(1, Settle.Get(th))
		}
		s.now, s.inked = ink, true
	}
	s.laid = true
	pad, tw := StatusSwitchPadding.Get(th), StatusSwitchTrackW.Get(th)
	w := pad + tw + pad
	var extra geom.Size
	if s.Extra != nil {
		k := kids.At(0)
		extra = k.Layout(gunim.Constraints{Max: geom.Sz(c.Max.W, StatusSwitchHeight.Get(th))})
		if extra.W > 0 {
			w += extra.W + 8
		}
	}
	if !s.Bare {
		w += s.text.shape(faceIn(Font, th), s.Label, BadgeSize.Get(th)).Advance + 8
	}
	s.size = c.Constrain(geom.Sz(w, StatusSwitchHeight.Get(th)))
	if s.Extra != nil {
		k := kids.At(0)
		k.Place(geom.Pt(s.size.W-pad-tw-8-extra.W, (s.size.H-extra.H)/2))
	}
	return s.size
}

// colour is the chip's colour now, fading from the state before's.
func (s *StatusSwitch) colour() color.NRGBA {
	return anim.Mix(anim.ColorCodec, s.was, s.now, min(max(s.tone.Value(), 0), 1))
}

// Paint implements [gunim.Node].
func (s *StatusSwitch) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, kids gunim.Children) {
	defer s.faint(p, box)()
	th := f.Theme
	ink := s.colour()
	lit := min(max(s.hover.Value(), s.ring.Value()), 1)
	r := geom.Rect{Max: box.Point()}
	radius := box.H / 2
	fill := ink
	fill.A = uint8(min(max(StatusSwitchFill.Get(th), 0), 255))
	if s.Bare {
		fill.A = 0
	}
	fill = anim.Mix(anim.ColorCodec, fill, ChipHover.Get(th), lit*0.6)
	if s.held {
		fill = ChipHover.Get(th)
	}
	p.RRect(r, radius, paint.Solid(fill))
	s.paintRing(p, r, radius, th)
	pad, tw, thh := StatusSwitchPadding.Get(th), StatusSwitchTrackW.Get(th), StatusSwitchTrackH.Get(th)
	x := box.W - pad - tw
	if s.Bare && s.Extra == nil {
		x = (box.W - tw) / 2
	}
	if !s.Bare {
		run := s.text.shape(faceIn(Font, th), s.Label, BadgeSize.Get(th))
		run.PaintFaded(p, geom.Pt(pad, box.H/2-run.Height()/2), ink, max(x-pad-8, 0), TextFade.Get(th))
	}
	if s.Extra != nil {
		kids.At(0).Paint(p)
	}
	s.paintSwitch(p, f, geom.Rc(x, box.H/2-thh/2, tw, thh), ink)
}

// paintSwitch draws the switch in track: filled in the state's colour
// while on, an outline in it while off, its knob breathing while Busy.
func (s *StatusSwitch) paintSwitch(p *paint.Painter, f gunim.Frame, track geom.Rect, ink color.NRGBA) {
	th := f.Theme
	t := min(max(s.knob.Value(), 0), 1)
	fill := ink
	fill.A = uint8(float32(0xd0) * t)
	h := track.Size().H
	p.RRect(track, h/2, paint.Solid(fill))
	p.RRectStroke(track.Inset(geom.Uniform(0.5)), h/2, paint.Fill{}, paint.Stroke{Width: 1, Color: ink})
	d := h - 4
	kx := track.Min.X + 2 + (track.Size().W-4-d)*t
	kc := anim.Mix(anim.ColorCodec, ink, CardFill.Get(th), t)
	if s.Busy {
		// Breathing, a second each way.
		phase := math.Mod(float64(f.Now.UnixMilli())/1000, 2)
		if phase > 1 {
			phase = 2 - phase
		}
		kc.A = uint8(110 + 145*phase)
		f.RedrawAt(f.Now.Add(time.Second / 30))
	}
	p.RRect(geom.Rc(kx, track.Min.Y+2, d, d), d/2, paint.Solid(kc))
}

// Handle implements [gunim.Handler]: a click, or Space or Enter with
// the keyboard on it, flips the switch.
func (s *StatusSwitch) Handle(e input.Event, u *gunim.UI) bool {
	s.showTip(e, u, s)
	th := u.Theme()
	if s.Disabled {
		return s.handleDisabled(e, u, nil)
	}
	if s.ringFollows(e, th) {
		_, lost := e.(input.FocusLost)
		return !lost
	}
	switch e := e.(type) {
	case input.PointerEnter:
		s.hover.Animate(1, Quick.Get(th))
	case input.PointerLeave:
		s.hover.Animate(0, Settle.Get(th))
		s.held = false
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		s.held = true
	case input.PointerUp:
		if !s.held {
			return false
		}
		s.held = false
		if over(e.Pos, s.size) == 0 {
			s.flip(u)
		}
	case input.KeyPress:
		if e.Mods != 0 || e.Key != input.KeySpace && e.Key != input.KeyEnter && e.Key != input.KeyKPEnter {
			return false
		}
		if !e.Repeat {
			s.flip(u)
		}
	default:
		return false
	}
	u.Invalidate()
	return true
}

// flip runs OnToggle with the state asked for.
func (s *StatusSwitch) flip(u *gunim.UI) {
	if s.OnToggle == nil {
		return
	}
	on := !s.Checked
	act0(u, s, gunim.CuePress, func(u *gunim.UI) gunim.Intent { return s.OnToggle(on, u) })
}

// Access implements [gunim.Accessible]: a switch, named by its words.
func (s *StatusSwitch) Access() access.Info {
	st := s.accessState() | access.StateCheckable
	if s.Checked {
		st |= access.StateChecked
	}
	return access.Info{Role: access.RoleSwitch, Name: s.accessName(s.Label), State: st, Actions: []string{access.ActionPress}}
}
