package audioui

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/anim"
	"github.com/marrasen/gunim/audio/calibrate"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/text"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// The calibrator's sizes.
var (
	// CalibratorTarget is the widest the beat target grows.
	CalibratorTarget = theme.Length("audio.calibrator.target", 220)
	// CalibratorWidth is the widest the calibrator's column grows; it
	// sits in the middle of a wider room.
	CalibratorWidth = theme.Length("audio.calibrator.width", 520)
	// CalibratorTitle is the size of a step's title, CalibratorCount of
	// the count inside the target, and CalibratorFigure of the delay
	// found.
	CalibratorTitle  = theme.FontSize("audio.calibrator.title", 20)
	CalibratorCount  = theme.FontSize("audio.calibrator.count", 40)
	CalibratorFigure = theme.FontSize("audio.calibrator.figure", 52)
)

// Calibrator runs a [calibrate.Calibration] on screen: the listener
// taps along with a beat on a big round target, and reads the delay it
// found. It steps through the calibration's two passes, shown as steps
// at the top with a title and a line of what to do:
//
//   - Watch: a ball bounces on the target, which pulses as it lands, in
//     silence. The listener taps as it lands.
//   - Listen: the clicks sound, and the target stands still, so the
//     listener goes by ear alone. Each tap ripples out from it.
//   - Result: the delay found in large figures, how sure it is, and a
//     strip of the taps round it. The target pulses with each click as
//     the listener hears it once the delay counts, for a check by eye.
//     Try again starts over; the accept button hands the result to
//     OnAccept and stops the clicks.
//
// A ring round the target fills as a pass counts its taps; the count
// shows inside, and a countdown through the practice beats. The result
// panel and the target's new size come in on a spring.
//
// A tap is a press on the target, by the pointer or a finger, or Space
// or Enter while it has the keyboard. It counts as it goes down, with
// the time the system gave the press, since a tap in time lands as the
// finger meets the glass. Before the calibration starts, a tap starts
// it. A key held down taps once. The target takes the keyboard by Tab,
// first in the calibrator, and a ring fades in round it.
//
// The calibrator plays no sound cue of its own: the clicks are the
// only sound in the listen pass. Stop the calibration as the view goes
// away, or the clicks play on.
type Calibrator struct {
	anim.Group
	// Label names the calibrator for a screen reader. Empty means
	// "Sound delay".
	Label string
	// OnAccept runs on the UI goroutine as the listener accepts the
	// result. A non-nil result is sent to the application as the
	// calibrator's intent. The application sets the delay, as with
	// [speaker.Speaker.SetDelay], and keeps it.
	OnAccept func(r calibrate.Result, u *gunim.UI) gunim.Intent

	cal *calibrate.Calibration
	// done runs from 0 to 1 as the result comes in, and moves counts
	// its frames, for the keys of the parts it moves.
	done  *anim.Float
	moves anim.Moves
	laid  bool
	// version counts the changes of what the still parts show: a stage,
	// a tap counted, a practice beat passed.
	version uint64
	// seen is what the still parts showed at the last version.
	seen calSeen

	target *calTarget
	steps  *widget.Steady
	title  *widget.Label
	note   *widget.Label
	result *widget.Steady
	again  *widget.Button
	accept *widget.Button
	// wrapped are the labels, each carried while the version holds.
	wrapped []*widget.Steady
}

// calSeen is what the still parts of a calibrator show.
type calSeen struct {
	stage    calibrate.Stage
	counted  int
	practice int
}

// NewCalibrator returns a calibrator of c, ready to start.
func NewCalibrator(c *calibrate.Calibration) *Calibrator {
	k := &Calibrator{cal: c, done: anim.NewFloat(0)}
	k.Add(k.done)
	k.target = &calTarget{k: k, ring: anim.NewFloat(0), hover: anim.NewFloat(0)}
	k.title = widget.NewLabel("")
	k.title.Face, k.title.Size = widget.BoldFont, CalibratorTitle
	k.note = widget.NewLabel("")
	key := func(gunim.Frame) (any, bool) { return k.version, true }
	k.wrapped = []*widget.Steady{widget.NewSteady(k.title, key), widget.NewSteady(k.note, key)}
	k.steps = widget.NewSteadyPaint(k.paintSteps, key)
	k.result = widget.NewSteadyPaint(k.paintResult, func(gunim.Frame) (any, bool) {
		return [2]uint64{k.version, k.moves.Count()}, true
	})
	k.again = widget.NewButton("Start over")
	k.again.OnClick = func(u *gunim.UI) gunim.Intent {
		k.start(u.Now(), u)
		u.Focus(k.target)
		return nil
	}
	k.accept = widget.NewButton("Use")
	k.accept.Kind = widget.ButtonPrimary
	k.accept.OnClick = func(u *gunim.UI) gunim.Intent {
		k.cal.Stop()
		k.changed(u)
		if k.OnAccept == nil {
			return nil
		}
		return k.OnAccept(k.cal.Result(), u)
	}
	k.words()
	return k
}

