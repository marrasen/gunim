package filemanager

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

func entryNames(es []entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Name
	}
	return out
}

func TestListDirReadsKindsSizesAndHiddenFiles(t *testing.T) {
	root := t.TempDir()
	tree(t, root, "notes.txt", "photo.JPG", "sub/", ".secret", "Makefile")
	es, err := listDir(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]entry{}
	for _, e := range es {
		byName[e.Name] = e
	}
	if len(byName) != 5 {
		t.Fatalf("listed %v, want 5 entries", entryNames(es))
	}
	if e := byName["sub"]; e.Kind != KindFolder || !e.Dir || e.Type != "Folder" {
		t.Fatalf("sub reads as %+v", e)
	}
	if e := byName["notes.txt"]; e.Kind != KindFile || e.Size != int64(len("notes.txt")) || e.Type != "Text" {
		t.Fatalf("notes.txt reads as %+v", e)
	}
	if e := byName["photo.JPG"]; e.Type != "JPEG image" {
		t.Fatalf("photo.JPG is a %q", e.Type)
	}
	if e := byName["Makefile"]; e.Type != "File" {
		t.Fatalf("Makefile is a %q", e.Type)
	}
	if !byName[".secret"].Hidden || byName["notes.txt"].Hidden {
		t.Fatal("only the dot file should be hidden")
	}
}

func TestListDirSaysWhatItCannotRead(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	if _, err := listDir(context.Background(), missing); err == nil || !strings.Contains(err.Error(), "gone") {
		t.Fatalf("listing a missing folder returned %v", err)
	}
}

func TestListDirFollowsLinks(t *testing.T) {
	root := t.TempDir()
	tree(t, root, "target/", "file.txt")
	if err := os.Symlink(filepath.Join(root, "target"), filepath.Join(root, "to-dir")); err != nil {
		t.Skipf("this system will not make a link here: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "nowhere"), filepath.Join(root, "broken")); err != nil {
		t.Fatal(err)
	}
	es, err := listDir(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		switch e.Name {
		case "to-dir":
			if e.Kind != KindLink || !e.Dir {
				t.Fatalf("the link to a folder reads as %+v", e)
			}
		case "broken":
			if e.Kind != KindLink || !e.Broken {
				t.Fatalf("the broken link reads as %+v", e)
			}
		}
	}
}

// An entry that cannot be read shows by its name and why; the rest of the
// folder lists as ever.
func TestListDirShowsWhatItCannotReadOfAnEntry(t *testing.T) {
	root := t.TempDir()
	tree(t, root, "file.txt")
	if err := os.Symlink("loop", filepath.Join(root, "loop")); err != nil {
		t.Skipf("this system will not make a link here: %v", err)
	}
	es, err := listDir(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	got := entryNames(es)
	slices.Sort(got)
	if !slices.Equal(got, []string{"file.txt", "loop"}) {
		t.Fatalf("listed %v", got)
	}
	for _, e := range es {
		switch e.Name {
		case "loop":
			if e.Kind != KindLink || !e.Broken || e.Err == "" {
				t.Fatalf("the link to itself reads as %+v", e)
			}
		case "file.txt":
			if e.Err != "" {
				t.Fatalf("the file reads as %+v", e)
			}
		}
	}
}

// A link into a folder that cannot be read shows the read error. The test
// locks the folder with chmod, which works on Unix alone: on Windows, chmod
// only sets the read-only attribute and the folder stays readable.
func TestListDirShowsALinkIntoAFolderItCannotRead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod cannot lock a folder on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads a locked folder all the same")
	}
	root := t.TempDir()
	tree(t, root, "file.txt", "locked/inside.txt")
	if err := os.Symlink(filepath.Join(root, "locked", "inside.txt"), filepath.Join(root, "shut")); err != nil {
		t.Skipf("this system will not make a link here: %v", err)
	}
	if err := os.Chmod(filepath.Join(root, "locked"), 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Join(root, "locked"), 0o755) })
	es, err := listDir(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	got := entryNames(es)
	slices.Sort(got)
	if !slices.Equal(got, []string{"file.txt", "locked", "shut"}) {
		t.Fatalf("listed %v", got)
	}
	for _, e := range es {
		if e.Name == "shut" && (e.Kind != KindLink || !e.Broken || e.Err == "") {
			t.Fatalf("the link into a folder that cannot be read reads as %+v", e)
		}
	}
}

