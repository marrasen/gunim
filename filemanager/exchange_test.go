package filemanager

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/geom"
	"github.com/marrasen/gunim/input"
	"github.com/marrasen/gunim/widget"
)

// topHarness is a window kept inside the folder top of a harness's dir,
// with no places but those the window finds itself.
func topHarness(t *testing.T, set func(o *Options), spec ...string) *harness {
	t.Helper()
	return newHarnessWith(t, func(o *Options) {
		o.Places = nil
		o.Dir = filepath.Join(filepath.Dir(o.Dir), "dir", "top", "inner")
		o.Top, o.TopName = filepath.Join(filepath.Dir(o.Dir), "..", "top"), "Shared"
		if set != nil {
			set(o)
		}
	}, spec...)
}

func TestATopIsAllTheWindowShows(t *testing.T) {
	h := topHarness(t, nil, "top/inner/a.txt", "top/b.txt", "outside.txt")
	top := filepath.Join(h.dir, "top")
	h.until("the inner folder shows", func() bool { return slices.Equal(h.shown(), []string{"a.txt"}) })
	l := Listing{Crumbs: h.a.topCrumbs(h.a.nav.path)}
	if len(l.Crumbs) != 2 || l.Crumbs[0].Name != "Shared" || !SystemPaths.Same(l.Crumbs[0].Path, top) {
		t.Fatalf("the path bar starts %+v", l.Crumbs)
	}
	h.do(Command{Name: CmdUp})
	h.until("up reaches the top", func() bool { return SystemPaths.Same(h.a.nav.path, top) && !h.a.nav.loading })
	if h.b.path.up.Disabled != true {
		t.Error("Up is not dimmed at the top")
	}
	h.do(Command{Name: CmdUp})
	h.do(Navigate{Path: h.dir, Typed: true})
	h.do(Navigate{Path: h.root})
	h.frames(10)
	if !SystemPaths.Same(h.a.nav.path, top) {
		t.Fatalf("the window went above the top, to %s", h.a.nav.path)
	}
	if !strings.Contains(h.a.bannerText, "outside Shared") {
		t.Errorf("the window says %q", h.a.bannerText)
	}
	h.until("the places arrive", func() bool { return len(h.a.places) == 1 })
	if p := h.a.places[0]; p.Name != "Shared" || !SystemPaths.Same(p.Path, top) {
		t.Errorf("the place is %+v", p)
	}
	if got := h.a.withinTop([]Place{{Path: h.dir}, {Path: filepath.Join(top, "inner")}}); len(got) != 1 {
		t.Errorf("places above the top are kept: %+v", got)
	}
	h.do(Command{Name: CmdHome})
	h.until("home is the top", func() bool { return SystemPaths.Same(h.a.nav.path, top) })
}

// A window asked to open above its top opens at the top.
func TestAWindowOpenedAboveItsTopOpensThere(t *testing.T) {
	h := topHarness(t, func(o *Options) { o.Dir = filepath.Dir(o.Top) }, "top/b.txt")
	h.until("the top shows", func() bool { return slices.Equal(h.shown(), []string{"b.txt"}) })
}

// readOnlyMenus reports, for each command, whether the menu bar dims it.
func readOnlyMenus(h *harness) map[string]bool {
	out := map[string]bool{}
	t := h.b.title
	for m, cmds := range t.cmds {
		for i, c := range cmds {
			out[c] = t.bar.Menus[m].Items[i].Disabled
		}
	}
	return out
}

