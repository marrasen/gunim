package widget

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// PinPad tokens.
var (
	// PinKeySize is a [PinPad] key's height, and its width where the pad has room; a key is never smaller than
	// PinKeyLeast either way.
	PinKeySize  = theme.TextRow("pinpad.key", 64)
	PinKeyLeast = theme.Length("pinpad.key.least", 48)
	PinKeyText  = theme.FontSize("pinpad.key.text", 24)
	PinKeyGap   = theme.Length("pinpad.gap", 8)
	// PinDot is the width of a dot for a digit typed, PinDotGap the room between dots, and PinDotRow the height of
	// the row of dots over the keys.
	PinDot    = theme.Length("pinpad.dot", 14)
	PinDotGap = theme.Length("pinpad.dot.gap", 16)
	PinDotRow = theme.Length("pinpad.dot.row", 40)
	// PinShake is how far the row of dots swings as it shakes for a wrong PIN.
	PinShake = theme.Length("pinpad.shake", 12)
	// PinDelete is the delete key's name for a screen reader.
	PinDelete = theme.Choice("pinpad.delete", "Delete")
)

// The keys of a [PinPad], in the order they are laid out: 1 to 9, delete, 0 and the key that commits.
const (
	pinDelete = 9
	pinZero   = 10
	pinEnter  = 11
	pinKeys   = 12
)

// PinPad takes a PIN: the digits 0 to 9, delete, and a key that hands the PIN over, in a 3 by 4 grid of large keys.
// Typed digits show as dots, never as digits, and never reach a screen reader.
//
// A dot fills with a small bounce for each digit, and empties as one is deleted. A key lights and glows under the
// finger. [PinPad.SetWaiting] shakes the row of dots, for a wrong PIN, and shows the wait in their place as a ring
// that drains; the pad takes no digits until it has. Once its digits are handed over, the pad clears them.
//
// It also takes a keyboard: the digits, on the number row or the number pad, Backspace to delete, Enter to hand the
// PIN over and Escape to clear what has been typed.
type PinPad struct {
	Control

	// Label is the pad's name for a screen reader. NewPinPad sets it to "PIN".
	Label string
	// Length is the most digits a PIN has, and the number of dots shown.
	Length int
	// Enter is the word on the key that hands the PIN over, which says what the PIN is for, as "Unlock".
	Enter string
	// OnCommit runs as the commit key or Enter hands over the digits typed, with them. The pad has cleared them by
	// then. It runs only with one digit or more.
	OnCommit func(pin string, u *gunim.UI) gunim.Intent

	digits []byte
	// dots fill from 0 to 1 as a digit is typed; lit lights each key while it is pressed.
	dots []*anim.Float
	lit  [pinKeys]*anim.Float
	// shake swings the dots for a wrong PIN; wait runs from 1 to 0 over the wait, on the clock, and dim fades the
	// keys while it does.
	shake, wait, dim *anim.Float
	waiting          bool
	endWait          func()
	// until is when a wait set before the pad was mounted ends, and span how long it was set for: the pad starts
	// the wait's clock as it is first laid out.
	until time.Time
	span  time.Duration
	click Click[int]
	// pressed is the key a press is on, or -1.
	pressed int
	keys    [pinKeys]geom.Rect
	dotRow  geom.Rect
	size    geom.Size
	texts   [pinKeys]shapedText
	ring    []paint.RibbonPoint
}

// NewPinPad returns a pad for a PIN of up to length digits, with a key saying enter that hands it over.
func NewPinPad(length int, enter string) *PinPad {
	p := &PinPad{
		Control: newControl(),
		Label:   "PIN",
		Length:  max(length, 1),
		Enter:   enter,
		shake:   anim.NewFloat(0),
		wait:    anim.NewFloat(0),
		dim:     anim.NewFloat(0),
		pressed: -1,
	}
	p.wait.RealTime()
	p.Add(p.shake, p.wait, p.dim)
	for i := range p.lit {
		p.lit[i] = anim.NewFloat(0)
		p.Add(p.lit[i])
	}
	return p
}

// Len returns how many digits have been typed.
func (p *PinPad) Len() int { return len(p.digits) }

// Waiting reports whether the pad waits after a wrong PIN, and takes no digits.
func (p *PinPad) Waiting() bool { return p.waiting }

