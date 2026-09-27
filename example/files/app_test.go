package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
)

// harness runs the app half against the real views in an offscreen
// window, on a folder in a temporary folder, with the trash there too.
type harness struct {
	t    *testing.T
	w    *gunim.Window
	a    *app
	b    *browser
	root string
	dir  string
}

// grab is a patch that hands the test the browser.
type grab struct{}

func newHarness(t *testing.T, spec ...string) *harness {
	t.Helper()
	h := &harness{t: t, root: t.TempDir()}
	h.dir = filepath.Join(h.root, "dir")
	if err := os.MkdirAll(h.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	tree(t, h.dir, spec...)
	h.w = gunim.NewOffscreen(geom.Sz(1100, 700), &root{})
	registerViews(h.w)
	gunim.RegisterPatch(h.w, "browser", func(b *browser, _ grab, _ *gunim.UI) { h.b = b })
	ctx, cancel := context.WithCancel(context.Background())
	a, err := launch(ctx, h.w.Client(), options{dir: h.dir, prefsPath: filepath.Join(h.root, "prefs.json"),
		noPoll: true, trash: xdgTrash{dir: filepath.Join(h.root, "Trash")}})
	if err != nil {
		t.Fatal(err)
	}
	h.a = a
	t.Cleanup(func() {
		cancel()
		a.stopAll()
	})
	if err := h.w.Client().Patch(string(browserID), grab{}); err != nil {
		t.Fatal(err)
	}
	h.until("the folder shows", func() bool { return !a.nav.loading && h.b != nil && h.b.listing.cur != nil })
	h.frames(30)
	return h
}

// pump runs what is waiting on either side, and draws a frame.
func (h *harness) pump() {
	for {
		select {
		case fn := <-h.a.done:
			fn()
			continue
		case ev := <-h.w.Client().Intents():
			h.a.handle(h.a.handlers, ev.Intent)
			continue
		default:
		}
		break
	}
	h.w.Frame(time.Second / 60)
}

func (h *harness) frames(n int) {
	for range n {
		h.pump()
	}
}

// until pumps until cond holds, and fails the test after five seconds.
func (h *harness) until(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			h.t.Fatalf("waited five seconds for this: %s", what)
		}
		h.pump()
		time.Sleep(time.Millisecond)
	}
}

// do runs an intent through the app half as the window would send it.
func (h *harness) do(in gunim.Intent) {
	h.a.handle(h.a.handlers, in)
	h.frames(2)
}

// answer closes the dialog showing, as its buttons do, and hands the
// app its answer.
func (h *harness) answer(in gunim.Intent) {
	h.t.Helper()
	if len(h.a.ops.dialogs) == 0 {
		h.t.Fatal("no dialog is showing")
	}
	if err := h.w.Client().Unmount(h.a.ops.dialogs[0].id); err != nil {
		h.t.Fatal(err)
	}
	h.do(in)
}

// pick selects the rows named, as clicks in the grid would.
func (h *harness) pick(names ...string) {
	h.t.Helper()
	var runs [][2]int
	for i, e := range h.a.nav.rows {
		if slices.Contains(names, e.Name) {
			runs = append(runs, [2]int{i, i + 1})
		}
	}
	if len(runs) != len(names) {
		h.t.Fatalf("found %d of the rows %v", len(runs), names)
	}
	h.do(Selected{Gen: h.a.nav.gen, Runs: runs, Cursor: runs[0][0]})
}

// shown returns the names the grid draws, in order.
func (h *harness) shown() []string {
	h.t.Helper()
	pg := h.b.listing.cur
	var out []string
	for i := range pg.grid.Rows() {
		r, ok := pg.view(i)
		if !ok {
			return nil
		}
		out = append(out, r.Name)
	}
	return out
}

// exists reports whether rel names something under the folder.
func (h *harness) exists(rel string) bool {
	_, err := os.Lstat(filepath.Join(h.dir, filepath.FromSlash(rel)))
	return err == nil
}

func TestTheGridShowsTheFolderFoldersFirst(t *testing.T) {
	h := newHarness(t, "b.txt", "a.txt", "zeta/", ".hidden")
	want := []string{"zeta", "a.txt", "b.txt"}
	h.until("the rows arrive", func() bool { return slices.Equal(h.shown(), want) })
	if got := h.b.status.left.Text; got != "3 items (1 hidden)" {
		t.Fatalf("the status bar says %q", got)
	}
}

func TestTheFilterNarrowsTheRows(t *testing.T) {
	h := newHarness(t, "report.pdf", "notes.txt", "old-report.txt")
	h.do(FilterChanged{Text: "REPORT"})
	h.until("the filter applies", func() bool {
		return slices.Equal(h.shown(), []string{"old-report.txt", "report.pdf"})
	})
	h.do(FilterChanged{Text: ""})
	h.until("the filter clears", func() bool { return len(h.shown()) == 3 })
}

