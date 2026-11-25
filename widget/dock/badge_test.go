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
	"github.com/marrasen/gunim/text"
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
// what the title leaves, then to half, then to a dot, and at last to the
// icon's corner, never jumping; and what it says shows in the tooltip.
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

// words are the words drawn in pill, with how far each shows.
func words(w *gunim.Window, pill geom.Rect, runs map[string]text.Run) map[string]float32 {
	out := map[string]float32{}
	for _, op := range w.Offscreen().Ops() {
		op, ok := op.(*paint.TextOp)
		if !ok || len(op.Glyphs) == 0 || !pill.Inset(geom.Uniform(-2)).Contains(op.Transform.Apply(op.Glyphs[0].At)) {
			continue
		}
		for s, run := range runs {
			if slices.EqualFunc(op.Glyphs, run.Glyphs, func(a, b paint.Glyph) bool { return a.ID == b.ID }) {
				out[s] += float32(op.Color.A) / 255
			}
		}
	}
	return out
}

// TestABadgeChangedAgainInItsFadePopsNothing: before the fade is half
// done and past it, to longer words and to shorter, the pill's edges and
// the words in it go on from where they are on every frame.
func TestABadgeChangedAgainInItsFadePopsNothing(t *testing.T) {
	ink := widget.ToastErrorInk.Default()
	long, short := "Ada Lovelace", "Al"
	runs := map[string]text.Run{}
	for _, s := range []string{long, short, "Bo", "Grace Hopper"} {
		runs[s] = widget.Font.Default().Shape(s, widget.BadgeSize.Default())
	}
	for _, c := range [][3]string{{long, short, "Bo"}, {long, short, "Grace Hopper"}, {short, long, "Bo"}, {long, short, long}, {long, "Grace Hopper", ""}} {
		limit := float32(9)
		if c[2] == "" {
			// It shrinks away as its width sets out.
			limit = 14
		}
		for after := 1; after <= 24; after++ {
			b := NewBar("g", 1)
			tabs := func(badge string) barTabs {
				return barTabs{tabs: []Tab{{ID: "1", Face: Face{Title: "log", Badge: badge, BadgeInk: &widget.ToastErrorInk}}, {ID: "2"}}, front: "2"}
			}
			w, run, _ := stageBar(t, b, tabs(c[0]))
			run(30)
			pill, _, _ := pillOn(w, b, "1", ink)
			said := words(w, pill, runs)
			if said[c[0]] < 0.99 {
				t.Fatalf("the badge says %v", said)
			}
			for frame := range 90 {
				if frame == 0 || frame == after {
					say := c[1]
					if frame == after {
						say = c[2]
					}
					if err := w.Client().Update("v", tabs(say)); err != nil {
						t.Fatal(err)
					}
				}
				run(1)
				now, fill, ok := pillOn(w, b, "1", ink)
				saying := words(w, now, runs)
				if c[2] == "" && frame > after && (!ok || fill.A < 20) {
					// Gone, or nearly.
					continue
				}
				if d := now.Size().W - pill.Size().W; d > limit || d < -limit {
					t.Fatalf("%q, after %d frames, frame %d: the badge goes from %v wide to %v", c, after, frame, pill.Size().W, now.Size().W)
				}
				if d := now.Min.X - pill.Min.X; d > limit || d < -limit {
					t.Fatalf("%q, after %d frames, frame %d: the badge goes from %v to %v", c, after, frame, pill, now)
				}
				for s := range runs {
					if d := saying[s] - said[s]; d > 0.2 || d < -0.2 {
						t.Fatalf("%q, after %d frames, frame %d: %q goes from %.2f to %.2f, with %v", c, after, frame, s, said[s], saying[s], saying)
					}
				}
				pill, said = now, saying
			}
			if c[2] == "" {
				if len(b.badges) != 0 || !w.Still() {
					t.Fatalf("%q, after %d frames: %d badges are kept, and the bar rests %v", c, after, len(b.badges), w.Still())
				}
				continue
			}
			want := runs[c[2]].Advance + 2*widget.BadgePadding.Default()
			if d := pill.Size().W - want; said[c[2]] < 0.99 || len(said) != 1 || d > 0.5 || d < -0.5 || !w.Still() {
				t.Fatalf("%q, after %d frames: the badge says %v, %v wide, want %v, and rests %v", c, after, said, pill.Size().W, want, w.Still())
			}
		}
	}
}