// SetWaiting shakes the row of dots, for a wrong PIN, and holds the pad for d: it takes no digits, and a ring in
// place of the dots drains over d. A d of 0 or less ends a wait. Before the pad is laid out, or with a nil u, the
// pad holds for d from now with no shake, and shows the rest of the wait once it is laid out.
func (p *PinPad) SetWaiting(d time.Duration, u *gunim.UI) {
	if p.endWait != nil {
		p.endWait()
		p.endWait = nil
	}
	p.wipe(u)
	if d <= 0 {
		p.stopWaiting(u)
		return
	}
	p.waiting = true
	p.wait.Jump(1)
	p.wait.Animate(0, anim.Tween{Duration: d, Ease: anim.Linear})
	if u == nil || !p.laid {
		p.dim.Jump(1)
		p.until, p.span = time.Now().Add(d), d
		return
	}
	th := u.Theme()
	p.dim.Animate(1, Quick.Get(th))
	// The dots swing out a dot's width and settle back, as a head shakes no: Bounce set off from rest at the speed
	// that carries it that far. A spring at its target stays at rest, so it heads for a hair from it, which shows
	// as nothing.
	anim.Fling(p.shake, 2*math.Pi/max(Bounce.Get(th).Response, 0.05), anim.Decay{})
	p.shake.Retarget(1e-4, Bounce.Get(th))
	u.Cue(gunim.CueClose, p)
	p.endWait = u.After(d, p.stopWaiting)
	u.Invalidate()
}

// stopWaiting ends the wait: the keys come back, and take digits again.
func (p *PinPad) stopWaiting(u *gunim.UI) {
	p.endWait = nil
	p.waiting = false
	p.until = time.Time{}
	if u == nil || !p.laid {
		p.dim.Jump(0)
		p.wait.Jump(0)
		return
	}
	p.dim.Animate(0, Settle.Get(u.Theme()))
	p.wait.Animate(0, Quick.Get(u.Theme()))
	u.Invalidate()
}

// wipe wipes the digits typed, in place, so they leave no copy behind, and empties their dots: on th's Settle
// once the pad is laid out, and at once before then or with no u.
func (p *PinPad) wipe(u *gunim.UI) {
	clear(p.digits)
	p.digits = p.digits[:0]
	for _, d := range p.dots {
		if u == nil || !p.laid {
			d.Jump(0)
		} else {
			d.Animate(0, Settle.Get(u.Theme()))
		}
	}
}

// Clear wipes the digits typed, and sends no intent.
func (p *PinPad) Clear(u *gunim.UI) {
	p.wipe(u)
	u.Invalidate()
}

// press does what key i does: types a digit, deletes one, or hands the PIN over.
func (p *PinPad) press(i int, u *gunim.UI) {
	if p.waiting || p.Disabled {
		return
	}
	th := u.Theme()
	switch i {
	case pinDelete:
		if n := len(p.digits); n > 0 {
			p.digits[n-1] = 0
			p.digits = p.digits[:n-1]
			p.dot(n-1).Animate(0, Settle.Get(th))
			u.Cue(gunim.CueTick, p)
		}
	case pinEnter:
		if len(p.digits) == 0 {
			return
		}
		pin := string(p.digits)
		p.wipe(u)
		act(u, p, gunim.CuePress, p.OnCommit, pin)
	default:
		if len(p.digits) >= p.Length {
			return
		}
		digit := byte('0' + (i+1)%10)
		if i == pinZero {
			digit = '0'
		}
		p.digits = append(p.digits, digit)
		p.dot(len(p.digits)-1).Animate(1, Bounce.Get(th))
		u.Cue(gunim.CueTick, p)
	}
	u.Invalidate()
}

// dot returns the spring of the i-th dot, making it where the length grew.
func (p *PinPad) dot(i int) *anim.Float {
	for len(p.dots) <= i {
		d := anim.NewFloat(0)
		p.dots = append(p.dots, d)
		p.Add(d)
	}
	return p.dots[i]
}

// keyAt returns the key under pos, or -1.
func (p *PinPad) keyAt(pos geom.Point) int {
	for i, r := range p.keys {
		if r.Contains(pos) {
			return i
		}
	}
	return -1
}