// Calibration returns the calibration the calibrator runs.
func (k *Calibrator) Calibration() *calibrate.Calibration { return k.cal }

// Step implements [anim.Stepper]: the result panel's spring.
func (k *Calibrator) Step(dt time.Duration) bool { return k.moves.Note(k.Group.Step(dt)) }

// start starts the calibration at t, or again.
func (k *Calibrator) start(t time.Time, u *gunim.UI) {
	k.cal.Start(t)
	k.changed(u)
}

// changed redoes the words after the calibration moved on, and tells a
// screen reader of a new stage.
func (k *Calibrator) changed(u *gunim.UI) {
	was := k.seen.stage
	k.words()
	k.version++
	to := float32(0)
	if k.cal.Stage() == calibrate.Done {
		to = 1
	}
	if u != nil && k.laid {
		k.done.Animate(to, widget.Settle.Get(u.Theme()))
	} else {
		k.done.Jump(to)
	}
	if u != nil {
		if k.seen.stage != was {
			u.Announce(k.title.Text + ". " + k.note.Text)
		}
		u.Invalidate()
	}
}

// words sets the title, the note and the buttons for where the
// calibration is.
func (k *Calibrator) words() {
	c := k.cal
	k.seen.stage = c.Stage()
	k.again.Label = "Start over"
	switch c.Stage() {
	case calibrate.Ready:
		k.title.Text = "Measure the sound's delay"
		k.note.Text = "Tap along with a beat, first one you see, then one you hear. " +
			"Tap the circle, or press Space or Enter. It takes about half a minute."
	case calibrate.Watch:
		k.title.Text = "Tap as the ball lands"
		k.note.Text = "Keep to the beat. The first beats are practice."
	case calibrate.Listen:
		k.title.Text = "Tap with each click"
		k.note.Text = "Go by ear: the circle stays still. The first beats are practice."
	case calibrate.Done:
		r := c.Result()
		k.again.Label = "Try again"
		k.accept.Label = "Use " + msText(r.Delay)
		switch {
		case !r.Enough:
			k.title.Text = "Too few taps to tell"
			k.note.Text = "Tap on every beat you can, and try again."
		case !r.Sure():
			k.title.Text = "A rough measure"
			k.note.Text = "The taps lay far apart. Try again for a closer one. The circle flashes with each click, as it is now."
		default:
			k.title.Text = "The sound's delay"
			k.note.Text = "The circle flashes with each click, with this delay. They should land together."
		}
	}
}

// msText is d in whole milliseconds, with its sign.
func msText(d time.Duration) string {
	return fmt.Sprintf("%+d ms", d.Round(time.Millisecond).Milliseconds())
}

// Children implements [gunim.Composite]. The target comes first, so a
// stage it moves on is in the words laid out after it.
func (k *Calibrator) Children() []gunim.Node {
	return []gunim.Node{k.target, k.steps, k.wrapped[0], k.wrapped[1], k.result, k.again, k.accept}
}

// Access implements [gunim.Accessible]: a group, named by Label.
func (k *Calibrator) Access() access.Info {
	name := k.Label
	if name == "" {
		name = "Sound delay"
	}
	return access.Info{Role: access.RoleGroup, Name: name}
}

// The calibrator's room: the steps, gaps, the result panel and the
// buttons.
const (
	calSteps  = 28
	calGap    = 12
	calResult = 150
)

