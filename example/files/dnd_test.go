package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/access"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"
)

// inUI is a patch that runs fn in the window, with the browser and the
// UI, as a test needs for what only the window knows.
type inUI struct {
	fn func(b *browser, u *gunim.UI)
}

// openHarness opens a window of the app on dir, in hub h, with the
// settings and the trash under root.
func openHarness(t *testing.T, h *hub, base, dir string) *harness {
	t.Helper()
	hs := &harness{t: t, root: base, dir: dir}
	hs.w = gunim.NewOffscreen(geom.Sz(1100, 700), &root{})
	registerViews(hs.w)
	gunim.RegisterPatch(hs.w, "browser", func(b *browser, _ grab, _ *gunim.UI) { hs.b = b })
	gunim.RegisterPatch(hs.w, "browser", func(b *browser, p inUI, u *gunim.UI) { p.fn(b, u) })
	ctx, cancel := context.WithCancel(context.Background())
	a, err := launch(ctx, hs.w.Client(), options{dir: dir, prefsPath: filepath.Join(base, "prefs.json"),
		noPoll: true, trash: xdgTrash{dir: filepath.Join(base, "Trash")}, hub: h})
	if err != nil {
		t.Fatal(err)
	}
	hs.a = a
	t.Cleanup(func() {
		cancel()
		a.stopAll()
	})
	if err := hs.w.Client().Patch(string(browserID), grab{}); err != nil {
		t.Fatal(err)
	}
	hs.until("the folder shows", func() bool { return !a.nav.loading && hs.b != nil && hs.b.listing.cur != nil })
	hs.until("the places and their volumes are known", func() bool { return len(a.places) > 0 && len(hs.b.dnd.vols) > len(a.places) })
	hs.frames(60)
	return hs
}

// ui runs fn in the window.
func (h *harness) ui(fn func(b *browser, u *gunim.UI)) {
	h.t.Helper()
	if err := h.w.Client().Patch(string(browserID), inUI{fn}); err != nil {
		h.t.Fatal(err)
	}
	h.frames(1)
}

// script runs steps of a script, each as -do would, with frames between.
func (h *harness) script(steps ...string) {
	h.t.Helper()
	for _, s := range steps {
		verb, arg, _ := strings.Cut(s, ":")
		if !h.a.scriptDnd(verb, arg) {
			h.t.Fatalf("the script does not know %q", s)
		}
		h.frames(12)
	}
}

// point returns where the window shows the item or place name.
func (h *harness) point(name string) geom.Point {
	h.t.Helper()
	var at geom.Point
	var ok bool
	h.ui(func(b *browser, u *gunim.UI) { at, ok = scriptTarget(b, name, u) })
	if !ok {
		h.t.Fatalf("the window does not show %q", name)
	}
	return at
}

// idle waits for the operations to finish.
func (h *harness) idle() {
	h.t.Helper()
	h.until("the operations finish", func() bool { return len(h.a.ops.running) == 0 })
	h.frames(5)
}

func TestDraggingTheSelectionOntoAFolderMovesIt(t *testing.T) {
	h := newDndHarness(t, "a.txt", "b.txt", "c.txt", "sub/")
	h.choose("a.txt", "b.txt")
	h.script("drag-start:a.txt", "drag-over:sub")
	var hint widget.DropSpot
	h.ui(func(b *browser, _ *gunim.UI) { hint, _ = b.dnd.listing.Over() })
	if got, ok := hint.Hint.(widget.DropHint); !ok || got.Text != "Move to sub" || got.Effect != widget.DropMove {
		t.Fatalf("over sub the drop says %v, want Move to sub", hint.Hint)
	}
	h.script("drop")
	h.until("both files arrive in sub", func() bool { return h.exists("sub/a.txt") && h.exists("sub/b.txt") })
	h.idle()
	if h.exists("a.txt") || h.exists("b.txt") || !h.exists("c.txt") {
		t.Fatal("the move left the wrong files behind")
	}
	if len(h.a.ops.undo) != 1 {
		t.Fatal("the move cannot be undone")
	}
}

