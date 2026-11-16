package filemanager

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// A folder copied into a setgid folder keeps the setgid bit it takes
// from there, so what goes in it later keeps the folder's group: on the
// computer's own files, and where the folder is made open to all to
// read, so the copy narrows its mode.
func TestAFolderCopiedIntoASetgidFolderStaysSetgid(t *testing.T) {
	for _, c := range []struct {
		name  string
		mkdir fs.FileMode
	}{{"as asked", 0}, {"open to all", 0o755 | fs.ModeSetgid}} {
		t.Run(c.name, func(t *testing.T) {
			e := &endings{}
			h := newHarnessWith(t, func(o *Options) {
				o.FS = refusingFS{FS: LocalFS(), mkdirMode: c.mkdir}
				o.Ended = e.ended
			}, "src/a.txt", "web/")
			if err := os.Chmod(filepath.Join(h.dir, "web"), 0o755|fs.ModeSetgid); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(filepath.Join(h.dir, "src"), 0o700); err != nil {
				t.Fatal(err)
			}
			got := copyFolder(t, h, e)
			if got&fs.ModeSetgid == 0 || got.Perm() != 0o700 {
				t.Fatalf("the copy's mode is %v, want it setgid and open to its owner alone", got)
			}
		})
	}
}
