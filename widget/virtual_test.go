package widget

import (
	"strconv"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
)

// block is a row of a fixed height.
type block struct {
	key Key
	h   float32
}

func (b *block) Layout(c gunim.Constraints, _ gunim.Frame, _ gunim.Children) geom.Size {
	return c.Constrain(geom.Sz(c.Min.W, b.h))
}

func (b *block) Paint(p *paint.Painter, _ gunim.Frame, box geom.Size, _ gunim.Children) {
	p.RRect(geom.Rect{Max: box.Point()}, 0, paint.Solid(Ink.Default()))
}

func keys(n int) []Key {
	out := make([]Key, n)
	for i := range out {
		out[i] = Key(strconv.Itoa(i))
	}
	return out
}

// shownKeys is the state of a view holding a virtual list.
type shownKeys struct{ Keys []Key }

// newVirtual mounts a 300 by 400 virtual list of the given keys, whose
// rows are height tall, and returns it with its window.
func newVirtual(t *testing.T, ks []Key, height float32) (*gunim.Window, *VirtualList, func(int)) {
	t.Helper()
	blocks := map[Key]*block{}
	l := NewVirtualList(func(k Key) gunim.Node {
		b := &block{key: k, h: height}
		blocks[k] = b
		return b
	})
	w := gunim.NewOffscreen(geom.Sz(300, 400), nil)
	gunim.RegisterView(w, "v", func(shownKeys) gunim.Node { return l },
		func(_ gunim.Node, s shownKeys, u *gunim.UI) { l.SetKeys(s.Keys, u) })
	if err := w.Client().Mount(gunim.Root, "v", "v", shownKeys{ks}); err != nil {
		t.Fatal(err)
	}
	run := func(n int) {
		for range n {
			w.Frame(time.Second / 60)
		}
	}
	run(2)
	return w, l, run
}

func TestVirtualListBuildsOnlyWhatIsInView(t *testing.T) {
	w, l, run := newVirtual(t, keys(100_000), 40)
	// 400 of view and 300 beyond it below, at 46 a row.
	if b := l.Built(); b < 10 || b > 30 {
		t.Fatalf("%d rows built for a view of about 9, want a few dozen", b)
	}
	if l.Len() != 100_000 {
		t.Fatalf("Len %d, want 100000", l.Len())
	}
	for range 50 {
		w.Input(input.Scroll{Pos: geom.Pt(100, 100), Delta: geom.Pt(0, -4000)})
		run(2)
	}
	run(120)
	if o := l.Offset(); o < 150_000 {
		t.Fatalf("scrolled to %v, want far down", o)
	}
	if b := l.Built(); b < 10 || b > 40 {
		t.Fatalf("%d rows built after scrolling, want a few dozen", b)
	}
	for k := range l.live {
		i := keyIndex(t, k)
		if y := float32(i) * 46; y < l.Offset()-400 || y > l.Offset()+800 {
			t.Fatalf("row %d, at %v, is built far from the view at %v", i, y, l.Offset())
		}
	}
}

func TestVirtualListAnimatesARemovedRowInView(t *testing.T) {
	w, l, run := newVirtual(t, keys(50), 40)
	r := l.live["2"]
	if err := w.Client().Update("v", shownKeys{append(keys(2), keys(50)[3:]...)}); err != nil {
		t.Fatal(err)
	}
	run(3)
	if _, ok := l.live["2"]; !ok || r.height.Value() >= 40 || r.height.Value() <= 0 {
		t.Fatalf("row 2 is not collapsing: height %v", r.height.Value())
	}
	run(120)
	if _, ok := l.live["2"]; ok || l.Len() != 49 {
		t.Fatalf("row 2 stayed, or the list holds %d", l.Len())
	}
	// Row 3 has closed the gap.
	if y := l.live["3"].y.Value(); y != 2*46 {
		t.Fatalf("row 3 at %v, want 92", y)
	}
}