func TestCtrlCopiesAndShiftMoves(t *testing.T) {
	h := newDndHarness(t, "a.txt", "sub/", "other/")
	h.choose("a.txt")
	h.script("drag-start:a.txt", "drag-over:sub+ctrl", "drop:+ctrl")
	h.until("the copy arrives", func() bool { return h.exists("sub/a.txt") })
	h.idle()
	if !h.exists("a.txt") {
		t.Fatal("Ctrl moved the file instead of copying it")
	}
	h.choose("a.txt")
	h.script("drag-start:a.txt", "drag-over:other+shift", "drop:+shift")
	h.until("the move arrives", func() bool { return h.exists("other/a.txt") })
	h.idle()
	if h.exists("a.txt") {
		t.Fatal("Shift copied the file instead of moving it")
	}
}

func TestADropThatWouldDoNothingIsRefused(t *testing.T) {
	h := newDndHarness(t, "a.txt", "sub/inner/", "zeta.txt")
	h.choose("sub")
	// A folder onto itself.
	h.script("drag-start:sub", "drag-over:sub")
	var spot widget.DropSpot
	h.ui(func(b *browser, _ *gunim.UI) { spot, _ = b.dnd.listing.Over() })
	if !spot.Refused {
		t.Fatalf("a folder over itself is not refused: %v", spot.Hint)
	}
	h.script("drop")
	// A file onto the folder it is in.
	h.choose("a.txt")
	h.script("drag-start:a.txt", "drag-over:zeta.txt")
	h.ui(func(b *browser, _ *gunim.UI) { spot, _ = b.dnd.listing.Over() })
	if hint, _ := spot.Hint.(widget.DropHint); !spot.Refused || hint.Text != "Already in dir" {
		t.Fatalf("a file over its own folder says %v, refused %v", spot.Hint, spot.Refused)
	}
	h.script("drop")
	h.frames(10)
	if len(h.a.ops.running) != 0 || len(h.a.ops.undo) != 0 {
		t.Fatal("a refused drop started an operation")
	}
	if !h.exists("sub/inner") || !h.exists("a.txt") {
		t.Fatal("a refused drop moved something")
	}
}

func TestDraggingAFolderOntoTheFavouritesPinsIt(t *testing.T) {
	h := newDndHarness(t, "a.txt", "sub/")
	h.choose("sub")
	h.script("drag-start:sub", "drag-over:favourites")
	var spot widget.DropSpot
	h.ui(func(b *browser, _ *gunim.UI) { spot, _ = b.dnd.side.Over() })
	if hint, _ := spot.Hint.(widget.DropHint); hint.Text != "Pin to favourites" || hint.Effect != widget.DropLink {
		t.Fatalf("over the favourites the drop says %v", spot.Hint)
	}
	h.script("drop")
	h.until("the folder is pinned", func() bool {
		return slices.ContainsFunc(h.a.prefs.Favourites, func(f string) bool { return samePath(f, filepath.Join(h.dir, "sub")) })
	})
	if !h.exists("sub") {
		t.Fatal("pinning moved the folder")
	}
	h.choose("a.txt")
	h.script("drag-start:a.txt", "drag-over:favourites")
	h.ui(func(b *browser, _ *gunim.UI) { spot, _ = b.dnd.side.Over() })
	if !spot.Refused {
		t.Fatal("a file over the favourites is not refused")
	}
	h.script("drop")
}

func TestFilesDroppedFromAnotherProgramCopyIntoTheFolderUnderThem(t *testing.T) {
	h := newDndHarness(t, "sub/", "here.txt")
	outside := filepath.Join(h.root, "elsewhere")
	tree(t, outside, "x.txt", "y.txt")
	h.w.Input(input.Drop{Pos: h.point("sub"), Paths: []string{filepath.Join(outside, "x.txt")}})
	h.until("the file is copied into sub", func() bool { return h.exists("sub/x.txt") })
	h.idle()
	if _, err := os.Stat(filepath.Join(outside, "x.txt")); err != nil {
		t.Fatal("a drop from outside moved the file without Shift")
	}
	h.w.Input(input.Drop{Pos: h.point("here.txt"), Paths: []string{filepath.Join(outside, "y.txt")}, Mods: input.ModShift})
	h.until("Shift moves the file into the folder showing", func() bool { return h.exists("y.txt") })
	h.idle()
	if _, err := os.Stat(filepath.Join(outside, "y.txt")); err == nil {
		t.Fatal("Shift copied the file instead of moving it")
	}
}

