package filemanager

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim/input"
)

// bareFS is the computer's own disk with nothing but what every file
// system can do, under an ID of its own, as a file system elsewhere would
// be: no trash, no free space, no links, and no programs to open its
// files with.
type bareFS struct{ FS }

func (bareFS) ID() string { return "elsewhere" }

func TestAFileSystemWithoutATrashDeletesForGoodAfterAsking(t *testing.T) {
	h := newHarnessWith(t, onBareFS, "keep.txt", "gone.txt")
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
	h := newHarnessWith(t, onBareFS, "a.txt", "sub/")
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
	h.do(Command{Name: CmdUndo})
	h.idle()
	if !h.exists("sub/a.txt") {
		t.Fatal("undo took the copy away with no trash to take it to")
	}
}

func TestAFileSystemElsewhereOpensNothingWithTheSystem(t *testing.T) {
	h := newHarnessWith(t, onBareFS, "a.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	h.pick("a.txt")
	h.do(Command{Name: CmdOpen})
	h.until("the banner says why", func() bool { return strings.Contains(h.b.banner.label.Text, "cannot open") })
}

func TestADropFromAnotherFileSystemIsRefused(t *testing.T) {
	d := FileDrag{Paths: []string{"/a/x"}, FS: "elsewhere"}
	if _, hint, ok := dropPlan(SystemPaths, "", false, d, "/b", "", "", 0); ok || !strings.Contains(hint.Text, "another file system") {
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

// onBareFS sets a window's file system to a bare one.
func onBareFS(o *Options) { o.FS = bareFS{LocalFS()} }

// slashFS shows the folder root as a file system of its own, with paths
// from "/" written with '/' alone, as a server's are. A path that is
// not one, as a path of the computer's own file system would be on
// Windows, fails, so a mix-up shows.
type slashFS struct{ root string }

func (slashFS) ID() string            { return "slash" }
func (slashFS) Paths() PathStyle      { return SlashPaths }
func (slashFS) Home() (string, error) { return "/", nil }

// real is where p is on the computer's own file system.
func (s slashFS) real(p string) (string, error) {
	if !strings.HasPrefix(p, "/") || strings.Contains(p, `\`) {
		return "", fmt.Errorf("%q is not a path of this file system", p)
	}
	return filepath.Join(s.root, filepath.FromSlash(p)), nil
}

func (s slashFS) ReadDir(ctx context.Context, dir string) ([]fs.DirEntry, error) {
	r, err := s.real(dir)
	if err != nil {
		return nil, err
	}
	return LocalFS().ReadDir(ctx, r)
}

func (s slashFS) Stat(p string) (fs.FileInfo, error) {
	r, err := s.real(p)
	if err != nil {
		return nil, err
	}
	return os.Stat(r)
}

func (s slashFS) Lstat(p string) (fs.FileInfo, error) {
	r, err := s.real(p)
	if err != nil {
		return nil, err
	}
	return os.Lstat(r)
}

func (s slashFS) Open(p string) (io.ReadSeekCloser, error) {
	r, err := s.real(p)
	if err != nil {
		return nil, err
	}
	return LocalFS().Open(r)
}

func (s slashFS) Create(p string) (io.WriteCloser, error) {
	r, err := s.real(p)
	if err != nil {
		return nil, err
	}
	return LocalFS().Create(r)
}

func (s slashFS) Mkdir(p string, perm fs.FileMode) error {
	r, err := s.real(p)
	if err != nil {
		return err
	}
	return os.Mkdir(r, perm)
}

func (s slashFS) Rename(from, to string) error {
	rf, err := s.real(from)
	if err != nil {
		return err
	}
	rt, err := s.real(to)
	if err != nil {
		return err
	}
	return os.Rename(rf, rt)
}

func (s slashFS) Remove(p string) error {
	r, err := s.real(p)
	if err != nil {
		return err
	}
	return os.Remove(r)
}

// onSlashFS shows the harness's folder as the top of a slash file system.
func onSlashFS(o *Options) {
	o.FS, o.Dir = slashFS{root: o.Dir}, "/"
}

func TestAWindowWorksOnSlashPaths(t *testing.T) {
	h := newHarnessWith(t, onSlashFS, "a.txt", "sub/b.txt")
	h.until("the top lists", func() bool { return slices.Equal(h.shown(), []string{"sub", "a.txt"}) })
	if h.a.nav.path != "/" || !slices.Equal(crumbs(SlashPaths, "/"), []Crumb{{Name: "/", Path: "/"}}) {
		t.Fatalf("the window is at %q", h.a.nav.path)
	}
	h.pick("a.txt")
	h.do(Command{Name: CmdCopy})
	h.do(Navigate{Path: "/sub"})
	h.until("the folder opens", func() bool { return slices.Equal(h.shown(), []string{"b.txt"}) })
	if h.a.nav.path != "/sub" {
		t.Fatalf("the window is at %q", h.a.nav.path)
	}
	h.do(Command{Name: CmdPaste})
	h.until("the copy is made", func() bool { return h.exists("sub/a.txt") })
	h.until("the copy lists", func() bool { return slices.Equal(h.shown(), []string{"a.txt", "b.txt"}) })
	h.pick("b.txt")
	h.do(Command{Name: CmdRename})
	h.until("the prompt shows", func() bool { return len(h.a.ops.dialogs) == 1 })
	p, _ := h.a.ops.dialogs[0].state.(Prompt)
	h.answer(Prompted{Token: p.Token, Text: "c.txt", OK: true})
	h.until("the file is renamed", func() bool { return h.exists("sub/c.txt") && !h.exists("sub/b.txt") })
	h.do(Command{Name: CmdUp})
	h.until("up goes to the top", func() bool { return h.a.nav.path == "/" && slices.Equal(h.shown(), []string{"sub", "a.txt"}) })
	if h.a.nav.sel["sub"] != true {
		t.Fatal("going up does not select the folder it came from")
	}
	if h.b.banner.label.Text != "" {
		t.Fatalf("the banner says %q", h.b.banner.label.Text)
	}
}

// unsureFS is a file system that has a trash, it says, until it is used.
type unsureFS struct{ bareFS }

func (unsureFS) Trash(string) (string, error) { return "", errors.ErrUnsupported }

func (unsureFS) Restore(string, string, time.Time, string) error { return errors.ErrUnsupported }

func (unsureFS) Describe(string) string { return "" }

func (unsureFS) Space(string) (free, total uint64, err error) { return 0, 0, errors.ErrUnsupported }

func TestATrashThatTurnsOutMissingDeletesAfterAsking(t *testing.T) {
	h := newHarnessWith(t, func(o *Options) { o.FS = unsureFS{bareFS{LocalFS()}} }, "gone.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	if h.b.shell.NoTrash {
		t.Fatal("the window says there is no trash before it tried one")
	}
	h.pick("gone.txt")
	h.w.Input(input.KeyPress{Key: input.KeyDelete})
	h.until("the question shows", func() bool { return len(h.a.ops.dialogs) == 1 })
	c, ok := h.a.ops.dialogs[0].state.(Confirm)
	if !ok {
		t.Fatalf("the dialog is %+v", h.a.ops.dialogs[0].state)
	}
	h.answer(Confirmed{Token: c.Token, OK: true})
	h.until("the file is gone", func() bool { return !h.exists("gone.txt") })
	if !h.b.shell.NoTrash || h.b.title.bar.Menus[1].Items[slices.Index(h.b.title.cmds[1], CmdTrash)] != "Delete…" {
		t.Fatal("the menu still offers the trash")
	}
	if h.b.status.right.Text != "" || h.b.banner.label.Text != "" {
		t.Fatalf("an unknown free space shows as %q, %q", h.b.status.right.Text, h.b.banner.label.Text)
	}
}

func TestAFileSystemWithoutATrashSaysDeleteInTheMenus(t *testing.T) {
	h := newHarnessWith(t, onBareFS, "a.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	if got := h.b.title.bar.Menus[1].Items[slices.Index(h.b.title.cmds[1], CmdTrash)]; got != "Delete…" {
		t.Fatalf("the Edit menu says %q", got)
	}
}
