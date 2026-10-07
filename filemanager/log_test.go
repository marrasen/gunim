package filemanager

import (
	"slices"
	"sync"
	"testing"
)

// The failures and warnings a window shows reach Options.Log, and what
// went well does not.
func TestFailuresAndWarningsAreLogged(t *testing.T) {
	var mu sync.Mutex
	var lines []string
	h := newHarnessWith(t, func(o *Options) {
		o.Log = func(l string) {
			mu.Lock()
			lines = append(lines, l)
			mu.Unlock()
		}
	}, "a.txt")
	h.a.fail("Opening x: gone")
	h.a.patch(Notice{Title: "Stopped: copying", Body: "2 done.", Kind: "warning"})
	h.a.patch(Notice{Title: "Copied", Kind: "success"})
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(lines, []string{"files: Opening x: gone", "files: Stopped: copying: 2 done."}) {
		t.Fatalf("the log has %q", lines)
	}
}