// TestATabClosedWithItsBadgeInAFadeGoes: with more words waiting for the fade's end.
func TestATabClosedWithItsBadgeInAFadeGoes(t *testing.T) {
	b := NewBar("g", 1)
	w, run, _ := stageBar(t, b, badged(3, "Ada"))
	run(30)
	for _, badge := range []string{"Bo", "Cy"} {
		if err := w.Client().Update("v", badged(3, badge)); err != nil {
			t.Fatal(err)
		}
		run(2)
	}
	tabs := badged(3, "Cy")
	tabs.tabs = slices.Delete(tabs.tabs, 1, 2)
	if err := w.Client().Update("v", tabs); err != nil {
		t.Fatal(err)
	}
	run(90)
	if len(b.gone) != 0 || !w.Still() {
		t.Errorf("%d tabs are still going, and the bar rests %v", len(b.gone), w.Still())
	}
}

// TestABadgeBesideAShortTitleShowsWhole: in a tab as wide as it asks to
// be, whichever of the two is the longer, and with a count as well.
func TestABadgeBesideAShortTitleShowsWhole(t *testing.T) {
	ink := widget.ToastErrorInk.Default()
	for _, f := range []Face{
		{Title: "log", Badge: "Bob"},
		{Title: "log", Badge: "Ada Lovelace"},
		{Title: "log", More: "+2", Badge: "Ada Lovelace", Closable: true},
		{Title: "a log of it all", Badge: "Al"},
		{Title: "log", Badge: "Bob", Icon: icon.SquareTerminal, Closable: true},
	} {
		f.BadgeInk = &widget.ToastErrorInk
		b := NewBar("g", 1)
		w, run, _ := stageBar(t, b, barTabs{tabs: []Tab{{ID: "1", Face: f}, {ID: "2"}}, front: "2"})
		run(30)
		g := b.badges["1"]
		pill, _, ok := pillOn(w, b, "1", ink)
		if want := g.run.Advance + 2*widget.BadgePadding.Default(); !ok || pill.Size().W < want-0.5 || g.cut {
			t.Errorf("beside %q the badge %q is %v wide, want %v, cut %v", f.Title, f.Badge, pill.Size().W, want, g.cut)
		}
		if tip := b.tipOf(b.boxes[0].Center()); tip != "" || b.cut[0] {
			t.Errorf("with the badge %q the title %q is cut, its tooltip %q", f.Badge, f.Title, tip)
		}
	}
}

// TestABadgeStandsStillAsItsTitleChanges: the tab widens and narrows for
// the title, and the badge keeps its width on every frame.
func TestABadgeStandsStillAsItsTitleChanges(t *testing.T) {
	ink := widget.ToastErrorInk.Default()
	b := NewBar("g", 1)
	tabs := func(title string) barTabs {
		return barTabs{tabs: []Tab{{ID: "1", Face: Face{Title: title, Badge: "Ada Lovelace", BadgeInk: &widget.ToastErrorInk}}, {ID: "2"}}, front: "2"}
	}
	w, run, _ := stageBar(t, b, tabs("a log"))
	run(30)
	pill, _, _ := pillOn(w, b, "1", ink)
	for _, title := range []string{"a log of it all", "a log"} {
		if err := w.Client().Update("v", tabs(title)); err != nil {
			t.Fatal(err)
		}
		for frame := range 60 {
			run(1)
			now, _, ok := pillOn(w, b, "1", ink)
			if d := now.Size().W - pill.Size().W; !ok || d > 0.5 || d < -0.5 {
				t.Fatalf("becoming %q, frame %d: the badge goes from %v wide to %v", title, frame, pill.Size().W, now.Size().W)
			}
		}
		if !w.Still() {
			t.Fatalf("at %q the bar never rests", title)
		}
	}
}

// TestABadgeAndItsTitleShareWhatThereIs: what one of them leaves is the
// other's, and where neither fits in half they have half each.
func TestABadgeAndItsTitleShareWhatThereIs(t *testing.T) {
	th := theme.NewLive(widget.Dark())
	g := newTabBadge()
	g.say("Ada Lovelace", nil, true, th)
	whole, dot := g.width(th), TabBadgeDot.Default()
	if whole <= 40 {
		t.Fatalf("the badge is %v wide", whole)
	}
	fit := func(both, title float32) float32 {
		g.stand(title, true, th)
		return g.fit(both+badgeGap, th)
	}
	for _, c := range []struct {
		name              string
		both, title, want float32
	}{
		{"room for both", whole + 30.5, 30, whole},
		{"room to spare", whole + 90, 30, whole},
		{"a short title", 20 + whole - 10, 20, whole - 10},
		{"a short badge", 2*whole + 20, 2*whole + 100, whole},
		{"both too long", whole + 10, 2 * whole, (whole + 10) / 2},
		{"hardly any room", 2*dot - 4, 100, dot},
		{"hardly any beside a short title", 2*dot - 4, 10, dot},
		{"none", -20, 10, dot},
	} {
		if got := fit(c.both, c.title); got < c.want-0.5 || got > c.want+0.01 {
			t.Errorf("%s: of %v beside a title of %v the badge has %v, want %v", c.name, c.both, c.title, got, c.want)
		}
	}
	// With more room it is never narrower, and the title is cut only where it is long.
	last := float32(0)
	for both := float32(0); both < whole+60; both += 0.5 {
		w := fit(both, 40)
		if w < last || w-last > 0.51 && last > 0 {
			t.Fatalf("of %v the badge has %v, from %v", both, w, last)
		}
		if left := both - w; left < 40 && left < both/2-dot {
			t.Fatalf("of %v the title has %v", both, left)
		}
		last = w
	}
}

