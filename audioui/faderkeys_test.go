package audioui

import (
	"math"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
)

// A fader takes the keyboard by Tab, rings its cap, and steps its gain by the keys as a slider does, the cap gliding
// to each new gain.
func TestAFaderWorksFromTheKeyboard(t *testing.T) {
	var gain float32
	f := NewFader(func() float32 { return gain }, func(v float32, _ *gunim.UI) gunim.Intent { gain = v; return nil })
	w := gunimtest.New(t, geom.Sz(24, 224), nil)
	gunim.RegisterView(w, "fader", func(struct{}) gunim.Node { return f }, nil)
	if err := w.Client().Mount(gunim.Root, "fader", "fader", nil); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(1)
	w.Input(input.KeyPress{Key: input.KeyTab})
	run(30)
	if r := f.ring.Value(); r < 0.99 {
		t.Fatalf("Tab left the fader's ring at %v, want it on the fader", r)
	}
	for _, c := range []struct {
		k    input.KeyPress
		want float32
	}{
		{input.KeyPress{Key: input.KeyUp}, 0.5},
		{input.KeyPress{Key: input.KeyUp, Mods: input.ModShift}, 0.6},
		{input.KeyPress{Key: input.KeyPageUp}, 5.4},
		{input.KeyPress{Key: input.KeyLeft}, 4.9},
		{input.KeyPress{Key: input.KeyEnd}, 24},
		{input.KeyPress{Key: input.KeyHome}, -24},
		{input.KeyPress{Key: input.KeyPageDown}, -24},
	} {
		was := gain
		w.Input(c.k)
		run(1)
		if math.Abs(float64(gain-c.want)) > 0.01 {
			t.Fatalf("%v took the gain to %.2f dB, want %.2f", c.k.Key, gain, c.want)
		}
		// The cap shows the old gain first and glides to the new one, every frame nearer.
		last := float32(math.Abs(float64(f.nudge.Value())))
		first := last
		if was != gain && last < 0.01 {
			t.Fatalf("a frame after %v, the cap shows the new gain already: it jumped", c.k.Key)
		}
		for k := range 40 {
			run(1)
			n := float32(math.Abs(float64(f.nudge.Value())))
			// A spring may bounce a little past where it rests.
			if n > last+0.15*first {
				t.Fatalf("frame %d of %v, the cap drew away from the gain, %v to %v dB off", k+1, c.k.Key, last, n)
			}
			last = n
		}
		if last > 0.01 {
			t.Fatalf("after %v the cap rests %v dB off the gain", c.k.Key, last)
		}
	}
}
