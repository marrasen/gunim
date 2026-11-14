package audioui

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/calibrate"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// calStage is a calibrator in an offscreen window, with a mixer stepped along with the frames: the speaker hears
// what the mixer mixed latency ago, by the window's clock.
type calStage struct {
	t     *testing.T
	w     *gunim.Window
	k     *Calibrator
	mix   *audio.Mixer
	base  time.Time
	now   time.Time
	frame time.Duration
}

const calLatency = 30 * time.Millisecond

func newCalStage(t *testing.T, size geom.Size, o calibrate.Options) *calStage {
	t.Helper()
	s := &calStage{t: t, mix: audio.NewMixer(), frame: time.Second / 60}
	o.HeardAt = func(at time.Time) int64 {
		if s.base.IsZero() {
			s.base = at
		}
		return max(0, audio.FramesAt(at.Sub(s.base)-calLatency, s.mix.Rate()))
	}
	s.k = NewCalibrator(calibrate.New(s.mix, nil, o))
	s.w = gunimtest.New(t, size, nil)
	s.w.RegisterTheme(widget.Light().With(Light...))
	gunim.RegisterView(s.w, "cal", func(struct{}) gunim.Node { return s.k }, nil)
	if err := s.w.Client().Mount(gunim.Root, "cal", "cal", nil); err != nil {
		t.Fatal(err)
	}
	s.run(1)
	return s
}

// run steps n frames, the mixer along with them.
func (s *calStage) run(n int) {
	for range n {
		s.w.Frame(s.frame)
		s.now = s.k.target.at
		s.mix.Mix(make([]float32, 2*audio.FramesAt(s.frame, s.mix.Rate())))
	}
}

// tapAlong taps with Space on every beat of the pass running, off after the beat as the listener taps it, and runs
// frames until the stage changes. each, where set, is asked on every frame.
func (s *calStage) tapAlong(off time.Duration, each func()) {
	s.t.Helper()
	stage := s.k.cal.Stage()
	period := s.k.cal.Options().Period
	if period == 0 {
		period = 800 * time.Millisecond
	}
	last := int64(math.MinInt64)
	for i := 0; s.k.cal.Stage() == stage; i++ {
		if i > 60*60 {
			s.t.Fatalf("the %v pass never ended", stage)
		}
		s.run(1)
		if each != nil {
			each()
		}
		// The beat the speaker says is heard now; in the listen pass the beat shown is that too.
		k, phase, ok := s.k.cal.Beat(s.now)
		if !ok || k == last || s.now.Sub(s.now.Add(-time.Duration(phase*float64(period)))) < off {
			continue
		}
		last = k
		at := s.now.Add(-time.Duration(phase * float64(period))).Add(off)
		s.w.Input(input.KeyPress{Key: input.KeySpace, Time: at})
	}
}

// A calibrator runs from the keyboard: Tab takes it to the target, Space starts it and taps, and the delay comes
// out. The result panel and the target's new size come in without a jump, frame by frame.
func TestACalibratorRunsFromTheKeyboard(t *testing.T) {
	s := newCalStage(t, geom.Sz(380, 640), calibrate.Options{Taps: 8, Practice: 2})
	var got *calibrate.Result
	s.k.OnAccept = func(r calibrate.Result, _ *gunim.UI) gunim.Intent { got = &r; return nil }
	s.w.Input(input.KeyPress{Key: input.KeyTab})
	s.run(20)
	if r := s.k.target.ring.Value(); r < 0.99 {
		t.Fatalf("Tab left the target's ring at %v", r)
	}
	s.w.Input(input.KeyPress{Key: input.KeySpace})
	s.run(1)
	if s.k.cal.Stage() != calibrate.Watch {
		t.Fatalf("Space left the calibration at %v, want watch", s.k.cal.Stage())
	}
	s.tapAlong(-20*time.Millisecond, nil)
	if s.k.cal.Stage() != calibrate.Listen {
		t.Fatalf("after the watch pass the calibration is at %v", s.k.cal.Stage())
	}
	// The listener hears each click 200 ms after the speaker says, and taps 20 ms ahead of it.
	heights := make([]float32, 0, 60)
	s.tapAlong(180*time.Millisecond, nil)
	if s.k.cal.Stage() != calibrate.Done {
		t.Fatalf("after the listen pass the calibration is at %v", s.k.cal.Stage())
	}
	r := s.k.cal.Result()
	// A tap's time is to the frame it is sent in, and the frames are 17 ms apart.
	if math.Abs(float64(r.Delay-200*time.Millisecond)) > float64(20*time.Millisecond) {
		t.Fatalf("the calibrator found %v, want 200ms", r.Delay)
	}
	// The result comes in on a spring: the target's room shrinks every frame, with no jump.
	for range 60 {
		s.run(1)
		heights = append(heights, s.k.target.size.H)
	}
	for i := 1; i < len(heights); i++ {
		if d := heights[i-1] - heights[i]; d < -2 || d > 40 {
			t.Fatalf("frame %d the target's room went from %v to %v", i, heights[i-1], heights[i])
		}
	}
	if d := s.k.done.Value(); d < 0.99 {
		t.Fatalf("a second on the result panel is in %v of the way", d)
	}
	if !s.k.cal.Running() {
		t.Fatal("the clicks stopped before the result was accepted")
	}
	// Space on the target does nothing once done: Tab moves on to Try again, and on to Use.
	s.w.Input(input.KeyPress{Key: input.KeySpace})
	s.run(1)
	if s.k.cal.Stage() != calibrate.Done {
		t.Fatalf("Space once done took the calibration to %v", s.k.cal.Stage())
	}
	s.w.Input(input.KeyPress{Key: input.KeyTab})
	s.w.Input(input.KeyPress{Key: input.KeyTab})
	s.run(2)
	s.w.Input(input.KeyPress{Key: input.KeyEnter})
	s.run(2)
	if got == nil || got.Delay != r.Delay {
		t.Fatalf("Use handed on %v, want the result %+v", got, r)
	}
	if s.k.cal.Running() {
		t.Fatal("the clicks play on after the result was accepted")
	}
}