// Layout implements [gunim.Node]: a column as wide as CalibratorWidth
// at most, in the middle: the steps, the title and the note at the top,
// the buttons at the bottom, and the target in the room between, which
// the result panel shares once done.
func (k *Calibrator) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	if !k.laid {
		k.laid = true
		k.done.Jump(k.done.Target())
	}
	size := c.Max
	w := min(size.W-32, CalibratorWidth.Get(f.Theme))
	x := (size.W - w) / 2
	done := k.done.Value()
	stage := k.cal.Stage()

	// The words first, from what the target moved on to this frame: the
	// target is laid out after them, in the room they leave, but moves
	// the calibration on first.
	k.target.update(f.Present)
	y := float32(16) + f.Safe.Top
	steps := kids.At(1)
	steps.Layout(gunim.Tight(geom.Sz(w, calSteps)))
	steps.Place(geom.Pt(x, y))
	y += calSteps + calGap
	for i := 2; i <= 3; i++ {
		kid := kids.At(i)
		h := kid.Layout(gunim.Constraints{Min: geom.Sz(w, 0), Max: geom.Sz(w, size.H)}).H
		kid.Place(geom.Pt(x, y))
		y += h + 4
	}
	y += calGap - 4

	// The buttons, at the bottom: Start over while a pass runs, Try
	// again and Use once done, none before it starts.
	bottom := size.H - 16 - f.Safe.Bottom
	again, accept := kids.At(5), kids.At(6)
	var bh float32
	switch stage {
	case calibrate.Ready:
		again.Layout(gunim.Tight(geom.Size{}))
		accept.Layout(gunim.Tight(geom.Size{}))
		again.Place(geom.Pt(-1e4, 0))
		accept.Place(geom.Pt(-1e4, 0))
	case calibrate.Done:
		as := accept.Layout(gunim.Loose(geom.Sz(w, size.H)))
		gs := again.Layout(gunim.Loose(geom.Sz(w, size.H)))
		bh = max(as.H, gs.H)
		accept.Place(geom.Pt(x+w-as.W, bottom-bh))
		again.Place(geom.Pt(x+w-as.W-calGap-gs.W, bottom-bh))
	default:
		accept.Layout(gunim.Tight(geom.Size{}))
		accept.Place(geom.Pt(-1e4, 0))
		gs := again.Layout(gunim.Loose(geom.Sz(w, size.H)))
		bh = gs.H
		again.Place(geom.Pt(x+(w-gs.W)/2, bottom-bh))
	}
	if bh > 0 {
		bottom -= bh + calGap
	}

	// The result panel slides up from the buttons as it comes in, and
	// the target shrinks to leave it room.
	panel := calResult * done
	res := kids.At(4)
	res.Layout(gunim.Tight(geom.Sz(w, calResult)))
	res.Place(geom.Pt(x, bottom-panel))
	bottom -= panel
	t := kids.At(0)
	t.Layout(gunim.Tight(geom.Sz(w, max(0, bottom-y))))
	t.Place(geom.Pt(x, y))
	return size
}

// Paint implements [gunim.Node].
func (k *Calibrator) Paint(p *paint.Painter, _ gunim.Frame, _ geom.Size, kids gunim.Children) {
	i := 0
	for kid := range kids.All {
		if i != 4 || k.done.Value() > 0.001 {
			kid.Paint(p)
		}
		i++
	}
}

// paintSteps draws the steps as a row of pills: the one under way
// lit, those done in the colour of sound, those to come faint.
func (k *Calibrator) paintSteps(p *paint.Painter, f gunim.Frame, box geom.Size) {
	ink, accent, sound := widget.Ink.Get(f.Theme), widget.Accent.Get(f.Theme), Sound.Get(f.Theme)
	names := []string{"Watch", "Listen", "Result"}
	stages := []calibrate.Stage{calibrate.Watch, calibrate.Listen, calibrate.Done}
	if k.cal.Options().SkipWatch {
		names, stages = names[1:], stages[1:]
	}
	size := widget.CaptionSize.Get(f.Theme)
	gap := float32(6)
	pw := (box.W - gap*float32(len(names)-1)) / float32(len(names))
	now := k.cal.Stage()
	for i, name := range names {
		r := geom.Rc(float32(i)*(pw+gap), 0, pw, box.H)
		var c color.NRGBA
		label := fmt.Sprintf("%d  %s", i+1, name)
		switch {
		case stages[i] == now:
			p.RRect(r, box.H/2, paint.Solid(paint.Faded(accent, 0.28)))
			c = ink
		case stages[i] < now:
			p.RRect(r, box.H/2, paint.Solid(paint.Faded(sound, 0.14)))
			c = sound
		default:
			p.RRectStroke(r, box.H/2, paint.Fill{}, paint.Stroke{Width: 1, Color: paint.Faded(ink, 0.18)})
			c = paint.Faded(ink, 0.55)
		}
		run := Shaped(label, size, true, false)
		if run.Advance > pw-8 {
			run = Shaped(name, size, true, false)
		}
		if run.Advance <= pw-8 {
			run.Paint(p, geom.Pt(r.Min.X+(pw-run.Advance)/2, (box.H-run.Height())/2), c)
		}
	}
}

