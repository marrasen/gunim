package filemanager

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/input"
)

// said reports whether the window said what is read only, of all its
// notices.
func saidReadOnly(said func() []Notice) bool {
	return slices.ContainsFunc(said(), func(n Notice) bool { return n.Title == readOnlyHint })
}

// Turned read only, a window changes nothing by any way in: Disk usage,
// an Undo offered before, a question already asked, the trash, and the
// attributes of the Properties dialog.
func TestReadOnlyClosesEveryWayIn(t *testing.T) {
	h := newHarness(t, "a.txt", "b.txt")
	said := notices(h)
	h.until("the rows arrive", func() bool { return len(h.shown()) == 2 })

	// An Undo offered while the window could change things.
	h.pick("a.txt")
	h.do(Command{Name: CmdDuplicate})
	h.until("the copy is made", func() bool { return h.exists("a (2).txt") && len(h.a.ops.undo) == 1 })
	undo := h.a.ops.undo[0]

	// A question asked while it could.
	h.do(Command{Name: CmdNewFolder})
	h.until("the name is asked", func() bool { return len(h.a.ops.dialogs) == 1 })

	h.a.setReadOnly(true)
	h.frames(4)
	if len(h.a.ops.dialogs) != 0 {
		t.Error("the question is still asked")
	}
	if len(h.a.ops.undo) != 0 {
		t.Error("Ctrl+Z still undoes")
	}
	h.do(UndoOp{ID: undo})
	h.frames(10)
	if !h.exists("a (2).txt") {
		t.Fatal("an Undo offered before ran")
	}
	h.until("the refusal is said", func() bool { return saidReadOnly(said) })

	// Disk usage deletes nothing, and offers no delete.
	h.do(Command{Name: CmdUsage})
	h.until("the folder is read", func() bool { return h.a.usage.open && h.a.usage.scan != nil && !h.a.usage.scan.reading() })
	if !h.a.usageState().ReadOnly {
		t.Error("Disk usage is not told it may delete nothing")
	}
	h.a.usageAct("a.txt", UsageDelete)
	h.frames(10)
	if !h.exists("a.txt") {
		t.Fatal("Disk usage deleted a file")
	}

	// The trash, and the attributes.
	h.do(Command{Name: CmdEmptyTrash})
	h.a.trashAct([]string{"x"}, TrashDelete)
	h.a.applyAttrs(filepath.Join(h.dir, "a.txt"), true, true)
	h.frames(4)
	if len(h.a.ops.dialogs) != 0 || len(h.a.ops.running) != 0 {
		t.Error("something was asked or begun")
	}
	if !changes(CmdEmptyTrash) {
		t.Error("emptying the trash is no change")
	}
}

// What is dragged out of a window that is read only is copied, never
// moved, whatever is held.
func TestADragOutOfAReadOnlyWindowCopies(t *testing.T) {
	d := FileDrag{Paths: []string{"/a/x"}, Volume: "/", ReadOnly: true}
	plan, _, ok := planDrop(dropTarget{fs: "", ps: SlashPaths, dir: "/b", name: "b", vol: "/", own: true}, false, d, input.ModShift)
	if !ok || !plan.Copy {
		t.Fatalf("the drop plans %+v", plan)
	}
}

// A window kept inside a top has no trash, which is elsewhere, and
// writes every path from the top's name, as the path bar does: a path
// typed so goes there.
func TestATopHasNoTrashAndNamesItsPaths(t *testing.T) {
	h := topHarness(t, nil, "top/inner/a.txt")
	top := filepath.Join(h.dir, "top")
	if h.a.trash != nil || !h.a.shell.NoTrash || !h.a.shell.NoTrashCan {
		t.Error("a window under a top has a trash")
	}
	h.do(Navigate{Path: TrashPath})
	if h.a.bin.open {
		t.Error("the trash opened")
	}
	in := filepath.Join(top, "inner", "a.txt")
	want := "Shared" + string(filepath.Separator) + filepath.Join("inner", "a.txt")
	if got := h.a.shown(in); got != want {
		t.Errorf("shown as %q, want %q", got, want)
	}
	if got := h.a.shell.show(in); got != want {
		t.Errorf("the view shows %q, want %q", got, want)
	}
	if got := h.a.untop("reading " + in + ": no"); got != "reading "+want+": no" {
		t.Errorf("an error says %q", got)
	}
	h.do(Navigate{Path: "Shared" + string(filepath.Separator) + "inner", Typed: true})
	h.until("the typed folder shows", func() bool {
		return SystemPaths.Same(h.a.nav.path, filepath.Join(top, "inner")) && !h.a.nav.loading
	})
	if strings.Contains(h.b.path.addr.Text(), h.dir) {
		t.Errorf("the path bar shows %q", h.b.path.addr.Text())
	}
}