// Handle implements [gunim.Handler].
func (p *PinPad) Handle(e input.Event, u *gunim.UI) bool {
	p.showTip(e, u, p)
	th := u.Theme()
	if p.Disabled {
		return p.handleDisabled(e, u, nil)
	}
	switch e := e.(type) {
	case input.PointerDown:
		if e.Button != input.ButtonPrimary {
			return false
		}
		i := p.keyAt(e.Pos)
		if i < 0 {
			return true
		}
		p.held, p.pressed = true, i
		p.click.Press(e, i)
		if !p.touch.waiting {
			p.lit[i].Animate(1, Quick.Get(th))
		}
	case input.PointerUp:
		if !p.held {
			return false
		}
		p.held = false
		if p.pressed >= 0 {
			p.lit[p.pressed].Animate(0, Settle.Get(th))
		}
		p.pressed = -1
		if i := p.keyAt(e.Pos); p.click.Release(e, i) && i >= 0 {
			p.press(i, u)
		}
	case input.KeyPress:
		return p.key(e, u)
	case input.TextInput:
		// The digits come as keys; their text has been dealt with.
		return len(e.Text) == 1 && e.Text[0] >= '0' && e.Text[0] <= '9'
	default:
		return p.ringFollows(e, th)
	}
	return true
}

// key takes a key from a keyboard, and reports whether it was one of the pad's.
func (p *PinPad) key(e input.KeyPress, u *gunim.UI) bool {
	if e.Mods&^input.ModShift != 0 && e.Key != input.KeyBackspace {
		return false
	}
	var i int
	switch {
	case e.Key >= input.Key1 && e.Key <= input.Key9 && e.Mods == 0:
		i = int(e.Key - input.Key1)
	case e.Key >= input.KeyKP1 && e.Key <= input.KeyKP9:
		i = int(e.Key - input.KeyKP1)
	case e.Key == input.Key0 && e.Mods == 0, e.Key == input.KeyKP0:
		i = pinZero
	case e.Key == input.KeyBackspace:
		i = pinDelete
	case e.Key == input.KeyEnter || e.Key == input.KeyKPEnter:
		i = pinEnter
	case e.Key == input.KeyEscape:
		if len(p.digits) == 0 {
			return false
		}
		p.Clear(u)
		return true
	default:
		return false
	}
	// The key lights as it would under a finger.
	p.lit[i].Retarget(1, Quick.Get(u.Theme()))
	p.lit[i].Animate(0, Settle.Get(u.Theme()))
	p.press(i, u)
	return true
}

// Layout implements [gunim.Node]: the row of dots over a 3 by 4 grid of keys, as large as the room allows, between
// PinKeyLeast and PinKeySize tall and up to half as wide again.
func (p *PinPad) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	th := f.Theme
	if !p.laid {
		for i, d := range p.dots {
			d.Jump(anim.OnOff(i < len(p.digits)))
		}
		p.dim.Jump(anim.OnOff(p.waiting))
	}
	if u := f.UI(); u != nil && p.waiting && p.endWait == nil && !p.until.IsZero() {
		// A wait set before the pad was mounted starts its clock now, with what is left of it.
		left := max(time.Until(p.until), 0)
		p.wait.Jump(float32(left) / float32(max(p.span, 1)))
		p.wait.Animate(0, anim.Tween{Duration: left, Ease: anim.Linear})
		p.endWait = u.After(left, p.stopWaiting)
		p.until = time.Time{}
	}
	p.follow(th)
	gap, least, want := PinKeyGap.Get(th), PinKeyLeast.Get(th), PinKeySize.Get(th)
	row := PinDotRow.Get(th)
	h := want
	if bounded(c.Max.H) {
		h = min(want, (c.Max.H-row-4*gap)/4)
	}
	h = max(h, least)
	w := 1.5 * want
	if bounded(c.Max.W) {
		w = min(w, (c.Max.W-2*gap)/3)
	}
	w = max(w, least)
	p.size = c.Constrain(geom.Sz(3*w+2*gap, row+gap+4*h+3*gap))
	// The grid sits in the middle of what the pad was given.
	x0 := (p.size.W - (3*w + 2*gap)) / 2
	p.dotRow = geom.Rc(x0, 0, 3*w+2*gap, row)
	for i := range pinKeys {
		col, line := i%3, i/3
		p.keys[i] = geom.Rc(x0+float32(col)*(w+gap), row+gap+float32(line)*(h+gap), w, h)
	}
	return p.size
}

// Paint implements [gunim.Node].
func (p *PinPad) Paint(pt *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	defer p.faint(pt, box)()
	th := f.Theme
	p.paintRing(pt, geom.Rect{Max: box.Point()}, ButtonRadius.Get(th), th)
	p.paintDots(pt, th)
	func() {
		defer Faint(pt, box, p.dim.Value())()
		radius := min(ButtonRadius.Get(th), p.keys[0].Size().H/2)
		for i, r := range p.keys {
			p.paintKey(pt, th, i, r, radius)
		}
	}()
}

