package filemanager

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"
)

// driveFS shows the folder root as drive C: of a Windows machine reached
// over SFTP: paths as SFTP writes them, /C:/Users, where case does not
// tell names apart, and / the list of the drives. It counts the paths it
// is asked to true-case.
type driveFS struct {
	root  string
	cased *atomic.Int32
}

func (driveFS) ID() string            { return "drives" }
func (driveFS) Paths() PathStyle      { return DrivePaths }
func (driveFS) Home() (string, error) { return "/C:/Users", nil }

// real is where p is on the computer's own file system, found in any
// case, or ok false for /, which is no folder there. A path that is not
// one of the file system's, as a Windows path would be, fails, so a
// mix-up shows.
func (d driveFS) real(p string) (at string, ok bool, err error) {
	if p == "/" {
		return "", false, nil
	}
	if strings.Contains(p, `\`) || !isDrive(p) || p[1] != 'C' {
		return "", false, fmt.Errorf("%q is not a path of this file system", p)
	}
	at = d.root
	for _, name := range splitSlash(p[3:]) {
		es, _ := os.ReadDir(at)
		i := slices.IndexFunc(es, func(e os.DirEntry) bool { return strings.EqualFold(e.Name(), name) })
		if i >= 0 {
			name = es[i].Name()
		}
		at = filepath.Join(at, name)
	}
	return at, true, nil
}

// TrueCase implements [TrueCaser].
func (d driveFS) TrueCase(p string) (string, error) {
	d.cased.Add(1)
	r, ok, err := d.real(p)
	if err != nil || !ok {
		return p, err
	}
	if _, serr := os.Stat(r); serr != nil {
		return "", serr
	}
	rel, err := filepath.Rel(d.root, r)
	if err != nil {
		return "", err
	}
	return DrivePaths.Join("/C:", filepath.ToSlash(rel)), nil
}

// drivesInfo describes the list of the drives, and drive C: in it.
type drivesInfo struct{ name string }

func (i drivesInfo) Name() string     { return i.name }
func (drivesInfo) Size() int64        { return 0 }
func (drivesInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o755 }
func (drivesInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (drivesInfo) IsDir() bool        { return true }
func (drivesInfo) Sys() any           { return nil }
func (d driveFS) statAt(p string, stat func(string) (fs.FileInfo, error)) (fs.FileInfo, error) {
	r, ok, err := d.real(p)
	if err != nil {
		return nil, err
	}
	if !ok {
		return drivesInfo{"/"}, nil
	}
	return stat(r)
}

func (d driveFS) ReadDir(ctx context.Context, dir string) ([]fs.DirEntry, error) {
	r, ok, err := d.real(dir)
	if err != nil {
		return nil, err
	}
	if !ok {
		return []fs.DirEntry{fs.FileInfoToDirEntry(drivesInfo{"C:"})}, nil
	}
	return LocalFS().ReadDir(ctx, r)
}

func (d driveFS) Stat(p string) (fs.FileInfo, error)  { return d.statAt(p, os.Stat) }
func (d driveFS) Lstat(p string) (fs.FileInfo, error) { return d.statAt(p, os.Lstat) }

func (d driveFS) Open(p string) (io.ReadSeekCloser, error) {
	r, _, err := d.real(p)
	if err != nil {
		return nil, err
	}
	return LocalFS().Open(r)
}

func (d driveFS) Create(p string) (io.WriteCloser, error) {
	r, _, err := d.real(p)
	if err != nil {
		return nil, err
	}
	return LocalFS().Create(r)
}

func (d driveFS) Mkdir(p string, perm fs.FileMode) error {
	r, _, err := d.real(p)
	if err != nil {
		return err
	}
	return os.Mkdir(r, perm)
}

func (d driveFS) Rename(from, to string) error {
	rf, _, err := d.real(from)
	if err != nil {
		return err
	}
	rt, _, err := d.real(to)
	if err != nil {
		return err
	}
	return os.Rename(rf, rt)
}

func (d driveFS) Remove(p string) error {
	r, _, err := d.real(p)
	if err != nil {
		return err
	}
	return os.Remove(r)
}

// onDriveFS shows the harness's folder as drive C: of a Windows machine,
// opening on /C:/Users/Marcus, and returns the count of paths
// true-cased.
func onDriveFS(cased *atomic.Int32) func(o *Options) {
	return func(o *Options) { o.FS, o.Dir = driveFS{root: o.Dir, cased: cased}, "/C:/Users/Marcus" }
}

// newDriveHarness runs a window on drive C: of a Windows machine, as
// onDriveFS sets it, which h.ui can reach.
func newDriveHarness(t *testing.T, cased *atomic.Int32, spec ...string) *harness {
	t.Helper()
	h := newHarnessWith(t, onDriveFS(cased), spec...)
	gunim.RegisterPatch(h.w, "browser", func(b *browser, p inUI, u *gunim.UI) { p.fn(b, u) })
	return h
}

// clipboard returns what the window's clipboard holds.
func (h *harness) clipboard() string {
	var s string
	h.ui(func(_ *browser, u *gunim.UI) { s = u.Clipboard() })
	return s
}

// previewPath returns the path the preview pane shows.
func (h *harness) previewPath() string {
	var s string
	h.ui(func(b *browser, _ *gunim.UI) {
		if b.preview.cur != nil && b.preview.cur.path != nil {
			s = b.preview.cur.path.Text
		}
	})
	return s
}

func placeNames(ps []widget.AddressPlace) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}

func TestDrivePathsShowAsWindowsWritesThem(t *testing.T) {
	var cased atomic.Int32
	h := newDriveHarness(t, &cased, "Users/Marcus/a.txt", "Workspace/b.txt")
	h.until("the folder lists", func() bool { return slices.Equal(h.shown(), []string{"a.txt"}) })
	if h.a.nav.path != "/C:/Users/Marcus" {
		t.Fatalf("the window is at %q", h.a.nav.path)
	}
	if got := placeNames(h.places()); !slices.Equal(got, []string{"Drives", "C:", "Users", "Marcus"}) {
		t.Fatalf("the path bar shows %q", got)
	}
	if h.b.title.folder != "Marcus" {
		t.Fatalf("the title names %q", h.b.title.folder)
	}
	h.until("the preview shows the folder's path", func() bool { return h.previewPath() == `C:\Users\Marcus` })
	h.press(input.KeyL, input.ModControl)
	if got := h.b.path.addr.Text(); got != `C:\Users\Marcus` {
		t.Fatalf("the path bar edits %q", got)
	}
	h.press(input.KeyEscape, 0)

	h.click(h.point("a.txt"))
	h.until("the preview shows the file's path", func() bool { return h.previewPath() == `C:\Users\Marcus\a.txt` })
	h.press(input.KeyC, input.ModControl|input.ModShift)
	if got := h.clipboard(); got != `C:\Users\Marcus\a.txt` {
		t.Fatalf("Copy path copied %q", got)
	}

	h.do(Navigate{Path: "/C:"})
	h.until("the drive lists", func() bool { return slices.Equal(h.shown(), []string{"Users", "Workspace"}) })
	if h.b.title.folder != "C:" || h.previewPath() != `C:\` {
		t.Fatalf("the drive is called %q, at %q", h.b.title.folder, h.previewPath())
	}
	h.do(Navigate{Path: "/"})
	h.until("the drives list", func() bool { return slices.Equal(h.shown(), []string{"C:"}) })
	if got := placeNames(h.places()); h.b.title.folder != "Drives" || !slices.Equal(got, []string{"Drives"}) {
		t.Fatalf("the drives are called %q, with the path bar showing %q", h.b.title.folder, got)
	}
	if n := cased.Load(); n != 0 {
		t.Fatalf("paths not typed were true-cased %d times", n)
	}
	if h.b.banner.label.Text != "" {
		t.Fatalf("the banner says %q", h.b.banner.label.Text)
	}
}

func TestATypedDrivePathTakesTheFoldersCase(t *testing.T) {
	var cased atomic.Int32
	h := newDriveHarness(t, &cased, "Users/Marcus/a.txt", "Workspace/Deep/b.txt")
	h.until("the folder lists", func() bool { return slices.Equal(h.shown(), []string{"a.txt"}) })
	h.press(input.KeyL, input.ModControl)
	h.w.Input(input.TextInput{Text: `c:\workspace\deep`})
	h.press(input.KeyEnter, 0)
	h.until("the typed folder opens", func() bool { return slices.Equal(h.shown(), []string{"b.txt"}) })
	if h.a.nav.path != "/C:/Workspace/Deep" || cased.Load() != 1 {
		t.Fatalf("the window is at %q, after %d paths true-cased", h.a.nav.path, cased.Load())
	}
	if got := placeNames(h.places()); !slices.Equal(got, []string{"Drives", "C:", "Workspace", "Deep"}) {
		t.Fatalf("the path bar shows %q", got)
	}

	// A click on a folder of the path goes there without asking.
	h.frames(30)
	drive := h.places()[1]
	h.click(geom.Pt(drive.Rect.Min.X+8, drive.Rect.Center().Y))
	h.until("the drive opens", func() bool { return h.a.nav.path == "/C:" })
	h.do(Command{Name: CmdBack})
	h.until("back goes back", func() bool { return h.a.nav.path == "/C:/Workspace/Deep" })
	if n := cased.Load(); n != 1 {
		t.Fatalf("paths not typed were true-cased, %d in all", n)
	}

	// A path that is not there goes as typed, and says so there.
	h.press(input.KeyL, input.ModControl)
	h.w.Input(input.TextInput{Text: `C:\Nowhere`})
	h.press(input.KeyEnter, 0)
	h.until("the missing folder shows why", func() bool { return h.a.nav.path == "/C:/Nowhere" && h.a.nav.err != nil })
}

func TestDrivePathsTypedAndCompared(t *testing.T) {
	ps := DrivePaths
	for in, want := range map[string]string{
		`C:\Users\me`:        "/C:/Users/me",
		`c:/Users/me`:        "/C:/Users/me",
		`/c:/Users`:          "/C:/Users",
		`\c:\Users`:          "/C:/Users",
		`\\?\D:\x`:           "/D:/x",
		`C:`:                 "/C:",
		`c:\`:                "/C:",
		`/`:                  "/",
		`D:\a\..\b`:          "/D:/b",
		`/C:/Users/../../..`: "/",
	} {
		got, err := ps.Abs(in)
		if err != nil || got != want {
			t.Errorf("%q is %q, %v; want %q", in, got, err, want)
		}
	}
	if got, err := ps.Abs(" C:\\Users\\me\\ "); err != nil || got != "/C:/Users/me" {
		t.Errorf("a path typed with spaces around is %q, %v", got, err)
	}
	for _, in := range []string{"C:x", "relative", `\Users`, `\\server\share`, "1:/x"} {
		if got, err := ps.Abs(in); err == nil {
			t.Errorf("%q is %q, want refused", in, got)
		}
	}
	for path, want := range map[string]string{
		"/":              "/",
		"/C:":            `C:\`,
		"/C:/":           `C:\`,
		"/C:/Users/me":   `C:\Users\me`,
		"Users/me":       `Users\me`,
		"/not-a-drive/x": "/not-a-drive/x",
	} {
		if got := ps.Show(path); got != want {
			t.Errorf("%q shows as %q, want %q", path, got, want)
		}
	}
	if SlashPaths.Show("/C:/Users") != "/C:/Users" || SystemPaths.Show("/a/b") != "/a/b" {
		t.Error("other styles do not show paths as they are")
	}
	if !ps.Same("/c:/users", "/C:/Users/") || !ps.Same("/c:", "/C:") || ps.Same("/C:/a", "/C:/b") {
		t.Error("Same is wrong")
	}
	if SlashPaths.Same("/c:/users", "/C:/Users") {
		t.Error("slash paths lost their case")
	}
	if !ps.inside("/c:/users/me", "/C:/Users") || ps.inside("/C:/Usersx", "/C:/Users") {
		t.Error("inside is wrong")
	}
	if ps.placeName("/") != "Drives" || ps.placeName("/C:") != "C:" || ps.placeName("/C:/Users") != "Users" {
		t.Errorf("the places are called %q, %q, %q", ps.placeName("/"), ps.placeName("/C:"), ps.placeName("/C:/Users"))
	}
	got := crumbs(ps, "/C:/Users")
	want := []Crumb{{Name: "Drives", Path: "/"}, {Name: "C:", Path: "/C:"}, {Name: "Users", Path: "/C:/Users"}}
	if !slices.Equal(got, want) {
		t.Fatalf("the crumbs are %v", got)
	}
	if err := checkName(ps, "a:b"); err == nil {
		t.Error("a Windows name may hold a colon")
	}
}