// failingEntry is an entry of a folder whose Lstat fails with err.
type failingEntry struct {
	fs.DirEntry
	err error
}

func (failingEntry) Name() string { return "x.txt" }
func (d failingEntry) Info() (fs.FileInfo, error) {
	return nil, &fs.PathError{Op: "lstat", Path: "x.txt", Err: d.err}
}

func TestAnEntryDeletedWhileListingIsLeftOut(t *testing.T) {
	if _, ok := readEntry(t.TempDir(), failingEntry{err: fs.ErrNotExist}); ok {
		t.Fatal("an entry deleted while the folder was listed is kept")
	}
	e, ok := readEntry(t.TempDir(), failingEntry{err: fs.ErrPermission})
	if !ok || e.Name != "x.txt" || e.Err == "" || !e.Broken {
		t.Fatalf("an entry that cannot be read reads as %+v, kept %v", e, ok)
	}
}

func TestSortingPutsFoldersFirstAndNumbersInOrder(t *testing.T) {
	now := time.Now()
	es := []entry{
		{Name: "file10.txt", Size: 5, Mod: now},
		{Name: "file2.txt", Size: 50, Mod: now.Add(-time.Hour)},
		{Name: "Beta", Dir: true},
		{Name: "alpha", Dir: true},
		{Name: "File1.txt", Size: 500, Mod: now.Add(time.Hour)},
	}
	for i := range es {
		es[i].lower = strings.ToLower(es[i].Name)
	}
	sortEntries(es, SortName, false)
	if got, want := entryNames(es), []string{"alpha", "Beta", "File1.txt", "file2.txt", "file10.txt"}; !slices.Equal(got, want) {
		t.Fatalf("by name: %v, want %v", got, want)
	}
	sortEntries(es, SortSize, true)
	if got, want := entryNames(es), []string{"Beta", "alpha", "File1.txt", "file2.txt", "file10.txt"}; !slices.Equal(got, want) {
		t.Fatalf("by size, largest first: %v, want %v", got, want)
	}
	sortEntries(es, SortModified, false)
	if got, want := entryNames(es), []string{"alpha", "Beta", "file2.txt", "file10.txt", "File1.txt"}; !slices.Equal(got, want) {
		t.Fatalf("by time: %v, want %v", got, want)
	}
}

func TestNaturalCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{
		{"a2", "a10", -1}, {"a10", "a2", 1}, {"a02", "a2", 1}, {"a", "a1", -1}, {"x9y", "x9z", -1}, {"same", "same", 0},
	} {
		if got := naturalCompare(c.a, c.b); got != c.want {
			t.Fatalf("naturalCompare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestFilterMatchesPartsOfNamesAndHidesDotFiles(t *testing.T) {
	es := []entry{{Name: "Report.pdf"}, {Name: "report-old.pdf"}, {Name: ".report", Hidden: true}, {Name: "notes"}}
	for i := range es {
		es[i].lower = strings.ToLower(es[i].Name)
	}
	if got := entryNames(filterEntries(es, "REPORT", false)); !slices.Equal(got, []string{"Report.pdf", "report-old.pdf"}) {
		t.Fatalf("filtering for REPORT kept %v", got)
	}
	if got := entryNames(filterEntries(es, "report", true)); len(got) != 3 {
		t.Fatalf("with hidden files shown, the filter kept %v", got)
	}
}