// A click on the target starts the calibration and taps, at the time the press came; a key held down taps once.
func TestAClickOrAKeyTapsOnce(t *testing.T) {
	s := newCalStage(t, geom.Sz(380, 640), calibrate.Options{})
	mid := func() geom.Point {
		return geom.Pt(190, 320)
	}
	s.w.Input(input.PointerDown{Pos: mid(), Button: input.ButtonPrimary, Clicks: 1})
	s.w.Input(input.PointerUp{Pos: mid(), Button: input.ButtonPrimary})
	s.run(1)
	if s.k.cal.Stage() != calibrate.Watch {
		t.Fatalf("a click on the target left the calibration at %v", s.k.cal.Stage())
	}
	// Run to the counted beats, past the practice ones.
	for range 60 * 5 {
		s.run(1)
		if _, _, practice := s.k.cal.Progress(s.now); practice == 0 {
			if k, _, _ := s.k.cal.Beat(s.now); k > 3 {
				break
			}
		}
	}
	s.w.Input(input.PointerDown{Pos: mid(), Button: input.ButtonPrimary, Clicks: 1, Time: s.now})
	s.w.Input(input.PointerUp{Pos: mid(), Button: input.ButtonPrimary})
	s.w.Input(input.KeyPress{Key: input.KeyEnter, Time: s.now.Add(time.Millisecond)})
	s.w.Input(input.KeyPress{Key: input.KeyEnter, Repeat: true, Time: s.now.Add(2 * time.Millisecond)})
	s.run(1)
	if n := len(s.k.cal.Taps()); n != 2 {
		t.Fatalf("a click, a key and its repeat counted %d taps, want 2", n)
	}
}

