package widget

import (
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// stageUI returns the UI of w, a window from stage.
func stageUI(t *testing.T, w *gunim.Window, run func(int)) *gunim.UI {
	t.Helper()
	var u *gunim.UI
	gunim.RegisterPatch(w, "stage", func(_ gunim.Node, _ probeFocus, ui *gunim.UI) { u = ui })
	if err := w.Client().Patch("stage", probeFocus{}); err != nil {
		t.Fatal(err)
	}
	run(1)
	return u
}

// rightClick presses and lets go of the secondary button at x, y.
func rightClick(w *gunim.Window, x, y float32) {
	w.Input(input.PointerDown{Pos: geom.Pt(x, y), Button: input.ButtonSecondary, Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(x, y), Button: input.ButtonSecondary, Time: time.Now()})
}

// doubleClick clicks twice at x, y, the second press counting two.
func doubleClick(w *gunim.Window, x, y float32) {
	click(w, x, y)
	w.Input(input.PointerDown{Pos: geom.Pt(x, y), Clicks: 2, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(x, y), Time: time.Now()})
}

// fingerScroll lands a finger at x, y and drags it up, as to scroll what lies under it.
func fingerScroll(w *gunim.Window, run func(int), x, y float32) {
	w.Input(input.PointerDown{Pos: geom.Pt(x, y), Clicks: 1, Touch: true, Time: time.Now()})
	for i := range 6 {
		w.Input(input.PointerMove{Pos: geom.Pt(x-float32(i)*12, y), Touch: true, Time: time.Now()})
		run(1)
	}
	w.Input(input.PointerUp{Pos: geom.Pt(x-60, y), Touch: true, Time: time.Now()})
	run(1)
}

func TestAButtonActsOnceForAPrimaryClickLetGoOverIt(t *testing.T) {
	b := NewButton("Go")
	b.On = pressed{1}
	w, run := stage(t, &frame{child: Row(b), size: geom.Sz(300, 40)})
	mid := geom.Pt(b.size.W/2, b.size.H/2)
	rightClick(w, mid.X, mid.Y)
	run(1)
	if got := sent(w); len(got) != 0 {
		t.Fatalf("a right click sent %v, want nothing", got)
	}
	doubleClick(w, mid.X, mid.Y)
	run(1)
	if got := sent(w); len(got) != 1 {
		t.Fatalf("a double click sent %v, want one intent", got)
	}
	w.Input(input.PointerDown{Pos: mid, Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(280, mid.Y), Time: time.Now()})
	run(1)
	if got := sent(w); len(got) != 0 {
		t.Fatalf("a press let go off the button sent %v, want nothing", got)
	}
}

func TestATabTitleChosenOnlyByAPrimaryClickOnIt(t *testing.T) {
	titles := make([]string, 12)
	pages := make([]gunim.Node, 12)
	for i := range titles {
		titles[i] = "Title " + string(rune('A'+i))
		pages[i] = &recorder{}
	}
	tabs := NewTabs(titles, pages...)
	w, run := stage(t, &frame{child: tabs, size: geom.Sz(300, 200)})
	x := tabs.spans[1][0] + 5
	rightClick(w, x, 10)
	run(1)
	if s := tabs.Selected(); s != 0 {
		t.Fatalf("a right click chose tab %d, want the first kept", s)
	}
	fingerScroll(w, run, x, 10)
	if s := tabs.Selected(); s != 0 {
		t.Fatalf("a finger scrolling the row chose tab %d, want the first kept", s)
	}
	if tabs.offTo <= 0 {
		t.Fatal("the finger did not scroll the row")
	}
	run(30)
	// The title now under the middle of the row, and the one after it.
	x = 150
	i := tabs.bar.titleAt(geom.Pt(x, 10))
	if i < 1 {
		t.Fatalf("title %d lies under the middle of the scrolled row", i)
	}
	w.Input(input.PointerDown{Pos: geom.Pt(x, 10), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(tabs.spans[i-1][0]+5-tabs.off.Value(), 10), Time: time.Now()})
	run(1)
	if s := tabs.Selected(); s != 0 {
		t.Fatalf("a press let go on the title before chose tab %d, want the first kept", s)
	}
	click(w, x, 10)
	run(1)
	if s := tabs.Selected(); s != i {
		t.Fatalf("a click on title %d chose tab %d", i, s)
	}
}

func TestALinkFollowsOnceForADoubleClickAndOnRelease(t *testing.T) {
	l := NewLink("show 12 more")
	l.On = followed{}
	w, run := stage(t, &frame{child: Row(l), size: geom.Sz(400, 40)})
	w.Input(input.PointerDown{Pos: geom.Pt(5, 5), Clicks: 1, Time: time.Now()})
	run(1)
	if got := sent(w); len(got) != 0 {
		t.Fatalf("a press alone sent %v, want nothing until the release", got)
	}
	w.Input(input.PointerUp{Pos: geom.Pt(5, 5), Time: time.Now()})
	w.Input(input.PointerDown{Pos: geom.Pt(5, 5), Clicks: 2, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(5, 5), Time: time.Now()})
	run(1)
	if got := sent(w); len(got) != 1 {
		t.Fatalf("a double click sent %v, want the link followed once", got)
	}
}

func TestAChipsCrossRemovesOnceForADoubleClick(t *testing.T) {
	c := NewChip("level", "error")
	c.OnRemove = func() gunim.Intent { return removed{"error"} }
	w, run := stage(t, &frame{child: Row(c), size: geom.Sz(400, 40)})
	doubleClick(w, c.crossX, 12)
	run(1)
	if got := sent(w); len(got) != 1 {
		t.Fatalf("a double click on the cross sent %v, want one removal", got)
	}
}

func TestAPaletteRowPicksItsOwnItemOnAClickAndNothingForAFinger(t *testing.T) {
	w, o, run := newPaletteStage(t)
	focusOpener(w, run)
	w.Input(input.KeyPress{Key: input.KeyF1})
	run(20)
	c := o.p.card
	var other *paletteRow
	for _, r := range c.list.live {
		if pr, ok := r.child.(*paletteRow); ok && pr.index != c.hotIndex() {
			other = pr
		}
	}
	if other == nil {
		t.Fatal("no row but the highlighted one is built")
	}
	mid := geom.Pt(other.size.W/2, other.size.H/2)
	// A finger that lands on the row to scroll is let go far away.
	other.Handle(input.PointerDown{Pos: mid, Clicks: 1, Touch: true}, o.u)
	other.Handle(input.PointerUp{Pos: input.Away, Touch: true}, o.u)
	if len(o.picked) != 0 || !o.p.IsOpen() {
		t.Fatalf("a finger scrolling picked %v, want nothing and the palette open", o.picked)
	}
	other.Handle(input.PointerDown{Pos: mid, Clicks: 1}, o.u)
	other.Handle(input.PointerUp{Pos: mid}, o.u)
	if len(o.picked) != 1 || o.picked[0] != other.index {
		t.Fatalf("a click on item %d picked %v, want it, the keyboard's row being %d", other.index, o.picked, c.hotIndex())
	}
}

func TestAnEmojiIsPickedOnAClickAndNotByAFingerLandingToScroll(t *testing.T) {
	picker, w, run, picked := openPicker(t)
	u := stageUI(t, w, run)
	c := picker.card
	var row *emojiRow
	for _, r := range c.list.live {
		if er, ok := r.child.(*emojiRow); ok && len(er.row.emoji) > 0 {
			row = er
			break
		}
	}
	if row == nil {
		t.Fatal("no row of emoji is built")
	}
	mid := geom.Pt(row.cell/2, row.cell/2)
	row.Handle(input.PointerDown{Pos: mid, Clicks: 1, Touch: true}, u)
	if len(*picked) != 0 {
		t.Fatalf("a finger landing picked %q, want nothing yet", *picked)
	}
	row.Handle(input.PointerUp{Pos: input.Away, Touch: true}, u)
	if len(*picked) != 0 || !picker.IsOpen() {
		t.Fatalf("a finger scrolling picked %q, want nothing and the picker open", *picked)
	}
	row.Handle(input.PointerDown{Pos: mid, Clicks: 1}, u)
	row.Handle(input.PointerUp{Pos: geom.Pt(mid.X+row.cell, mid.Y)}, u)
	if len(*picked) != 0 {
		t.Fatalf("a press let go on the next emoji picked %q, want nothing", *picked)
	}
	row.Handle(input.PointerDown{Pos: mid, Clicks: 1}, u)
	row.Handle(input.PointerUp{Pos: mid}, u)
	if len(*picked) != 1 || (*picked)[0] != row.row.emoji[0].Text {
		t.Fatalf("a click picked %q, want the first emoji", *picked)
	}
}

func TestARightPressLeavesAToast(t *testing.T) {
	w, h, wait := newToastStage(t)
	w.Input(input.KeyPress{Key: input.KeyF2})
	wait(500 * time.Millisecond)
	w.Input(input.PointerMove{Pos: geom.Pt(20, 20), Time: time.Now()})
	rightClick(w, 20, 20)
	wait(100 * time.Millisecond)
	if n := h.t.Len(); n != 1 {
		t.Fatalf("after a right click, %d toasts showing, want 1", n)
	}
}

func TestASegmentedOptionChosenOnlyByAPrimaryClickOnIt(t *testing.T) {
	s := NewSegmented("One", "Two", "Three")
	s.OnChange = func(i int) gunim.Intent { return segmentChosen{i} }
	w, run := stage(t, &frame{child: Row(s), size: geom.Sz(600, 40)})
	rightClick(w, s.width*1.5, 14)
	run(1)
	w.Input(input.PointerDown{Pos: geom.Pt(s.width*1.5, 14), Clicks: 1, Time: time.Now()})
	w.Input(input.PointerUp{Pos: geom.Pt(s.width*2.5, 14), Time: time.Now()})
	run(1)
	if got := sent(w); s.Selected() != 0 || len(got) != 0 {
		t.Fatalf("a right click and a press let go on the next option chose %d and sent %v, want neither", s.Selected(), got)
	}
}

func TestATreeRowActivatesOnceForADoubleClick(t *testing.T) {
	w, _, run, apply := newTreeStage(t)
	doubleClick(w, 50, TreeRowHeight.Default()/2)
	run(1)
	if got := apply(); len(got) != 1 {
		t.Fatalf("a double click sent %v, want one activation", got)
	}
}

func TestARichTextLinkFollowsOnceForADoubleClick(t *testing.T) {
	r := NewRichText(RichSpan{Text: "manual", On: openedLink{"https://example.com"}})
	w, run := stage(t, &frame{child: r, size: geom.Sz(200, 300)})
	pc := r.laid.Lines[0].Pieces[0]
	at := geom.Pt(pc.At.X+pc.Run.Advance/2, pc.At.Y+pc.Run.Height()/2)
	doubleClick(w, at.X, at.Y)
	run(1)
	if got := sent(w); len(got) != 1 {
		t.Fatalf("a double click on the link sent %v, want it followed once", got)
	}
}