// TestACountThatDoesNotFitFadesAway: the tab narrowing, with a badge and
// without, the count after the title stays clear of the icon and of the
// title on the way, and fades as the room goes.
func TestACountThatDoesNotFitFadesAway(t *testing.T) {
	for _, f := range []Face{
		{Title: "Terminal", More: "+2", Icon: icon.SquareTerminal, Closable: true, Badge: "Ada Lovelace", BadgeInk: &widget.ToastErrorInk},
		{Title: "Terminal", More: "+2", Icon: icon.SquareTerminal},
		{Title: "Terminal", More: "+2", Badge: "Ada Lovelace", BadgeInk: &widget.ToastErrorInk},
		{Title: "log", More: "+12"},
		{Title: "log", More: "+2", Icon: icon.SquareTerminal},
		{Title: "log", More: "+2", Icon: icon.SquareTerminal, Badge: "Ada Lovelace", BadgeInk: &widget.ToastErrorInk},
	} {
		var last, lastTitle, full float32
		for wide := float32(300); wide >= 20; wide -= 0.5 {
			b := NewBar("g", 1)
			w, run, _ := stageBar(t, b, barTabs{tabs: []Tab{{ID: "1", Face: f}}, front: "1"})
			onUIDock(t, w, run, func(u *gunim.UI) { b.reserve = 800 - wide })
			run(60)
			tab, _ := b.drawnAt(0)
			left, right := tab.Min.X+TabPadding.Default(), tab.Max.X-TabPadding.Default()
			if f.Icon != nil {
				left += TabIcon.Default() + 6
			}
			// How far the title reaches, and where the count starts and how it shows.
			var alpha, title float32
			at := float32(-1)
			for _, op := range w.Offscreen().Ops() {
				op, ok := op.(*paint.TextOp)
				if !ok || len(op.Glyphs) == 0 {
					continue
				}
				same := func(r text.Run) bool {
					return slices.EqualFunc(op.Glyphs, r.Glyphs, func(a, b paint.Glyph) bool { return a.ID == b.ID })
				}
				x := op.Transform.Apply(op.Glyphs[0].At).X
				switch {
				case same(b.more[0]):
					at, alpha = x, float32(op.Color.A)/255
					if wide == 300 && op.FadeTo > 0 {
						t.Fatalf("%+v: with room the count is cut", f)
					}
				case same(b.titles[0]):
					if title = x + b.titles[0].Advance; op.FadeTo > 0 {
						title = min(title, x+op.FadeTo)
					}
				}
			}
			if alpha > 0 && (at < left-0.5 || at > right || title > at+0.5) {
				t.Fatalf("%+v in a tab at %v: the count is at %v, after an icon that ends at %v and a title at %v", f, tab, at, left, title)
			}
			if alpha > full-0.01 && at+b.more[0].Advance > right+0.5 {
				t.Fatalf("%+v in a tab at %v: the count is cut at %v", f, tab, at)
			}
			if d := alpha - last; wide < 300 && (d > 0.1 || d < -0.1) {
				t.Fatalf("%+v in a tab %v wide: the count goes from %.2f to %.2f", f, tab.Size().W, last, alpha)
			}
			if d := title - lastTitle; wide < 300 && title > 0 && lastTitle > 0 && (d > 3 || d < -3) {
				t.Fatalf("%+v in a tab %v wide: the title ends at %v, from %v", f, tab.Size().W, title, lastTitle)
			}
			if wide == 300 {
				full = alpha
			}
			if full < 0.5 {
				t.Fatalf("%+v: with room the count shows %v", f, alpha)
			}
			last, lastTitle = alpha, title
		}
		if last != 0 {
			t.Errorf("%+v: with no room the count shows %v", f, last)
		}
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
