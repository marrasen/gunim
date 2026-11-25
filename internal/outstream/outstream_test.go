package outstream

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim/driver"
)

// kept is a Target that keeps what it hears.
type kept struct {
	bytes.Buffer
	log     []string
	openErr error
	write   func(p []byte) (int, error)
	finish  error
	commit  error
	remove  error
}

func (k *kept) Write(p []byte) (int, error) {
	if k.write != nil {
		return k.write(p)
	}
	return k.Buffer.Write(p)
}

func (k *kept) target() Target {
	say := func(s string) { k.log = append(k.log, s) }
	return Target{
		Open:    func() (io.Writer, error) { say("open"); return k, k.openErr },
		Finish:  func(int64) error { say("finish"); return k.finish },
		Commit:  func() error { say("commit"); return k.commit },
		Abandon: func() { say("abandon") },
		Remove:  func() error { say("remove"); return k.remove },
	}
}

func (k *kept) heard(t *testing.T, want string) {
	t.Helper()
	if got := strings.Join(k.log, " "); got != want {
		t.Errorf("heard %q, want %q", got, want)
	}
}

func within(t *testing.T, what string, do func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		do()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("%s still waits", what)
	}
}

func TestAFileIsWrittenWholeAndFinishedOnce(t *testing.T) {
	k := &kept{}
	w := New(k.target())
	data := bytes.Repeat([]byte("0123456789"), 100_000)
	if n, err := io.Copy(w, bytes.NewReader(data)); err != nil || n != int64(len(data)) {
		t.Fatal(n, err)
	}
	for range 2 {
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(k.Bytes(), data) {
		t.Error("the bytes differ")
	}
	if err := w.Remove(); err == nil {
		t.Error("a whole file was taken away")
	}
	if _, err := w.Write([]byte("x")); err == nil {
		t.Error("a whole file took more")
	}
	k.heard(t, "open finish commit")
}

func TestAnEmptyFileIsOpenedToo(t *testing.T) {
	k := &kept{}
	if err := New(k.target()).Close(); err != nil {
		t.Fatal(err)
	}
	k.heard(t, "open finish commit")
}

func TestAnErrorLeavesTheFileNotWhole(t *testing.T) {
	boom := errors.New("boom")
	for name, c := range map[string]struct {
		k    *kept
		want string
	}{
		"opening":      {&kept{openErr: boom}, "open abandon remove"},
		"writing":      {&kept{write: func([]byte) (int, error) { return 0, boom }}, "open abandon remove"},
		"short writes": {&kept{write: func(p []byte) (int, error) { return len(p) - 1, nil }}, "open abandon remove"},
	} {
		w := New(c.k.target())
		if _, err := w.Write([]byte("x")); err == nil {
			t.Errorf("%s: no error", name)
		}
		if err := w.Close(); err == nil {
			t.Errorf("%s: it closes as whole", name)
		}
		for range 2 {
			if err := w.Remove(); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		}
		c.k.heard(t, c.want)
	}
	k := &kept{finish: boom, remove: boom}
	w := New(k.target())
	if err := w.Close(); !errors.Is(err, boom) {
		t.Errorf("finishing: %v", err)
	}
	if err := w.Remove(); !errors.Is(err, boom) {
		t.Errorf("removing: %v", err)
	}
	k.heard(t, "open finish abandon remove")
}

func TestRemoveEndsAWriteThatWaits(t *testing.T) {
	began, free := make(chan struct{}), make(chan struct{})
	k := &kept{write: func(p []byte) (int, error) {
		close(began)
		<-free
		return len(p), nil
	}}
	w := New(k.target())
	got := make(chan error, 1)
	p := []byte("mine")
	go func() {
		_, err := w.Write(p)
		got <- err
	}()
	<-began
	within(t, "Remove", func() {
		if err := w.Remove(); err != nil {
			t.Error(err)
		}
	})
	within(t, "Write", func() {
		if err := <-got; err == nil {
			t.Error("the write passed")
		}
	})
	if err := w.Close(); err == nil {
		t.Error("a file taken away closes as whole")
	}
	close(free)
	k.heard(t, "open abandon remove")
}

func TestRemoveEndsACloseThatWaits(t *testing.T) {
	began, free := make(chan struct{}), make(chan struct{})
	defer close(free)
	tg := (&kept{}).target()
	tg.Finish = func(int64) error {
		close(began)
		<-free
		return nil
	}
	w := New(tg)
	got := make(chan error, 1)
	go func() { got <- w.Close() }()
	<-began
	within(t, "Remove", func() { _ = w.Remove() })
	within(t, "Close", func() {
		if err := <-got; err == nil {
			t.Error("it closes as whole")
		}
	})
}

// Remove comes too late once the file is being kept, and does nothing: the
// file is whole, or Close says it is not, and then Remove takes it away.
func TestRemoveDoesNothingWhileTheFileIsKept(t *testing.T) {
	for _, fails := range []error{nil, errors.New("boom")} {
		began, free := make(chan struct{}), make(chan struct{})
		k := &kept{}
		tg := k.target()
		tg.Commit = func() error {
			close(began)
			<-free
			return fails
		}
		w := New(tg)
		got := make(chan error, 1)
		go func() { got <- w.Close() }()
		<-began
		within(t, "Remove", func() {
			if err := w.Remove(); err == nil {
				t.Error("it was taken away as it was kept")
			}
		})
		close(free)
		if err := <-got; !errors.Is(err, fails) {
			t.Errorf("Close: %v, want %v", err, fails)
		}
		err := w.Remove()
		if (err == nil) != (fails != nil) {
			t.Errorf("Remove after Close gave %v: %v", fails, err)
		}
	}
}

// A file that may not be kept is all there, and its program says what will
// do: it leaves it, or takes it away.
func TestAFileThatMayNotBeKeptIsTheProgramsToJudge(t *testing.T) {
	k := &kept{finish: driver.ErrMayNotBeKept}
	w := New(k.target())
	for range 2 {
		if err := w.Close(); !errors.Is(err, driver.ErrMayNotBeKept) {
			t.Errorf("%v", err)
		}
	}
	k.heard(t, "open finish commit")
	if err := w.Remove(); err != nil {
		t.Error(err)
	}
	k.heard(t, "open finish commit abandon remove")
}

func TestAFileThatMayNotBeKeptIsLeftAtNoCost(t *testing.T) {
	before := runtime.NumGoroutine()
	for range 50 {
		w := New((&kept{finish: driver.ErrMayNotBeKept}).target())
		if _, err := w.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); !errors.Is(err, driver.ErrMayNotBeKept) {
			t.Fatal(err)
		}
		if w.buf != nil {
			t.Fatal("it keeps its buffer")
		}
	}
	for range 100 {
		if runtime.NumGoroutine() <= before+5 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Errorf("%d goroutines, %d before", runtime.NumGoroutine(), before)
}

// list returns the names in dir.
func list(t *testing.T, dir string) string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			names = append(names, filepath.ToSlash(rel))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(names, " ")
}

