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
	read := func(dev *inputDev) bool {
		found <- filepath.Base(dev.path)
		select {
		case <-goneOf(dev.path):
			return true
		case <-quit:
			return false
		}
	}
	if err := watchKeyboards(dir, nil, []string{filepath.Join(dir, "event0")}, stop, open, read, run); err != nil {
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
// switch, for a test whose readers run on goroutines of their own. active
// is the VT it says shows.
type switchConsole struct {
	switched chan int
	active   int
}

func (c switchConsole) open(int) (int, error) { return 7, nil }
func (c switchConsole) ioctl(_ int, req uintptr, arg unsafe.Pointer) error {
	if req == vtGetState {
		(*vtStat)(arg).active = uint16(c.active)
	}
	return nil
}
func (c switchConsole) close(int) error { return nil }
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
	// broken says each open gives a keyboard whose reader ends at once;
	// breaks says so for the nth open of a node, from 1, where set.
	broken bool
	breaks func(n int) bool
	times  []time.Time // when each open was
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
	k.times = append(k.times, time.Now())
	if k.broken || k.breaks != nil && k.breaks(k.opens[p]) {
		_ = unix.Close(fds[1])
		k.typed[p] = -1
	}
	k.mu.Unlock()
	return &inputDev{fd: fds[0], path: p, name: string(b), keys: string(b) == "keyboard"}, nil
}

// typeKeys types Ctrl+Alt+Fn on the newest keyboard at p.
func (k *fakeKeyboards) typeKeys(p string, fn uint16) {
	k.send(p,
		event{typ: evKey, code: keyLeftCtrl, value: 1}, event{typ: evKey, code: keyLeftAlt, value: 1}, event{typ: evKey, code: fn, value: 1},
		event{typ: evKey, code: fn, value: 0}, event{typ: evKey, code: keyLeftAlt, value: 0}, event{typ: evKey, code: keyLeftCtrl, value: 0})
}

// send writes events on the newest keyboard at p.
func (k *fakeKeyboards) send(p string, events ...event) {
	k.mu.Lock()
	fd := k.typed[p]
	k.mu.Unlock()
	buf := make([]byte, 0, len(events)*liveEventSize)
	for _, e := range events {
		b := make([]byte, liveEventSize)
		encodeEvent(b, e)
		buf = append(buf, b...)
	}
	if _, err := unix.Write(fd, buf); err != nil {
		k.t.Fatal(err)
	}
}

// unplug ends the newest keyboard at p, as a device that goes away, or
// one whose reader sees POLLHUP while its node stays.
func (k *fakeKeyboards) unplug(p string) {
	k.mu.Lock()
	fd := k.typed[p]
	k.typed[p] = -1
	k.mu.Unlock()
	if fd >= 0 {
		_ = unix.Close(fd)
	}
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
	if err := watchKeyboards(dir, nil, nil, stop, kb.open, d.readKeys, run); err != nil {
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
		found: func(dev *inputDev) bool { found <- dev.path; return false },
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
	if err := watchKeyboards(dir, nil, nil, stop, open, func(dev *inputDev) bool { found <- dev.path; return false }, run); err != nil {
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

// shortReopen makes the watcher open a keyboard again after first, up to
// most, for a test.
func shortReopen(t *testing.T, first, most time.Duration) {
	t.Helper()
	f, m, r := keyReopenFirst, keyReopenMax, keyReopenReset
	keyReopenFirst, keyReopenMax = first, most
	t.Cleanup(func() { keyReopenFirst, keyReopenMax, keyReopenReset = f, m, r })
}

// openFDs counts this process's open files.
func openFDs(t *testing.T) int {
	t.Helper()
	ents, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	return len(ents)
}

// waitFor waits until ok, for up to 5 seconds.
func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal(what)
		}
	}
}

