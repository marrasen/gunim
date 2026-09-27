package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTheSystemTrashRestoresOverANewFile(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	tr, err := systemTrash()
	if err != nil {
		t.Fatal(err)
	}
	rec := trashAndRefill(t, root, tr)
	info := contents(t, filepath.Join(root, "data", "Trash", "info", "a.txt.trashinfo"))
	if !strings.Contains(info, "Path="+at(root, "work/a.txt")+"\n") || !strings.Contains(info, "DeletionDate=") {
		t.Fatalf("the record reads %q", info)
	}
	undoWith(t, rec, tr, choiceKeepBoth)
	if got := names(t, at(root, "work")); !slices.Equal(got, []string{"a (2).txt", "a.txt"}) {
		t.Fatalf("after the restore the folder holds %v", got)
	}
	if es, err := os.ReadDir(filepath.Join(root, "data", "Trash", "info")); err != nil || len(es) != 0 {
		t.Fatalf("the restore left %d records, or %v", len(es), err)
	}
}