func TestContextMenuItemsReachTheirIntents(t *testing.T) {
	h := newDndHarness(t, "a.txt", "sub/")
	h.choose("a.txt")
	h.script("menu:a.txt")
	pick := func(label string) {
		t.Helper()
		h.ui(func(b *browser, u *gunim.UI) {
			m := b.listing.cur.menu
			i := slices.Index(m.Items, label)
			if i < 0 {
				t.Fatalf("the menu %v has no %q", m.Items, label)
			}
			if m.Disabled[i] {
				t.Fatalf("%q is dimmed", label)
			}
			m.Picked(i, u)
		})
		h.frames(3)
	}
	var paste bool
	h.ui(func(b *browser, _ *gunim.UI) {
		m := b.listing.cur.menu
		paste = m.Disabled[slices.Index(m.Items, "Paste")]
	})
	if !paste {
		t.Fatal("Paste is offered with nothing to paste")
	}
	pick("Copy")
	if !slices.Equal(h.a.ops.clip, []string{filepath.Join(h.dir, "a.txt")}) || h.a.ops.cut {
		t.Fatalf("Copy left the clipboard %v, cut %v", h.a.ops.clip, h.a.ops.cut)
	}
	h.script("menu:a.txt")
	h.ui(func(b *browser, _ *gunim.UI) {
		m := b.listing.cur.menu
		paste = m.Disabled[slices.Index(m.Items, "Paste")]
	})
	if paste {
		t.Fatal("Paste stays dimmed after a copy")
	}
	pick("Duplicate")
	h.until("the duplicate appears", func() bool { return h.exists("a (2).txt") })
	h.idle()
	h.choose("a.txt")
	h.script("menu:a.txt")
	pick("Rename")
	if len(h.a.ops.dialogs) != 1 || h.a.ops.dialogs[0].view != "prompt" {
		t.Fatal("Rename asked for no name")
	}
	h.answer(Prompted{Token: h.a.ops.tokens, Text: "b.txt", OK: true})
	h.until("the rename happens", func() bool { return h.exists("b.txt") })
	h.idle()
	// Beside the rows the menu is about the folder showing.
	h.ui(func(b *browser, u *gunim.UI) {
		b.listing.cur.menu.Open(geom.Pt(100, 400), u)
		if !slices.Contains(b.listing.cur.menu.Items, "New folder") {
			t.Fatalf("beside the rows the menu is %v", b.listing.cur.menu.Items)
		}
	})
	pick("New folder")
	if len(h.a.ops.dialogs) != 1 || h.a.ops.dialogs[0].view != "prompt" {
		t.Fatal("New folder asked for no name")
	}
	h.answer(Prompted{Token: h.a.ops.tokens, Text: "made", OK: true})
	h.until("the folder is made", func() bool { return h.exists("made") })
}

func TestTheSidebarMenuUnpinsAndRenamesFavourites(t *testing.T) {
	h := newDndHarness(t, "sub/")
	h.choose("sub")
	h.do(Command{Name: CmdPin})
	h.frames(10)
	fav := filepath.Join(h.dir, "sub")
	var items []string
	h.ui(func(b *browser, u *gunim.UI) {
		m, ok := b.dnd.side.Children()[0].(*widget.ContextMenu)
		row, found := b.side.favs.Row(widget.Key(fav))
		if !ok || !found {
			t.Fatal("the sidebar has no menu, or no row for the favourite")
		}
		mr, _ := u.Bounds(m)
		rr, _ := u.Bounds(row)
		m.Open(rr.Center().Sub(mr.Min), u)
		items = slices.Clone(m.Items)
	})
	if !slices.Contains(items, "Unpin") {
		t.Fatalf("the favourite's menu is %v", items)
	}
	h.do(RenameFavourite{Path: fav})
	h.answer(Prompted{Token: h.a.ops.tokens, Text: "My stuff", OK: true})
	h.until("the favourite takes its name", func() bool { return h.a.favName(fav) == "My stuff" })
	h.do(Unpin{Path: fav})
	if len(h.a.prefs.Favourites) != 0 {
		t.Fatal("Unpin left the favourite")
	}
}