func TestVirtualListKeepsTheViewStillAsRowsAboveAreMeasured(t *testing.T) {
	// Rows are 80 tall, where the list assumes 40, so every row built
	// above the view moves the rows below it.
	_, l, run := newVirtual(t, keys(1000), 80)
	l.ScrollTo(20_000, Quick.Default())
	run(120)
	// Take the first row in view, then scroll up, which builds rows
	// above it. It should move down the screen by the scroll alone.
	first, before := firstInView(l)
	l.ScrollTo(l.Offset()-300, Quick.Default())
	run(120)
	after := l.live[first].y.Value() - l.Offset()
	if d := after - before; d < 299 || d > 301 {
		t.Fatalf("row %s moved %v down the screen for a scroll of 300", first, d)
	}
	// And the rows in view sit a measured row apart.
	a, b := l.live[first], l.live[nextKey(t, first)]
	if gap := b.y.Value() - a.y.Value(); gap != 86 {
		t.Fatalf("rows in view are %v apart, want 86", gap)
	}
}

// firstInView returns the key of the topmost row in view, and its top
// on screen.
func firstInView(l *VirtualList) (first Key, top float32) {
	top = 1e9
	for k, r := range l.live {
		if y := r.y.Value() - l.Offset(); y >= 0 && y < top {
			first, top = k, y
		}
	}
	return first, top
}

func keyIndex(t *testing.T, k Key) int {
	t.Helper()
	i, err := strconv.Atoi(string(k))
	if err != nil {
		t.Fatalf("key %q is no number", k)
	}
	return i
}

func nextKey(t *testing.T, k Key) Key { return Key(strconv.Itoa(keyIndex(t, k) + 1)) }

func TestFenwickMatchesPlainSums(t *testing.T) {
	vals := []float32{3, 0, 5, 2, 0, 0, 7, 1, 4}
	var f fenwick
	f.reset(vals)
	f.add(4, 6)
	vals[4] += 6
	run := float64(0)
	for i, v := range vals {
		if got := f.sum(i); got != run {
			t.Fatalf("sum(%d) = %v, want %v", i, got, run)
		}
		run += float64(v)
	}
	for y := 0.0; y < run+2; y += 0.5 {
		want, acc := len(vals), 0.0
		for i, v := range vals {
			acc += float64(v)
			if acc > y {
				want = i
				break
			}
		}
		if got := f.find(y); got != want {
			t.Fatalf("find(%v) = %d, want %d", y, got, want)
		}
	}
}

func TestVirtualListGrowsAnAddedRowInView(t *testing.T) {
	w, l, run := newVirtual(t, keys(50), 40)
	ks := append([]Key{"new"}, keys(50)...)
	if err := w.Client().Update("v", shownKeys{ks}); err != nil {
		t.Fatal(err)
	}
	run(3)
	r, ok := l.live["new"]
	if !ok || r.instant || r.height.Value() <= 0 || r.height.Value() >= 40 {
		t.Fatalf("the new row is not growing in: built %v", ok)
	}
	run(120)
	if y := l.live["0"].y.Value(); y != 46 {
		t.Fatalf("row 0 at %v after the new row opened, want 46", y)
	}
}

// BenchmarkVirtualListFrame times a frame of a 100,000-row list
// scrolling, with rows built and dropped as they pass.
func BenchmarkVirtualListFrame(b *testing.B) {
	blocksT := &testing.T{}
	w, l, _ := newVirtual(blocksT, keys(100_000), 40)
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		w.Input(input.Scroll{Pos: geom.Pt(100, 100), Delta: geom.Pt(0, -60)})
		w.Frame(time.Second / 60)
		_ = i
	}
	b.ReportMetric(float64(l.Built()), "rows/frame")
}

func TestVirtualListHoldsTheViewAsItemsArriveAbove(t *testing.T) {
	w, l, run := newVirtual(t, keys(1000), 40)
	l.ScrollTo(4600, Quick.Default())
	run(120)
	first, before := firstInView(l)
	ks := append([]Key{"a", "b", "c"}, keys(1000)...)
	if err := w.Client().Update("v", shownKeys{ks}); err != nil {
		t.Fatal(err)
	}
	run(60)
	if after := l.live[first].y.Value() - l.Offset(); after != before {
		t.Fatalf("row %s moved from %v to %v on screen as items arrived above", first, before, after)
	}
}
