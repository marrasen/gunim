// Command synccheck measures how late gunim's sound reaches the ears,
// and then shows by eye whether the picture keeps in step with it.
//
// It opens on audioui's Calibrator: the listener taps along with a
// bouncing ball, then with clicks they hear, and the calibrator finds
// the delay the speaker leaves out, as a Bluetooth headset's on
// Windows. Use sets it on the speaker. The check then plays four rising
// notes and shows their numbers, 1 to 4, as the speaker says each is
// heard, now counting the delay measured: they should land together.
//
// Buttons switch between gunim's own driver named, as AAudio on Android
// or PipeWire on Linux, and the system's own sound, turn Options.Quick
// on and off, and calibrate again. The delay measured is kept for each
// driver and route, as an application would keep it. Done logs a line
// that starts "SYNCCHECK", with what the speaker reports and the delay
// measured. On Android the speaker's log is on, for the delays AAudio
// and the Bluetooth route report.
//
//	go run ./tools/gunimapk -run ./example/synccheck
//	go run ./example/synccheck   # on Linux, through PipeWire
package main

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"log"
	"math"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio"
	"github.com/marrasen/gunim/audio/calibrate"
	audiodriver "github.com/marrasen/gunim/audio/driver"
	"github.com/marrasen/gunim/audio/speaker"
	"github.com/marrasen/gunim/audioui"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

const name = "gunim sync check"

