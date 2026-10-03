package widget

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
)

// cueLog records the cues a window plays.
type cueLog struct {
	mu   sync.Mutex
	cues []gunim.Cue
	pans []float32
}

func (l *cueLog) PlayCue(c gunim.Cue, pan float32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cues = append(l.cues, c)
	l.pans = append(l.pans, pan)
}

// take returns the cues played since it was last asked.
func (l *cueLog) take() []gunim.Cue {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := l.cues
	l.cues, l.pans = nil, nil
	return c
}

func listens(w *gunim.Window) *cueLog {
	l := &cueLog{}
	w.SetCues(l)
	return l
}

func cueClick(w *gunim.Window, run func(int), p geom.Point) {
	w.Input(input.PointerMove{Pos: p})
	w.Input(input.PointerDown{Pos: p, Clicks: 1, Button: input.ButtonPrimary})
	w.Input(input.PointerUp{Pos: p, Button: input.ButtonPrimary})
	run(1)
}

func wantCues(t *testing.T, l *cueLog, what string, want ...gunim.Cue) {
	t.Helper()
	if got := l.take(); !slices.Equal(got, want) {
		t.Errorf("%s played %v, want %v", what, got, want)
	}
}

func TestAButtonSoundsAsItActsFromItsSide(t *testing.T) {
	a, b := NewButton("Left"), NewButton("Right")
	w, run := stage(t, &frame{child: Row(a, NewSized(NewSpacer(), 600, 1), b), size: geom.Sz(800, 36)})
	l := listens(w)
	cueClick(w, run, geom.Pt(10, 18))
	if len(l.pans) != 1 || l.pans[0] >= 0 {
		t.Errorf("a button at the left played at pans %v, want one to the left", l.pans)
	}
	wantCues(t, l, "a click on a button", gunim.CuePress)
	cueClick(w, run, geom.Pt(a.size.W+600+2*8+b.size.W/2, 18))
	if len(l.pans) != 1 || l.pans[0] <= 0 {
		t.Errorf("a button at the right played at pans %v, want one to the right", l.pans)
	}
	wantCues(t, l, "a click on the other button", gunim.CuePress)
}

func TestASwitchSoundsHigherOnThanOff(t *testing.T) {
	s := NewSwitch("Wi-Fi")
	w, run := stage(t, &frame{child: s, size: geom.Sz(300, 36)})
	l := listens(w)
	cueClick(w, run, geom.Pt(10, 18))
	wantCues(t, l, "turning a switch on", gunim.CueToggleOn)
	cueClick(w, run, geom.Pt(10, 18))
	wantCues(t, l, "turning it off", gunim.CueToggleOff)
}

func TestASliderTicksAsItPassesSteps(t *testing.T) {
	s := NewSlider(0, 100)
	s.Snap = 10
	w, run := stage(t, &frame{child: s, size: geom.Sz(300, 36)})
	l := listens(w)
	w.Input(input.PointerDown{Pos: geom.Pt(5, 18), Clicks: 1, Button: input.ButtonPrimary})
	run(1)
	w.Input(input.FocusRing{})
	l.take()
	w.Input(input.KeyPress{Key: input.KeyRight})
	run(1)
	wantCues(t, l, "a step with the keyboard", gunim.CueTick)
	// A press at the end it is at moves nothing and makes no sound.
	w.Input(input.KeyPress{Key: input.KeyHome})
	run(1)
	l.take()
	w.Input(input.KeyPress{Key: input.KeyHome})
	run(1)
	wantCues(t, l, "Home at the start")
}

func TestAMenuSoundsAsItOpensPicksAndGoesUnpicked(t *testing.T) {
	d := NewDropdown("Apple", "Banana", "Cherry")
	w, run := stage(t, &frame{child: d, size: geom.Sz(200, 36)})
	l := listens(w)
	cueClick(w, run, geom.Pt(20, 18))
	wantCues(t, l, "opening a drop-down", gunim.CueOpen)
	for _, k := range []input.Key{input.KeyDown, input.KeyEnter} {
		w.Input(input.KeyPress{Key: k})
		run(1)
	}
	wantCues(t, l, "picking from it", gunim.CuePress)
	cueClick(w, run, geom.Pt(20, 18))
	l.take()
	w.Input(input.KeyPress{Key: input.KeyEscape})
	run(1)
	wantCues(t, l, "Escape out of it", gunim.CueClose)
}

func TestADialogSoundsAsItComesAndGoes(t *testing.T) {
	w := gunimtest.New(t, geom.Sz(800, 600), nil)
	l := listens(w)
	gunim.RegisterView(w, "confirm", NewDialog, nil)
	c := w.Client()
	if err := c.Mount(gunim.Root, "confirm", "confirm", "Delete?"); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	wantCues(t, l, "a dialog arriving", gunim.CueOpen)
	if err := c.Unmount("confirm"); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	wantCues(t, l, "a dialog leaving", gunim.CueClose)
}

func TestTabsSoundAsOneIsChosen(t *testing.T) {
	tabs := NewTabs([]string{"One", "Two"}, NewLabel("1"), NewLabel("2"))
	w, run := stage(t, &frame{child: tabs, size: geom.Sz(400, 200)})
	l := listens(w)
	w.Input(input.KeyPress{Key: input.KeyTab})
	run(1)
	w.Input(input.KeyPress{Key: input.KeyRight})
	run(1)
	wantCues(t, l, "choosing the next tab", gunim.CueSelect)
}
