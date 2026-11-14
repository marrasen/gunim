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

// While another VT shows, the driver reads no keyboard: each one is masked
// as the VT goes, one plugged in meanwhile is masked as it is found, and
// none of their keys switches VT. As the driver's VT comes back, every mask
// clears, and Ctrl+Alt+Fn switches again.
func TestKeys_NotReadWhileAnotherVTShows(t *testing.T) {
	dir := t.TempDir()
	stop, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unix.Close(stop) }()
	con := switchConsole{switched: make(chan int, 8)}
	var maskMu sync.Mutex
	muted := map[string]bool{}
	d := &Driver{o: Options{VTSwitch: true}, vt: &vt{c: con, fd: 7, num: 1}, quit: make(chan struct{}), stop: stop,
		blank: newBlanker(Blank{}, 1, time.Now())}
	d.maskKeys = func(dev *inputDev, mute bool) error {
		maskMu.Lock()
		muted[filepath.Base(dev.path)] = mute
		maskMu.Unlock()
		return nil
	}
	isMuted := func(name string) (bool, bool) {
		maskMu.Lock()
		defer maskMu.Unlock()
		m, ok := muted[name]
		return m, ok
	}
	r := &renderThread{d: d}
	kb := &fakeKeyboards{t: t, typed: map[string]int{}, opens: map[string]int{}}
	run := func(fn func()) {
		d.readers.Add(1)
		go func() { defer d.readers.Done(); fn() }()
	}
	if err := watchKeyboards(dir, nil, stop, kb.open, d.readKeys, run); err != nil {
		t.Fatal(err)
	}
	plug := func(name string) string {
		t.Helper()
		tmp := filepath.Join(dir, "new")
		if err := os.WriteFile(tmp, []byte("keyboard"), 0o644); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, name)
		if err := os.Rename(tmp, p); err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(5 * time.Second); kb.opened(p) == 0; time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%s wasn't opened", name)
			}
		}
		// The reader counts itself in just after the open.
		time.Sleep(50 * time.Millisecond)
		return p
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
	noSwitch := func() {
		t.Helper()
		select {
		case got := <-con.switched:
			t.Fatalf("switched to VT %d while another VT showed", got)
		case <-time.After(300 * time.Millisecond):
		}
	}

	a := plug("event1")
	kb.typeKeys(a, keyF1+1)
	wantSwitch(2)

	// The kernel switched to VT 2: the driver let go of the screen.
	r.left()
	if m, _ := isMuted("event1"); !m {
		t.Fatal("event1 wasn't masked as the VT went")
	}
	kb.typeKeys(a, keyF1+2)
	noSwitch()
	b := plug("event2") // plugged in while away
	if m, ok := isMuted("event2"); !ok || !m {
		t.Fatal("event2, found while away, wasn't masked")
	}
	kb.typeKeys(b, keyF1+2)
	noSwitch()

	// Someone switched back to the driver's VT.
	r.returned(time.Now())
	for _, name := range []string{"event1", "event2"} {
		if m, _ := isMuted(name); m {
			t.Fatalf("%s stayed masked after the VT came back", name)
		}
	}
	kb.typeKeys(b, keyF1+3)
	wantSwitch(4)
	kb.typeKeys(a, keyF1+4)
	wantSwitch(5)

	close(d.quit)
	if _, err := unix.Write(stop, []byte{1, 0, 0, 0, 0, 0, 0, 0}); err != nil {
		t.Fatal(err)
	}
	kb.unplug(a)
	kb.unplug(b)
	done := make(chan struct{})
	go func() { d.readers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the readers didn't stop")
	}
	if len(d.keyDevs) != 0 {
		t.Fatalf("%d keyboards still counted after their readers ended", len(d.keyDevs))
	}
}

// The mask goes to the kernel in whole longs: on the trial Pi's 6.18 kernel, EVIOCSMASK refused 4
// bytes with EINVAL and took 8.
func TestKeys_MaskIsWholeLongs(t *testing.T) {
	if maskBytes != 8 || maskBytes*8 < evCnt {
		t.Fatalf("the mask is %d bytes", maskBytes)
	}
}
