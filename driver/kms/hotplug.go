//go:build linux && (arm64 || amd64)

package kms

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
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
//
// A keyboard whose reader ends while its node is still there, as after a
// transient POLLERR, is opened again from the same node: after
// keyReopenFirst, then twice as long each time, up to keyReopenMax. A
// reader that ran for keyReopenReset starts the count again. It stops once
// the node has gone or the driver quits.
type keyWatcher struct {
	dir string
	// stop is the driver's stop: readable once it quits.
	stop int
	// open opens a device and says what it is: openInput, or a test's.
	open func(path string) (*inputDev, error)
	// found starts reading a keyboard it found, and returns once the
	// keyboard has gone, which it reports, or the driver quits.
	found func(dev *inputDev) (gone bool)

	mu sync.Mutex
	// seen holds the nodes being read, or found to be no keyboard, so
	// each is opened once while it is there: with the device read from
	// it, or with none.
	seen map[string]*inputDev
}

// watchMask is what the watch on the folder hears: a node made, renamed
// in, given its group, or removed. The kernel adds IN_Q_OVERFLOW and
// IN_IGNORED itself.
const watchMask = unix.IN_CREATE | unix.IN_ATTRIB | unix.IN_MOVED_TO | unix.IN_DELETE

// keyWatchRetry is how often the watcher looks for the folder while it
// isn't there, as before the kernel's input core has made it.
var keyWatchRetry = 5 * time.Second

// How soon a keyboard whose reader ended while its node stays is opened
// again: first after keyReopenFirst, then twice as long each time, up to
// keyReopenMax. A reader that ran for keyReopenReset starts again at
// keyReopenFirst. Tests shorten them.
var (
	keyReopenFirst = time.Second
	keyReopenMax   = 30 * time.Second
	keyReopenReset = time.Minute
)

// watchKeyboards watches dir for keyboards until stop is readable, and
// hands each new one to found. keys are the keyboards Open found: the
// watcher reads them through found from now on, and closes them as it does
// the ones it finds. known are other nodes Open read already, as the touch
// screen's. A folder that isn't there yet is looked for again every
// keyWatchRetry. It returns an error only where inotify itself fails, and
// then has taken none of keys.
func watchKeyboards(dir string, keys []*inputDev, known []string, stop int, open func(string) (*inputDev, error), found func(*inputDev) bool, run func(func())) error {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return err
	}
	_, err = unix.InotifyAddWatch(fd, dir, watchMask)
	if err != nil && !errors.Is(err, unix.ENOENT) {
		_ = unix.Close(fd)
		return err
	}
	watching := err == nil
	w := &keyWatcher{dir: dir, stop: stop, open: open, found: found, seen: map[string]*inputDev{}}
	for _, p := range known {
		w.seen[p] = nil
	}
	for _, k := range keys {
		w.seen[k.path] = k
		w.read(k, run)
	}
	run(func() {
		defer func() { _ = unix.Close(fd) }()
		if !watching && !w.waitForDir(fd, stop) {
			return
		}
		// A keyboard plugged in between Open's look and the watch.
		w.rescan(run)
		w.loop(fd, stop, run)
	})
	return nil
}

// waitForDir looks for the folder every keyWatchRetry, and watches it once
// it is there. It returns false where stop came first, or the watch failed
// in another way.
func (w *keyWatcher) waitForDir(fd, stop int) bool {
	log.Printf("gunim: kms: %s isn't there; looking for it every %v, for keyboards plugged in later", w.dir, keyWatchRetry)
	fds := []unix.PollFd{{Fd: int32(stop), Events: unix.POLLIN}}
	for {
		fds[0].Revents = 0
		n, err := unix.Poll(fds, int(keyWatchRetry/time.Millisecond))
		if err != nil && err != unix.EINTR {
			log.Printf("gunim: kms: watching %s for keyboards: %v", w.dir, err)
			return false
		}
		if n > 0 && fds[0].Revents != 0 {
			return false
		}
		_, err = unix.InotifyAddWatch(fd, w.dir, watchMask)
		switch {
		case err == nil:
			log.Printf("gunim: kms: %s is there now; watching it for keyboards", w.dir)
			return true
		case !errors.Is(err, unix.ENOENT):
			log.Printf("gunim: kms: watching %s for keyboards: %v", w.dir, err)
			return false
		}
	}
}

// rescan tries each event node in the folder.
func (w *keyWatcher) rescan(run func(func())) {
	paths, _ := filepath.Glob(filepath.Join(w.dir, "event*"))
	for _, p := range paths {
		w.try(p, run)
	}
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
			if w.handle(ev.Mask, name, run) {
				// The folder went, as /dev/input never does on its own:
				// look for it again, and start afresh once it is back.
				if !w.waitForDir(fd, stop) {
					return
				}
				w.rescanAfresh(run)
			}
		}
	}
}

