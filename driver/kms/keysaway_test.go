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

// These tests stand in for the kernel's mask with maskKeys: they show what
// the driver asks, and that its readers act on no key while muted. That
// the kernel then queues nothing is shown only on a real kernel; see #447.

// keysWorld is a driver on VT 1, with the keyboard watcher on a folder of
// fake keyboards, and its render thread's side of leaving and returning.
type keysWorld struct {
	t   *testing.T
	d   *Driver
	r   *renderThread
	con switchConsole
	kb  *fakeKeyboards
	dir string

	maskMu  sync.Mutex
	muted   map[string]bool
	reopens map[string]int
	// failUnmask is how many unmasks of each keyboard fail.
	failUnmask int
	fails      map[string]int
}

func newKeysWorld(t *testing.T, active int) *keysWorld {
	t.Helper()
	w := &keysWorld{t: t, dir: t.TempDir(), muted: map[string]bool{}, reopens: map[string]int{}, fails: map[string]int{}}
	stop, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		t.Fatal(err)
	}
	w.con = switchConsole{switched: make(chan int, 8), active: active}
	w.d = &Driver{o: Options{VTSwitch: true}, vt: &vt{c: w.con, fd: 7, num: 1}, quit: make(chan struct{}), stop: stop,
		blank: newBlanker(Blank{}, 1, time.Now())}
	w.d.maskKeys = func(dev *inputDev, mute bool) error {
		w.maskMu.Lock()
		defer w.maskMu.Unlock()
		name := filepath.Base(dev.path)
		if !mute && w.fails[name] < w.failUnmask {
			w.fails[name]++
			return unix.EIO
		}
		w.muted[name] = mute
		return nil
	}
	w.d.reopenKeys = func(dev *inputDev) error {
		w.maskMu.Lock()
		defer w.maskMu.Unlock()
		name := filepath.Base(dev.path)
		w.reopens[name]++
		w.muted[name] = false // a new file has no mask
		return nil
	}
	w.r = &renderThread{d: w.d}
	w.kb = &fakeKeyboards{t: t, typed: map[string]int{}, opens: map[string]int{}}
	run := func(fn func()) {
		w.d.readers.Add(1)
		go func() { defer w.d.readers.Done(); fn() }()
	}
	if err := watchKeyboards(w.dir, nil, nil, stop, w.kb.open, w.d.readKeys, run); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.close)
	return w
}

// plug adds a keyboard at name, and waits until the driver reads it.
func (w *keysWorld) plug(name string) string {
	w.t.Helper()
	tmp := filepath.Join(w.dir, "new")
	if err := os.WriteFile(tmp, []byte("keyboard"), 0o644); err != nil {
		w.t.Fatal(err)
	}
	p := filepath.Join(w.dir, name)
	w.d.keysMu.Lock()
	before := len(w.d.keyDevs)
	w.d.keysMu.Unlock()
	if err := os.Rename(tmp, p); err != nil {
		w.t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		w.d.keysMu.Lock()
		n := len(w.d.keyDevs)
		w.d.keysMu.Unlock()
		if n > before {
			return p
		}
		if time.Now().After(deadline) {
			w.t.Fatalf("%s wasn't read", name)
		}
	}
}

func (w *keysWorld) isMuted(name string) (muted, asked bool) {
	w.maskMu.Lock()
	defer w.maskMu.Unlock()
	m, ok := w.muted[name]
	return m, ok
}

func (w *keysWorld) wantSwitch(n int) {
	w.t.Helper()
	select {
	case got := <-w.con.switched:
		if got != n {
			w.t.Fatalf("switched to VT %d, want %d", got, n)
		}
	case <-time.After(5 * time.Second):
		w.t.Fatalf("no switch to VT %d", n)
	}
}

func (w *keysWorld) noSwitch(why string) {
	w.t.Helper()
	select {
	case got := <-w.con.switched:
		w.t.Fatalf("switched to VT %d %s", got, why)
	case <-time.After(300 * time.Millisecond):
	}
}