// paintResult draws the result: the delay large, how sure it is, and
// a strip of the taps round it. It comes in with done.
func (k *Calibrator) paintResult(p *paint.Painter, f gunim.Frame, box geom.Size) {
	done := k.done.Value()
	if done <= 0.001 {
		return
	}
	defer p.Layer(paint.LayerOpts{Bounds: geom.Rect{Max: box.Point()}, Opacity: min(1, done)})()
	th := f.Theme
	ink, sound, near, over, spread := widget.Ink.Get(th), Sound.Get(th), Near.Get(th), Over.Get(th), Spread.Get(th)
	r := k.cal.Result()
	caption := widget.CaptionSize.Get(th)
	// The card behind it.
	p.RRect(geom.Rc(0, 0, box.W, box.H), 14, paint.Solid(paint.Faded(Ground.Get(th), 0.9)))
	pad := float32(16)
	Shaped("EXTRA DELAY", caption, true, false).Paint(p, geom.Pt(pad, 12), paint.Faded(ink, 0.6))
	// The figure, and its unit smaller beside it, on one baseline.
	num := fmt.Sprintf("%+d", r.Delay.Round(time.Millisecond).Milliseconds())
	fig := Shaped(num, CalibratorFigure.Get(th), true, false)
	if fig.Advance > box.W*0.5 {
		fig = Shaped(num, CalibratorFigure.Get(th)*box.W*0.5/fig.Advance, true, false)
	}
	unit := Shaped("ms", CalibratorTitle.Get(th), true, false)
	fig.Paint(p, geom.Pt(pad-2, 26), ink)
	unit.Paint(p, geom.Pt(pad+fig.Advance+4, 26+fig.Ascent-unit.Ascent), paint.Faded(ink, 0.7))
	// How sure: the spread either side, and a word in its colour.
	word, wc := "Sure", sound
	switch {
	case !r.Enough:
		word, wc = "Too few taps", over
	case !r.Sure():
		word, wc = "Rough", near
	}
	plus := Shaped(fmt.Sprintf("± %d ms", max(1, r.Within.Round(time.Millisecond).Milliseconds())),
		widget.TextSize.Get(th), false, true)
	right := box.W - pad
	plus.Paint(p, geom.Pt(right-plus.Advance, 30), paint.Faded(ink, 0.8))
	wr := Shaped(word, caption, true, false)
	pill := geom.Rc(right-wr.Advance-20, 54, wr.Advance+20, 22)
	p.RRect(pill, 11, paint.Solid(paint.Faded(wc, 0.18)))
	wr.Paint(p, geom.Pt(pill.Min.X+10, pill.Min.Y+(22-wr.Height())/2), wc)

	// The strip: each tap of the listen pass, less the habit, round the
	// delay measured; the band is how sure it is.
	strip := geom.Rc(pad, 92, box.W-2*pad, 32)
	p.RRect(strip, 8, paint.Solid(paint.Faded(ink, 0.06)))
	span := 100 * time.Millisecond
	if s := 4 * r.Listen.Spread; s > span {
		span = s.Round(50 * time.Millisecond)
	}
	mid := strip.Min.X + strip.Size().W/2
	scale := (strip.Size().W/2 - 8) / float32(span.Seconds())
	if bw := float32(r.Within.Seconds()) * scale; bw > 0.5 {
		bw = min(bw, strip.Size().W/2)
		p.RRect(geom.Rc(mid-bw, strip.Min.Y+2, 2*bw, strip.Size().H-4), 6, paint.Solid(paint.Faded(spread, 0.25)))
	}
	p.RRect(geom.Rc(mid-1, strip.Min.Y+3, 2, strip.Size().H-6), 1, paint.Solid(paint.Faded(ink, 0.7)))
	for i, off := range r.Listen.Offsets {
		d := float32((off - r.Listen.Median).Seconds()) * scale
		d = max(-strip.Size().W/2+6, min(d, strip.Size().W/2-6))
		y := strip.Min.Y + 8 + float32(i%3)*8
		p.RRect(geom.Rc(mid+d-3.5, y-3.5, 7, 7), 3.5, paint.Solid(paint.Faded(sound, 0.85)))
	}
	lo := Shaped(fmt.Sprintf("−%d ms", span.Milliseconds()), caption, false, true)
	hi := Shaped(fmt.Sprintf("+%d ms", span.Milliseconds()), caption, false, true)
	lo.Paint(p, geom.Pt(strip.Min.X, strip.Max.Y+4), paint.Faded(ink, 0.55))
	hi.Paint(p, geom.Pt(strip.Max.X-hi.Advance, strip.Max.Y+4), paint.Faded(ink, 0.55))
	words := fmt.Sprintf("%d taps", len(r.Listen.Offsets))
	if !k.cal.Options().SkipWatch {
		words += ", your own timing " + msText(r.Habit)
	}
	each := Shaped(words, caption, false, false)
	if each.Advance > strip.Size().W-2*hi.Advance-16 {
		each = Shaped(fmt.Sprintf("%d taps", len(r.Listen.Offsets)), caption, false, false)
	}
	each.Paint(p, geom.Pt(mid-each.Advance/2, strip.Max.Y+4), paint.Faded(ink, 0.55))
}

