package filemanager

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/marrasen/gunim/input"
)

// walk indexes root within b, reading folders with read, and returns
// what the walk found.
func walk(t *testing.T, root string, b bounds, read func(string) ([]os.DirEntry, error)) ([]indexed, []walkError, string) {
	t.Helper()
	var items []indexed
	var errs []walkError
	stopped, done := "", false
	walkIndex(context.Background(), SystemPaths, root, b, read, func(is []indexed, es []walkError, s string, d bool) {
		items, errs = append(items, is...), append(errs, es...)
		stopped, done = s, d
	})
	if !done {
		t.Fatal("the walk never said it was done")
	}
	return items, errs, stopped
}

func TestTheWalkIndexesEverythingShallowFirst(t *testing.T) {
	root := t.TempDir()
	tree(t, root, "a/b/deep.txt", "top.txt", "a/mid.txt")
	items, errs, stopped := walk(t, root, bounds{items: 100, time: time.Minute}, os.ReadDir)
	rels := make([]string, 0, len(items))
	for _, it := range items {
		rels = append(rels, filepath.ToSlash(it.rel))
	}
	want := []string{"a", "top.txt", "a/b", "a/mid.txt", "a/b/deep.txt"}
	if !slices.Equal(rels, want) || len(errs) != 0 || stopped != "" {
		t.Fatalf("the walk found %v with errors %v, stopped %q; want %v", rels, errs, stopped, want)
	}
}

func TestTheWalkStopsAtItsBoundsAndSaysSo(t *testing.T) {
	root := t.TempDir()
	tree(t, root, "a/1", "a/2", "b/3", "b/4", "c/5")
	items, _, stopped := walk(t, root, bounds{items: 4, time: time.Minute}, os.ReadDir)
	if len(items) != 4 || !strings.Contains(stopped, "4 items") {
		t.Fatalf("with room for 4 items the walk found %d and said %q", len(items), stopped)
	}
	slow := func(dir string) ([]os.DirEntry, error) {
		time.Sleep(20 * time.Millisecond)
		return os.ReadDir(dir)
	}
	items, _, stopped = walk(t, root, bounds{items: 100, time: 30 * time.Millisecond}, slow)
	if len(items) >= 8 || !strings.Contains(stopped, "Stopped after") {
		t.Fatalf("with 30 ms to walk the walk found %d items and said %q", len(items), stopped)
	}
}

func TestAFolderTheWalkCannotReadIsListed(t *testing.T) {
	root := t.TempDir()
	tree(t, root, "open/a.txt", "locked/b.txt")
	denied := errors.New("access is denied")
	read := func(dir string) ([]os.DirEntry, error) {
		if filepath.Base(dir) == "locked" {
			return nil, denied
		}
		return os.ReadDir(dir)
	}
	items, errs, _ := walk(t, root, bounds{items: 100, time: time.Minute}, read)
	if len(errs) != 1 || errs[0].rel != "locked" || !errors.Is(errs[0].err, denied) {
		t.Fatalf("the walk reported %v", errs)
	}
	hits := rankIndex(context.Background(), SystemPaths, root, items, errs, "")
	last := hits[len(hits)-1]
	if !last.Problem || last.Title != "Could not read locked" || last.Detail != "access is denied" {
		t.Fatalf("the palette shows the folder as %+v", last)
	}
}

