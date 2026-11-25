package outstream

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
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
	k.heard(t, "open finish")
}

func TestAnEmptyFileIsOpenedToo(t *testing.T) {
	k := &kept{}
	if err := New(k.target()).Close(); err != nil {
		t.Fatal(err)
	}
	k.heard(t, "open finish")
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
