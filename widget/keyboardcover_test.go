package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/paint"
)

func TestAViewHearsHowFarUpTheKeyboardReaches(t *testing.T) {
	w := gunimtest.New(t, geom.Sz(400, 800), NewSurface())
	k := &keyboardProbe{}
	gunim.RegisterView(w, "view", func(struct{}) gunim.Node { return k }, nil)
	if err := w.Client().Mount(gunim.Root, "v", "view", struct{}{}); err != nil {
		t.Fatal(err)
	}
	// The keyboard slides in over three frames, and out again.
	for i, h := range []float32{0, 120, 260, 300, 0} {
		w.Offscreen().SetKeyboardCover(h)
		w.Input(driver.Redraw{})
		w.Frame(time.Second / 60)
		if k.got != h {
			t.Fatalf("frame %d: the view hears the keyboard reach up %v, want %v", i, k.got, h)
		}
	}
}

// keyboardProbe records how far up the keyboard reached in the frame it
// was last laid out in.
type keyboardProbe struct{ got float32 }

func (p *keyboardProbe) Layout(c gunim.Constraints, f gunim.Frame, _ gunim.Children) geom.Size {
	p.got = f.Keyboard
	return c.Max
}

func (p *keyboardProbe) Paint(*paint.Painter, gunim.Frame, geom.Size, gunim.Children) {}