// close stops the driver's readers, and checks they all end and forget
// their keyboards.
func (w *keysWorld) close() {
	close(w.d.quit)
	if _, err := unix.Write(w.d.stop, []byte{1, 0, 0, 0, 0, 0, 0, 0}); err != nil {
		w.t.Fatal(err)
	}
	w.kb.mu.Lock()
	for _, fd := range w.kb.typed {
		_ = unix.Close(fd)
	}
	w.kb.mu.Unlock()
	done := make(chan struct{})
	go func() { w.d.readers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		w.t.Fatal("the readers didn't stop")
	}
	_ = unix.Close(w.d.stop)
	if len(w.d.keyDevs) != 0 {
		w.t.Fatalf("%d keyboards still counted after their readers ended", len(w.d.keyDevs))
	}
}

// While another VT shows, the driver reads no keyboard: each one is masked
// as the VT goes, one plugged in meanwhile is masked as it is found, and
// none of their keys switches VT. As the driver's VT comes back, every mask
// clears, and Ctrl+Alt+Fn switches again.
func TestKeys_NotReadWhileAnotherVTShows(t *testing.T) {
	w := newKeysWorld(t, 2)
	a := w.plug("event1")
	w.kb.typeKeys(a, keyF1+1)
	w.wantSwitch(2)

	// The kernel switched to VT 2: the driver let go of the screen.
	w.r.left()
	if m, _ := w.isMuted("event1"); !m {
		t.Fatal("event1 wasn't masked as the VT went")
	}
	w.kb.typeKeys(a, keyF1+2)
	w.noSwitch("while another VT showed")
	b := w.plug("event2") // plugged in while away
	if m, asked := w.isMuted("event2"); !asked || !m {
		t.Fatal("event2, found while away, wasn't masked")
	}
	w.kb.typeKeys(b, keyF1+2)
	w.noSwitch("while another VT showed")

	// Someone switched back to the driver's VT.
	w.r.returned(time.Now())
	for _, name := range []string{"event1", "event2"} {
		if m, _ := w.isMuted(name); m {
			t.Fatalf("%s stayed masked after the VT came back", name)
		}
	}
	w.kb.typeKeys(b, keyF1+3)
	w.wantSwitch(4)
	w.kb.typeKeys(a, keyF1+4)
	w.wantSwitch(5)
}

// Ctrl and Alt held as the VT goes, and let go of while another VT shows,
// count as up after the return: the reader asks the kernel which keys are
// down, so F3 alone switches nothing, and Ctrl+Alt+F3 does.
func TestKeys_AKeyHeldAcrossTheSwitch(t *testing.T) {
	w := newKeysWorld(t, 2)
	a := w.plug("event1")
	// Ctrl and Alt go down, and F2 switches; the driver leaves before they
	// come up.
	w.kb.send(a, event{typ: evKey, code: keyLeftCtrl, value: 1}, event{typ: evKey, code: keyLeftAlt, value: 1},
		event{typ: evKey, code: keyF1 + 1, value: 1}, event{typ: evKey, code: keyF1 + 1, value: 0})
	w.wantSwitch(2)
	w.r.left()
	// Ctrl and Alt are let go of on VT 2. The kernel's mask queues none of
	// it for the driver, so the fake sends nothing: the driver's last word
	// on them is "down".
	w.r.returned(time.Now())
	// The fake device can't say which keys are down, as a device that
	// can't counts none down: the kernel's answer for keys let go of.
	w.kb.send(a, event{typ: evKey, code: keyF1 + 2, value: 1}, event{typ: evKey, code: keyF1 + 2, value: 0})
	w.noSwitch("on F3 alone, with Ctrl and Alt let go of while away")
	w.kb.typeKeys(a, keyF1+2)
	w.wantSwitch(3)
}

