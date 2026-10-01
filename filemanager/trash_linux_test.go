package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTheSystemTrashFollowsXDGDataHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	tr, err := systemTrash()
	if err != nil {
		t.Fatal(err)
	}
	tree(t, root, "work/a.txt")
	rec, err := runJob(context.Background(), job{kind: OpTrash, srcs: []string{at(root, "work/a.txt")}}, env{trash: tr})
	if err != nil {
		t.Fatal(err)
	}
	trash := filepath.Join(root, "data", "Trash")
	if got := names(t, filepath.Join(trash, "files")); !slices.Equal(got, []string{"a.txt"}) {
		t.Fatalf("the trash holds %v", got)
	}
	info := contents(t, filepath.Join(trash, "info", "a.txt.trashinfo"))
	if !strings.Contains(info, "Path="+at(root, "work/a.txt")+"\n") {
		t.Fatalf("the record reads %q", info)
	}
	if _, err := os.Stat(at(root, "work/a.txt")); err == nil {
		t.Fatal("the file is still where it was")
	}
	if _, err := runJob(context.Background(), job{kind: OpUndo, undo: &rec}, env{trash: tr}); err != nil {
		t.Fatal(err)
	}
	if contents(t, at(root, "work/a.txt")) != "work/a.txt" {
		t.Fatal("undo did not bring the file back")
	}
}