// The target reads as a button that says where the calibration has got to, and a screen reader's press starts it.
func TestTheTargetReadsWhereTheCalibrationIs(t *testing.T) {
	s := newCalStage(t, geom.Sz(380, 640), calibrate.Options{})
	info := s.k.target.Access()
	if info.Role != access.RoleButton || info.Name != "Beat target" || info.Value != "Press to start" {
		t.Fatalf("before it starts the target reads %+v", info)
	}
	s.w.Offscreen().ListenForAccess()
	s.run(2)
	tree := s.w.Offscreen().AccessTree()
	var target *access.Node
	var walk func(n *access.Node)
	walk = func(n *access.Node) {
		if n.Role == access.RoleButton && n.Name == "Beat target" {
			target = n
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(tree.Root)
	if target == nil {
		t.Fatal("the access tree holds no beat target")
	}
	s.w.Input(access.Request{ID: target.ID, Action: access.ActionPress})
	s.run(60 * 3)
	if info := s.k.target.Access(); s.k.cal.Stage() != calibrate.Watch || info.Value == "Press to start" {
		t.Fatalf("after a screen reader's press, at %v the target reads %q", s.k.cal.Stage(), info.Value)
	}
	if a := s.k.Access(); a.Role != access.RoleGroup || a.Name != "Sound delay" {
		t.Fatalf("the calibrator reads %+v", a)
	}
}

// Once it rests, before it starts, the calibrator asks for no frames; a pass running moves only the target, and its
// words and steps are carried from frame to frame.
func TestOnlyTheTargetMovesAsAPassRuns(t *testing.T) {
	s := newCalStage(t, geom.Sz(380, 640), calibrate.Options{})
	s.run(30)
	if s.k.target.Step(s.frame) || s.k.Step(s.frame) {
		t.Fatal("the calibrator moves before it starts")
	}
	s.w.Input(input.KeyPress{Key: input.KeyTab})
	s.w.Input(input.KeyPress{Key: input.KeySpace})
	s.run(3)
	v := s.k.version
	for range 20 {
		s.run(1)
		if !s.k.target.Step(0) {
			t.Fatal("the target rests while the watch pass runs")
		}
	}
	if s.k.version-v > 2 {
		t.Fatalf("the words changed %d times in 20 frames of the watch pass", s.k.version-v)
	}
	// The target draws a few dozen quads at most.
	var p paint.Painter
	s.k.target.Paint(&p, gunim.Frame{Present: s.now, Theme: theme.NewLive(widget.Dark())}, s.k.target.size, gunim.Children{})
	if n := len(p.Ops()); n > 24 {
		t.Fatalf("the target paints %d ops", n)
	}
	box := s.k.target.size
	for _, op := range p.Ops() {
		if r, ok := op.(*paint.RRectOp); ok && (r.Rect.Min.X < -8 || r.Rect.Min.Y < -8 || r.Rect.Max.X > box.W+8 ||
			r.Rect.Max.Y > box.H+8) {
			t.Fatalf("the target paints a rect at %v, outside its box %v", r.Rect, box)
		}
	}
}

// TestCalibratorPictures draws the calibrator in each state, in the dark theme and the light, on a phone and a
// desktop, and writes them to GUNIM_PIXELS_DIR where it is set.
func TestCalibratorPictures(t *testing.T) {
	dir := os.Getenv("GUNIM_PIXELS_DIR")
	for _, size := range []geom.Size{geom.Sz(380, 680), geom.Sz(760, 560)} {
		for _, th := range []string{"dark", "light"} {
			s := newCalStage(t, size, calibrate.Options{})
			if th == "light" {
				if err := s.w.Client().SetTheme("light"); err != nil {
					t.Fatal(err)
				}
			}
			s.run(30)
			shot := func(name string) {
				img := gunimtest.Pixels(t, s.w)
				if dir != "" {
					gunimtest.WritePNG(t, filepath.Join(dir, fmt.Sprintf("calibrator-%s-%dx%d-%s.png", name,
						int(size.W), int(size.H), th)), img)
				}
			}
			shot("ready")
			s.w.Input(input.KeyPress{Key: input.KeyTab})
			s.w.Input(input.KeyPress{Key: input.KeySpace})
			// Partway through the watch pass, the ball on its way down.
			n := 0
			s.tapAlong(-15*time.Millisecond, func() {
				if _, counted, _ := s.k.cal.Progress(s.now); counted < 0 {
					return
				}
				counted, _, _ := s.k.cal.Progress(s.now)
				if _, phase, _ := s.k.cal.Beat(s.now); counted == 6 && n == 0 && phase > 0.75 {
					n++
					shot("watch")
				}
			})
			n = 0
			s.tapAlong(210*time.Millisecond, func() {
				counted, _, _ := s.k.cal.Progress(s.now)
				if _, phase, _ := s.k.cal.Beat(s.now); counted == 9 && n == 0 && phase > 0.30 {
					n++
					shot("listen")
				}
			})
			s.run(70)
			for range 120 {
				if _, phase, _ := s.k.cal.Beat(s.now); phase < 0.05 {
					break
				}
				s.run(1)
			}
			shot("result")
		}
	}
}

// A calibration started again from outside, through Calibration, as an application's own button does, takes the
// words and the result panel back to the first pass: the panel slides away, frame by frame, with no jump.
func TestAStartFromOutsideTakesTheResultAway(t *testing.T) {
	s := newCalStage(t, geom.Sz(380, 640), calibrate.Options{Taps: 4, Practice: 1})
	s.w.Input(input.KeyPress{Key: input.KeyTab})
	s.w.Input(input.KeyPress{Key: input.KeySpace})
	s.run(1)
	s.tapAlong(0, nil)
	s.tapAlong(100*time.Millisecond, nil)
	s.run(60)
	if s.k.cal.Stage() != calibrate.Done || s.k.done.Value() < 0.99 {
		t.Fatalf("at %v the result panel is in %v of the way", s.k.cal.Stage(), s.k.done.Value())
	}
	s.k.Calibration().Start(s.now)
	last := float32(1)
	for i := range 60 {
		s.run(1)
		d := s.k.done.Value()
		if i == 0 && d < 0.5 {
			t.Fatalf("the frame after the start the result panel is at %v: it jumped away", d)
		}
		if d > last+0.05 {
			t.Fatalf("frame %d the result panel came back, %v to %v", i, last, d)
		}
		last = d
	}
	if last > 0.01 || s.k.title.Text == "The sound's delay" || s.k.again.Label != "Start over" {
		t.Fatalf("a second after the start, at %v, the panel is at %v, the title reads %q and the button %q",
			s.k.cal.Stage(), last, s.k.title.Text, s.k.again.Label)
	}
}