// rescanAfresh forgets every node read so far that has gone, and tries the
// folder again, as after it was watched anew.
func (w *keyWatcher) rescanAfresh(run func(func())) {
	w.mu.Lock()
	for p := range w.seen {
		if _, err := os.Lstat(p); err != nil {
			delete(w.seen, p)
		}
	}
	w.mu.Unlock()
	w.rescan(run)
}

// handle takes one inotify event. It says watchGone where the kernel
// dropped the folder's watch, as when the folder itself went.
func (w *keyWatcher) handle(mask uint32, name string, run func(func())) (watchGone bool) {
	switch {
	case mask&unix.IN_Q_OVERFLOW != 0:
		// The kernel's queue overflowed, and some events are lost: look at
		// every node again.
		log.Printf("gunim: kms: watching %s for keyboards: events were lost; looking at every node again", w.dir)
		w.rescan(run)
		return false
	case mask&unix.IN_IGNORED != 0:
		return true
	case !strings.HasPrefix(name, "event"):
		return false
	}
	p := filepath.Join(w.dir, name)
	if mask&unix.IN_DELETE != 0 {
		w.mu.Lock()
		delete(w.seen, p)
		w.mu.Unlock()
		return false
	}
	w.try(p, run)
	return false
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
	w.read(dev, run)
}

// read reads a keyboard whose node seen holds, and closes it once its
// reader ends. Where the reader ended as the device went, and the node
// stays, and seen still holds the keyboard, it
// opens the node again and reads it anew, backing off as it goes; see
// keyWatcher. The keyboard stays in seen while it waits, so nothing else
// opens the node meanwhile.
func (w *keyWatcher) read(dev *inputDev, run func(func())) {
	run(func() {
		p := dev.path
		wait := keyReopenFirst
		// quiet says the tries are keyReopenMax apart already, and logged
		// once as such: a device that keeps failing doesn't fill the log.
		quiet := false
		for {
			began := time.Now()
			gone := w.found(dev)
			dev.close()
			if !gone {
				// The driver quits: Close writes stop only after the
				// readers see quit, so stop alone can't tell yet.
				w.forget(p, dev)
				return
			}
			if time.Since(began) >= keyReopenReset {
				wait, quiet = keyReopenFirst, false
			}
			for {
				if !w.sleep(0) || !w.ours(p, dev) {
					w.forget(p, dev)
					return
				}
				switch {
				case wait < keyReopenMax:
					log.Printf("gunim: kms: %s: the reader ended, but the node is still there; reading it again in %v", p, wait)
				case !quiet:
					log.Printf("gunim: kms: %s: the reader keeps ending; reading it again every %v, without a word each time", p, wait)
					quiet = true
				}
				if !w.sleep(wait) {
					w.forget(p, dev)
					return
				}
				wait = min(2*wait, keyReopenMax)
				next, again := w.reopen(p, dev, quiet)
				if next != nil {
					dev = next
					break
				}
				if !again {
					return
				}
			}
		}
	})
}

// ours says whether seen still holds dev for p, and the node is there.
func (w *keyWatcher) ours(p string, dev *inputDev) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.seen[p] != dev {
		return false
	}
	_, err := os.Lstat(p)
	return err == nil
}

// reopen opens p again in old's place, where seen still holds old: the
// keyboard to read next, or nil with again saying whether to try once
// more later. It opens with mu held, so try can't open the node meanwhile.
// quiet leaves out the log of a keyboard read again.
func (w *keyWatcher) reopen(p string, old *inputDev, quiet bool) (dev *inputDev, again bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.seen[p] != old {
		// Removed, or a new device in its place: that one is read already.
		return nil, false
	}
	dev, err := w.open(p)
	switch {
	case errors.Is(err, unix.ENOENT):
		delete(w.seen, p)
		return nil, false
	case err != nil:
		if !quiet {
			log.Printf("gunim: kms: %s: %v", p, err)
		}
		return nil, true
	case !dev.keys:
		w.seen[p] = nil
		dev.close()
		return nil, false
	}
	w.seen[p] = dev
	if !quiet {
		log.Printf("gunim: kms: keyboard %q on %s, read again", dev.name, p)
	}
	return dev, false
}

// sleep waits for d, and says false where the driver quit meanwhile, or
// had quit already.
func (w *keyWatcher) sleep(d time.Duration) bool {
	fds := []unix.PollFd{{Fd: int32(w.stop), Events: unix.POLLIN}}
	for deadline := time.Now().Add(d); ; {
		left := time.Until(deadline)
		if left < 0 {
			left = 0
		}
		fds[0].Revents = 0
		n, err := unix.Poll(fds, int((left+time.Millisecond-1)/time.Millisecond))
		if err == unix.EINTR {
			continue
		}
		if err != nil || n > 0 {
			return false
		}
		if left == 0 || time.Now().After(deadline) {
			return true
		}
	}
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