func TestThePaletteRanksNamesAndShortPathsFirst(t *testing.T) {
	root := t.TempDir()
	tree(t, root, "reports/q1.txt", "deep/down/report.txt", "report.txt", "old/report-2019.txt", "repo/x.txt")
	items, _, _ := walk(t, root, bounds{items: 100, time: time.Minute}, os.ReadDir)
	hits := rankIndex(context.Background(), SystemPaths, root, items, nil, "report")
	got := make([]string, 0, len(hits))
	for _, h := range hits {
		got = append(got, h.Title+" in "+filepath.ToSlash(h.Detail))
	}
	base := filepath.Base(root)
	want := []string{
		// The whole word first, and then the shallower.
		"report.txt in " + base,
		"report-2019.txt in " + base + "/old",
		"report.txt in " + base + "/deep/down",
		"reports in " + base,
		// Found in the path alone, after every name.
		"q1.txt in " + base + "/reports",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("report ranks\n%v\nwant\n%v", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if !slices.Equal(hits[0].At, []int{0, 1, 2, 3, 4, 5}) || hits[4].At != nil {
		t.Fatalf("the marks are %v and %v", hits[0].At, hits[4].At)
	}
}

func TestAGreaterThanSignFindsCommands(t *testing.T) {
	hits := rankCommands("hidden", []Place{{Name: "Home", Path: "/home/me"}}, nil, "")
	if len(hits) == 0 || hits[0].Key != "cmd:"+CmdHidden || hits[0].Hint != "Ctrl+H" {
		t.Fatalf("hidden finds %+v first", hits)
	}
	hits = rankCommands("go home", []Place{{Name: "Home", Path: "/home/me"}}, nil, "")
	if len(hits) == 0 || hits[0].Key != "go:/home/me" {
		t.Fatalf("go home finds %+v first", hits)
	}
}

func TestThePaletteGoesToAFileAndSelectsIt(t *testing.T) {
	h := newHarness(t, "top.txt", "sub/inner/needle.txt")
	h.w.Input(input.KeyPress{Key: input.KeyP, Mods: input.ModControl})
	h.frames(5)
	if !h.b.palette.p.IsOpen() {
		t.Fatal("Ctrl+P did not open the palette")
	}
	h.w.Input(input.TextInput{Text: "needle"})
	h.until("the palette finds the file", func() bool {
		return len(h.b.palette.hits) > 0 && h.b.palette.hits[0].Title == "needle.txt"
	})
	if !strings.Contains(h.b.palette.p.Status, "items under dir") {
		t.Fatalf("the palette's status says %q", h.b.palette.p.Status)
	}
	h.w.Input(input.KeyPress{Key: input.KeyEnter})
	h.until("the file's folder opens with it selected", func() bool {
		return SystemPaths.Same(h.a.nav.path, filepath.Join(h.dir, "sub", "inner")) && h.a.nav.sel["needle.txt"] &&
			slices.Equal(h.shown(), []string{"needle.txt"})
	})
}

func TestThePaletteRunsACommand(t *testing.T) {
	h := newHarness(t, "a.txt", ".hidden")
	h.until("the rows arrive", func() bool { return len(h.shown()) == 1 })
	if err := h.w.Client().Patch(string(browserID), OpenPalette{Query: ">show hidden"}); err != nil {
		t.Fatal(err)
	}
	h.until("the command is found", func() bool {
		return len(h.b.palette.hits) > 0 && h.b.palette.hits[0].Key == "cmd:"+CmdHidden
	})
	h.w.Input(input.KeyPress{Key: input.KeyEnter})
	h.until("the hidden file shows", func() bool { return len(h.shown()) == 2 })
}

// Lower case can change a path's length: a Kelvin sign shrinks from
// three bytes to one, and a dotted capital I grows. A file under such a
// folder is still found by its name, and the ranking never reads its
// name at the wrong place.
func TestANameIsFoundUnderAFolderWhoseLowerCaseChangesLength(t *testing.T) {
	root := t.TempDir()
	tree(t, root, "KKK/alpha.txt", "İİİİ/beta.txt")
	items, _, _ := walk(t, root, bounds{items: 100, time: time.Minute}, os.ReadDir)
	for _, q := range []string{"alpha", "beta"} {
		hits := rankIndex(context.Background(), SystemPaths, root, items, nil, q)
		if len(hits) == 0 || hits[0].Title != q+".txt" || hits[0].At == nil {
			t.Fatalf("%s found %+v, want %s.txt found by its name", q, hits, q)
		}
	}
}