// native is the driver of gunim's own the check plays through first:
// PipeWire on Linux, WASAPI on Windows, Core Audio on macOS and AAudio
// on Android.
var native = func() string {
	switch runtime.GOOS {
	case "linux":
		return audiodriver.PipeWire
	case "windows":
		return audiodriver.WASAPI
	case "darwin":
		return audiodriver.CoreAudio
	}
	return audiodriver.AAudio
}()

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if runtime.GOOS == "android" && os.Getenv("GUNIM_DEBUG_AUDIO") == "" {
		// Android passes the application no environment to ask for the
		// speaker's log.
		_ = os.Setenv("GUNIM_DEBUG_AUDIO", "1")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := gunim.Main(ctx, func(a *gunim.App) error {
		w, err := a.NewWindow(gunim.WindowOptions{Title: name, Size: geom.Sz(400, 680), Root: widget.NewSurface()})
		if err != nil {
			return err
		}
		w.RegisterTheme(widget.Light().With(audioui.Light...))
		mix := audio.NewMixer()
		spk, err := speaker.Open(mix, speaker.Options{Name: name, Driver: native})
		if err != nil {
			return err
		}
		defer func() { _ = spk.Close() }()
		gunim.RegisterView(w, "sync", func(struct{}) *view { return newView(mix, spk) }, nil)
		c := w.Client()
		if err := c.Mount(gunim.Root, "sync", "sync", struct{}{}); err != nil {
			return err
		}
		// Comparing a picture with a sound takes a while, with no touch.
		if err := c.KeepAwake(true); err != nil && !errors.Is(err, driver.ErrNoAwaker) {
			log.Print(err)
		}
		for range c.Intents() {
		}
		return c.Err()
	})
	if errors.Is(err, driver.ErrNoDriver) || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

// The notes: a beat every 500 ms, four notes rising, then a beat's rest.
const (
	beat  = 500 * time.Millisecond
	beats = 5
	tone  = 80 * time.Millisecond
)

var pitches = [4]float64{440, 554.37, 659.25, 880}

// notes plays the four notes over and over.
type notes struct {
	rate int
	n    int64
	mix  *audio.Mixer
	// start is the mixer frame the first note starts at, or -1 before
	// the mixer first reads.
	start atomic.Int64
}

func (s *notes) Read(dst []float32) (int, error) {
	if s.start.Load() < 0 {
		s.start.Store(s.mix.Mixed())
	}
	per := int64(s.rate) * int64(beat) / int64(time.Second)
	long := int64(s.rate) * int64(tone) / int64(time.Second)
	for i := 0; i+1 < len(dst); i += 2 {
		v := float32(0)
		b, at := (s.n/per)%beats, s.n%per
		if b < 4 && at < long {
			env := 1 - float64(at)/float64(long)
			v = float32(0.4 * env * math.Sin(2*math.Pi*pitches[b]*float64(at)/float64(s.rate)))
		}
		dst[i], dst[i+1] = v, v
		s.n++
	}
	return len(dst) / 2, nil
}

var (
	digitSize = theme.FontSize("synccheck.digit", 160)
	boxDim    = theme.Color("synccheck.box", color.NRGBA{R: 60, G: 60, B: 70, A: 255})
	boxLit    = theme.Color("synccheck.box.lit", color.NRGBA{R: 80, G: 220, B: 120, A: 255})
)

// route is what a delay is kept for: the driver and the route playing.
type route struct {
	driver, info string
	route        speaker.Route
}

func routeOf(st speaker.State) route {
	r := route{driver: st.Driver, route: st.Route}
	if st.Info != nil {
		r.info = st.Info.Driver
	}
	return r
}

type view struct {
	mix *audio.Mixer
	spk *speaker.Speaker
	// cal runs the calibration while checking is false; notes plays
	// the check while it is true.
	cal      *audioui.Calibrator
	checking bool
	notes    *notes
	voice    *audio.Voice
	// result is the last calibration accepted, and delays the delay
	// kept for each route.
	result calibrate.Result
	delays map[route]time.Duration
	// driver and quick are what the speaker was last set to.
	driver string
	quick  bool
	// saved says what Done logged last, shown a few seconds from savedAt.
	saved   string
	savedAt time.Time
	digit   *widget.Label
	info    *widget.Label
	button  []*widget.Button
	present time.Time
}

// set opens the speaker as v.driver and v.quick say, and sets the
// delay kept for the route it now plays through.
func (v *view) set() {
	if err := v.spk.Set(speaker.Options{Name: name, Driver: v.driver, Quick: v.quick}); err != nil {
		log.Print(err)
	}
	v.spk.SetDelay(v.delays[routeOf(v.spk.State())])
}

// through names the driver playing.
func through(st speaker.State) string {
	if st.Driver == "" {
		return "the system's sound"
	}
	return st.Driver
}

// onOff names a switch's state.
func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

func newView(mix *audio.Mixer, spk *speaker.Speaker) *view {
	v := &view{mix: mix, spk: spk, driver: native, digit: widget.NewLabel(""), info: widget.NewLabel(""),
		delays: map[route]time.Duration{}}
	v.digit.Size = digitSize
	v.cal = audioui.NewCalibrator(calibrate.New(mix, spk, calibrate.Options{}))
	v.cal.OnAccept = func(r calibrate.Result, u *gunim.UI) gunim.Intent {
		v.result = r
		v.spk.SetDelay(r.Delay)
		v.delays[routeOf(v.spk.State())] = r.Delay
		v.check(true, u)
		return nil
	}
	again := widget.NewButton("Calibrate again")
	again.OnClick = func(u *gunim.UI) gunim.Intent {
		v.check(false, u)
		return nil
	}
	sw := widget.NewButton("Switch driver")
	sw.OnClick = func(*gunim.UI) gunim.Intent {
		if v.driver == native {
			v.driver = ""
		} else {
			v.driver = native
		}
		v.set()
		return nil
	}
	quick := widget.NewButton("Quick")
	quick.OnClick = func(*gunim.UI) gunim.Intent {
		v.quick = !v.quick
		quick.Active = v.quick
		v.set()
		return nil
	}
	done := widget.NewButton("Done")
	done.Kind = widget.ButtonPrimary
	done.OnClick = func(*gunim.UI) gunim.Intent {
		st := v.spk.State()
		device := time.Duration(0)
		if st.Info != nil && st.Rate > 0 {
			device = audio.DurationAt(int64(st.Info.Latency), st.Rate)
		}
		r := v.result
		log.Printf("SYNCCHECK driver=%q quick=%v route=%v latency=%v device=%v delay=%v within=%v habit=%v sure=%v",
			st.Driver, v.quick, st.Route, st.Latency.Round(100*time.Microsecond), device.Round(100*time.Microsecond),
			st.Delay.Round(time.Millisecond), r.Within.Round(time.Millisecond), r.Habit.Round(time.Millisecond), r.Sure())
		v.saved, v.savedAt = fmt.Sprintf("Saved: %s, Quick %s, delay %v.", through(st), onOff(v.quick),
			st.Delay.Round(time.Millisecond)), time.Now()
		return nil
	}
	v.button = append(v.button, again, sw, quick, done)
	return v
}

// check moves to the check by eye, or back to the calibration, which
// starts again.
func (v *view) check(on bool, u *gunim.UI) {
	v.checking = on
	if on {
		v.cal.Calibration().Stop()
		v.notes = &notes{rate: v.mix.Rate(), mix: v.mix}
		v.notes.start.Store(-1)
		v.voice = v.mix.Play(v.notes, audio.Options{})
	} else {
		if v.voice != nil {
			v.voice.Stop(0)
			v.voice = nil
		}
		v.cal.Start(u)
	}
	u.Invalidate()
}

func (v *view) Children() []gunim.Node {
	if !v.checking {
		return []gunim.Node{v.cal}
	}
	kids := make([]gunim.Node, 0, 2+len(v.button))
	kids = append(kids, v.digit, v.info)
	for _, b := range v.button {
		kids = append(kids, b)
	}
	return kids
}

// Step keeps the window drawing every frame while the check plays.
func (v *view) Step(time.Duration) bool { return v.checking }

// lit returns the note, 0 to 3, whose number shows at the frame's
// present time, or -1: a number shows for 250 ms from when its note is
// heard, with the delay set on the speaker.
func (v *view) lit() int {
	if v.notes == nil {
		return -1
	}
	start := v.notes.start.Load()
	if start < 0 {
		return -1
	}
	rate := int64(v.notes.rate)
	h := v.mix.HeardAt(v.present) - start
	if h < 0 {
		return -1
	}
	per := rate * int64(beat) / int64(time.Second)
	b, at := (h/per)%beats, h%per
	if b < 4 && at < rate/4 {
		return int(b)
	}
	return -1
}

func (v *view) Layout(c gunim.Constraints, f gunim.Frame, kids gunim.Children) geom.Size {
	v.present = f.Present
	if !v.checking {
		kid := kids.At(0)
		kid.Layout(gunim.Tight(c.Max))
		kid.Place(geom.Point{})
		return c.Max
	}
	if b := v.lit(); b >= 0 {
		v.digit.Text = strconv.Itoa(b + 1)
	} else {
		v.digit.Text = ""
	}
	st := v.spk.State()
	v.info.Text = fmt.Sprintf("Playing through %s, route %v, Quick %s.\nReported latency %v, delay measured %v.\n\n"+
		"Each number should show exactly as its note sounds. If they part, calibrate again.",
		through(st), st.Route, onOff(v.quick), st.Latency.Round(100*time.Microsecond), st.Delay.Round(time.Millisecond))
	if time.Since(v.savedAt) < 4*time.Second {
		v.info.Text += "\n\n" + v.saved
	}
	w := c.Max.W - 32
	i := 0
	x, y := float32(16), c.Max.H-140
	for kid := range kids.All {
		switch i {
		case 0:
			kid.Layout(gunim.Tight(geom.Sz(w, 220)))
			kid.Place(geom.Pt(c.Max.W/2-40, 60))
		case 1:
			kid.Layout(gunim.Tight(geom.Sz(w, 220)))
			kid.Place(geom.Pt(16, 320))
		default:
			sz := kid.Layout(gunim.Loose(geom.Sz(w, 64)))
			if x+sz.W > c.Max.W-16 {
				x, y = 16, y+sz.H+12
			}
			kid.Place(geom.Pt(x, y))
			x += sz.W + 12
		}
		i++
	}
	return c.Max
}

func (v *view) Paint(p *paint.Painter, f gunim.Frame, size geom.Size, kids gunim.Children) {
	if v.checking {
		// Four boxes under the number, the one heard now lit.
		lit := v.lit()
		bw := (size.W - 32 - 3*12) / 4
		for i := range 4 {
			r := geom.Rect{Min: geom.Pt(16+float32(i)*(bw+12), 280), Max: geom.Pt(16+float32(i)*(bw+12)+bw, 300)}
			c := boxDim.Get(f.Theme)
			if i == lit {
				c = boxLit.Get(f.Theme)
			}
			p.RRect(r, 6, paint.Solid(c))
		}
	}
	for kid := range kids.All {
		kid.Paint(p)
	}
}
