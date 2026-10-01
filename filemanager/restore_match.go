package filemanager

import (
	"path/filepath"
	"strings"
	"time"
)

// binItem is what the Recycle Bin says of an item it holds.
type binItem struct {
	// path is where the item came from, as the bin shows it.
	path string
	// ext is the extension of the file the bin keeps the item as, where
	// extKnown is set.
	ext      string
	extKnown bool
	// when is when the item went to the bin.
	when time.Time
}

// rankRecycled says how well item matches the item that went from want
// at the time at: 2 for the whole name, extension too, 1 for the name
// without its extension where the bin keeps no extension to tell by,
// and 0 for no match. The bin may show the name without its extension,
// as Explorer hides it.
func rankRecycled(item binItem, want string, at time.Time) int {
	if item.when.Before(at.Add(-2 * time.Second)) {
		return 0
	}
	if item.extKnown {
		if SystemPaths.Same(item.path+item.ext, want) ||
			SystemPaths.Same(item.path, want) && strings.EqualFold(filepath.Ext(want), item.ext) {
			return 2
		}
		return 0
	}
	switch {
	case SystemPaths.Same(item.path, want):
		return 2
	case SystemPaths.Same(item.path, strings.TrimSuffix(want, filepath.Ext(want))):
		return 1
	}
	return 0
}

// pickRecycled returns the index of the item in items that went from want
// at the time at, or -1 for none: the best match, and of those the one
// that went closest to at.
func pickRecycled(items []binItem, want string, at time.Time) int {
	best, bestRank := -1, 0
	for i, it := range items {
		r := rankRecycled(it, want, at)
		if r == 0 || r < bestRank {
			continue
		}
		if r == bestRank && it.when.Sub(at).Abs() >= items[best].when.Sub(at).Abs() {
			continue
		}
		best, bestRank = i, r
	}
	return best
}
