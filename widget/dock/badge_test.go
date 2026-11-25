package dock

import (
	"image/color"
	"slices"
	"strconv"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/icon"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
	"github.com/marrasen/gunim/widget"
)

// badged are tabs as many as n, the second saying badge.
func badged(n int, badge string) barTabs {
	out := barTabs{front: "1"}
	for i := range n {
		out.tabs = append(out.tabs, Tab{ID: strconv.Itoa(i + 1), Face: Face{Title: "Terminal", Icon: icon.SquareTerminal, Closable: true}})
	}
	out.tabs[1].Badge, out.tabs[1].BadgeInk = badge, &widget.ToastErrorInk
	return out
}

// pillOn returns the pill of tab id's badge as drawn, in ink, and false
// where none is.
func pillOn(w *gunim.Window, b *Bar, id string, ink color.NRGBA) (pill geom.Rect, fill color.NRGBA, ok bool) {
	i := b.boxOf(id)
	tab, _ := b.drawnAt(i)
	for _, op := range w.Offscreen().Ops() {
		r, is := op.(*paint.RRectOp)
		if !is || r.Fill.Solid.A == 0 {
			continue
		}
		c := r.Fill.Solid
		c.A = ink.A
		mid := r.Rect.Center()
		if c == ink && tab.Contains(mid) && r.Rect.Size().H > 3 {
			return r.Rect, r.Fill.Solid, true
		}
	}
	return geom.Rect{}, color.NRGBA{}, false
}

// smooth follows a badge on the second tab frame by frame, and fails
// where it or its tab jumps, or it leaves the tab.
func smooth(t *testing.T, w *gunim.Window, b *Bar, run func(int), what string, ink color.NRGBA) {
	t.Helper()
	tab, _ := b.drawnAt(1)
	pill, fill, _ := pillOn(w, b, "2", ink)
	for frame := range 60 {
		run(1)
		nowTab, _ := b.drawnAt(1)
		nowPill, nowFill, on := pillOn(w, b, "2", ink)
		far := func(a, b float32, most float32) bool { return a-b > most || b-a > most }
		if far(nowTab.Size().W, tab.Size().W, 24) {
			t.Fatalf("%s, frame %d: the tab went from %v to %v wide", what, frame, tab.Size().W, nowTab.Size().W)
		}
		if far(nowPill.Size().W, pill.Size().W, 24) || int(nowFill.A)-int(fill.A) > 60 || int(fill.A)-int(nowFill.A) > 60 {
			t.Fatalf("%s, frame %d: the badge went from %v wide at %d to %v at %d", what, frame, pill.Size().W, fill.A, nowPill.Size().W, nowFill.A)
		}
		if on && (nowPill.Min.X < nowTab.Min.X-0.5 || nowPill.Max.X > nowTab.Max.X+0.5 || nowPill.Min.Y < nowTab.Min.Y || nowPill.Max.Y > nowTab.Max.Y) {
			t.Fatalf("%s, frame %d: the badge is at %v, outside its tab at %v", what, frame, nowPill, nowTab)
		}
		tab, pill, fill = nowTab, nowPill, nowFill
	}
}