// A keyboard whose reader ends while its node stays, as after a transient
// POLLERR, is opened again and read: Ctrl+Alt+F2 from it switches VT. That
// keyboard is how the owner reaches a login on another VT. It counts as one
// keyboard throughout, as the driver's own keyboards, handed to the
// watcher, do.
func TestKeyWatcher_ReadsAKeyboardAgainWhoseReaderEnded(t *testing.T) {
	shortReopen(t, 20*time.Millisecond, 100*time.Millisecond)
	dir := t.TempDir()
	node := filepath.Join(dir, "event4")
	if err := os.WriteFile(node, []byte("keyboard"), 0o644); err != nil {
		t.Fatal(err)
	}
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
	// The keyboard Open found, handed to the watcher.
	first, err := kb.open(node)
	if err != nil {
		t.Fatal(err)
	}
	if err := watchKeyboards(dir, []*inputDev{first}, nil, stop, kb.open, d.readKeys, run); err != nil {
		t.Fatal(err)
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
	kb.typeKeys(node, keyF1+1)
	wantSwitch(2)
	for i := 2; i <= 3; i++ {
		kb.unplug(node) // the reader ends; the node stays
		waitFor(t, "the keyboard wasn't opened again", func() bool { return kb.opened(node) >= i })
		waitFor(t, "the keyboard wasn't read again", func() bool {
			d.keysMu.Lock()
			defer d.keysMu.Unlock()
			return len(d.keyDevs) == 1
		})
		kb.typeKeys(node, keyF1+uint16(i))
		wantSwitch(i + 1)
	}
	time.Sleep(200 * time.Millisecond)
	if n := kb.opened(node); n != 3 {
		t.Fatalf("%s opened %d times, want 3", node, n)
	}

	// The node goes after the reader ended: nothing opens it again.
	kb.unplug(node)
	if err := os.Remove(node); err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if n := kb.opened(node); n != 3 {
		t.Fatalf("a removed node was opened again: %d opens", n)
	}

	close(d.quit)
	if _, err := unix.Write(stop, []byte{1, 0, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { d.readers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the readers didn't stop")
	}
}

// A keyboard that fails each time it is opened is tried less and less
// often, up to keyReopenMax apart, and never in a busy loop. Each failed
// open's file is closed.
func TestKeyWatcher_BacksOffAKeyboardThatKeepsFailing(t *testing.T) {
	shortReopen(t, 10*time.Millisecond, 80*time.Millisecond)
	dir := t.TempDir()
	stop, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Close(stop) }()
	kb := &fakeKeyboards{t: t, typed: map[string]int{}, opens: map[string]int{}, broken: true}
	var readers sync.WaitGroup
	run := func(fn func()) { readers.Add(1); go func() { defer readers.Done(); fn() }() }
	d := &Driver{o: Options{VTSwitch: true}, quit: make(chan struct{}), stop: stop}
	before := openFDs(t)
	if err := watchKeyboards(dir, nil, nil, stop, kb.open, d.readKeys, run); err != nil {
		t.Fatal(err)
	}
	node := filepath.Join(dir, "event7")
	if err := os.WriteFile(node, []byte("keyboard"), 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	n := kb.opened(node)
	// 10, 20, 40, then 80 ms apart: about 15 opens in a second. A busy loop
	// would make thousands.
	if n < 4 || n > 20 {
		t.Fatalf("%d opens in a second, want about 15", n)
	}
	kb.mu.Lock()
	times := append([]time.Time(nil), kb.times...)
	kb.mu.Unlock()
	for i := 4; i < len(times); i++ {
		if gap := times[i].Sub(times[i-1]); gap < 70*time.Millisecond {
			t.Fatalf("opens %d and %d were %v apart, want keyReopenMax", i-1, i, gap)
		}
	}
	if _, err := unix.Write(stop, []byte{1, 0, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	readers.Wait()
	if after := openFDs(t); after != before {
		t.Fatalf("%d files open after the watcher stopped, %d before", after, before)
	}
}

// The driver closing while a keyboard waits to be opened again ends the
// wait at once: the reader's goroutine ends, and no file stays open.
func TestKeyWatcher_CloseEndsTheWaitToReadAgain(t *testing.T) {
	shortReopen(t, time.Hour, time.Hour)
	dir := t.TempDir()
	node := filepath.Join(dir, "event2")
	if err := os.WriteFile(node, []byte("keyboard"), 0o644); err != nil {
		t.Fatal(err)
	}
	stop, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Close(stop) }()
	before := openFDs(t)
	kb := &fakeKeyboards{t: t, typed: map[string]int{}, opens: map[string]int{}}
	d := &Driver{o: Options{VTSwitch: true}, quit: make(chan struct{}), stop: stop}
	run := func(fn func()) { d.readers.Add(1); go func() { defer d.readers.Done(); fn() }() }
	if err := watchKeyboards(dir, nil, nil, stop, kb.open, d.readKeys, run); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the keyboard wasn't read", func() bool {
		d.keysMu.Lock()
		defer d.keysMu.Unlock()
		return len(d.keyDevs) == 1
	})
	kb.unplug(node)
	waitFor(t, "the reader didn't end", func() bool {
		d.keysMu.Lock()
		defer d.keysMu.Unlock()
		return len(d.keyDevs) == 0
	})
	time.Sleep(50 * time.Millisecond) // into the wait
	began := time.Now()
	close(d.quit)
	if _, err := unix.Write(stop, []byte{1, 0, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { d.readers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the wait to read the keyboard again didn't end")
	}
	if took := time.Since(began); took > time.Second {
		t.Fatalf("the readers took %v to stop", took)
	}
	if n := kb.opened(node); n != 1 {
		t.Fatalf("%s opened %d times, want 1", node, n)
	}
	if after := openFDs(t); after != before {
		t.Fatalf("%d files open after the watcher stopped, %d before", after, before)
	}
}

// keysDriver is a driver on VT 1 that reads keyboards, for a test, and
// stops it at the test's end.
func keysDriver(t *testing.T) (*Driver, switchConsole, func(func())) {
	t.Helper()
	stop, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		t.Fatal(err)
	}
	con := switchConsole{switched: make(chan int, 4)}
	d := &Driver{o: Options{VTSwitch: true}, vt: &vt{c: con, fd: 7, num: 1}, quit: make(chan struct{}), stop: stop}
	run := func(fn func()) { d.readers.Add(1); go func() { defer d.readers.Done(); fn() }() }
	t.Cleanup(func() { _ = unix.Close(stop) })
	return d, con, run
}

// stopKeys stops d's readers, as Close does, and waits for them.
func stopKeys(t *testing.T, d *Driver) {
	t.Helper()
	select {
	case <-d.quit:
	default:
		close(d.quit)
	}
	if _, err := unix.Write(d.stop, []byte{1, 0, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { d.readers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the readers didn't stop")
	}
}

func wantSwitchTo(t *testing.T, con switchConsole, n int) {
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

// A reader that ends as the driver quits is not a keyboard gone: Close
// closes quit first and writes stop later, and nothing opens the node
// again in between.
func TestKeyWatcher_NoReadingAgainAsTheDriverQuits(t *testing.T) {
	shortReopen(t, 10*time.Millisecond, 10*time.Millisecond)
	dir := t.TempDir()
	node := filepath.Join(dir, "event3")
	if err := os.WriteFile(node, []byte("keyboard"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, _, run := keysDriver(t)
	kb := &fakeKeyboards{t: t, typed: map[string]int{}, opens: map[string]int{}}
	if err := watchKeyboards(dir, nil, nil, d.stop, kb.open, d.readKeys, run); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the keyboard wasn't read", func() bool {
		d.keysMu.Lock()
		defer d.keysMu.Unlock()
		return len(d.keyDevs) == 1
	})
	close(d.quit)
	// A key wakes the reader, which sees quit and ends, before stop is
	// written.
	kb.send(node, event{typ: evKey, code: keyA, value: 1})
	waitFor(t, "the reader didn't end on quit", func() bool {
		d.keysMu.Lock()
		defer d.keysMu.Unlock()
		return len(d.keyDevs) == 0
	})
	time.Sleep(200 * time.Millisecond)
	if n := kb.opened(node); n != 1 {
		t.Fatalf("%s opened %d times as the driver quit, want 1", node, n)
	}
	stopKeys(t, d)
	kb.unplug(node)
}

// A reader that ran for keyReopenReset starts the back-off again: after a
// run of failures has grown the wait, the next end is retried after
// keyReopenFirst, not after the grown wait.
func TestKeyWatcher_BackOffStartsAgainAfterALongRead(t *testing.T) {
	shortReopen(t, 20*time.Millisecond, 10*time.Second)
	keyReopenReset = 150 * time.Millisecond
	dir := t.TempDir()
	node := filepath.Join(dir, "event5")
	if err := os.WriteFile(node, []byte("keyboard"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, con, run := keysDriver(t)
	// Opens 2 to 6 fail at once: the waits grow 20, 40, 80, 160, 320 ms,
	// and the next would be 640 ms.
	kb := &fakeKeyboards{t: t, typed: map[string]int{}, opens: map[string]int{},
		breaks: func(n int) bool { return n >= 2 && n <= 6 }}
	if err := watchKeyboards(dir, nil, nil, d.stop, kb.open, d.readKeys, run); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the keyboard wasn't read", func() bool { return kb.opened(node) == 1 })
	kb.unplug(node)
	waitFor(t, "the keyboard wasn't read after its failures", func() bool {
		if kb.opened(node) != 7 {
			return false
		}
		d.keysMu.Lock()
		defer d.keysMu.Unlock()
		return len(d.keyDevs) == 1
	})
	kb.typeKeys(node, keyF1+1)
	wantSwitchTo(t, con, 2)
	time.Sleep(300 * time.Millisecond) // longer than keyReopenReset
	ended := time.Now()
	kb.unplug(node)
	waitFor(t, "the keyboard wasn't opened again", func() bool { return kb.opened(node) == 8 })
	if took := time.Since(ended); took > 400*time.Millisecond {
		t.Fatalf("opened again after %v: the back-off didn't start again", took)
	}
	stopKeys(t, d)
	kb.unplug(node)
}

// startInput hands Open's keyboards to the watcher, which closes them, so
// Close mustn't: d.keys goes nil. Where the watch can't start, as when
// inotify fails, d.keys stays for Close, and each keyboard is read as
// before.
func TestStartInput_KeyboardsWithAndWithoutTheWatcher(t *testing.T) {
	for _, watchFails := range []bool{false, true} {
		dir := t.TempDir()
		node := filepath.Join(dir, "event1")
		if err := os.WriteFile(node, []byte("keyboard"), 0o644); err != nil {
			t.Fatal(err)
		}
		d, con, _ := keysDriver(t)
		kb := &fakeKeyboards{t: t, typed: map[string]int{}, opens: map[string]int{}}
		dev, err := kb.open(node)
		if err != nil {
			t.Fatal(err)
		}
		d.keys = []*inputDev{dev}
		saved := watchInputs
		watchInputs = func(keys []*inputDev, known []string, stop int, found func(*inputDev) bool, run func(func())) error {
			if watchFails {
				return unix.EMFILE
			}
			return watchKeyboards(dir, keys, known, stop, kb.open, found, run)
		}
		d.startInput()
		watchInputs = saved
		if watchFails != (len(d.keys) == 1) {
			t.Fatalf("watch fails %v: %d keyboards left for Close", watchFails, len(d.keys))
		}
		kb.typeKeys(node, keyF1+1)
		wantSwitchTo(t, con, 2)
		stopKeys(t, d)
		for _, k := range d.keys {
			k.close()
		}
		kb.unplug(node)
	}
}
