package filemanager

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/marrasen/gunim/input"
)

// bareFS is the computer's own disk with nothing but what every file
// system can do, under an ID of its own, as a file system elsewhere would
// be: no trash, no free space, no links, and no programs to open its
// files with.
type bareFS struct{ FS }

func (bareFS) ID() string { return "elsewhere" }

func TestAFileSystemWithoutATrashDeletesForGoodAfterAsking(t *testing.T) {
	h := newHarnessOn(t, bareFS{LocalFS()}, "keep.txt", "gone.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 2 })
	h.pick("gone.txt")
	h.w.Input(input.KeyPress{Key: input.KeyDelete})
	h.until("the question shows", func() bool { return len(h.a.ops.dialogs) == 1 })
	c, ok := h.a.ops.dialogs[0].state.(Confirm)
	if !ok || !strings.Contains(c.Title, "for good") {
		t.Fatalf("the dialog is %+v", h.a.ops.dialogs[0].state)
	}
	h.answer(Confirmed{Token: c.Token, OK: true})
	h.until("the file is gone", func() bool { return !h.exists("gone.txt") })
	h.until("the listing drops it", func() bool { return slices.Equal(h.shown(), []string{"keep.txt"}) })
	if h.b.status.right.Text != "" {
		t.Fatalf("the status bar says %q of a file system that knows no free space", h.b.status.right.Text)
	}
}

func TestAFileSystemWithoutATrashCopiesWithoutUndo(t *testing.T) {
	h := newHarnessOn(t, bareFS{LocalFS()}, "a.txt", "sub/")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 2 })
	h.pick("a.txt")
	h.do(Command{Name: CmdCopy})
	h.do(Navigate{Path: filepath.Join(h.dir, "sub")})
	h.do(Command{Name: CmdPaste})
	h.until("the copy is made", func() bool { return h.exists("sub/a.txt") })
	h.idle()
	if contents(t, filepath.Join(h.dir, "sub", "a.txt")) != "a.txt" {
		t.Fatal("the copy holds something else")
	}
	if len(h.a.ops.undo) != 0 {
		t.Fatal("a copy that only a trash could take back is offered to undo")
	}
	if _, err := os.Stat(filepath.Join(h.root, "Trash")); err == nil {
		t.Fatal("something went to the trash of the computer's own disk")
	}
}

func TestAFileSystemElsewhereOpensNothingWithTheSystem(t *testing.T) {
	h := newHarnessOn(t, bareFS{LocalFS()}, "a.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	h.pick("a.txt")
	h.do(Command{Name: CmdOpen})
	h.until("the banner says why", func() bool { return strings.Contains(h.b.banner.label.Text, "cannot open") })
}

func TestADropFromAnotherFileSystemIsRefused(t *testing.T) {
	d := FileDrag{Paths: []string{"/a/x"}, FS: "elsewhere"}
	if _, hint, ok := dropPlan(SystemPaths, "", d, "/b", "", "", 0); ok || !strings.Contains(hint.Text, "another file system") {
		t.Fatalf("a drop from elsewhere is planned, saying %q", hint.Text)
	}
	if _, err := d.ExportFiles(); err == nil {
		t.Fatal("files of another file system are handed to other programs")
	}
}

func TestSlashPathsJoinAndSplitWithSlashes(t *testing.T) {
	ps := SlashPaths
	if got := ps.Join("/home/me", "a"); got != "/home/me/a" {
		t.Fatalf("Join gives %q", got)
	}
	if got := ps.Dir("/home/me/a"); got != "/home/me" {
		t.Fatalf("Dir gives %q", got)
	}
	if ps.Same("/Home", "/home") || !ps.Same("/home/me/", "/home/me") {
		t.Fatal("Same is wrong about case or a trailing slash")
	}
	for _, c := range []struct {
		base, target, want string
	}{
		{"/a/b", "/a/b/c/d", "c/d"},
		{"/a/b", "/a/b", "."},
		{"/a/b/c", "/a/x", "../../x"},
		{"/", "/a", "a"},
	} {
		if got, err := ps.Rel(c.base, c.target); err != nil || got != c.want {
			t.Fatalf("Rel(%q, %q) gives %q, %v; want %q", c.base, c.target, got, err, c.want)
		}
	}
	if !ps.inside("/a/b/c", "/a/b") || ps.inside("/a/bc", "/a/b") {
		t.Fatal("inside is wrong")
	}
	if _, err := ps.Abs("relative/path"); err == nil {
		t.Fatal("a relative slash path is taken")
	}
	got := crumbs(ps, "/srv/data")
	want := []Crumb{{Name: "/", Path: "/"}, {Name: "srv", Path: "/srv"}, {Name: "data", Path: "/srv/data"}}
	if !slices.Equal(got, want) {
		t.Fatalf("the crumbs are %v", got)
	}
	if err := checkName(ps, `a:b`); err != nil {
		t.Fatalf("a slash path's name may hold a colon: %v", err)
	}
}