// TestATabsBadgeComesAndGoes: grown and faded in, its tab widening for
// it, its words changed in a fade, and away, on every frame.
func TestATabsBadgeComesAndGoes(t *testing.T) {
	b := NewBar("g", 1)
	w, run, _ := stageBar(t, b, badged(3, ""))
	run(30)
	ink := widget.ToastErrorInk.Default()
	show := func(badge string) {
		t.Helper()
		if err := w.Client().Update("v", badged(3, badge)); err != nil {
			t.Fatal(err)
		}
	}
	bare := b.boxes[1].Size().W
	show("Ada")
	smooth(t, w, b, run, "coming", ink)
	pill, _, ok := pillOn(w, b, "2", ink)
	if grew := b.boxes[1].Size().W - bare; !ok || grew < pill.Size().W || grew > pill.Size().W+8 {
		t.Fatalf("with a badge %v wide the tab is %v wider", pill.Size().W, grew)
	}
	if run := b.badges["2"].run; pill.Size().W < run.Advance {
		t.Fatalf("the badge is %v wide, its words %v", pill.Size().W, run.Advance)
	}
	// The × that shows under the pointer, and goes, moves it nowhere.
	for _, at := range []geom.Point{barTabAt(t, b, "2"), geom.Pt(790, 18)} {
		w.Input(input.PointerMove{Pos: at})
		run(1)
		if now, _, _ := pillOn(w, b, "2", ink); now != pill {
			t.Fatalf("with the pointer at %v the badge went from %v to %v", at, pill, now)
		}
	}
	show("Ada and Grace")
	smooth(t, w, b, run, "changing its words", ink)
	if wider, _, _ := pillOn(w, b, "2", ink); wider.Size().W <= pill.Size().W {
		t.Fatalf("with more to say the badge is %v wide, from %v", wider.Size().W, pill.Size().W)
	}
	show("Bo")
	run(2)
	show("Cy")
	smooth(t, w, b, run, "changing its words in a fade", ink)
	if acc := b.Access(); acc.Parts[1].Name != "Terminal, Cy" || acc.Parts[0].Name != "Terminal" {
		t.Errorf("a screen reader reads %q and %q", acc.Parts[0].Name, acc.Parts[1].Name)
	}
	show("")
	smooth(t, w, b, run, "going", ink)
	if _, _, ok := pillOn(w, b, "2", ink); ok || len(b.badges) != 0 || b.boxes[1].Size().W != bare || !w.Still() {
		t.Errorf("gone, the badge shows %v, %d are kept, the tab is %v wide, from %v, and the bar rests %v", ok, len(b.badges), b.boxes[1].Size().W, bare, w.Still())
	}
	// Away in the middle of coming, and its tab with it.
	show("Ada")
	run(4)
	show("")
	smooth(t, w, b, run, "away on its way in", ink)
	show("Ada")
	run(30)
	tabs := badged(3, "Ada")
	tabs.tabs = slices.Delete(tabs.tabs, 1, 2)
	if err := w.Client().Update("v", tabs); err != nil {
		t.Fatal(err)
	}
	run(1)
	if len(b.gone) != 1 || b.gone[0].badge == nil || len(b.badges) != 0 {
		t.Errorf("the tab closing left %d, and %d badges", len(b.gone), len(b.badges))
	}
	run(60)
}

// TestABadgeIsThereAsItsTabIs: on the first frame, and on a tab new on
// the bar, which grows in with it.
func TestABadgeIsThereAsItsTabIs(t *testing.T) {
	b := NewBar("g", 1)
	w, run, _ := stageBar(t, b, badged(2, "Ada"))
	ink := widget.ToastErrorInk.Default()
	if pill, _, ok := pillOn(w, b, "2", ink); !ok || pill.Size().W < b.badges["2"].run.Advance || !w.Still() {
		t.Fatalf("on the first frames the badge is %v, %v", pill, ok)
	}
	tabs := badged(3, "Ada")
	tabs.tabs[2].Badge = "Bo"
	if err := w.Client().Update("v", tabs); err != nil {
		t.Fatal(err)
	}
	run(1)
	if g := b.badges["3"]; g == nil || g.shown() != 1 {
		t.Error("a badge on a new tab grows in apart from it")
	}
}

