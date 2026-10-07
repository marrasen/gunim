package calendar

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// faintness is the opacity of the first layer in ops that clips nothing, or 1.
func faintness(ops []paint.Op) float32 {
	for _, op := range ops {
		if l, ok := op.(*paint.LayerOp); ok && !l.Opts.Clip {
			return l.Opts.Opacity
		}
	}
	return 1
}

// A date field disabled fades frame by frame, takes no click, key or focus, and fades back when enabled again.
func TestADisabledDateFieldFadesAndTakesNothing(t *testing.T) {
	f := NewDateField(monday)
	w, run, sent := stage(t, &frame{child: f, size: geom.Sz(170, 32)})
	set := func(on bool) { withUI(t, w, func(u *gunim.UI) { f.Disabled = on; u.Invalidate() }) }
	for _, on := range []bool{true, false} {
		set(on)
		last := float32(1)
		if !on {
			last = 0
		}
		for k := range 30 {
			run(1)
			o := faintness(w.Offscreen().Ops())
			if (on && o > last+1e-4) || (!on && o < last-1e-4) {
				t.Fatalf("frame %d after Disabled=%v, the field went from opacity %v to %v", k+1, on, last, o)
			}
			if k == 1 && (o <= 0.401 || o >= 0.999) {
				t.Fatalf("two frames after Disabled=%v, the field paints at opacity %v, want part way", on, o)
			}
			last = o
		}
		if !on {
			break
		}
		click(w, run, geom.Pt(20, 16))
		w.Input(input.KeyPress{Key: input.KeyTab})
		w.Input(input.KeyPress{Key: input.KeyDown})
		run(2)
		if f.popup != nil || len(sent()) != 0 || f.Value() != monday {
			t.Fatal("the disabled date field opened its month, sent, or moved its day")
		}
		if f.Focusable() {
			t.Fatal("the disabled date field takes the keyboard")
		}
	}
}

// A disabled time field opens no list on a click.
func TestADisabledTimeFieldOpensNoList(t *testing.T) {
	f := NewTimeField(9 * time.Hour)
	f.Disabled = true
	w, run, _ := stage(t, &frame{child: f, size: geom.Sz(90, 32)})
	click(w, run, geom.Pt(20, 16))
	if f.list != nil {
		t.Fatal("a click on a disabled time field opened its list")
	}
}
