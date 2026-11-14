//go:build linux && (arm64 || amd64)

package kms

import (
	"errors"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// keyWatcher finds keyboards plugged in after Open, for VTSwitch. The
// driver turns the kernel's own keyboard handling off on its VT, so a
// keyboard it doesn't read can't switch VT at all: without this, a
// keyboard plugged in weeks after the program started could never reach
// a login on another VT.
//
// It watches the folder with inotify. The kernel makes a device's node
// as root's, and udev gives it its group a moment later, so a node that
// can't be opened yet is tried again when its mode or owner changes
// (IN_ATTRIB). A keyboard that goes away is forgotten, so the same node
// coming back is read again.
type keyWatcher struct {
	dir string
	// open opens a device and says what it is: openInput, or a test's.
	open func(path string) (*inputDev, error)
	// found starts reading a keyboard it found, and returns once the
	// keyboard has gone or the driver quits.
	found func(dev *inputDev)

	mu sync.Mutex
	// seen holds the nodes being read, or found to be no keyboard, so
	// each is opened once while it is there: with the device read from
	// it, or with none.
	seen map[string]*inputDev
}

// watchKeyboards watches dir for keyboards until stop is readable, and
// hands each new one to found. known are the nodes Open read already. It
// returns at once, with an error, where the folder can't be watched.
func watchKeyboards(dir string, known []string, stop int, open func(string) (*inputDev, error), found func(*inputDev), run func(func())) error {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return err
	}
	if _, err := unix.InotifyAddWatch(fd, dir, unix.IN_CREATE|unix.IN_ATTRIB|unix.IN_MOVED_TO|unix.IN_DELETE); err != nil {
		_ = unix.Close(fd)
		return err
	}
	w := &keyWatcher{dir: dir, open: open, found: found, seen: map[string]*inputDev{}}
	for _, p := range known {
		w.seen[p] = nil
	}
	run(func() {
		defer func() { _ = unix.Close(fd) }()
		// A keyboard plugged in between Open's look and the watch.
		paths, _ := filepath.Glob(filepath.Join(dir, "event*"))
		for _, p := range paths {
			w.try(p, run)
		}
		w.loop(fd, stop, run)
	})
	return nil
}

// loop reads inotify's events until stop is readable.
func (w *keyWatcher) loop(fd, stop int, run func(func())) {
	buf := make([]byte, 64*(unix.SizeofInotifyEvent+unix.NAME_MAX+1))
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}, {Fd: int32(stop), Events: unix.POLLIN}}
	for {
		fds[0].Revents, fds[1].Revents = 0, 0
		if _, err := unix.Poll(fds, -1); err != nil && err != unix.EINTR {
			log.Printf("gunim: kms: watching %s for keyboards: %v", w.dir, err)
			return
		}
		if fds[1].Revents != 0 {
			return
		}
		n, err := unix.Read(fd, buf)
		if err == unix.EINTR || err == unix.EAGAIN {
			continue
		}
		if err != nil || n <= 0 {
			log.Printf("gunim: kms: watching %s for keyboards: %v", w.dir, err)
			return
		}
		for o := 0; o+unix.SizeofInotifyEvent <= n; {
			ev := (*unix.InotifyEvent)(unsafe.Pointer(&buf[o]))
			name := cstr(buf[o+unix.SizeofInotifyEvent : o+unix.SizeofInotifyEvent+int(ev.Len)])
			o += unix.SizeofInotifyEvent + int(ev.Len)
			if !strings.HasPrefix(name, "event") {
				continue
			}
			p := filepath.Join(w.dir, name)
			if ev.Mask&unix.IN_DELETE != 0 {
				w.mu.Lock()
				delete(w.seen, p)
				w.mu.Unlock()
				continue
			}
			w.try(p, run)
		}
	}
}

// try opens a node not seen yet, and reads it if it is a keyboard. A node
// that can't be opened yet stays unseen, for its next change.
func (w *keyWatcher) try(p string, run func(func())) {
	w.mu.Lock()
	_, seen := w.seen[p]
	w.mu.Unlock()
	if seen {
		return
	}
	dev, err := w.open(p)
	if err != nil {
		if !errors.Is(err, unix.EACCES) && !errors.Is(err, unix.ENOENT) {
			log.Printf("gunim: kms: %s: %v", p, err)
		}
		return
	}
	w.mu.Lock()
	if _, seen := w.seen[p]; seen {
		w.mu.Unlock()
		dev.close()
		return
	}
	if !dev.keys {
		w.seen[p] = nil
		w.mu.Unlock()
		dev.close()
		return
	}
	w.seen[p] = dev
	w.mu.Unlock()
	log.Printf("gunim: kms: keyboard %q on %s, plugged in after the start", dev.name, p)
	run(func() {
		w.found(dev)
		dev.close()
		w.forget(p, dev)
	})
}

// forget lets a node be opened again once the keyboard read from it has
// gone, unless the node already holds a new one.
func (w *keyWatcher) forget(p string, dev *inputDev) {
	w.mu.Lock()
	if w.seen[p] == dev {
		delete(w.seen, p)
	}
	w.mu.Unlock()
}
