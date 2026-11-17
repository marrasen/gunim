package widget

import (
	"image/color"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/paint"
)

// tagged is a row of a fixed height that paints its key's number in its colour, so a test can tell rows apart in
// what was drawn.
type tagged struct {
	n int
	h float32
}

func (b *tagged) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(geom.Sz(c.Min.W, b.h))
}

func (b *tagged) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(color.NRGBA{R: uint8(b.n), G: 0x80, A: 0xff}))
}

// newTagged mounts a 300 by 400 virtual list of keys, whose rows are height tall and paint their keys, and returns
// it with its window and a step that runs one frame and returns what it drew.
func newTagged(t *testing.T, ks []Key, height float32) (*gunim.Window, *VirtualList, func() []drawnRow) {
	t.Helper()
	l := NewVirtualList(func(k Key) gunim.Node {
		n, err := strconv.Atoi(string(k))
		if err != nil {
			t.Fatalf("key %q is not a number", k)
		}
		return &tagged{n: n, h: height}
	})
	w := gunimtest.New(t, geom.Sz(300, 400), nil)
	gunim.RegisterView(w, "v", func(shownKeys) gunim.Node { return l },
		func(_ gunim.Node, s shownKeys, u *gunim.UI) { l.SetKeys(s.Keys, u) })
	if err := w.Client().Mount(gunim.Root, "v", "v", shownKeys{ks}); err != nil {
		t.Fatal(err)
	}
	step := func() []drawnRow {
		w.Frame(time.Second / 60)
		return drawnRows(w.Offscreen().Ops())
	}
	for range 10 {
		step()
	}
	return w, l, step
}

// drawnRow is one tagged row as drawn: its key, its top in the window and the opacity of the layers round it.
type drawnRow struct {
	key     Key
	top     float32
	opacity float32
}

// drawnRows returns the tagged rows in ops.
func drawnRows(ops []paint.Op) []drawnRow {
	var out []drawnRow
	opacity := []float32{1}
	for _, op := range ops {
		switch op := op.(type) {
		case *paint.LayerOp:
			opacity = append(opacity, opacity[len(opacity)-1]*op.Opts.Opacity)
		case *paint.LayerEndOp:
			opacity = opacity[:len(opacity)-1]
		case *paint.RRectOp:
			if c := op.Fill.Solid; c.G == 0x80 && c.A == 0xff {
				out = append(out, drawnRow{
					key:     Key(strconv.Itoa(int(c.R))),
					top:     op.Transform.Apply(op.Rect.Min).Y,
					opacity: opacity[len(opacity)-1],
				})
			}
		}
	}
	return out
}

// drawnAt returns the opacity key is drawn at with its top at y, and false where it is not drawn there.
func drawnAt(rows []drawnRow, key Key, y float32) (float32, bool) {
	for _, r := range rows {
		if r.key == key && abs32(r.top-y) < 0.5 {
			return r.opacity, true
		}
	}
	return 0, false
}

// A row moved to the end keeps drawing at its old place, fading out as the place closes, and fades in at its new one.
func TestVirtualListFadesAMovedRowOutWhereItWas(t *testing.T) {
	w, l, step := newTagged(t, keys(6), 40)
	old := 40 + l.gap
	next := []Key{"0", "2", "3", "4", "5", "1"}
	if err := w.Client().Update("v", shownKeys{next}); err != nil {
		t.Fatal(err)
	}
	was, between := float32(1), false
	first, fadedIn := float32(-1), float32(0)
	for f := range 60 {
		rows := step()
		at, ok := drawnAt(rows, "1", old)
		if f == 0 && (!ok || at < 0.5) {
			t.Fatalf("on the first frame row 1 shows at its old place at %v (drawn %v), want most of it", at, ok)
		}
		if ok {
			if at > was+1e-3 {
				t.Fatalf("frame %d: row 1 at its old place grew stronger, from %v to %v", f, was, at)
			}
			was, between = at, between || at > 0.05 && at < 0.95
		}
		for _, r := range rows {
			if r.key == "1" && r.top > old+20 {
				if first < 0 {
					first = r.opacity
				}
				fadedIn = r.opacity
			}
		}
	}
	if !between {
		t.Fatal("row 1 at its old place never showed part faded")
	}
	if rows := step(); slices.ContainsFunc(rows, func(r drawnRow) bool { return r.key == "1" && abs32(r.top-old) < 0.5 }) {
		t.Fatal("row 1 still shows at its old place at rest")
	}
	if first < 0 || first > 0.9 || fadedIn < 0.999 {
		t.Fatalf("at its new place row 1 came in at %v and ended at %v, want it to fade in", first, fadedIn)
	}
}

// Rows moved in a list that fits its view keep the content inside the view on every frame, so no bar shows.
func TestVirtualListThatFitsShowsNoBarWhileRowsMove(t *testing.T) {
	w, l, step := newTagged(t, keys(6), 60)
	if l.scrollable() {
		t.Fatalf("at rest the list of %v scrolls in a view of %v", l.content, l.viewport)
	}
	for _, next := range [][]Key{{"3", "4", "5", "0", "1", "2"}, {"3", "5", "0", "1", "2", "4"}, keys(6)} {
		if err := w.Client().Update("v", shownKeys{next}); err != nil {
			t.Fatal(err)
		}
		for f := range 60 {
			step()
			if l.scrollable() {
				t.Fatalf("frame %d after moving to %q, the content is %v tall in a view of %v", f, next, l.content, l.viewport)
			}
		}
	}
}

// A list turned round fades from the old order to the new one, drawing each row in view at most twice on a frame.
func TestVirtualListCrossFadesALargeReorder(t *testing.T) {
	w, l, step := newTagged(t, keys(20), 40)
	reversed := slices.Clone(keys(20))
	slices.Reverse(reversed)
	if err := w.Client().Update("v", shownKeys{reversed}); err != nil {
		t.Fatal(err)
	}
	inView := int(l.viewport/(40+l.gap)) + 2
	both := false
	for f := range 60 {
		rows := step()
		if len(rows) > 2*inView {
			t.Fatalf("frame %d drew %d rows, want at most %d", f, len(rows), 2*inView)
		}
		oldTop, okOld := drawnAt(rows, "0", 0)
		newTop, okNew := drawnAt(rows, "19", 0)
		if f == 0 && (!okOld || oldTop < 0.5) {
			t.Fatalf("on the first frame the old first row shows at %v (drawn %v), want most of it", oldTop, okOld)
		}
		if f == 0 && okNew && newTop > 0.5 {
			t.Fatalf("on the first frame the new first row shows at %v, want it to fade in", newTop)
		}
		both = both || okOld && okNew && oldTop > 0.05 && newTop > 0.05
	}
	if !both {
		t.Fatal("the old order and the new never showed together")
	}
	rows := step()
	if at, ok := drawnAt(rows, "19", 0); !ok || at < 0.999 {
		t.Fatalf("at rest the new first row shows at %v (drawn %v)", at, ok)
	}
	if _, ok := drawnAt(rows, "0", 0); ok {
		t.Fatal("at rest the old first row still shows")
	}
	wantStacked(t, l, reversed)
}