func TestSortingBySizeFromTheHeader(t *testing.T) {
	h := newHarness(t, "a.txt", "bbbbbbbbbbbb.txt", "cccccc.txt")
	h.do(SortClicked{Column: int(SortSize)})
	h.until("the rows sort by size", func() bool {
		return slices.Equal(h.shown(), []string{"a.txt", "cccccc.txt", "bbbbbbbbbbbb.txt"})
	})
	h.do(SortClicked{Column: int(SortSize)})
	h.until("a second click turns the order round", func() bool {
		return slices.Equal(h.shown(), []string{"bbbbbbbbbbbb.txt", "cccccc.txt", "a.txt"})
	})
}

func TestGoingIntoAFolderAndBack(t *testing.T) {
	h := newHarness(t, "inner/deep.txt", "top.txt")
	first := h.b.listing.cur
	h.do(Activated{Gen: h.a.nav.gen, Row: 0})
	h.until("the folder opens", func() bool { return slices.Equal(h.shown(), []string{"deep.txt"}) })
	if h.b.listing.cur == first || h.b.listing.cur == nil {
		t.Fatal("going into a folder kept the old page")
	}
	if h.b.listing.deck.top.travel != 1 {
		t.Fatalf("the new page came in with travel %d, want 1", h.b.listing.deck.top.travel)
	}
	h.do(Command{Name: CmdBack})
	h.until("back shows the folder again", func() bool { return slices.Equal(h.shown(), []string{"inner", "top.txt"}) })
	if h.b.listing.deck.top.travel != -1 {
		t.Fatalf("going back, the page came in with travel %d, want -1", h.b.listing.deck.top.travel)
	}
	h.do(Command{Name: CmdForward})
	h.until("forward goes in again", func() bool { return slices.Equal(h.shown(), []string{"deep.txt"}) })
	h.do(Command{Name: CmdUp})
	h.until("up comes out, with the folder left selected", func() bool {
		return len(h.shown()) == 2 && h.a.nav.sel["inner"]
	})
}