func TestAWindowReadOnlyChangesNothing(t *testing.T) {
	h := newHarnessWith(t, func(o *Options) { o.ReadOnly = true }, "a.txt", "sub/")
	said := notices(h)
	h.until("the rows arrive", func() bool { return len(h.shown()) == 2 })
	dimmed := readOnlyMenus(h)
	for _, c := range []string{CmdCut, CmdPaste, CmdRename, CmdTrash, CmdNewFolder, CmdUndo} {
		if !dimmed[c] {
			t.Errorf("%s is not dimmed", c)
		}
	}
	if dimmed[CmdCopy] || dimmed[CmdOpen] {
		t.Error("what only reads is dimmed")
	}
	h.pick("a.txt")
	for _, c := range []string{CmdTrash, CmdDelete, CmdRename, CmdNewFolder, CmdDuplicate} {
		h.do(Command{Name: c})
	}
	h.frames(10)
	if !h.exists("a.txt") || len(h.a.ops.dialogs) > 0 || len(h.a.ops.running) > 0 {
		t.Fatal("a change went ahead")
	}
	h.until("the window says why", func() bool {
		return slices.ContainsFunc(said(), func(n Notice) bool { return n.Title == readOnlyHint })
	})
	h.do(DropFiles{Paths: []string{filepath.Join(h.dir, "a.txt")}, Into: filepath.Join(h.dir, "sub"), FS: "", To: ""})
	h.frames(10)
	if !h.exists("a.txt") || h.exists("sub/a.txt") {
		t.Fatal("a drop moved a file")
	}
	spot := h.b.dnd.spot(input.Drop{Data: FileDrag{Paths: []string{"/x"}}}, spotKey{"here", h.dir}, geom.Rect{}, h.dir, h.dir, false)
	if !spot.Refused {
		t.Error("the listing takes a drop")
	}

	// Changeable again, the same file goes.
	w := &Window{a: h.a, ready: make(chan struct{}), done: make(chan struct{})}
	close(w.ready)
	w.SetReadOnly(false)
	h.until("the menus come back", func() bool { return !readOnlyMenus(h)[CmdRename] })
	h.pick("a.txt")
	h.do(Command{Name: CmdDelete})
	h.until("the delete asks", func() bool { return len(h.a.ops.dialogs) == 1 })
}

// browserHarness is a window as it is in a browser.
func browserHarness(t *testing.T, set func(o *Options), spec ...string) *harness {
	t.Helper()
	return newHarnessWith(t, func(o *Options) {
		o.browser = true
		if set != nil {
			set(o)
		}
	}, spec...)
}

func TestABrowserOffersDownloadAndUploadAndNoneOfTheComputers(t *testing.T) {
	h := browserHarness(t, nil, "a.txt")
	cmds := readOnlyMenus(h)
	for _, c := range []string{CmdDownload, CmdUpload} {
		if _, ok := cmds[c]; !ok {
			t.Errorf("the menus have no %s", c)
		}
	}
	for _, c := range []string{CmdNewWindow, CmdReveal, CmdCloseApp, CmdTrashCan, CmdUploadAsk} {
		if _, ok := cmds[c]; ok {
			t.Errorf("the menus have %s", c)
		}
	}
	if got := (Shell{}).offered(rowItems); slices.ContainsFunc(got, func(m menuItem) bool { return m.cmd == CmdDownload }) {
		t.Error("a window on a desktop offers Download")
	}
	if got := (Shell{Browser: true}).offered(rowItems); slices.ContainsFunc(got, func(m menuItem) bool {
		return m.cmd == localOpenWith || m.cmd == CmdOpenSystem || m.cmd == localOpenWindow
	}) {
		t.Error("a browser offers to open with the computer's programs")
	}
	// Ctrl+N asks for nothing a browser has.
	h.do(Command{Name: CmdNewWindow})
	if h.a.bannerText != "" {
		t.Errorf("the window says %q", h.a.bannerText)
	}
}