func TestDropPlansFollowTheVolumesAndTheKeys(t *testing.T) {
	d := FileDrag{Paths: []string{filepath.FromSlash("/a/x.txt")}, Volume: "one"}
	dir := filepath.FromSlash("/b")
	for _, c := range []struct {
		vol  string
		mods input.Mods
		copy bool
	}{
		{"one", 0, false},
		{"two", 0, true},
		{"one", input.ModControl, true},
		{"two", input.ModShift, false},
		{"", 0, true},
	} {
		plan, _, ok := dropPlan(d, dir, c.vol, "", c.mods)
		if !ok || plan.Copy != c.copy {
			t.Fatalf("to volume %q with %v the plan copies %v, want %v", c.vol, c.mods, plan.Copy, c.copy)
		}
	}
	if _, hint, ok := dropPlan(d, dir, "one", "denied", 0); ok || hint.Text != "Cannot read b" {
		t.Fatalf("a folder whose volume cannot be read says %q", hint.Text)
	}
	folder := FileDrag{Paths: []string{filepath.FromSlash("/a/f")}, Dirs: []bool{true}}
	if _, _, ok := dropPlan(folder, filepath.FromSlash("/a/f/deeper"), "", "", 0); ok {
		t.Fatal("a folder may go inside itself")
	}
	if _, _, ok := pinPlan(folder, []string{filepath.FromSlash("/a/f")}); ok {
		t.Fatal("a favourite may be pinned again")
	}
}

// newDndHarness is newHarness with the volumes known, for drags.
func newDndHarness(t *testing.T, spec ...string) *harness {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "dir")
	tree(t, dir, spec...)
	return openHarness(t, nil, root, dir)
}

