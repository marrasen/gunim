package widget

import (
	"image/color"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// LogView tokens.
var (
	// LogViewHeight is a log view's height, and LogViewSize its words'.
	LogViewHeight = theme.Length("logview.height", 440)
	LogViewSize   = theme.Length("logview.size", 12.5)
	// LogViewPadding is the room round its lines.
	LogViewPadding = theme.Length("logview.padding", 12)
	// LogViewNew is how long a line just come takes to fade in.
	LogViewNew = 400 * time.Millisecond
)

// LogLine is one line of a [LogView]: a faint Lead, such as the time it
// was written, then its Text in Ink, the theme's [Ink] when unset.
type LogLine struct {
	Lead, Text string
	Ink        color.NRGBA
}

// LogSource gives a [LogView] its lines: the last most of them, oldest
// first, and the number past the last line ever written, which grows by
// one with each line, so the view can tell what is new however many
// lines the source keeps.
type LogSource func(most int) (lines []LogLine, end int64)

// LogView shows lines as they are written, as a connection's log or a
// build's output: in the theme's mono face, newest at the bottom, each
// line fading in as it comes. At the bottom it follows new lines;
// scrolled back by the wheel, what is shown stays where it is as more
// come. The wheel at either end, or with a key held, passes on to what
// scrolls round it.
//
// It looks for new lines while it is drawn, a few times a second.
type LogView struct {
	// Source gives the lines.
	Source LogSource
	// OnSeen runs on the UI goroutine as the view shows the newest line,
	// with the end the source said, so an application can count what
	// has not been read; a non-nil result is sent to the application as
	// the view's intent.
	OnSeen func(end int64, u *gunim.UI) gunim.Intent
	// seen is the end OnSeen was last told.
	seen int64

	// back is how many lines it is scrolled up from the newest, end the
	// source's end as last drawn, wheel what the wheel has turned short
	// of a line, and came when each line from first on arrived.
	back  int
	end   int64
	wheel float32
	came  map[int64]time.Time
}

// NewLogView returns a log view of source.
func NewLogView(source LogSource) *LogView {
	return &LogView{Source: source, came: map[int64]time.Time{}}
}

// Layout implements [gunim.Node].
func (v *LogView) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(geom.Sz(c.Max.W, LogViewHeight.Get(f.Theme)))
}

// logStep is the room a line takes.
func logStep(th *theme.Live) float32 { return LogViewSize.Get(th) * 1.55 }

// Paint implements [gunim.Node]: a box of the lines that fit, each
// one's lead faint, fading in as it comes.
func (v *LogView) Paint(p *paint.Painter, f gunim.Frame, box geom.Size, _ gunim.Children) {
	th := f.Theme
	r := geom.Rect{Max: box.Point()}
	p.RRect(r, 8, paint.Solid(FieldFill.Get(th)))
	p.RRectStroke(r.Inset(geom.Uniform(0.5)), 8, paint.Fill{}, paint.Stroke{Width: 1, Color: DialogBorder.Get(th)})
	if v.Source == nil {
		return
	}
	f.RedrawAt(f.Now.Add(250 * time.Millisecond))
	pad, size, step := LogViewPadding.Get(th), LogViewSize.Get(th), logStep(th)
	fits := max(int((box.H-2*pad)/step), 1)
	lines, end := v.Source(fits + v.back + 1)
	// Scrolled back, the lines looked at stay where they are as more
	// come: as far back again as there are new ones.
	if v.back > 0 && v.end > 0 && end > v.end {
		v.back += int(end - v.end)
		lines, end = v.Source(fits + v.back + 1)
	}
	if v.came == nil {
		v.came = map[int64]time.Time{}
	}
	first := end - int64(len(lines))
	if v.end > 0 {
		for n := max(v.end, first); n < end; n++ {
			v.came[n] = f.Now
		}
	}
	for n := range v.came {
		if n < first {
			delete(v.came, n)
		}
	}
	v.end = end
	v.back = min(v.back, max(0, len(lines)-fits))
	last := len(lines) - v.back
	start := max(0, last-fits)
	if v.back == 0 && v.OnSeen != nil && end != v.seen {
		v.seen = end
		if u := f.UI(); u != nil {
			if in := v.OnSeen(end, u); in != nil {
				u.Send(v, in)
			}
		}
	}
	face := faceIn(MonoFont, th)
	ink := Ink.Get(th)
	lead := ink
	lead.A = 0x90
	y := pad
	for i, l := range lines[start:last] {
		alpha := float32(1)
		if at, ok := v.came[first+int64(start+i)]; ok {
			if t := float32(f.Now.Sub(at)) / float32(LogViewNew); t < 1 {
				alpha = max(t, 0)
				f.RedrawAt(f.Now.Add(time.Second / 60))
			}
		}
		x := pad
		if l.Lead != "" {
			run := face.Shape(l.Lead, size)
			run.Paint(p, geom.Pt(x, y), fadeTo(lead, alpha))
			x += run.Advance + 12
		}
		c := l.Ink
		if c == (color.NRGBA{}) {
			c = ink
		}
		face.Shape(l.Text, size).PaintFaded(p, geom.Pt(x, y), fadeTo(c, alpha), box.W-pad-x, TextFade.Get(th))
		y += step
	}
}

// fadeTo is c at a of its alpha.
func fadeTo(c color.NRGBA, a float32) color.NRGBA {
	c.A = uint8(float32(c.A) * min(max(a, 0), 1))
	return c
}

// Handle implements [gunim.Handler]: the wheel scrolls back through the
// lines, and down to the newest, which the view then follows.
func (v *LogView) Handle(e input.Event, u *gunim.UI) bool {
	s, ok := e.(input.Scroll)
	if !ok || v.Source == nil || s.Mods != 0 {
		return false
	}
	n := s.Notches.Y
	if n == 0 {
		n = s.Delta.Y / 40
	}
	// Up a notch is back three lines; what falls short of a line is kept
	// for the next turn, as a touchpad turns in fractions.
	v.wheel += n * 3
	whole := int(v.wheel)
	v.wheel -= float32(whole)
	switch {
	case whole > 0:
		lines, _ := v.Source(v.back + whole + 1 + int(LogViewHeight.Get(u.Theme())/logStep(u.Theme())))
		if v.back+int(LogViewHeight.Get(u.Theme())/logStep(u.Theme())) >= len(lines) {
			// At the top: what is round it scrolls instead.
			return false
		}
	case whole < 0 && v.back == 0:
		return false
	}
	v.back = max(0, v.back+whole)
	u.Invalidate()
	return true
}

// Access implements [gunim.Accessible]: the lines shown, as text.
func (v *LogView) Access() access.Info {
	return access.Info{Role: access.RoleLabel, Name: "Log"}
}