func put(t *testing.T, f driver.SavedFile, text string) {
	t.Helper()
	if _, err := io.WriteString(f, text); err != nil {
		t.Fatal(err)
	}
}

func TestAFileOnTheDiskIsThereOnceItIsWhole(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	f := Save(p)
	if f.Made() || f.Name() != "a.txt" {
		t.Errorf("made %v, name %q", f.Made(), f.Name())
	}
	put(t, f, "new")
	if b, _ := os.ReadFile(p); string(b) != "old" {
		t.Errorf("it holds %q before it is whole", b)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "new" || list(t, dir) != "a.txt" {
		t.Errorf("it holds %q, in %s", b, list(t, dir))
	}
	// One taken away leaves what was there.
	f = Save(p)
	put(t, f, "newer")
	if err := f.Remove(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "new" || list(t, dir) != "a.txt" {
		t.Errorf("it holds %q, in %s", b, list(t, dir))
	}
}

func TestALaterRunTakesAwayAPartAndNothingElse(t *testing.T) {
	dir := t.TempDir()
	place := Save(filepath.Join(dir, "a.txt")).Place()
	if err := os.WriteFile(place, []byte("half"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemoveSaved(place); err != nil {
		t.Fatal(err)
	}
	if list(t, dir) != "" {
		t.Errorf("there is %s", list(t, dir))
	}
	if err := RemoveSaved(place); err != nil {
		t.Errorf("one that is gone: %v", err)
	}
	mine := filepath.Join(dir, "mine.txt")
	if err := os.WriteFile(mine, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{mine, "", "a" + mark, dir} {
		if err := RemoveSaved(p); !errors.Is(err, driver.ErrNotMade) {
			t.Errorf("%q: %v", p, err)
		}
	}
}

func TestRemoveAsTheFileOpensLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	opening, free := make(chan struct{}), make(chan struct{})
	place := filepath.Join(dir, part())
	fi := files{
		create: func(name string) (*os.File, error) {
			close(opening)
			<-free
			return os.Create(name)
		},
		rename: os.Rename, remove: os.Remove,
	}
	w := New(fi.target(place, filepath.Join(dir, "a.txt"), nil))
	go func() { _, _ = io.WriteString(w, "secret") }()
	<-opening
	if err := w.Remove(); err != nil {
		t.Fatal(err)
	}
	close(free)
	time.Sleep(100 * time.Millisecond)
	if got := list(t, dir); got != "" {
		t.Errorf("there is %s", got)
	}
}

func TestANewFileIsAsTheOldOneWas(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no such permissions")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(p, link); err != nil {
		t.Fatal(err)
	}
	f := Save(link)
	put(t, f, "new")
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("%v, %v", st.Mode(), err)
	}
	if to, err := os.Readlink(link); err != nil || to != p {
		t.Errorf("the link leads to %q, %v", to, err)
	}
	if b, _ := os.ReadFile(p); string(b) != "new" {
		t.Errorf("it holds %q", b)
	}
}

func TestALinkToNoFileYetStays(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "in"), 0o700); err != nil {
		t.Fatal(err)
	}
	first, second := filepath.Join(dir, "first"), filepath.Join(dir, "in", "second")
	if err := errors.Join(os.Symlink(filepath.Join("in", "second"), first), os.Symlink("a.txt", second)); err != nil {
		t.Skip(err)
	}
	f := Save(first)
	if !f.Made() {
		t.Error("it was there")
	}
	put(t, f, "new")
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "in", "a.txt")); err != nil || string(b) != "new" {
		t.Errorf("%q, %v", b, err)
	}
	for _, l := range []string{first, second} {
		if _, err := os.Readlink(l); err != nil {
			t.Error(err)
		}
	}
	// Links that lead round and round are no place to write.
	a, b := filepath.Join(dir, "a"), filepath.Join(dir, "b")
	if err := errors.Join(os.Symlink(b, a), os.Symlink(a, b)); err != nil {
		t.Fatal(err)
	}
	f = Save(a)
	if _, err := io.WriteString(f, "x"); err == nil {
		t.Error("it was written")
	}
	_ = f.Remove()
	if _, err := os.Readlink(a); err != nil {
		t.Error(err)
	}
}

