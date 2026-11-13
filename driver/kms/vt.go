//go:build linux

package kms

import (
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// The virtual terminal's ioctls, from linux/vt.h and linux/kd.h.
const (
	vtOpenQry     = 0x5600
	vtSetMode     = 0x5602
	vtGetState    = 0x5603
	vtRelDisp     = 0x5605
	vtActivate    = 0x5606
	vtWaitActive  = 0x5607
	vtDisallocate = 0x5608

	vtAuto    = 0
	vtProcess = 1
	vtAckAcq  = 2

	kdSetMode  = 0x4B3A
	kdText     = 0x00
	kdGraphics = 0x01
	kdGKBMode  = 0x4B44
	kdSKBMode  = 0x4B45
	kOff       = 0x04

	ttyMajor = 4
)

type vtMode struct {
	mode, waitv           int8
	relsig, acqsig, frsig int16
}

type vtStat struct {
	active, signal, state uint16
}

// vt is the virtual terminal the driver draws on. The text console draws
// on the same screen, so the driver puts its VT in graphics mode, where
// the console leaves the screen alone, and turns the VT's keyboard off,
// so keys don't reach a login behind it. It takes switching away and back
// in hand: the kernel asks before it switches, and the driver lets go of
// the screen first.
type vt struct {
	fd  int
	num int
	// prev is the VT that showed before the driver switched to its own, or
	// 0 where it switched none. opened says it opened a free one, which it
	// gives back.
	prev   int
	opened bool
	// kbMode is the VT's keyboard mode before, once kbSaved.
	kbMode  int32
	kbSaved bool
	// signals carries the kernel's asks: SIGUSR1 to switch away, SIGUSR2
	// on coming back.
	signals chan os.Signal
}

// openVT takes the VT named by which: "" for the one the program runs on,
// if it runs on one, "free" for a free one, or a number or /dev/ttyN.
// It returns nil with no error where there is no VT to take, as for a
// program started over SSH with which empty.
func openVT(which string) (*vt, error) {
	num := 0
	switch which {
	case "":
		var st unix.Stat_t
		if unix.Fstat(0, &st) != nil || unix.Major(st.Rdev) != ttyMajor || unix.Minor(st.Rdev) == 0 ||
			unix.Minor(st.Rdev) >= 64 || st.Mode&unix.S_IFMT != unix.S_IFCHR {
			return nil, nil
		}
		num = int(unix.Minor(st.Rdev))
	case "free":
	default:
		n, err := strconv.Atoi(strings.TrimPrefix(which, "/dev/tty"))
		if err != nil || n < 1 || n > 63 {
			return nil, fmt.Errorf("VT %q: want free, a number from 1 to 63, or /dev/ttyN", which)
		}
		num = n
	}
	t := &vt{fd: -1}
	tty0, err := unix.Open("/dev/tty0", unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("VT: %w", err)
	}
	defer func() { _ = unix.Close(tty0) }()
	var st vtStat
	if err := ioctl(tty0, vtGetState, unsafe.Pointer(&st)); err != nil {
		return nil, fmt.Errorf("VT: state: %w", err)
	}
	if num == 0 {
		var n int32
		if err := ioctl(tty0, vtOpenQry, unsafe.Pointer(&n)); err != nil || n < 1 {
			return nil, fmt.Errorf("VT: no free VT: %w", err)
		}
		num, t.opened = int(n), true
	}
	t.num = num
	t.fd, err = unix.Open("/dev/tty"+strconv.Itoa(num), unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("VT %d: %w", num, err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = t.close() // the error that brought it here says more
		}
	}()
	if int(st.active) != num {
		t.prev = int(st.active)
		if err := ioctlInt(t.fd, vtActivate, uintptr(num)); err != nil {
			return nil, fmt.Errorf("VT %d: switch to it: %w", num, err)
		}
		if err := ioctlInt(t.fd, vtWaitActive, uintptr(num)); err != nil {
			return nil, fmt.Errorf("VT %d: wait for it: %w", num, err)
		}
	}
	if err := ioctl(t.fd, kdGKBMode, unsafe.Pointer(&t.kbMode)); err != nil {
		return nil, fmt.Errorf("VT %d: keyboard mode: %w", num, err)
	}
	t.kbSaved = true
	if err := ioctlInt(t.fd, kdSKBMode, kOff); err != nil {
		return nil, fmt.Errorf("VT %d: keyboard off: %w", num, err)
	}
	if err := ioctlInt(t.fd, kdSetMode, kdGraphics); err != nil {
		return nil, fmt.Errorf("VT %d: graphics mode: %w", num, err)
	}
	t.signals = make(chan os.Signal, 4)
	signal.Notify(t.signals, syscall.SIGUSR1, syscall.SIGUSR2)
	m := vtMode{mode: vtProcess, relsig: int16(syscall.SIGUSR1), acqsig: int16(syscall.SIGUSR2)}
	if err := ioctl(t.fd, vtSetMode, unsafe.Pointer(&m)); err != nil {
		return nil, fmt.Errorf("VT %d: take switching: %w", num, err)
	}
	ok = true
	return t, nil
}

// activate switches to VT n, as Ctrl+Alt+Fn would.
func (t *vt) activate(n int) error {
	return ioctlInt(t.fd, vtActivate, uintptr(n))
}

// released tells the kernel the driver has let go of the screen, and the
// switch away can go ahead.
func (t *vt) released() error { return ioctlInt(t.fd, vtRelDisp, 1) }

// acquired tells the kernel the driver has the screen again.
func (t *vt) acquired() error { return ioctlInt(t.fd, vtRelDisp, vtAckAcq) }

// close puts the VT back as the driver found it, switches back to the VT
// that showed before, and gives back a VT it opened.
func (t *vt) close() error {
	if t == nil || t.fd < 0 {
		return nil
	}
	var errs []error
	if t.signals != nil {
		signal.Stop(t.signals)
	}
	m := vtMode{mode: vtAuto}
	errs = append(errs, ioctl(t.fd, vtSetMode, unsafe.Pointer(&m)), ioctlInt(t.fd, kdSetMode, kdText))
	if t.kbSaved {
		errs = append(errs, ioctlInt(t.fd, kdSKBMode, uintptr(t.kbMode)))
	}
	if t.prev > 0 {
		errs = append(errs, ioctlInt(t.fd, vtActivate, uintptr(t.prev)), ioctlInt(t.fd, vtWaitActive, uintptr(t.prev)))
	}
	errs = append(errs, unix.Close(t.fd))
	t.fd = -1
	if t.opened && t.prev > 0 {
		if tty0, err := unix.Open("/dev/tty0", unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOCTTY, 0); err == nil {
			// Busy where another program opened it meanwhile, which is
			// fine: it is theirs then.
			if err := ioctlInt(tty0, vtDisallocate, uintptr(t.num)); err != nil && !errors.Is(err, unix.EBUSY) {
				errs = append(errs, err)
			}
			_ = unix.Close(tty0)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("VT %d: put back: %w", t.num, err)
	}
	return nil
}

// vtKey returns the VT that Ctrl+Alt with key switches to, or 0.
func vtKey(key uint16) int {
	switch {
	case key >= keyF1 && key <= keyF10:
		return int(key-keyF1) + 1
	case key == keyF11:
		return 11
	case key == keyF12:
		return 12
	}
	return 0
}