// calTarget is the calibrator's beat target: the part that moves, and
// takes the taps.
type calTarget struct {
	k *Calibrator
	// ring and hover are the focus ring's and the pointer's springs.
	ring, hover *anim.Float
	// at is the present time of the frame laid out last.
	at time.Time
	// ripples are when the last taps landed, and whether each counted,
	// newest last.
	ripples [4]ripple
	size    geom.Size
}

// ripple is a tap shown spreading from the target.
type ripple struct {
	at      time.Time
	counted bool
}

// rippleFor is how long a tap's ripple spreads.
const rippleFor = 450 * time.Millisecond

// update moves the calibration on at present, the frame's present time,
// and redoes the words where it moved on.
func (t *calTarget) update(present time.Time) {
	t.at = present
	k := t.k
	if k.cal.Update(present) {
		k.changed(nil)
		return
	}
	counted, _, practice := k.cal.Progress(present)
	if s := (calSeen{k.cal.Stage(), counted, practice}); s != k.seen {
		k.seen = s
		k.version++
	}
}

// Step implements [anim.Stepper]: the target moves on every frame while
// the beat shows or a pass runs, and while a ripple spreads.
func (t *calTarget) Step(dt time.Duration) bool {
	moving := t.ring.Step(dt)
	moving = t.hover.Step(dt) || moving
	switch t.k.cal.Stage() {
	case calibrate.Watch, calibrate.Listen:
		return true
	case calibrate.Done:
		// The clicks sound on, with the target pulsing, until accepted.
		if t.k.cal.Running() {
			return true
		}
	case calibrate.Ready:
	}
	last := t.ripples[len(t.ripples)-1].at
	return moving || (!last.IsZero() && t.at.Sub(last) < rippleFor)
}

// Focusable implements [gunim.Focusable].
func (t *calTarget) Focusable() bool { return true }

// tap takes a tap at when, or at the frame's time where the system gave
// none.
func (t *calTarget) tap(when time.Time, u *gunim.UI) {
	if when.IsZero() {
		when = u.Now()
	}
	k := t.k
	switch k.cal.Stage() {
	case calibrate.Ready:
		k.start(when, u)
		return
	case calibrate.Done:
		return
	case calibrate.Watch, calibrate.Listen:
	}
	was := k.cal.Stage()
	how, _ := k.cal.Tap(when)
	copy(t.ripples[:], t.ripples[1:])
	t.ripples[len(t.ripples)-1] = ripple{at: when, counted: how == calibrate.Counted}
	if k.cal.Stage() != was {
		k.changed(u)
	}
	u.Invalidate()
}