// TestABadgeInANarrowTabIsADot: the bar narrowing, the badge keeps to
// half of what the title has, then to a dot, and at last to the icon's
// corner, never jumping; and what it says shows in the tooltip.
func TestABadgeInANarrowTabIsADot(t *testing.T) {
	ink := widget.ToastErrorInk.Default()
	last, had := geom.Rect{}, false
	for wide := float32(220); wide >= 22; wide -= 1 {
		b := NewBar("g", 1)
		b.tabs = []tab{{id: "1", Face: Face{Title: "Terminal", Icon: icon.SquareTerminal, Closable: true, Badge: "Ada Lovelace", BadgeInk: &widget.ToastErrorInk}}}
		w, run, _ := stageBar(t, b, barTabs{tabs: []Tab{{ID: "1", Face: b.tabs[0].Face}}, front: "1"})
		onUIDock(t, w, run, func(u *gunim.UI) { b.reserve = 800 - wide })
		run(60)
		tab, _ := b.drawnAt(0)
		if tab.Size().W < TabIcon.Default()+4 {
			// Nothing of it shows.
			break
		}
		pill, fill, ok := pillOn(w, b, "1", ink)
		if !ok {
			t.Fatalf("in a tab %v wide the badge is gone", tab.Size().W)
		}
		if pill.Min.X < tab.Min.X || pill.Max.X > tab.Max.X || pill.Min.Y < tab.Min.Y || pill.Max.Y > tab.Max.Y {
			t.Fatalf("in a tab at %v the badge is at %v", tab, pill)
		}
		if pill.Size().W < TabBadgeDot.Default()-0.01 || pill.Size().H < TabBadgeDot.Default()-0.01 {
			t.Fatalf("in a tab %v wide the badge is %v", tab.Size().W, pill.Size())
		}
		if had {
			for _, d := range []float32{pill.Min.X - last.Min.X, pill.Max.X - last.Max.X, pill.Min.Y - last.Min.Y, pill.Max.Y - last.Max.Y} {
				if d > 3 || d < -3 {
					t.Fatalf("in a tab %v wide the badge goes from %v to %v", tab.Size().W, last, pill)
				}
			}
		}
		if tab.Size().W < 30 && (pill.Size().W > 8.01 || fill.A != ink.A) {
			t.Fatalf("in a tab %v wide the badge is %v, filled %v", tab.Size().W, pill.Size(), fill)
		}
		if tip := b.tipOf(b.boxes[0].Center()); tab.Size().W < 100 && tip != "Terminal, Ada Lovelace" {
			t.Fatalf("in a tab %v wide the tooltip is %q", tab.Size().W, tip)
		}
		last, had = pill, true
	}
}

// TestADraggedTabKeepsItsBadge: faint on the bar, and whole on the picture the drag carries.
func TestADraggedTabKeepsItsBadge(t *testing.T) {
	b := NewBar("g", 1)
	w, run, _ := stageBar(t, b, badged(3, "Ada"))
	run(30)
	ink := widget.ToastErrorInk.Default()
	_, whole, _ := pillOn(w, b, "2", ink)
	at := barTabAt(t, b, "2")
	w.Input(input.PointerMove{Pos: at})
	w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	w.Input(input.PointerMove{Pos: at.Add(geom.Pt(20, 0))})
	run(4)
	if b.Carried() != "2" {
		t.Fatal("the tab was not lifted")
	}
	if _, faint, ok := pillOn(w, b, "2", ink); !ok || faint.A >= whole.A {
		t.Errorf("on the bar the badge is filled %v, from %v", faint, whole)
	}
	w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary})
	run(30)
	pic := &tabPicture{face: b.tabs[1].Face, title: b.titles[1], badge: b.badges["2"], size: b.boxes[1].Size()}
	var p paint.Painter
	pic.Paint(&p, gunim.Frame{Theme: theme.NewLive(widget.Dark())}, pic.size, gunim.Children{})
	found := false
	for _, op := range p.Ops() {
		if r, ok := op.(*paint.RRectOp); ok && r.Fill.Solid == whole {
			found = r.Rect.Min.X > 0 && r.Rect.Max.X < pic.size.W
		}
	}
	if !found {
		t.Error("the picture a drag carries shows no badge")
	}
}