func TestAFilesNameMayBeAsLongAsANameCanBe(t *testing.T) {
	dir := t.TempDir()
	name := strings.Repeat("a", 250)
	fo, err := Folder(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fo.Close() }()
	in, err := fo.Create(name + ".x")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []driver.SavedFile{Save(filepath.Join(dir, name)), in} {
		put(t, f, "x")
		if err := f.Close(); err != nil {
			t.Error(err)
		}
	}
	if got, want := list(t, dir), name+" "+name+".x"; got != want {
		t.Errorf("there is %s", got)
	}
}

func TestAFolderIsWrittenInAndNothingIsWrittenOver(t *testing.T) {
	dir := t.TempDir()
	fo, err := Folder(dir)
	if err != nil {
		t.Fatal(err)
	}
	f, err := fo.Create("a/b/c.txt")
	if err != nil {
		t.Fatal(err)
	}
	put(t, f, "c")
	if got := list(t, dir); strings.HasSuffix(got, "c.txt") || !strings.Contains(got, mark) {
		t.Errorf("before it is whole: %s", got)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if got := list(t, dir); got != "a/b/c.txt" {
		t.Errorf("once it is whole: %s", got)
	}
	if _, err := fo.Create("a/b/c.txt"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("over a file: %v", err)
	}
	if _, err := fo.Create("a/b"); !errors.Is(err, fs.ErrExist) {
		t.Errorf("over a folder: %v", err)
	}
	// One that comes as another is written keeps its own.
	f, err = fo.Create("d.txt")
	if err != nil {
		t.Fatal(err)
	}
	put(t, f, "d")
	if err := os.WriteFile(filepath.Join(dir, "d.txt"), []byte("theirs"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); !errors.Is(err, fs.ErrExist) {
		t.Errorf("over one that came: %v", err)
	}
	if err := f.Remove(); err != nil {
		t.Error(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "d.txt")); string(b) != "theirs" || list(t, dir) != "a/b/c.txt d.txt" {
		t.Errorf("%q, in %s", b, list(t, dir))
	}
	for range 2 {
		if err := fo.Mkdir("e/f"); err != nil {
			t.Error(err)
		}
	}
	for _, p := range []string{"", ".", "..", "../x", "/x", "a/../x", "a//b", "a/", "a\x00b", `a\b`} {
		if _, err := fo.Create(p); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("create %q: %v", p, err)
		}
		if err := fo.Mkdir(p); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("mkdir %q: %v", p, err)
		}
	}
	if err := fo.Close(); err != nil {
		t.Error(err)
	}
}

func TestAFolderIsNotLeftThroughALink(t *testing.T) {
	dir, out := t.TempDir(), t.TempDir()
	if err := os.Symlink(out, filepath.Join(dir, "out")); err != nil {
		t.Skip(err)
	}
	fo, err := Folder(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = fo.Close() }()
	if f, err := fo.Create("out/x.txt"); err == nil {
		if _, err := io.WriteString(f, "x"); err == nil {
			t.Error("it was written")
		}
		_ = f.Remove()
	}
	if got := list(t, out); got != "" {
		t.Errorf("outside there is %s", got)
	}
}