// Handle implements [gunim.Handler].
func (t *calTarget) Handle(e input.Event, u *gunim.UI) bool {
	switch e := e.(type) {
	case input.PointerEnter:
		t.hover.Animate(1, widget.Quick.Get(u.Theme()))
	case input.PointerLeave:
		t.hover.Animate(0, widget.Settle.Get(u.Theme()))
	case input.PointerDown:
		if e.Button != input.ButtonPrimary || !t.inside(e.Pos) {
			return false
		}
		t.tap(e.Time, u)
	case input.KeyPress:
		if e.Mods&^input.ModShift != 0 || (e.Key != input.KeySpace && e.Key != input.KeyEnter) {
			return false
		}
		if !e.Repeat {
			t.tap(e.Time, u)
		}
	case input.FocusRing:
		to := float32(0)
		if e.On {
			to = 1
		}
		t.ring.Animate(to, widget.RingFade.Get(u.Theme()))
	case input.FocusLost:
		t.ring.Animate(0, widget.RingFade.Get(u.Theme()))
	default:
		return false
	}
	u.Invalidate()
	return true
}

// Access implements [gunim.Accessible]: a button, its value where the
// calibration has got to.
func (t *calTarget) Access() access.Info {
	k := t.k
	counted, of, practice := k.cal.Progress(t.at)
	var v string
	switch k.cal.Stage() {
	case calibrate.Ready:
		v = "Press to start"
	case calibrate.Watch, calibrate.Listen:
		v = fmt.Sprintf("%s: %d of %d taps", k.title.Text, counted, of)
		if practice > 0 {
			v = fmt.Sprintf("%s: practice, %d beats left", k.title.Text, practice)
		}
	case calibrate.Done:
		r := k.cal.Result()
		v = fmt.Sprintf("Delay %s, within %d ms", msText(r.Delay), r.Within.Round(time.Millisecond).Milliseconds())
	}
	return access.Info{Role: access.RoleButton, Name: "Beat target", Value: v, Actions: []string{access.ActionPress}}
}

// AccessAct implements [gunim.AccessActor]: a press starts the
// calibration, or taps.
func (t *calTarget) AccessAct(r access.Request, u *gunim.UI) bool {
	if r.Action != access.ActionPress {
		return false
	}
	t.tap(time.Now(), u)
	return true
}

// geometry is where the target's circle sits in a box of size, its
// radius, and how high the ball bounces above it.
func (t *calTarget) geometry(size geom.Size, th *theme.Live) (centre geom.Point, radius, bounce float32) {
	ring := float32(14)
	d := min(size.W-2*ring, CalibratorTarget.Get(th))
	bounceRoom := float32(0)
	if t.k.cal.Stage() == calibrate.Watch {
		bounceRoom = min(d*0.8, 170)
	}
	d = max(0, min(d, size.H-2*ring-bounceRoom))
	if t.k.cal.Stage() == calibrate.Watch {
		bounceRoom = min(bounceRoom, size.H-2*ring-d)
	}
	radius = d / 2
	centre = geom.Pt(size.W/2, (size.H+bounceRoom)/2)
	return centre, radius, bounceRoom
}

// inside reports whether p lies in the target's room: all of it takes
// a tap, so a tap that misses the circle a little still counts.
func (t *calTarget) inside(p geom.Point) bool {
	return p.X >= 0 && p.Y >= 0 && p.X <= t.size.W && p.Y <= t.size.H
}

// Layout implements [gunim.Node]: the target takes the room it is given.
func (t *calTarget) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	t.size = c.Max
	return c.Max
}

// pulse is how bright a beat that landed phase of a beat ago shows,
// from 1 as it lands, dying away over 160 ms.
func pulse(phase float64, period time.Duration) float32 {
	return float32(math.Exp(-phase * period.Seconds() / 0.16))
}

