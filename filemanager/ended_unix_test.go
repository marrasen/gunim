//go:build unix

package filemanager

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// copyFolder copies the folder src of the window's folder into its
// folder web, on the computer's own files, and returns the copy's mode.
func copyFolder(t *testing.T, h *harness, e *endings) fs.FileMode {
	t.Helper()
	h.a.post(func() {
		h.a.startOp(job{kind: OpCopy, srcs: []string{filepath.Join(h.dir, "src")}, dest: filepath.Join(h.dir, "web")}, "Copying src")
	})
	h.until("the copy ends", func() bool { return len(e.all()) == 1 })
	if end := e.all()[0]; end.Err != nil || len(end.Warnings) != 0 {
		t.Fatalf("the copy ended as %+v", end)
	}
	info, err := os.Lstat(filepath.Join(h.dir, "web", "src"))
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode()
}

// A folder whose original was open to all to write is copied as the
// umask allows, as a new folder is made, not open to all.
func TestAFolderCopiedKeepsTheUmask(t *testing.T) {
	e := &endings{}
	h := newHarnessWith(t, func(o *Options) { o.Ended = e.ended }, "src/a.txt", "web/")
	probe := filepath.Join(h.root, "probe")
	if err := os.Mkdir(probe, 0o777); err != nil {
		t.Fatal(err)
	}
	made, err := os.Lstat(probe)
	if err != nil {
		t.Fatal(err)
	}
	if made.Mode().Perm() == 0o777 {
		t.Skip("the umask takes nothing away")
	}
	if err := os.Chmod(filepath.Join(h.dir, "src"), 0o777); err != nil {
		t.Fatal(err)
	}
	if got := copyFolder(t, h, e).Perm(); got != made.Mode().Perm() {
		t.Fatalf("the copy's mode is %v, want %v as a new folder gets", got, made.Mode().Perm())
	}
}