// On a desktop the path bar has no Upload, and no room kept for it.
func TestADesktopPathBarHasNoUploadRoom(t *testing.T) {
	h := newHarness(t, "a.txt")
	deep := h.bounds(func(b *browser) gunim.Node { return b.path.deep })
	bar := h.bounds(func(b *browser) gunim.Node { return b.path })
	if right := bar.Max.X - 8; deep.Max.X < right-1 {
		t.Errorf("the filter's button ends at %v, the bar's room at %v", deep.Max.X, right)
	}
}

// In a browser, Disk usage's Show in Files shows the item here, and a
// file viewed from elsewhere is gone to first.
func TestABrowserShowsAndViewsInItsOneWindow(t *testing.T) {
	h := browserHarness(t, nil, "sub/b.txt", "a.txt")
	h.a.usageAct("", UsageRead)
	h.do(Command{Name: CmdUsage})
	h.until("the folder is read", func() bool { return h.a.usage.open && h.a.usage.scan != nil && !h.a.usage.scan.reading() })
	h.a.usageAct("sub/b.txt", UsageShow)
	h.until("the item shows here", func() bool {
		return !h.a.usage.open && SystemPaths.Same(h.a.nav.path, filepath.Join(h.dir, "sub"))
	})
	h.do(Navigate{Path: h.dir})
	h.until("the folder shows", func() bool { return SystemPaths.Same(h.a.nav.path, h.dir) && !h.a.nav.loading })
	h.a.viewFile(filepath.Join(h.dir, "sub", "b.txt"))
	h.until("the file is viewed in its folder", func() bool {
		return SystemPaths.Same(h.a.nav.path, filepath.Join(h.dir, "sub")) && (h.a.reading.open || h.a.viewer.open)
	})
}

// An upload leaves out a name it cannot have and a folder, says so at
// the end, and asks nothing of the folder.
func TestAnUploadLeavesOutWhatIsNoFile(t *testing.T) {
	h := browserHarness(t, nil, "pics")
	said := notices(h)
	folder := input.NewStream("pics", "", -1, failing{input.ErrFolder})
	h.do(UploadDropped{Into: h.dir, Token: holdDropped([]*input.Stream{
		streamOf("..", "x"), folder, streamOf("ok.txt", "fine"),
	})})
	h.until("the good one arrives", func() bool { return h.exists("ok.txt") && len(h.a.ops.running) == 0 })
	if len(h.a.ops.dialogs) != 0 {
		t.Error("the folder was asked about")
	}
	h.until("what was left out is said", func() bool {
		return slices.ContainsFunc(said(), func(n Notice) bool {
			return strings.Contains(n.Body, "Left out pics") && strings.Contains(n.Body, "Left out ..")
		})
	})
	left, _ := os.ReadDir(h.dir)
	for _, e := range left {
		if strings.HasSuffix(e.Name(), ".part") {
			t.Errorf("%s is left", e.Name())
		}
	}
}

// An archive selected downloads as the file it is, not as a zip of what
// it holds.
func TestAnArchiveDownloadsAsItIs(t *testing.T) {
	h := browserHarness(t, nil)
	writeZip(t, filepath.Join(h.dir, "a.zip"), map[string]string{"in.txt": "IN"})
	out := filepath.Join(t.TempDir(), "got")
	var asked driver.SaveOptions
	h.w.Offscreen().SetSaver(func(o driver.SaveOptions) (string, error) { asked = o; return out, nil })
	h.do(Command{Name: CmdRefresh})
	h.until("the zip is listed", func() bool { return slices.Equal(h.shown(), []string{"a.zip"}) })
	h.pick("a.zip")
	h.do(Command{Name: CmdDownload})
	h.until("it is saved", func() bool { return len(h.a.ops.running) == 0 && contents(t, out) != "missing" })
	want, _ := os.ReadFile(filepath.Join(h.dir, "a.zip"))
	if got, _ := os.ReadFile(out); asked.Name != "a.zip" || !bytes.Equal(got, want) {
		t.Errorf("saved %d bytes as %q", len(got), asked.Name)
	}
}

// Windows that share a hub's memory decode one picture at a time, and
// keep their thumbnails within it between them.
func TestAHubsMemoryIsShared(t *testing.T) {
	h := newHarness(t, "a.txt")
	h.a.hub.SetMemory(64 << 20)
	m := h.a.hub.mem
	h.a.mem = m
	if got := h.a.thumbsShare(); got != 16<<20 {
		t.Errorf("one window's share is %d", got)
	}
	release := m.hold()
	got := make(chan struct{})
	go func() { defer m.hold()(); close(got) }()
	select {
	case <-got:
		t.Fatal("two pictures decode at once")
	case <-time.After(50 * time.Millisecond):
	}
	release()
	<-got
}