// paintDots draws a dot for each digit the PIN may have, filled for each typed, or in their place the ring of a wait.
func (p *PinPad) paintDots(pt *paint.Painter, th *theme.Live) {
	r := p.dotRow
	c := r.Center()
	if w := p.wait.Value(); p.waiting || w > 0.002 {
		d := r.Size().H * 0.7
		width := HoldRing.Get(th)
		col := Ink.Get(th)
		col.A = uint8(float32(col.A) * min(max(p.dim.Value(), 0), 1))
		p.ring = arc(p.ring[:0], c, d/2-width/2, width, w, col)
		pt.Ribbon(p.ring)
		return
	}
	size, gap := PinDot.Get(th), PinDotGap.Get(th)
	n := p.Length
	x := c.X - (float32(n)*size+float32(n-1)*gap)/2 + p.shake.Value()*PinShake.Get(th)
	ink, edge := Ink.Get(th), FieldBorder.Get(th)
	for i := range n {
		at := geom.Rc(x+float32(i)*(size+gap), c.Y-size/2, size, size)
		pt.RRectStroke(at, size/2, paint.Solid(FieldFill.Get(th)), paint.Stroke{Width: 1.5, Color: edge})
		if i < len(p.dots) {
			if t := p.dots[i].Value(); t > 0.01 {
				s := size * t
				pt.RRect(geom.Rc(at.Center().X-s/2, c.Y-s/2, s, s), s/2, paint.Solid(ink))
			}
		}
	}
}

// paintKey draws key i in r: its fill, lit while pressed, the glow of a finger on it, and its digit, icon or word.
func (p *PinPad) paintKey(pt *paint.Painter, th *theme.Live, i int, r geom.Rect, radius float32) {
	lit := min(max(p.lit[i].Value(), 0), 1)
	rest, hover, ink := ButtonFill, ButtonHover, Ink
	if i == pinEnter {
		rest, hover, ink = ButtonPrimaryFill, ButtonPrimaryHover, ButtonPrimaryInk
	}
	if lit > 0.001 {
		s := 1 - 2*ButtonSquash.Get(th)*lit
		defer pt.Push(paint.Scale(s, r.Center()))()
	}
	pt.RRect(r, radius, paint.Solid(anim.Mix(anim.ColorCodec, rest.Get(th), hover.Get(th), lit)))
	if i == p.pressed || p.touch.shine.Value() > 0.01 && r.Contains(p.touch.at) {
		p.paintGlow(pt, r, radius, th)
	}
	col := ink.Get(th)
	if i == pinDelete {
		s := IconSize.Get(th)
		drawIcon(pt, th, icon.Delete, geom.Rc(r.Center().X-s/2, r.Center().Y-s/2, s, s), col, 1)
		return
	}
	label, face, size := strconv.Itoa((i+1)%10), faceIn(BoldFont, th), PinKeyText.Get(th)
	if i == pinZero {
		label = "0"
	}
	if i == pinEnter {
		label, face, size = p.Enter, faceIn(BoldFont, th), TextSize.Get(th)
	}
	run := p.texts[i].shape(face, label, size)
	run = fitRun(run, &p.texts[i], r.Size().W-8)
	run.Paint(pt, geom.Pt(r.Center().X-run.Advance/2, r.Center().Y-run.Height()/2), col)
}

// keyName is key i's name for a screen reader.
func (p *PinPad) keyName(i int) string {
	switch i {
	case pinDelete:
		return PinDelete.Default()
	case pinEnter:
		return p.Enter
	case pinZero:
		return "0"
	}
	return strconv.Itoa(i + 1)
}

// Access implements [gunim.Accessible]: the pad, its count of digits typed and never the digits, and a part for each
// key.
func (p *PinPad) Access() access.Info {
	info := access.Info{
		Role:  access.RoleGroup,
		Name:  p.accessName(p.Label),
		Value: fmt.Sprintf("%d/%d", len(p.digits), p.Length),
		State: p.accessState(),
	}
	for i, r := range p.keys {
		k := access.Info{Role: access.RoleButton, Name: p.keyName(i), Bounds: r, Actions: []string{access.ActionPress}}
		if p.waiting || p.Disabled {
			k.State = access.StateDisabled
		}
		info.Parts = append(info.Parts, k)
	}
	return info
}

// AccessAct implements [gunim.AccessActor]: a press on a key does what the key does.
func (p *PinPad) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress || r.Part < 0 || r.Part >= pinKeys || p.Disabled || p.waiting {
		return false
	}
	p.press(r.Part, u)
	return true
}
