//go:build linux && (arm64 || amd64)

package kms

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"unsafe"

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

// switchConsole is a console that only switches VT, and tells of each
// switch, for a test whose readers run on goroutines of their own.
type switchConsole struct{ switched chan int }

func (c switchConsole) open(int) (int, error)                    { return 7, nil }
func (c switchConsole) ioctl(int, uintptr, unsafe.Pointer) error { return nil }
func (c switchConsole) close(int) error                          { return nil }
func (c switchConsole) ioctlInt(_ int, req, arg uintptr) error {
	if req == vtActivate {
		c.switched <- int(arg)
	}
	return nil
}

// fakeKeyboards are keyboards for the watcher: each node's file says
// "keyboard", and each open gives a pipe the test types into.
type fakeKeyboards struct {
	t     *testing.T
	mu    sync.Mutex
	typed map[string]int // the write end of the newest pipe for each node
	opens map[string]int
}

func (k *fakeKeyboards) open(p string) (*inputDev, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var fds [2]int
	// Not blocking, as a device's file: the driver drains it as another VT takes the screen.
	if err := unix.Pipe2(fds[:], unix.O_CLOEXEC|unix.O_NONBLOCK); err != nil {
		k.t.Fatal(err)
	}
	k.mu.Lock()
	k.typed[p] = fds[1]
	k.opens[p]++
	k.mu.Unlock()
	return &inputDev{fd: fds[0], path: p, name: string(b), keys: string(b) == "keyboard"}, nil
}

// typeKeys types Ctrl+Alt+Fn on the newest keyboard at p.
func (k *fakeKeyboards) typeKeys(p string, fn uint16) {
	k.mu.Lock()
	fd := k.typed[p]
	k.mu.Unlock()
	buf := make([]byte, 0, 6*liveEventSize)
	for _, e := range []event{
		{typ: evKey, code: keyLeftCtrl, value: 1}, {typ: evKey, code: keyLeftAlt, value: 1}, {typ: evKey, code: fn, value: 1},
		{typ: evKey, code: fn, value: 0}, {typ: evKey, code: keyLeftAlt, value: 0}, {typ: evKey, code: keyLeftCtrl, value: 0},
	} {
		b := make([]byte, liveEventSize)
		encodeEvent(b, e)
		buf = append(buf, b...)
	}
	if _, err := unix.Write(fd, buf); err != nil {
		k.t.Fatal(err)
	}
}

// unplug ends the newest keyboard at p, as a device that goes away.
func (k *fakeKeyboards) unplug(p string) {
	k.mu.Lock()
	fd := k.typed[p]
	k.mu.Unlock()
	_ = unix.Close(fd)
}

func (k *fakeKeyboards) opened(p string) int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.opens[p]
}

// A keyboard plugged in after Open switches VT through readKeys, as the
// driver's own keyboards do. A node removed and made again while its old
// reader still runs gets a reader of its own, and the old reader, as it
// ends, leaves the new one's place alone: a later change of the node opens
// nothing more.
func TestKeyWatcher_ALateKeyboardSwitchesVT(t *testing.T) {
	dir := t.TempDir()
	stop, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Close(stop) }()
	con := switchConsole{switched: make(chan int, 4)}
	d := &Driver{o: Options{VTSwitch: true}, vt: &vt{c: con, fd: 7, num: 1}, quit: make(chan struct{}), stop: stop}
	kb := &fakeKeyboards{t: t, typed: map[string]int{}, opens: map[string]int{}}
	run := func(fn func()) {
		d.readers.Add(1)
		go func() { defer d.readers.Done(); fn() }()
	}
	if err := watchKeyboards(dir, nil, stop, kb.open, d.readKeys, run); err != nil {
		t.Fatal(err)
	}
	node := filepath.Join(dir, "event4")
	plug := func() {
		t.Helper()
		tmp := filepath.Join(dir, "new")
		if err := os.WriteFile(tmp, []byte("keyboard"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(tmp, node); err != nil {
			t.Fatal(err)
		}
	}
	waitOpens := func(n int) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); kb.opened(node) < n; time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%s opened %d times, want %d", node, kb.opened(node), n)
			}
		}
	}
	wantSwitch := func(n int) {
		t.Helper()
		select {
		case got := <-con.switched:
			if got != n {
				t.Fatalf("switched to VT %d, want %d", got, n)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("no switch to VT %d", n)
		}
	}
	plug()
	waitOpens(1)
	kb.typeKeys(node, keyF1+1) // F2
	wantSwitch(2)

	// Removed and made again while the first reader still runs.
	kb.mu.Lock()
	first := kb.typed[node]
	kb.mu.Unlock()
	if err := os.Remove(node); err != nil {
		t.Fatal(err)
	}
	plug()
	waitOpens(2)
	// The first reader ends now, after the new one began.
	_ = unix.Close(first)
	time.Sleep(200 * time.Millisecond)
	if err := os.Chmod(node, 0o640); err != nil { // IN_ATTRIB on the node
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if n := kb.opened(node); n != 2 {
		t.Fatalf("the old reader's end let the node be opened again: %d opens", n)
	}
	kb.typeKeys(node, keyF1+2) // F3, on the new keyboard
	wantSwitch(3)

	close(d.quit)
	if _, err := unix.Write(stop, []byte{1, 0, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	kb.unplug(node)
	done := make(chan struct{})
	go func() { d.readers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the readers didn't stop")
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