// Paint implements [gunim.Node]: the ring of progress, the target with
// its count, the ripples of the taps, and the ball in the watch pass.
func (t *calTarget) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	k := t.k
	ink, raised, sound, near := widget.Ink.Get(th), Raised.Get(th), Sound.Get(th), Near.Get(th)
	centre, radius, bounce := t.geometry(box, th)
	if radius < 8 {
		return
	}
	stage := k.cal.Stage()
	period := k.cal.Options().Period
	if period <= 0 {
		period = 800 * time.Millisecond
	}
	beat, phase, ok := k.cal.Beat(f.Present)
	lit := float32(0)
	if ok && (stage == calibrate.Watch || stage == calibrate.Done) {
		lit = pulse(phase, period)
	}

	// The track of the progress ring, and the part done.
	track := radius + 9
	tr := geom.Rc(centre.X-track, centre.Y-track, 2*track, 2*track)
	p.RRectStroke(tr, track, paint.Fill{}, paint.Stroke{Width: 4, Color: paint.Faded(ink, 0.1)})
	counted, of, practice := k.cal.Progress(f.Present)
	if share := float32(counted) / float32(max(of, 1)); stage != calibrate.Ready && share > 0 {
		arc(p, centre, track, share, 4, sound)
	}
	widget.FocusRing(p, tr.Inset(geom.Uniform(-2)), track+2, t.ring.Value(), th)

	// The target, swelling a little as a beat lands.
	r := radius * (1 + 0.035*lit)
	fill := anim.MixColor(raised, anim.MixColor(raised, ink, 0.12), t.hover.Value())
	fill = anim.MixColor(fill, sound, 0.55*lit)
	p.RRect(geom.Rc(centre.X-r, centre.Y-r, 2*r, 2*r), r, paint.Solid(fill))

	// The ripples of the taps, spreading from its edge.
	for _, rp := range t.ripples {
		age := f.Present.Sub(rp.at)
		if rp.at.IsZero() || age < 0 || age >= rippleFor {
			continue
		}
		s := float32(age.Seconds() / rippleFor.Seconds())
		c := sound
		if !rp.counted {
			c = ink
		}
		rr := radius + 4 + 30*(1-(1-s)*(1-s))
		p.RRectStroke(geom.Rc(centre.X-rr, centre.Y-rr, 2*rr, 2*rr), rr, paint.Fill{},
			paint.Stroke{Width: 3 * (1 - s), Color: paint.Faded(c, 0.7*(1-s))})
	}

	// The words inside.
	small := widget.CaptionSize.Get(th)
	big := CalibratorCount.Get(th) * min(1, radius/90)
	inside := func(top, bottom string) {
		var a, b text.Run
		h := float32(0)
		if top != "" {
			a = Shaped(top, big, true, true)
			h += a.Height()
		}
		if bottom != "" {
			b = Shaped(bottom, small, true, false)
			h += b.Height()
		}
		y := centre.Y - h/2
		if top != "" && a.Advance < 2*r-16 {
			a.Paint(p, geom.Pt(centre.X-a.Advance/2, y), ink)
			y += a.Height()
		}
		if bottom != "" && b.Advance < 2*r-16 {
			b.Paint(p, geom.Pt(centre.X-b.Advance/2, y), paint.Faded(ink, 0.7))
		}
	}
	switch stage {
	case calibrate.Ready:
		inside("", "TAP TO START")
	case calibrate.Watch, calibrate.Listen:
		switch {
		case !ok || beat < 0:
			inside("", "GET READY")
		case practice > 0:
			inside(strconv.Itoa(practice), "PRACTICE")
		default:
			inside(strconv.Itoa(counted), "OF "+strconv.Itoa(of))
		}
	case calibrate.Done:
		inside("", "WATCH AND LISTEN")
	}

	// The ball, in the watch pass: it lands on the target on the beat,
	// squashing as it does.
	if stage == calibrate.Watch && ok {
		b := max(10, radius*0.22)
		h := float32(4*phase*(1-phase)) * max(0, bounce-b-6)
		squash := 0.3 * lit
		bw, bh := b*(1+squash), b*(1-squash)
		floor := centre.Y - r
		p.RRect(geom.Rc(centre.X-bw/2, floor-h-bh, bw, bh), min(bw, bh)/2, paint.Solid(near))
	}
}

// arc draws share of a ring of radius round centre, from the top
// clockwise, as one ribbon.
func arc(p *paint.Painter, centre geom.Point, radius, share, width float32, c color.NRGBA) {
	share = min(share, 1)
	n := max(2, int(2*math.Pi*float64(radius*share)/3))
	pts := make([]paint.RibbonPoint, n+1)
	for i := range pts {
		a := -math.Pi/2 + 2*math.Pi*float64(share)*float64(i)/float64(n)
		pts[i] = paint.RibbonPoint{At: geom.Pt(centre.X+radius*float32(math.Cos(a)), centre.Y+radius*float32(math.Sin(a))),
			Width: width, Color: c}
	}
	p.Ribbon(pts)
}
