package filemanager

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim/input"
)

// trashAndRefill trashes a.txt in root, puts a new a.txt in its place,
// and returns the record of the trash.
func trashAndRefill(t *testing.T, root string, tr Trasher) record {
	t.Helper()
	tree(t, root, "work/a.txt")
	rec, err := runJob(context.Background(), job{kind: OpTrash, srcs: []string{at(root, "work/a.txt")}}, env{fs: LocalFS(), trash: tr})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(at(root, "work/a.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	return rec
}

// undoWith undoes rec, answering a clash with c, and returns how many
// clashes were asked about.
func undoWith(t *testing.T, rec record, tr Trasher, c choice) int {
	t.Helper()
	asked := 0
	e := env{fs: LocalFS(), trash: tr, ask: func(_ context.Context, cl clash) (answer, error) {
		asked++
		if !strings.HasPrefix(cl.from, "in the trash, ") || !strings.HasSuffix(cl.dst, "a.txt") {
			t.Fatalf("the clash says the arriving item is %q, going to %s", cl.from, cl.dst)
		}
		return answer{choice: c}, nil
	}}
	if _, err := runJob(context.Background(), job{kind: OpUndo, undo: &rec}, e); err != nil {
		t.Fatal(err)
	}
	return asked
}

func TestRestoringOverANewItemCanKeepBoth(t *testing.T) {
	root := t.TempDir()
	tr := xdgTrash{dir: filepath.Join(root, "Trash")}
	rec := trashAndRefill(t, root, tr)
	if n := undoWith(t, rec, tr, choiceKeepBoth); n != 1 {
		t.Fatalf("the restore asked %d times, want once", n)
	}
	if contents(t, at(root, "work/a.txt")) != "new" || contents(t, at(root, "work/a (2).txt")) != "work/a.txt" {
		t.Fatal("keeping both did not restore beside the new file")
	}
	if got := names(t, filepath.Join(root, "Trash", "info")); len(got) != 0 {
		t.Fatalf("the restore left the records %v", got)
	}
}

func TestRestoringOverANewItemCanReplaceIt(t *testing.T) {
	root := t.TempDir()
	tr := xdgTrash{dir: filepath.Join(root, "Trash")}
	rec := trashAndRefill(t, root, tr)
	undoWith(t, rec, tr, choiceReplace)
	if contents(t, at(root, "work/a.txt")) != "work/a.txt" {
		t.Fatal("replacing did not restore the trashed file")
	}
	// The new file went to the trash rather than away for good.
	if got := names(t, filepath.Join(root, "Trash", "files")); !slices.Equal(got, []string{"a (2).txt"}) ||
		contents(t, filepath.Join(root, "Trash", "files", "a (2).txt")) != "new" {
		t.Fatalf("the trash holds %v", got)
	}
}

func TestRestoringOverANewItemCanSkip(t *testing.T) {
	root := t.TempDir()
	tr := xdgTrash{dir: filepath.Join(root, "Trash")}
	rec := trashAndRefill(t, root, tr)
	undoWith(t, rec, tr, choiceSkip)
	if contents(t, at(root, "work/a.txt")) != "new" || contents(t, filepath.Join(root, "Trash", "files", "a.txt")) != "work/a.txt" {
		t.Fatal("skipping changed a file")
	}
}

func TestRestoreFollowsTheTrashRecord(t *testing.T) {
	root := t.TempDir()
	tr := xdgTrash{dir: filepath.Join(root, "Trash")}
	tree(t, root, "a.txt")
	to, err := tr.Trash(at(root, "a.txt"))
	if err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(root, "Trash", "info", "a.txt.trashinfo")
	if werr := os.WriteFile(record, []byte("[Trash Info]\nPath=/somewhere/else.txt\n"), 0o600); werr != nil {
		t.Fatal(werr)
	}
	err = tr.Restore(at(root, "a.txt"), to, time.Time{}, at(root, "a.txt"))
	if err == nil || !strings.Contains(err.Error(), "came from") {
		t.Fatalf("restoring against a record from elsewhere returned %v", err)
	}
	if err := os.Remove(record); err != nil {
		t.Fatal(err)
	}
	if err := tr.Restore(at(root, "a.txt"), to, time.Time{}, at(root, "a.txt")); err == nil {
		t.Fatal("restoring with no record worked")
	}
	if contents(t, to) != "a.txt" {
		t.Fatal("a refused restore moved the trashed file")
	}
}

func TestUndoAsksBeforeRestoringOverANewFile(t *testing.T) {
	h := newHarness(t, "keep.txt", "gone.txt")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 2 })
	h.pick("gone.txt")
	h.w.Input(input.KeyPress{Key: input.KeyDelete})
	h.until("the file goes to the trash", func() bool { return !h.exists("gone.txt") && len(h.a.ops.undo) == 1 })
	tree(t, h.dir, "gone.txt")
	// Ctrl+Shift+Z is another shortcut, and undoes nothing.
	h.w.Input(input.KeyPress{Key: input.KeyZ, Mods: input.ModControl | input.ModShift})
	h.frames(10)
	if len(h.a.ops.dialogs) != 0 || len(h.a.ops.undo) != 1 {
		t.Fatalf("Ctrl+Shift+Z asked %d questions and left %d to undo, want none and 1", len(h.a.ops.dialogs), len(h.a.ops.undo))
	}
	h.w.Input(input.KeyPress{Key: input.KeyZ, Mods: input.ModControl})
	h.until("the clash is asked about", func() bool { return len(h.a.ops.dialogs) == 1 })
	ask, ok := h.a.ops.dialogs[0].state.(ClashAsk)
	if !ok || ask.Name != "gone.txt" || !strings.HasPrefix(ask.New, "in the trash") {
		t.Fatalf("the dialog asks %+v", h.a.ops.dialogs[0].state)
	}
	h.answer(ClashAnswered{Op: ask.Op, Choice: ChoiceKeepBoth})
	h.until("both show", func() bool {
		return slices.Equal(h.shown(), []string{"gone (2).txt", "gone.txt", "keep.txt"})
	})
}