func TestTwoWindowsShareTheClipboardAndSeeEachOthersOperations(t *testing.T) {
	root := t.TempDir()
	left, right := filepath.Join(root, "left"), filepath.Join(root, "right")
	tree(t, left, "a.txt", "b.txt")
	tree(t, right, "c.txt")
	opened := make(chan string, 1)
	h := &hub{open: func(dir string) error { opened <- dir; return nil }}
	one := openHarness(t, h, root, left)
	two := openHarness(t, h, root, right)
	one.choose("a.txt")
	one.do(Command{Name: CmdCut})
	two.until("the other window hears of the cut", func() bool { return len(two.a.ops.clip) == 1 && two.a.ops.cut })
	two.do(Command{Name: CmdPaste})
	two.until("the paste moves the file", func() bool { return exists(filepath.Join(right, "a.txt")) })
	two.idle()
	one.until("the first window sees the file go", func() bool { return slices.Equal(one.shown(), []string{"b.txt"}) })
	if len(one.a.ops.clip) != 0 {
		t.Fatal("the first window still holds the cut after the paste")
	}
	// A drag from the first window dropped on the second.
	drag := FileDrag{Paths: []string{filepath.Join(left, "b.txt")}, Dirs: []bool{false}, Volume: one.b.dnd.vols[left]}
	two.w.Input(input.Drop{Pos: two.point("c.txt"), Data: drag})
	two.until("the drop moves the file across", func() bool { return exists(filepath.Join(right, "b.txt")) })
	two.idle()
	one.until("the first window empties", func() bool { return len(one.shown()) == 0 })
	one.do(Command{Name: CmdNewWindow})
	var dir string
	one.until("a new window is asked for", func() bool {
		select {
		case dir = <-opened:
			return true
		default:
			return false
		}
	})
	if !samePath(dir, left) {
		t.Fatalf("the new window opens on %s, want %s", dir, left)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestPropertiesCountAFolderAsItGoes(t *testing.T) {
	h := newDndHarness(t, "sub/a.txt", "sub/deep/b.txt")
	h.choose("sub")
	h.do(Command{Name: CmdProperties})
	h.until("the dialog shows", func() bool { return len(h.a.ops.dialogs) == 1 })
	h.until("the folder is counted", func() bool {
		p, ok := h.a.ops.dialogs[0].state.(Props)
		return ok && !p.Counting && p.Holds == "3 items"
	})
	p, _ := h.a.ops.dialogs[0].state.(Props)
	if p.Type != "Folder" || p.Name != "sub" || p.Err != "" {
		t.Fatalf("the dialog says %+v", p)
	}
	h.answer(DialogClosed{})
	if len(h.a.ops.dialogs) != 0 {
		t.Fatal("the dialog stayed")
	}
}

// choose selects the items named in the grid, as clicks would.
func (h *harness) choose(names ...string) {
	h.t.Helper()
	for i, n := range names {
		verb := "add"
		if i == 0 {
			verb = "select"
		}
		if verb == "select" {
			h.a.scriptStep("select:" + n)
		} else if !h.a.scriptDnd(verb, n) {
			h.t.Fatal("the script cannot add to the selection")
		}
	}
	h.frames(3)
}

func TestPropertiesSetTheAttributesForReal(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("read-only and hidden are attributes of Windows")
	}
	h := newDndHarness(t, "a.txt")
	path := filepath.Join(h.dir, "a.txt")
	t.Cleanup(func() {
		if err := setAttrs(path, false, false); err != nil {
			t.Error(err)
		}
	})
	h.choose("a.txt")
	h.do(Command{Name: CmdProperties})
	h.until("the dialog shows", func() bool { return len(h.a.ops.dialogs) == 1 })
	p, _ := h.a.ops.dialogs[0].state.(Props)
	if !p.Attrs || p.ReadOnly || p.Hidden {
		t.Fatalf("the dialog offers attributes %v, read-only %v, hidden %v", p.Attrs, p.ReadOnly, p.Hidden)
	}
	h.answer(PropsApplied{Token: p.Token, ReadOnly: true, Hidden: true})
	h.until("the attributes change", func() bool {
		ro, hidden, _, err := readAttrs(path)
		return err == nil && ro && hidden
	})
	if err := os.WriteFile(path, []byte("x"), 0o644); err == nil {
		t.Fatal("a read-only file took a write")
	}
}

// TestDndWire checks that what drag and drop, the menus and the windows
// exchange would also cross a socket.
func TestDndWire(t *testing.T) {
	err := gunim.CheckWire(
		DropFiles{Paths: []string{"/a/x"}, Into: "/b", Copy: true},
		PinFolders{Paths: []string{"/a"}},
		Volumes{Of: map[string]string{"/a": "C:"}, Errs: map[string]string{"/b": "denied"}},
		ClipState{Count: 2, Cut: true},
		OpenWindow{Path: "/a"},
		RenameFavourite{Path: "/a"},
		Props{Token: 1, Title: "Properties of a", Name: "a", Size: "1 KB", Attrs: true, ReadOnly: true},
		PropsCounted{Token: 1, Size: "2 KB", Holds: "3 items", Counting: true},
		PropsApplied{Token: 1, Hidden: true},
		ScriptDrag{Step: "over", Name: "sub", Mods: 2},
	)
	if err != nil {
		t.Fatal(err)
	}
}

func TestDraggingOutOfTheWindowHandsTheFilesOver(t *testing.T) {
	h := newDndHarness(t, "a.txt", "b.txt", "c.txt")
	h.choose("a.txt", "b.txt")
	at := h.point("a.txt")
	h.w.Input(input.PointerMove{Pos: at})
	h.w.Input(input.PointerDown{Pos: at, Button: input.ButtonPrimary, Clicks: 1})
	h.w.Input(input.PointerMove{Pos: at.Add(geom.Pt(12, 6))})
	h.frames(3)
	// Far past the window's edge, the drag leaves for other programs.
	h.w.Input(input.PointerMove{Pos: geom.Pt(-400, -400)})
	h.frames(3)
	out := h.w.Offscreen().DraggedOut()
	want := []string{filepath.Join(h.dir, "a.txt"), filepath.Join(h.dir, "b.txt")}
	if len(out) != 1 || !slices.Equal(out[0], want) {
		t.Fatalf("handed out %v, want %v", out, want)
	}
	if len(h.a.ops.running) != 0 || !h.exists("a.txt") {
		t.Fatal("handing the files over changed them")
	}
}

func TestDraggingTilesOntoAFolderMovesThem(t *testing.T) {
	h := newDndHarness(t, "a.txt", "b.txt", "sub/")
	h.do(Command{Name: CmdViewIcons})
	h.until("the icons settle", func() bool { return h.icons().on && h.icons().in.Value() == 1 })
	// Folders sort first: sub, a.txt, b.txt
	h.ui(func(_ *browser, u *gunim.UI) { h.icons().grid.SetSelected([][2]int{{1, 3}}, 1, u) })
	h.script("drag-start:a.txt", "drag-over:sub")
	var spot widget.DropSpot
	h.ui(func(b *browser, _ *gunim.UI) { spot, _ = b.dnd.listing.Over() })
	if hint, ok := spot.Hint.(widget.DropHint); !ok || hint.Text != "Move to sub" {
		t.Fatalf("over the sub tile the drop says %v, want Move to sub", spot.Hint)
	}
	h.script("drop")
	h.until("both files arrive in sub", func() bool { return h.exists("sub/a.txt") && h.exists("sub/b.txt") })
	h.idle()
}

// dialogShown reports whether the window's accessibility tree holds a dialog.
func (h *harness) dialogShown() bool {
	var find func(n *access.Node) bool
	find = func(n *access.Node) bool {
		if n == nil {
			return false
		}
		if n.Role == access.RoleDialog {
			return true
		}
		return slices.ContainsFunc(n.Children, find)
	}
	t := h.w.Offscreen().AccessTree()
	return t != nil && find(t.Root)
}

func TestThePropertiesButtonsCloseTheDialog(t *testing.T) {
	for _, key := range []input.Key{input.KeyEscape, input.KeyEnter} {
		h := newDndHarness(t, "sub/a.txt", "b.txt")
		h.w.Offscreen().ListenForAccess()
		for _, name := range []string{"sub", "b.txt"} {
			h.choose(name)
			h.do(Command{Name: CmdProperties})
			h.until("the dialog shows", func() bool { h.frames(1); return h.dialogShown() })
			h.w.Input(input.KeyPress{Key: key})
			h.until("the dialog leaves", func() bool { h.frames(1); return !h.dialogShown() && len(h.a.ops.dialogs) == 0 })
		}
	}
}

// A drop plans by the volume of the folder it lands on. A folder of the
// path, or a link to a folder, can be on a volume the folder showing is
// not on.
func TestADropPlansByTheVolumeOfTheFolderUnderIt(t *testing.T) {
	h := newDndHarness(t, "a.txt")
	tree(t, h.root, "stick/")
	link := filepath.Join(h.dir, "link")
	if err := os.Symlink(filepath.Join(h.root, "stick"), link); err != nil {
		t.Skip("this system makes no links:", err)
	}
	h.a.poll()
	h.until("the link shows", func() bool { return slices.Equal(h.shown(), []string{"link", "a.txt"}) })
	h.until("the window knows the volumes of the folder above and the link", func() bool {
		_, above := h.b.dnd.vols[h.root]
		_, ln := h.b.dnd.vols[link]
		return above && ln
	})
	// Both stand for a USB stick.
	h.ui(func(b *browser, _ *gunim.UI) { b.dnd.vols[h.root], b.dnd.vols[link] = "stick", "stick" })
	var spot widget.DropSpot
	h.choose("a.txt")
	h.script("drag-start:a.txt", "drag-over:"+filepath.Base(h.root))
	h.ui(func(b *browser, _ *gunim.UI) { spot, _ = b.dnd.crumbs.Over() })
	if hint, _ := spot.Hint.(widget.DropHint); hint.Effect != widget.DropCopy {
		t.Fatalf("over a folder of the path on another volume the drop says %v, want a copy", spot.Hint)
	}
	h.script("drag-over:link")
	h.ui(func(b *browser, _ *gunim.UI) { spot, _ = b.dnd.listing.Over() })
	if hint, _ := spot.Hint.(widget.DropHint); hint.Effect != widget.DropCopy {
		t.Fatalf("over a link to a folder on another volume the drop says %v, want a copy", spot.Hint)
	}
	h.script("drop")
	h.idle()
	if !h.exists("a.txt") {
		t.Fatal("the drop moved the file to another volume")
	}
}