// A release the kernel refuses, as after a stray SIGUSR1 with no switch
// asked for, leaves the driver's VT showing: the driver takes the screen
// back, and the keyboards with it, so Ctrl+Alt+Fn works. Where another VT
// does show, it leaves them to that VT.
func TestKeys_AFailedReleaseLeavesKeysWorking(t *testing.T) {
	w := newKeysWorld(t, 1)
	a := w.plug("event1")
	w.r.left()
	took := false
	w.d.afterRelease(unix.EINVAL, func() { took = true; w.r.returned(time.Now()) })
	if !took {
		t.Fatal("the driver didn't take its VT back")
	}
	if m, _ := w.isMuted("event1"); m {
		t.Fatal("the keyboard stayed masked on the driver's own VT")
	}
	w.kb.typeKeys(a, keyF1+1)
	w.wantSwitch(2)

	other := newKeysWorld(t, 2)
	other.plug("event1")
	other.r.left()
	other.d.afterRelease(unix.EINVAL, func() { t.Fatal("it took the screen back while VT 2 showed") })
	other.d.afterRelease(nil, func() { t.Fatal("it took the screen back after a release that worked") })
	if m, _ := other.isMuted("event1"); !m {
		t.Fatal("the keyboard was unmasked while another VT showed")
	}
}

// A mask that won't clear would leave the keyboard deaf on the driver's own
// VT: the driver tries again, and then opens the device afresh.
func TestKeys_AFailedUnmaskReopens(t *testing.T) {
	for _, fails := range []int{1, 2} {
		w := newKeysWorld(t, 2)
		w.failUnmask = fails
		a := w.plug("event1")
		w.r.left()
		w.r.returned(time.Now())
		w.maskMu.Lock()
		reopens := w.reopens["event1"]
		w.maskMu.Unlock()
		if want := map[int]int{1: 0, 2: 1}[fails]; reopens != want {
			t.Fatalf("%d failed unmasks: %d reopens, want %d", fails, reopens, want)
		}
		if m, _ := w.isMuted("event1"); m {
			t.Fatalf("%d failed unmasks: the keyboard stayed masked", fails)
		}
		w.kb.typeKeys(a, keyF1+1)
		w.wantSwitch(2)
	}
}

// The mask goes to the kernel in whole longs: on the trial Pi's 6.18 kernel, EVIOCSMASK refused 4
// bytes with EINVAL and took 8.
func TestKeys_MaskIsWholeLongs(t *testing.T) {
	if maskBytes != 8 || maskBytes*8 < evCnt {
		t.Fatalf("the mask is %d bytes", maskBytes)
	}
}

// A keyboard read again while another VT shows, after its reader ended with
// its node still there, is masked as it is opened, and switches nothing
// until the driver's VT is back.
func TestKeys_AKeyboardReadAgainWhileAwayIsMasked(t *testing.T) {
	shortReopen(t, 20*time.Millisecond, 100*time.Millisecond)
	w := newKeysWorld(t, 2)
	a := w.plug("event1")
	w.r.left()
	w.maskMu.Lock()
	delete(w.muted, "event1")
	w.maskMu.Unlock()
	w.kb.unplug(a) // the reader ends; the node stays
	waitFor(t, "the keyboard wasn't opened again", func() bool { return w.kb.opened(a) == 2 })
	waitFor(t, "the keyboard wasn't read again", func() bool {
		w.d.keysMu.Lock()
		defer w.d.keysMu.Unlock()
		return len(w.d.keyDevs) == 1
	})
	if m, asked := w.isMuted("event1"); !asked || !m {
		t.Fatal("the keyboard read again while away wasn't masked")
	}
	w.kb.typeKeys(a, keyF1+2)
	w.noSwitch("while another VT showed")
	w.r.returned(time.Now())
	w.kb.typeKeys(a, keyF1+2)
	w.wantSwitch(3)
}
