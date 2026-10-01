package filemanager

import (
	"path/filepath"
	"testing"
	"time"
)

// The Recycle Bin code runs only on Windows; these test how it picks the
// item to restore, from what the bin says of each.
func TestTheRecycleBinItemMatchesByItsExtension(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	stem := filepath.Join(dir, "a")
	// Explorer hides the extensions: a.txt and a.log both show as a.
	hidden := []binItem{
		{path: stem, ext: ".txt", extKnown: true, when: at.Add(time.Second)},
		{path: stem, ext: ".log", extKnown: true, when: at.Add(3 * time.Second)},
	}
	for _, c := range []struct {
		what  string
		items []binItem
		want  string
		pick  int
	}{
		{"a.log, with a.txt closer in time", hidden, stem + ".log", 1},
		{"a.txt", hidden, stem + ".txt", 0},
		{"a, which has no extension", hidden, stem, -1},
		{"a shown with its extension", []binItem{
			{path: stem + ".log", ext: ".log", extKnown: true, when: at},
			{path: stem + ".txt", ext: ".txt", extKnown: true, when: at.Add(time.Second)},
		}, stem + ".txt", 1},
		{"a folder with a dot", []binItem{{path: stem + ".d", ext: ".d", extKnown: true, when: at}}, stem + ".d", 0},
		{"the stem, where the bin keeps no extension", []binItem{{path: stem, when: at}}, stem + ".txt", 0},
		{"a sure match before a stem", []binItem{
			{path: stem, when: at},
			{path: stem, ext: ".txt", extKnown: true, when: at.Add(time.Second)},
		}, stem + ".txt", 1},
		{"an item from before", []binItem{{path: stem + ".txt", ext: ".txt", extKnown: true, when: at.Add(-time.Minute)}},
			stem + ".txt", -1},
	} {
		if got := pickRecycled(c.items, c.want, at); got != c.pick {
			t.Errorf("%s: picked %d, want %d", c.what, got, c.pick)
		}
	}
}
