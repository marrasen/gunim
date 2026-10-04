package gunim

// A Cue names a sound a widget makes as the user works it: a button
// pressed, a switch flipped, a menu opening. Widgets play cues with
// [UI.Cue]; an application chooses the sounds, or none, with
// [App.SetCues]. Package audio/cues plays a set of quiet sounds made
// for them.
type Cue string

// The cues the widgets of package widget play.
const (
	// CuePress is a button, a menu item or a link acting.
	CuePress Cue = "press"
	// CueToggleOn and CueToggleOff are a checkbox or a switch turning
	// on or off.
	CueToggleOn  Cue = "toggle.on"
	CueToggleOff Cue = "toggle.off"
	// CueSelect is a tab, a segment or an item chosen.
	CueSelect Cue = "select"
	// CueTick is a slider or a number passing a step.
	CueTick Cue = "tick"
	// CueOpen and CueClose are a menu, a dialog or a drawer opening, or
	// going without anything chosen in it.
	CueOpen  Cue = "open"
	CueClose Cue = "close"
	// CueError is something the user tried that was refused.
	CueError Cue = "error"
)

// Cues for an application to play itself, with [UI.Cue], for what
// happens on its own rather than what the user does: no widget plays
// them.
const (
	// CueConnected and CueDisconnected are a connection made, and one
	// lost.
	CueConnected    Cue = "connected"
	CueDisconnected Cue = "disconnected"
	// CueDone and CueFailed are work the user left running finishing,
	// well or not.
	CueDone   Cue = "done"
	CueFailed Cue = "failed"
	// CueBell is something asking for the user, as a terminal's bell.
	CueBell Cue = "bell"
)

// A CuePlayer plays the sound for a cue, at pan, from -1 for the left
// of the window to 1 for its right. It is called on the UI goroutine
// and must return at once.
type CuePlayer interface {
	PlayCue(c Cue, pan float32)
}

// cuePlayer boxes a CuePlayer for an atomic pointer.
type cuePlayer struct{ p CuePlayer }

func store(dst interface{ Store(*cuePlayer) }, p CuePlayer) {
	if p == nil {
		dst.Store(nil)
		return
	}
	dst.Store(&cuePlayer{p})
}

// SetCues plays the cues of every window of the application with p,
// or none with nil, as by default.
func (a *App) SetCues(p CuePlayer) { store(&a.cues, p) }

// SetCues plays this window's cues with p, in place of the
// application's, as for a window of its own, or a test's.
func (w *Window) SetCues(p CuePlayer) { store(&w.cues, p) }

// Cue plays the sound for c, for node n: it sounds from where n is
// across the window, a little left or right, or from the middle for a
// nil n.
func (u *UI) Cue(c Cue, n Node) {
	if u == nil || u.w == nil {
		return
	}
	p := u.w.cues.Load()
	if p == nil && u.w.app != nil {
		p = u.w.app.cues.Load()
	}
	if p == nil {
		return
	}
	pan := float32(0)
	if n != nil {
		r, ok := u.Bounds(n)
		whole, rootOK := u.Bounds(u.Root())
		if ok && rootOK {
			if w := whole.Size().W; w > 0 {
				mid := (r.Min.X + r.Max.X) / 2
				// The sound leans, rather than jumps, toward the side.
				pan = 0.6 * max(-1, min(1, 2*(mid-whole.Min.X)/w-1))
			}
		}
	}
	p.p.PlayCue(c, pan)
}
