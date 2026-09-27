//go:build windows && (amd64 || arm64)

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestTheRecycleBinGivesBackATempFile trashes a file made for the test to
// the real Recycle Bin, and takes it back out. It runs only with
// GUNIM_FILES_RECYCLE_TEST=1.
func TestTheRecycleBinGivesBackATempFile(t *testing.T) {
	if os.Getenv("GUNIM_FILES_RECYCLE_TEST") != "1" {
		t.Skip("set GUNIM_FILES_RECYCLE_TEST=1 to use the real Recycle Bin")
	}
	root := t.TempDir()
	path := filepath.Join(root, "gunim-recycle-test.txt")
	if err := os.WriteFile(path, []byte("come back"), 0o644); err != nil {
		t.Fatal(err)
	}
	tr := recycleBin{}
	rec, err := runJob(context.Background(), job{kind: OpTrash, srcs: []string{path}}, env{trash: tr})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("the file is still there after the trash: %v", err)
	}
	if !restorable(rec) {
		t.Fatal("the record of the trash says it cannot be restored")
	}
	if _, err := runJob(context.Background(), job{kind: OpUndo, undo: &rec}, env{trash: tr}); err != nil {
		t.Fatal(err)
	}
	if got := contents(t, path); got != "come back" {
		t.Fatalf("after the restore the file holds %q", got)
	}
}
