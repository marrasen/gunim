package widget

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
)

// A finger that lands on a tree's row to scroll moves the cursor nowhere and opens nothing; a click puts the cursor
// on the row it lets go on.
func TestATreeRowTakesTheCursorOnAClickAndNotFromAFinger(t *testing.T) {
	w, tr, run, apply := newTreeStage(t)
	h := TreeRowHeight.Default()
	fingerScroll(w, run, 150, h*1.5)
	if k, on := tr.Cursor(); on {
		t.Fatalf("a finger landing on b to scroll put the cursor on %q", k)
	}
	if got := apply(); len(got) != 0 {
		t.Fatalf("a finger landing on b to scroll sent %v", got)
	}
	w.Input(input.PointerDown{Pos: geom.Pt(50, h*1.5), Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	run(1)
	if k, on := tr.Cursor(); on {
		t.Fatalf("a press on b, before its release, put the cursor on %q", k)
	}
	w.Input(input.PointerUp{Pos: geom.Pt(50, h*1.5), Button: input.ButtonPrimary, Time: time.Now()})
	run(1)
	if k, _ := tr.Cursor(); k != "b" {
		t.Fatalf("a click on b put the cursor on %q", k)
	}
}

// A press on a row that goes before its release, as a branch shuts under it, activates nothing.
func TestATreeRowGoneBeforeItsReleaseIsNotActivated(t *testing.T) {
	w, tr, run, apply := newTreeStage(t)
	h := TreeRowHeight.Default()
	click(w, 50, h/2)
	run(1)
	apply()
	run(60)
	// a/1 is the second row; press it, and shut a before the release.
	w.Input(input.PointerDown{Pos: geom.Pt(50, h*1.5), Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	run(1)
	w.Input(input.KeyPress{Key: input.KeyLeft})
	run(1)
	apply()
	for i := range 3 {
		run(1)
		if _, held := tr.index["a/1"]; held {
			t.Fatalf("frame %d: a/1 is still held after a shut", i)
		}
	}
	w.Input(input.PointerUp{Pos: geom.Pt(50, h*1.5), Button: input.ButtonPrimary, Time: time.Now()})
	run(1)
	for _, k := range apply() {
		if k == "a/1" {
			t.Fatal("a release on a/1, gone since its press, activated it")
		}
	}
}

// A row set in deep, with a long detail, keeps its text inside the tree, every frame of its branch opening.
func TestATreeRowKeepsItsTextInside(t *testing.T) {
	tr := NewTree()
	long := strings.Repeat("a long name ", 10)
	tr.Item = func(k Key) TreeItem {
		d, _ := strconv.Atoi(string(k))
		return TreeItem{Text: long, Depth: d, Detail: "12 345 678 bytes in a detail that runs on and on", Branch: true}
	}
	w := gunimtest.New(t, geom.Sz(200, 300), nil)
	keys := []Key{"0"}
	gunim.RegisterView(w, "t", func(struct{}) *Tree { return tr },
		func(tr *Tree, _ struct{}, u *gunim.UI) { tr.SetKeys(keys, u) })
	if err := w.Client().Mount(gunim.Root, "t", "t", nil); err != nil {
		t.Fatal(err)
	}
	if err := w.Client().Update("t", struct{}{}); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	keys = []Key{"0", "2", "5", "9", "14"}
	if err := w.Client().Update("t", struct{}{}); err != nil {
		t.Fatal(err)
	}
	for i := range 30 {
		w.Frame(time.Second / 60)
		var texts []spill
		for _, s := range shownOps(w.Offscreen().Ops()) {
			if s.r.Min.X < -0.5 || s.r.Max.X > 200.5 {
				t.Fatalf("frame %d: a %s shows from x=%v to %v, outside the tree", i, s.what, s.r.Min.X, s.r.Max.X)
			}
			if s.what == "text" {
				texts = append(texts, s)
			}
		}
		// The name and the detail of a row, on one line, keep apart.
		for a, s := range texts {
			for _, o := range texts[a+1:] {
				if s.r.Min.Y < o.r.Max.Y && o.r.Min.Y < s.r.Max.Y && s.r.Min.X < o.r.Max.X-0.5 && o.r.Min.X < s.r.Max.X-0.5 {
					t.Fatalf("frame %d: text from x=%v to %v runs over text from %v to %v", i, s.r.Min.X, s.r.Max.X,
						o.r.Min.X, o.r.Max.X)
				}
			}
		}
	}
	// The detail keeps to half the room, and the text to the rest.
	r, ok := tr.list.live["2"].child.(*treeRow)
	if !ok || !r.cut {
		t.Fatal("a long name beside a long detail is not cut short")
	}
}