func TestDeleteKeyTrashesAndUndoRestores(t *testing.T) {
	h := newHarness(t, "keep.txt", "gone.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 2 })
	h.pick("gone.txt")
	h.w.Input(input.KeyPress{Key: input.KeyDelete})
	h.until("the file goes to the trash", func() bool { return !h.exists("gone.txt") })
	h.until("the listing drops it", func() bool { return slices.Equal(h.shown(), []string{"keep.txt"}) })
	if contents(t, filepath.Join(h.root, "Trash", "files", "gone.txt")) != "gone.txt" {
		t.Fatal("the file is not in the trash")
	}
	if n := h.b.toasts.Len(); n == 0 {
		t.Fatal("no toast says the file went to the trash")
	}
	h.do(Command{Name: CmdUndo})
	h.until("undo puts it back", func() bool { return h.exists("gone.txt") })
	h.until("the listing shows it again", func() bool { return len(h.shown()) == 2 })
}

func TestPasteAsksAboutAClash(t *testing.T) {
	h := newHarness(t, "a.txt", "sub/a.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 2 })
	h.pick("a.txt")
	h.do(Command{Name: CmdCopy})
	h.do(Navigate{Path: filepath.Join(h.dir, "sub")})
	h.until("the folder opens", func() bool { return slices.Equal(h.shown(), []string{"a.txt"}) })
	h.do(Command{Name: CmdPaste})
	h.until("the clash is asked about", func() bool { return len(h.a.ops.dialogs) == 1 })
	ask, ok := h.a.ops.dialogs[0].state.(ClashAsk)
	if !ok || ask.Name != "a.txt" || !ask.SameKind {
		t.Fatalf("the dialog asks %+v", h.a.ops.dialogs[0].state)
	}
	h.answer(ClashAnswered{Op: ask.Op, Choice: ChoiceKeepBoth})
	h.until("both are kept", func() bool { return slices.Equal(h.shown(), []string{"a (2).txt", "a.txt"}) })
	if contents(t, filepath.Join(h.dir, "sub", "a.txt")) != "sub/a.txt" {
		t.Fatal("keeping both overwrote the file there")
	}
}

func TestRenameThroughThePrompt(t *testing.T) {
	h := newHarness(t, "old.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	h.pick("old.txt")
	h.w.Input(input.KeyPress{Key: input.KeyF2})
	h.until("the prompt shows", func() bool { return len(h.a.ops.dialogs) == 1 })
	p, ok := h.a.ops.dialogs[0].state.(Prompt)
	if !ok || p.Text != "old.txt" || p.Stem != 3 {
		t.Fatalf("the prompt is %+v", h.a.ops.dialogs[0].state)
	}
	h.answer(Prompted{Token: p.Token, Text: "new.txt", OK: true})
	h.until("the file is renamed and selected", func() bool {
		return slices.Equal(h.shown(), []string{"new.txt"}) && h.a.nav.sel["new.txt"]
	})
}

func TestPermanentDeleteAsksFirst(t *testing.T) {
	h := newHarness(t, "precious.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	h.pick("precious.txt")
	h.w.Input(input.KeyPress{Key: input.KeyDelete, Mods: input.ModShift})
	h.until("the question shows", func() bool { return len(h.a.ops.dialogs) == 1 })
	c, ok := h.a.ops.dialogs[0].state.(Confirm)
	if !ok {
		t.Fatalf("the dialog is %+v", h.a.ops.dialogs[0].state)
	}
	h.answer(Confirmed{Token: c.Token})
	h.frames(10)
	if !h.exists("precious.txt") {
		t.Fatal("a cancelled delete deleted the file")
	}
	h.w.Input(input.KeyPress{Key: input.KeyDelete, Mods: input.ModShift})
	h.until("the question shows again", func() bool { return len(h.a.ops.dialogs) == 1 })
	c, _ = h.a.ops.dialogs[0].state.(Confirm)
	h.answer(Confirmed{Token: c.Token, OK: true})
	h.until("the file is gone", func() bool { return !h.exists("precious.txt") })
	if _, err := os.Stat(filepath.Join(h.root, "Trash")); err == nil {
		t.Fatal("a permanent delete went through the trash")
	}
}

func TestANewFolderIsMadeAndSelected(t *testing.T) {
	h := newHarness(t, "New folder/")
	h.w.Input(input.KeyPress{Key: input.KeyN, Mods: input.ModControl | input.ModShift})
	h.until("the prompt shows", func() bool { return len(h.a.ops.dialogs) == 1 })
	p, _ := h.a.ops.dialogs[0].state.(Prompt)
	if p.Text != "New folder (2)" {
		t.Fatalf("the prompt offers %q, want a free name", p.Text)
	}
	h.answer(Prompted{Token: p.Token, Text: p.Text, OK: true})
	h.until("the folder is made and selected", func() bool { return h.exists("New folder (2)") && h.a.nav.sel["New folder (2)"] })
}

func TestThePollNoticesChanges(t *testing.T) {
	h := newHarness(t, "a.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	tree(t, h.dir, "b.txt")
	h.a.poll()
	h.until("the new file shows", func() bool { return slices.Equal(h.shown(), []string{"a.txt", "b.txt"}) })
	if err := os.RemoveAll(h.dir); err != nil {
		t.Fatal(err)
	}
	h.a.poll()
	h.until("the listing says the folder has gone", func() bool {
		return h.b.listing.cur.msg.Text != "" && h.a.nav.err != nil
	})
}

func TestAFolderThatCannotBeReadSaysWhy(t *testing.T) {
	h := newHarness(t)
	h.do(Navigate{Path: filepath.Join(h.dir, "missing")})
	h.until("the listing says why", func() bool { return h.b.listing.cur.msg.Text != "" })
	if h.a.nav.err == nil {
		t.Fatal("the app has no error for a missing folder")
	}
}

func TestPinningAddsAFavouriteAndSavesIt(t *testing.T) {
	h := newHarness(t, "work/")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	h.pick("work")
	h.w.Input(input.KeyPress{Key: input.KeyD, Mods: input.ModControl})
	h.until("the favourite shows", func() bool { return h.b.side.favs.Len() == 1 })
	p, err := loadPrefs(filepath.Join(h.root, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(p.Favourites, []string{filepath.Join(h.dir, "work")}) {
		t.Fatalf("the saved favourites are %v", p.Favourites)
	}
}

func TestAClickOnARowSelectsItAndPreviewsIt(t *testing.T) {
	h := newHarness(t, "notes.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	r, ok := func() (geom.Rect, bool) {
		var r geom.Rect
		var ok bool
		type bounds struct{}
		gunim.RegisterPatch(h.w, "browser", func(b *browser, _ bounds, u *gunim.UI) {
			r, ok = u.Bounds(b.listing.cur.grid)
		})
		if err := h.w.Client().Patch(string(browserID), bounds{}); err != nil {
			t.Fatal(err)
		}
		h.frames(1)
		return r, ok
	}()
	if !ok {
		t.Fatal("the grid was not drawn")
	}
	at := r.Min.Add(geom.Pt(60, 26+11))
	h.w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1, Time: time.Now()})
	h.w.Input(input.PointerUp{Pos: at, Button: input.ButtonPrimary, Time: time.Now()})
	h.until("the row is selected", func() bool { return h.a.nav.sel["notes.txt"] })
	h.until("the preview shows it", func() bool {
		return h.b.preview.cur != nil && h.a.preview.subject != "" && h.b.preview.seq == h.a.preview.seq
	})
}
