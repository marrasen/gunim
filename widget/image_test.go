package widget

import (
	"image"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/paint"
)

func picture(w, h int) *paint.Image { return paint.NewImage(image.NewRGBA(image.Rect(0, 0, w, h))) }

func TestFitRects(t *testing.T) {
	m := picture(200, 100)
	box := geom.Sz(100, 100)
	tests := []struct {
		fit      Fit
		dst, src geom.Rect
	}{
		{FitContain, geom.Rc(0, 25, 100, 50), geom.Rc(0, 0, 200, 100)},
		{FitCover, geom.Rc(0, 0, 100, 100), geom.Rc(50, 0, 100, 100)},
		{FitFill, geom.Rc(0, 0, 100, 100), geom.Rc(0, 0, 200, 100)},
	}
	for _, tt := range tests {
		dst, src := fitRects(tt.fit, box, m)
		if dst != tt.dst || src != tt.src {
			t.Errorf("fit %d: dst %v src %v, want %v and %v", tt.fit, dst, src, tt.dst, tt.src)
		}
	}
}

// imageOps returns the image ops in the window's last frame.
func imageOps(w interface{ Ops() []paint.Op }) []*paint.ImageOp {
	var out []*paint.ImageOp
	for _, op := range w.Ops() {
		if im, ok := op.(*paint.ImageOp); ok {
			out = append(out, im)
		}
	}
	return out
}

// shown is the state of a view showing one picture.
type shown struct{ Pic *paint.Image }

func TestImageCrossfadesToANewSource(t *testing.T) {
	a, b := picture(10, 10), picture(10, 10)
	img := NewImage(a)
	w := gunimtest.New(t, geom.Sz(100, 100), nil)
	gunim.RegisterView(w, "pic", func(shown) *frame { return &frame{child: img, size: geom.Sz(10, 10)} },
		func(_ *frame, s shown, u *gunim.UI) { img.SetSource(s.Pic, u) })
	if err := w.Client().Mount(gunim.Root, "pic", "pic", shown{a}); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	dw := w.Offscreen()
	run(120)
	if ops := imageOps(dw); len(ops) != 1 || ops[0].Image != a || ops[0].Opacity != 1 {
		t.Fatalf("settled on %d ops, want picture a at full opacity", len(ops))
	}

	if err := w.Client().Update("pic", shown{b}); err != nil {
		t.Fatal(err)
	}
	run(3)
	ops := imageOps(dw)
	if len(ops) != 2 || ops[0].Image != a || ops[1].Image != b {
		t.Fatalf("mid-fade drew %d ops, want a under b", len(ops))
	}
	if ops[1].Opacity <= 0 || ops[1].Opacity >= 1 {
		t.Fatalf("new picture at opacity %v mid-fade", ops[1].Opacity)
	}
	run(120)
	if ops := imageOps(dw); len(ops) != 1 || ops[0].Image != b {
		t.Fatal("the old picture stayed after the fade")
	}
}
