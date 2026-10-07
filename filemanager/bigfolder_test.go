package filemanager

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/gunimtest"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/paint"
	"github.com/marrasen/gunim/theme"
)

// bigFolder is the app half on an empty folder, given entries as if it had read them, and its window.
type bigFolder struct {
	a *app
	w *gunim.Window
}

// newBigFolder launches the app half and gives it n entries: files of a few types, and a folder in every hundred.
func newBigFolder(b *testing.B, n int) *bigFolder {
	b.Helper()
	dir := b.TempDir()
	w := gunim.NewOffscreen(geom.Sz(1100, 700), &root{})
	RegisterViews(w)
	ctx, cancel := context.WithCancel(context.Background())
	o := Options{Dir: dir, PrefsPath: filepath.Join(dir, "prefs.json"), Poll: -1,
		defaults: func() []Favourite { return nil }, trash: xdgTrash{dir: filepath.Join(dir, "Trash")}}
	a, err := launch(ctx, w.Client(), o, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		cancel()
		a.stopAll()
	})
	f := &bigFolder{a: a, w: w}
	for deadline := time.Now().Add(5 * time.Second); a.nav.loading; f.pump() {
		if time.Now().After(deadline) {
			b.Fatal("the empty folder did not load")
		}
	}
	types := []string{"JPEG image", "PNG image", "Text document", "PDF document", "Go source"}
	es := make([]entry, n)
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range es {
		name := fmt.Sprintf("Photo %06d.jpg", i)
		es[i] = entry{Name: name, Size: int64(i * 1000), Mod: at.Add(time.Duration(i) * time.Minute),
			Type: types[i%len(types)], Dir: i%100 == 0, lower: strings.ToLower(name)}
	}
	a.nav.all = es
	sortEntries(a.nav.all, a.nav.sort, a.nav.desc)
	a.refilter()
	for range 10 {
		f.pump()
	}
	return f
}

// pump runs what is waiting on either side, and draws a frame.
func (f *bigFolder) pump() {
	for {
		select {
		case fn := <-f.a.done:
			fn()
			continue
		case ev := <-f.w.Client().Intents():
			f.a.take(f.a.handlers, ev)
			continue
		default:
		}
		break
	}
	f.w.Frame(time.Second / 60)
}

// Narrowing the filter, widening it, and changing it both ways leave the rows a pass over every entry would, and
// say which went.
func TestTheFilterLeavesTheRowsAPassOverAllWould(t *testing.T) {
	h := newHarness(t, "alpha.txt", "alps.txt", "beta.txt", "albert.txt", "gamma.txt", "al.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 6 })
	n := &h.a.nav
	for _, text := range []string{"a", "al", "alp", "alps", "al", "be", "", "ta", "t"} {
		was := n.rows
		h.do(FilterChanged{Text: text})
		want := filterEntries(n.all, text, h.a.shell.ShowHidden)
		if !slices.EqualFunc(n.rows, want, func(a, b entry) bool { return a.Name == b.Name }) {
			t.Fatalf("filtered by %q the rows are %v, want %v", text, entryNames(n.rows), entryNames(want))
		}
		if got, want := goneAlong(n.all, was, n.rows), gone(was, n.rows); !slices.Equal(got, want) {
			t.Fatalf("filtered by %q, %v went, want %v", text, got, want)
		}
	}
}

// An icon view that has seen many thumbnails keeps those near the tiles in view.
func TestTheIconViewLetsFarThumbnailsGo(t *testing.T) {
	pg := &listingPage{blocks: map[int][]Row{}}
	for start := 0; start < 4000; start += blockRows {
		block := make([]Row, blockRows)
		for i := range block {
			block[i] = Row{Name: fmt.Sprintf("p%d.jpg", start+i)}
		}
		pg.blocks[start] = block
	}
	iv := &iconView{pg: pg, thumbs: map[string]tileThumb{}, first: 3000, count: 40}
	for i := range 4000 {
		iv.thumbs[fmt.Sprintf("p%d.jpg", i)] = tileThumb{size: 1}
		iv.trimThumbs()
		if len(iv.thumbs) > 512 {
			t.Fatalf("after %d thumbnails the view holds %d", i+1, len(iv.thumbs))
		}
	}
	for i := iv.first; i < iv.first+iv.count; i++ {
		if _, ok := iv.thumbs[fmt.Sprintf("p%d.jpg", i)]; !ok {
			t.Fatalf("the thumbnail of tile %d, in view, went", i)
		}
	}
}

// A favourite's icon is picked by a click let go on the icon pressed, and by nothing else.
func TestAFavouritesIconIsPickedOnAClick(t *testing.T) {
	g := newPickGrid([]string{"a", "b", "c", "d"}, "a", func(*paint.Painter, *theme.Live, int, geom.Rect) {})
	w := gunimtest.New(t, geom.Sz(400, 100), nil)
	gunim.RegisterView(w, "g", func(struct{}) gunim.Node { return g }, nil)
	if err := w.Client().Mount(gunim.Root, "g", "g", nil); err != nil {
		t.Fatal(err)
	}
	w.Frame(time.Second / 60)
	mid := func(i int) geom.Point { return g.cell(i).Center() }
	for _, c := range []struct {
		what     string
		down, up geom.Point
		touch    bool
		want     string
	}{
		{"a release on another", mid(2), mid(3), false, "a"},
		{"a finger that scrolls", mid(2), input.Away, true, "a"},
		{"a click", mid(2), mid(2), false, "c"},
	} {
		w.Input(input.PointerDown{Pos: c.down, Button: input.ButtonPrimary, Clicks: 1, Touch: c.touch, Time: time.Now()})
		w.Frame(time.Second / 60)
		if got := g.picked(); got != "a" {
			t.Fatalf("the press of %s picked %q", c.what, got)
		}
		w.Input(input.PointerUp{Pos: c.up, Button: input.ButtonPrimary, Touch: c.touch, Time: time.Now()})
		w.Frame(time.Second / 60)
		if got := g.picked(); got != c.want {
			t.Fatalf("%s picked %q, want %q", c.what, got, c.want)
		}
	}
}

// BenchmarkAFilterKeyIn200kEntries times a letter typed in the filter of a folder of 200 000 items, which narrows
// it, and the frame after it.
func BenchmarkAFilterKeyIn200kEntries(b *testing.B) {
	f := newBigFolder(b, 200_000)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		f.a.handle(f.a.handlers, FilterChanged{Text: "photo 1"})
		f.pump()
		b.StartTimer()
		f.a.handle(f.a.handlers, FilterChanged{Text: "photo 12"})
		f.pump()
	}
}

// BenchmarkASelectionChangeIn200kEntries times a row selected in a folder of 200 000 items, and the frame after it.
func BenchmarkASelectionChangeIn200kEntries(b *testing.B) {
	f := newBigFolder(b, 200_000)
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		row := 1 + i%50
		f.a.handle(f.a.handlers, Selected{Gen: f.a.nav.gen, Runs: [][2]int{{row, row + 1}}, Cursor: row})
		f.pump()
	}
}

// BenchmarkSortingByTypeOf200kEntries sorts a folder of 200 000 items by type.
func BenchmarkSortingByTypeOf200kEntries(b *testing.B) {
	f := newBigFolder(b, 200_000)
	es := make([]entry, len(f.a.nav.all))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		b.StopTimer()
		copy(es, f.a.nav.all)
		b.StartTimer()
		sortEntries(es, SortType, false)
	}
}