func TestOpeningAFileInABrowserViewsIt(t *testing.T) {
	h := browserHarness(t, nil, "a.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	h.do(Activated{Gen: h.a.nav.gen, Row: 0})
	h.until("the viewer opens", func() bool { return h.a.reading.open || h.a.viewer.open })
}

// streamOf is a file of the user's computer, as a browser hands it over.
func streamOf(name, body string) *input.Stream {
	return input.NewStream(name, "", int64(len(body)), io.NopCloser(strings.NewReader(body)))
}

func TestFilesChosenUploadIntoTheFolder(t *testing.T) {
	h := browserHarness(t, nil, "a.txt")
	src := t.TempDir()
	tree(t, src, "new.txt", "a.txt")
	h.w.Offscreen().SetChooser(func(o driver.ChooseOptions) ([]string, error) {
		if !o.Multiple {
			t.Error("the picker takes one file")
		}
		return []string{filepath.Join(src, "new.txt"), filepath.Join(src, "a.txt")}, nil
	})
	h.do(Command{Name: CmdUpload})
	h.until("the clash is asked about", func() bool { return len(h.a.ops.dialogs) == 1 })
	ask, _ := h.a.ops.dialogs[0].state.(ClashAsk)
	if ask.Name != "a.txt" {
		t.Fatalf("the dialog asks %+v", ask)
	}
	h.answer(ClashAnswered{Op: ask.Op, Choice: ChoiceKeepBoth})
	h.until("the files arrive", func() bool { return h.exists("new.txt") && h.exists("a (2).txt") })
	if got := contents(t, at(h.dir, "new.txt")); got != "new.txt" {
		t.Errorf("the upload holds %q", got)
	}
	if got := contents(t, at(h.dir, "a.txt")); got != "a.txt" {
		t.Errorf("the file kept holds %q", got)
	}
	h.until("the operation ends", func() bool { return len(h.a.ops.running) == 0 })
	left, _ := os.ReadDir(h.dir)
	for _, e := range left {
		if strings.HasSuffix(e.Name(), ".part") {
			t.Errorf("%s is left", e.Name())
		}
	}
}

func TestFilesDroppedUploadIntoTheFolderUnderThem(t *testing.T) {
	h := browserHarness(t, nil, "sub/")
	said := notices(h)
	sub := filepath.Join(h.dir, "sub")
	drag := input.Drop{Data: input.Files{}}
	spot := h.b.dnd.spot(drag, spotKey{"row", sub}, geom.Rect{}, sub, sub, true)
	if hint, _ := spot.Hint.(widget.DropHint); spot.Refused || hint.Text != "Upload to sub" || hint.Effect != widget.DropCopy {
		t.Fatalf("the spot is %+v", spot)
	}
	folder := input.NewStream("pics", "", -1, failing{input.ErrFolder})
	got := h.b.dnd.listing.OnDrop(spot, input.Drop{Data: input.Files{Streams: []*input.Stream{streamOf("b.txt", "bee"), folder}}}, nil)
	up, ok := got.(UploadDropped)
	if !ok || up.Into != sub {
		t.Fatalf("the drop sends %+v", got)
	}
	h.do(up)
	h.until("the file arrives", func() bool { return h.exists("sub/b.txt") })
	h.until("the folder is said to be left out", func() bool {
		return slices.ContainsFunc(said(), func(n Notice) bool { return strings.Contains(n.Body, "Left out pics") })
	})
	if takeDropped(up.Token) != nil {
		t.Error("the streams are still held")
	}

	// Read only, nothing goes up, and the streams are let go of.
	h.do(Command{Name: CmdHidden})
	h.a.setReadOnly(true)
	h.frames(4)
	if s := h.b.dnd.spot(drag, spotKey{"row", sub}, geom.Rect{}, sub, sub, true); !s.Refused {
		t.Error("a read-only window takes an upload")
	}
	closed := &closeCount{}
	h.do(UploadDropped{Into: sub, Token: holdDropped([]*input.Stream{input.NewStream("c.txt", "", 1, closed)})})
	if h.exists("sub/c.txt") || closed.n != 1 {
		t.Errorf("read only: uploaded %v, closed %d", h.exists("sub/c.txt"), closed.n)
	}
}

// failing is a file that fails as it is read.
type failing struct{ err error }

func (f failing) Read([]byte) (int, error) { return 0, f.err }
func (failing) Close() error               { return nil }

// closeCount counts its closes.
type closeCount struct{ n int }

func (c *closeCount) Read([]byte) (int, error) { return 0, io.EOF }
func (c *closeCount) Close() error             { c.n++; return nil }

func TestDownloadSavesAFileAsItIs(t *testing.T) {
	h := browserHarness(t, nil, "a.txt", "sub/b.txt", "sub/c/d.txt")
	out := filepath.Join(t.TempDir(), "got")
	var asked driver.SaveOptions
	h.w.Offscreen().SetSaver(func(o driver.SaveOptions) (string, error) {
		asked = o
		return out, nil
	})
	h.until("the rows arrive", func() bool { return len(h.shown()) == 2 })
	h.pick("a.txt")
	h.do(Command{Name: CmdDownload})
	h.until("the file is saved", func() bool { return len(h.a.ops.running) == 0 && contents(t, out) == "a.txt" })
	if asked.Name != "a.txt" {
		t.Errorf("the dialog was asked for %q", asked.Name)
	}

	// A folder goes as a zip of all it holds.
	_ = os.Remove(out)
	h.pick("sub")
	h.do(Command{Name: CmdDownload})
	h.until("the zip is saved", func() bool { return len(h.a.ops.running) == 0 && contents(t, out) != "missing" })
	if asked.Name != "sub.zip" {
		t.Errorf("the dialog was asked for %q", asked.Name)
	}
	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	if want := []string{"sub/", "sub/b.txt", "sub/c/", "sub/c/d.txt"}; !slices.Equal(names, want) {
		t.Errorf("the zip holds %q, want %q", names, want)
	}
}

// heldFile is a download a browser holds in memory until it is whole.
type heldFile struct {
	bytes.Buffer
	removed, closed bool
}

func (*heldFile) Name() string    { return "x" }
func (*heldFile) Place() string   { return "" }
func (*heldFile) Made() bool      { return true }
func (*heldFile) InMemory() bool  { return true }
func (f *heldFile) Close() error  { f.closed = true; return driver.ErrMayNotBeKept }
func (f *heldFile) Remove() error { f.removed = true; return nil }

// A download a browser holds in memory is refused over the most it is
// given, before a byte is read; under it, it goes, and its Close that may
// not keep it is no failure.
func TestADownloadHeldInMemoryHasAMost(t *testing.T) {
	dir := t.TempDir()
	tree(t, dir, "small.txt")
	big := filepath.Join(dir, "big.bin")
	if err := os.WriteFile(big, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(big, inMemoryMost+1); err != nil {
		t.Skip(err)
	}
	run := func(paths []string, one bool) (*heldFile, error) {
		f := &heldFile{}
		r := &runner{ctx: context.Background(), env: env{fs: LocalFS()}}
		return f, r.downloadTo(f, paths, one, driver.HeldInMemory(f))
	}
	f, err := run([]string{big}, true)
	var large *tooLarge
	if !errors.As(err, &large) || !f.removed || f.Len() != 0 {
		t.Errorf("a file too large: %v, removed %v, %d bytes written", err, f.removed, f.Len())
	}
	if f, err = run([]string{dir}, false); !errors.As(err, &large) || !f.removed {
		t.Errorf("a folder too large: %v", err)
	}
	if f, err = run([]string{filepath.Join(dir, "small.txt")}, true); err != nil || !f.closed || f.String() != "small.txt" {
		t.Errorf("a small file: %v, %q", err, f.String())
	}
}

func TestMemoryBoundsThePictures(t *testing.T) {
	if m := memoryOf(0); m != defaultMemory {
		t.Errorf("no memory set: %+v", m)
	}
	m := memoryOf(64 << 20)
	if m.thumbs != 16<<20 || m.image != 8<<20 || m.pixels != 16<<20 || m.workers != 1 {
		t.Errorf("64 MB: %+v", m)
	}
	dir := t.TempDir()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, 100, 100))); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "p.png")
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	small := memory{thumbs: 1, image: 1 << 20, pixels: 100 * 99, workers: 1}
	if _, _, err := small.decodePicture(LocalFS(), path); err == nil {
		t.Error("a picture over the most pixels is decoded")
	}
	if _, _, err := memoryOf(64<<20).decodePicture(LocalFS(), path); err != nil {
		t.Error(err)
	}
}
