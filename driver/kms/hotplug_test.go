//go:build linux && (arm64 || amd64)

package kms

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// A keyboard plugged in after Open is read from then on: one there before
// the watch began, one whose node opens only once udev gives it its group,
// and one that goes away and comes back. A device that isn't a keyboard, and
// one Open read already, are left alone.
func TestKeyWatcher_FindsKeyboardsPluggedInLater(t *testing.T) {
	dir := t.TempDir()
	// A node appears whole, as the kernel makes a device's node: written under another name, then
	// renamed into place. Written in place, the watcher could open it between the create and the
	// write, and find it empty.
	put := func(name, what string, mode os.FileMode) {
		t.Helper()
		tmp := filepath.Join(dir, "new-"+name)
		if err := os.WriteFile(tmp, []byte(what), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(tmp, mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	// What the fake devices are: a file's bytes say keyboard or not.
	open := func(p string) (*inputDev, error) {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		return &inputDev{fd: -1, path: p, name: string(b), keys: string(b) == "keyboard"}, nil
	}
	put("event0", "keyboard", 0o644) // read by Open
	put("event1", "keyboard", 0o644) // plugged in before the watch began
	stop, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Close(stop) }()
	var readers sync.WaitGroup
	run := func(fn func()) {
		readers.Add(1)
		go func() { defer readers.Done(); fn() }()
	}
	found := make(chan string, 16)
	gone := map[string]chan struct{}{}
	var mu sync.Mutex
	goneOf := func(p string) chan struct{} {
		mu.Lock()
		defer mu.Unlock()
		if gone[p] == nil {
			gone[p] = make(chan struct{})
		}
		return gone[p]
	}
	quit := make(chan struct{})
	read := func(dev *inputDev) {
		found <- filepath.Base(dev.path)
		select {
		case <-goneOf(dev.path):
		case <-quit:
		}
	}
	if err := watchKeyboards(dir, []string{filepath.Join(dir, "event0")}, stop, open, read, run); err != nil {
		t.Fatal(err)
	}
	want := func(name string) {
		t.Helper()
		select {
		case got := <-found:
			if got != name {
				t.Fatalf("found %s, want %s", got, name)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s wasn't found", name)
		}
	}
	none := func() {
		t.Helper()
		select {
		case got := <-found:
			t.Fatalf("found %s, which it should have left alone", got)
		case <-time.After(200 * time.Millisecond):
		}
	}
	want("event1")
	none()
	put("event2", "mouse", 0o644)
	none()
	if os.Geteuid() != 0 {
		// The kernel makes the node as root's; udev gives it its group after.
		put("event3", "keyboard", 0)
		none()
		if err := os.Chmod(filepath.Join(dir, "event3"), 0o644); err != nil {
			t.Fatal(err)
		}
	} else {
		put("event3", "keyboard", 0o644)
	}
	want("event3")
	// Unplugged and plugged in again, at the same node.
	close(goneOf(filepath.Join(dir, "event3")))
	if err := os.Remove(filepath.Join(dir, "event3")); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	delete(gone, filepath.Join(dir, "event3"))
	mu.Unlock()
	put("event3", "keyboard", 0o644)
	want("event3")
	none()
	// The driver stops: the watcher ends, and the readers with it.
	close(quit)
	if _, err := unix.Write(stop, []byte{1, 0, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { readers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the watcher didn't stop")
	}
}

// After the kernel's event queue overflows, the watcher looks at every
// node again, so a keyboard whose events were lost is still found.
func TestKeyWatcher_LooksAgainAfterAnOverflow(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "event9"), []byte("keyboard"), 0o644); err != nil {
		t.Fatal(err)
	}
	found := make(chan string, 1)
	w := &keyWatcher{dir: dir, seen: map[string]*inputDev{},
		open: func(p string) (*inputDev, error) {
			return &inputDev{fd: -1, path: p, keys: true}, nil
		},
		found: func(dev *inputDev) { found <- dev.path },
	}
	var readers sync.WaitGroup
	run := func(fn func()) { readers.Add(1); go func() { defer readers.Done(); fn() }() }
	if gone := w.handle(unix.IN_Q_OVERFLOW, "", run); gone {
		t.Fatal("an overflow counted as the watch gone")
	}
	select {
	case p := <-found:
		if filepath.Base(p) != "event9" {
			t.Fatalf("found %s", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no new look after the overflow")
	}
	readers.Wait()
	if !w.handle(unix.IN_IGNORED, "", run) {
		t.Fatal("IN_IGNORED didn't count as the watch gone")
	}
}

// A folder that isn't there yet is looked for until it is, and one that
// goes and comes back is watched again.
func TestKeyWatcher_WaitsForTheFolder(t *testing.T) {
	saved := keyWatchRetry
	keyWatchRetry = 20 * time.Millisecond
	t.Cleanup(func() { keyWatchRetry = saved })
	dir := filepath.Join(t.TempDir(), "input")
	stop, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Close(stop) }()
	found := make(chan string, 4)
	open := func(p string) (*inputDev, error) {
		if _, err := os.Stat(p); err != nil {
			return nil, err
		}
		return &inputDev{fd: -1, path: p, keys: true}, nil
	}
	var readers sync.WaitGroup
	run := func(fn func()) { readers.Add(1); go func() { defer readers.Done(); fn() }() }
	if err := watchKeyboards(dir, nil, stop, open, func(dev *inputDev) { found <- dev.path }, run); err != nil {
		t.Fatalf("a missing folder: %v", err)
	}
	plug := func(name string) {
		t.Helper()
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		tmp := filepath.Join(dir, "new")
		if err := os.WriteFile(tmp, []byte("keyboard"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	want := func(name string) {
		t.Helper()
		select {
		case p := <-found:
			if filepath.Base(p) != name {
				t.Fatalf("found %s, want %s", p, name)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s wasn't found", name)
		}
	}
	time.Sleep(60 * time.Millisecond)
	plug("event1")
	want("event1")
	// The folder goes, and comes back with a keyboard at the same node.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)
	plug("event1")
	want("event1")
	if _, err := unix.Write(stop, []byte{1, 0, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	readers.Wait()
}
