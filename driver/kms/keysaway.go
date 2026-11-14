//go:build linux && (arm64 || amd64)

package kms

import (
	"log"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// While another VT shows, the driver reads no keyboard. The kernel's own
// keyboard handling works there, Ctrl+Alt+F1 back included, so the driver
// has no need of the keys, and they are someone else's: the password typed
// at a login on tty2 too.
//
// So as its VT goes, the driver sets an event mask on each keyboard
// (EVIOCSMASK, by event type), and the kernel queues nothing at all for
// it: no keys, and no scan codes. It drains what was queued before. As its
// VT comes back, it clears the mask, and reads which keys are down from
// the kernel, as Ctrl and Alt may have changed meanwhile. A keyboard found
// while away is masked as it is found.

var (
	eviocsmask = iow('E', 0x93, unsafe.Sizeof(inputMask{}))
)

func eviocgkey(n uintptr) uintptr { return ioc(iocRead, 'E', 0x18, n) }

// inputMask is struct input_mask. Type 0 masks event types themselves.
type inputMask struct {
	typ, codesSize uint32
	codesPtr       uint64
}

// evCnt is EV_CNT, the event types: 32 bits.
const evCnt = 0x20

// maskBytes is the mask's size: EV_CNT bits, in whole longs, as the kernel
// reads it. A 6.18 kernel refuses 4 bytes on arm64 with EINVAL, and takes 8.
const maskBytes = (evCnt + 8*int(unsafe.Sizeof(uintptr(0))) - 1) / (8 * int(unsafe.Sizeof(uintptr(0)))) * int(unsafe.Sizeof(uintptr(0)))

// setEventMask lets the device's events through to this file, or none.
func setEventMask(dev *inputDev, mute bool) error {
	bits := make([]byte, maskBytes)
	if !mute {
		for i := range bits {
			bits[i] = 0xff
		}
	}
	m := inputMask{typ: 0, codesSize: uint32(len(bits)), codesPtr: uint64(uintptr(unsafe.Pointer(&bits[0])))}
	err := ioctl(dev.fd, eviocsmask, unsafe.Pointer(&m))
	runtime.KeepAlive(bits)
	return err
}

// modifiers reads from the kernel whether Ctrl and Alt are down on the
// device. A device that can't say has neither down.
func (dev *inputDev) modifiers() (ctrl, alt bool) {
	keys := make([]byte, 96)
	if ioctl(dev.fd, eviocgkey(uintptr(len(keys))), unsafe.Pointer(&keys[0])) != nil {
		return false, false
	}
	return bitSet(keys, keyLeftCtrl) || bitSet(keys, keyRightCtrl), bitSet(keys, keyLeftAlt) || bitSet(keys, keyRightAlt)
}

// addKeyboard counts a keyboard being read, and masks it at once where
// another VT shows.
func (d *Driver) addKeyboard(dev *inputDev) {
	d.keysMu.Lock()
	defer d.keysMu.Unlock()
	if d.keyDevs == nil {
		d.keyDevs = map[*inputDev]struct{}{}
	}
	d.keyDevs[dev] = struct{}{}
	if d.keysAway {
		d.muteKeyboard(dev, true)
	}
}

// dropKeyboard forgets a keyboard no longer read.
func (d *Driver) dropKeyboard(dev *inputDev) {
	d.keysMu.Lock()
	delete(d.keyDevs, dev)
	d.keysMu.Unlock()
}

// setKeysAway masks every keyboard as another VT takes the screen, or
// clears the masks as the driver's VT comes back.
func (d *Driver) setKeysAway(away bool) {
	d.keysMu.Lock()
	defer d.keysMu.Unlock()
	if d.keysAway == away {
		return
	}
	d.keysAway = away
	for dev := range d.keyDevs {
		d.muteKeyboard(dev, away)
	}
}

// muteKeyboard masks or clears one keyboard. It runs with keysMu held.
func (d *Driver) muteKeyboard(dev *inputDev, mute bool) {
	mask := d.maskKeys
	if mask == nil {
		mask = setEventMask
	}
	if mute {
		dev.muted.Store(true)
		if err := mask(dev, true); err != nil {
			// A kernel before 4.4 has no EVIOCSMASK. The reader still drops
			// what it reads while muted, but the keys reach this process.
			log.Printf("gunim: kms: %s: can't stop the kernel queueing keys while another VT shows: %v", dev.path, err)
		}
		drain(dev.fd)
		return
	}
	// A mask left in place would leave the keyboard deaf on the driver's own
	// VT, Ctrl+Alt+Fn too: try again, and then open the device afresh, as a
	// new file has no mask.
	if err := mask(dev, false); err != nil {
		log.Printf("gunim: kms: %s: clearing the mask: %v; trying again", dev.path, err)
		if err := mask(dev, false); err != nil {
			reopen := d.reopenKeys
			if reopen == nil {
				reopen = reopenInput
			}
			if err := reopen(dev); err != nil {
				log.Printf("gunim: kms: %s: can't read the keyboard again: %v", dev.path, err)
			}
		}
	}
	// The reader reads which keys are down before it reads keys again.
	dev.resumed.Add(1)
	dev.muted.Store(false)
}

// reopenInput opens the device again in place of the file the driver
// reads: the reader polls the same descriptor, now a new file with no mask.
func reopenInput(dev *inputDev) error {
	fd, err := unix.Open(dev.path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer func() { _ = unix.Close(fd) }()
	return unix.Dup3(fd, dev.fd, unix.O_CLOEXEC)
}
