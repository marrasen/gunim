//go:build windows && amd64

package asio

import (
	"errors"
	"fmt"
	"math"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// The methods of a driver's interface, by their place in its table,
// after the three of IUnknown.
const (
	mRelease         = 2
	mInit            = 3
	mGetErrorMessage = 6
	mStart           = 7
	mStop            = 8
	mGetChannels     = 9
	mGetLatencies    = 10
	mGetBufferSize   = 11
	mCanSampleRate   = 12
	mGetSampleRate   = 13
	mSetSampleRate   = 14
	mGetChannelInfo  = 18
	mCreateBuffers   = 19
	mDisposeBuffers  = 20
	mControlPanel    = 21
	mOutputReady     = 23
)

// The messages a driver sends through asioMessage.
const (
	msgSelectorSupported = 1
	msgEngineVersion     = 2
	msgResetRequest      = 3
	msgBufferSizeChange  = 4
	msgResyncRequest     = 5
	msgLatenciesChanged  = 6
	msgSupportsTimeInfo  = 7
)

// bufferInfo asks the driver for one channel's two buffers, and holds
// them once it has made them.
type bufferInfo struct {
	isInput int32
	channel int32
	buffers [2]uintptr
}

// channelInfo is what the driver says of one channel.
type channelInfo struct {
	channel  int32
	isInput  int32
	isActive int32
	group    int32
	typ      sampleType
	name     [32]byte
}

// callbacks are the functions the driver calls.
type callbacks struct {
	bufferSwitch, sampleRateDidChange, asioMessage, bufferSwitchTimeInfo uintptr
}

var (
	// openMu guards opening and closing; playing is the device open, as
	// the callbacks, which carry no pointer of their own, find it.
	openMu  sync.Mutex
	playing atomic.Pointer[Device]
	// cbs and infos are handed to the driver, which keeps them; as
	// package variables they stay where they are.
	cbOnce sync.Once
	cbs    callbacks
	infos  [2]bufferInfo
)

var (
	ole32                       = windows.NewLazySystemDLL("ole32.dll")
	procCoCreateInstance        = ole32.NewProc("CoCreateInstance")
	user32                      = windows.NewLazySystemDLL("user32.dll")
	procGetDesktopWindow        = user32.NewProc("GetDesktopWindow")
	procMsgWaitForMultipleObjEx = user32.NewProc("MsgWaitForMultipleObjectsEx")
	procPeekMessageW            = user32.NewProc("PeekMessageW")
	procTranslateMessage        = user32.NewProc("TranslateMessage")
	procDispatchMessageW        = user32.NewProc("DispatchMessageW")
)

// Drivers lists the ASIO drivers installed, by name.
func Drivers() ([]Driver, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\ASIO`, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("asio: listing the drivers: %w", err)
	}
	defer func() { _ = k.Close() }()
	names, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, fmt.Errorf("asio: listing the drivers: %w", err)
	}
	var out []Driver
	for _, n := range names {
		sk, err := registry.OpenKey(k, n, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		clsid, _, err := sk.GetStringValue("CLSID")
		_ = sk.Close()
		if err == nil && clsid != "" {
			out = append(out, Driver{Name: n, CLSID: clsid})
		}
	}
	slices.SortFunc(out, func(a, b Driver) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out, nil
}

// A Device is an open ASIO driver, playing. Its methods are safe from
// any goroutine.
type Device struct {
	t     *sta
	obj   uintptr
	info  Info
	typ   sampleType
	fill  func([]float32)
	reset func()
	// outs is how many channels play: two, or one for a driver of one
	// output. buf is the frames Fill fills.
	outs int
	buf  []float32
	// ready is the driver's outputReady, where it has one, which it
	// wants called once a buffer is filled.
	ready uintptr
	// made and started say the buffers are made and the driver runs.
	made, started bool
	resetOnce     sync.Once
	closeOnce     sync.Once
}

// Open opens the driver c names and starts it playing.
func Open(c Config) (*Device, error) {
	if c.Fill == nil {
		return nil, errors.New("asio: Config.Fill is nil")
	}
	openMu.Lock()
	defer openMu.Unlock()
	if playing.Load() != nil {
		return nil, ErrOpen
	}
	drivers, err := Drivers()
	if err != nil {
		return nil, err
	}
	i := slices.IndexFunc(drivers, func(d Driver) bool { return d.Name == c.Name })
	if i < 0 {
		return nil, fmt.Errorf("asio: no driver is called %q", c.Name)
	}
	clsid, err := windows.GUIDFromString(drivers[i].CLSID)
	if err != nil {
		return nil, fmt.Errorf("asio: %s: %w", c.Name, err)
	}
	t, err := newSTA()
	if err != nil {
		return nil, err
	}
	d := &Device{t: t, fill: c.Fill, reset: c.Reset}
	cbOnce.Do(makeCallbacks)
	t.run(func() { err = d.open(clsid, c) })
	if err != nil {
		t.run(d.release)
		t.stop()
		return nil, err
	}
	return d, nil
}

// open loads the driver and starts it. It runs on the device's thread.
func (d *Device) open(clsid windows.GUID, c Config) error {
	if hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsid)), 0, windows.CLSCTX_INPROC_SERVER,
		uintptr(unsafe.Pointer(&clsid)), uintptr(unsafe.Pointer(&d.obj))); hr != 0 {
		return fmt.Errorf("asio: loading %s: %w", c.Name, windows.Errno(hr))
	}
	hwnd, _, _ := procGetDesktopWindow.Call()
	if d.call(mInit, hwnd) != 1 {
		return &Error{Op: "starting " + c.Name, Message: d.message()}
	}
	var ins, outs int32
	if r := d.call(mGetChannels, uintptr(unsafe.Pointer(&ins)), uintptr(unsafe.Pointer(&outs))); r != codeOK {
		return d.fail("counting the channels", r)
	}
	if outs < 1 {
		return fmt.Errorf("asio: %s has no outputs", c.Name)
	}
	rate := d.rate()
	if c.Rate > 0 && rate != float64(c.Rate) && d.call(mCanSampleRate, uintptr(math.Float64bits(float64(c.Rate)))) == codeOK {
		// A driver that refuses still plays, at its own rate.
		_ = d.call(mSetSampleRate, uintptr(math.Float64bits(float64(c.Rate))))
		rate = d.rate()
	}
	if rate <= 0 {
		return fmt.Errorf("asio: %s plays at no rate it reports", c.Name)
	}
	var lo, hi, pref, gran int32
	if r := d.call(mGetBufferSize, uintptr(unsafe.Pointer(&lo)), uintptr(unsafe.Pointer(&hi)),
		uintptr(unsafe.Pointer(&pref)), uintptr(unsafe.Pointer(&gran))); r != codeOK {
		return d.fail("reading the buffer sizes", r)
	}
	size := fitBuffer(c.Buffer, int(lo), int(hi), int(pref), int(gran))
	ci := channelInfo{channel: 0, isInput: 0}
	if r := d.call(mGetChannelInfo, uintptr(unsafe.Pointer(&ci))); r != codeOK {
		return d.fail("reading the first output", r)
	}
	d.typ = ci.typ
	bytes, bits := d.typ.size()
	if bytes == 0 {
		return fmt.Errorf("asio: %s takes samples of type %d, which this package plays none of", c.Name, d.typ)
	}
	d.outs = min(int(outs), 2)
	infos = [2]bufferInfo{{channel: 0}, {channel: 1}}
	// A driver may send messages as it makes its buffers.
	playing.Store(d)
	if r := d.call(mCreateBuffers, uintptr(unsafe.Pointer(&infos[0])), uintptr(d.outs), uintptr(size),
		uintptr(unsafe.Pointer(&cbs))); r != codeOK {
		return d.fail("making the buffers", r)
	}
	d.made = true
	var inLat, outLat int32
	_ = d.call(mGetLatencies, uintptr(unsafe.Pointer(&inLat)), uintptr(unsafe.Pointer(&outLat)))
	d.buf = make([]float32, 2*size)
	if d.call(mOutputReady) == codeOK {
		d.ready = method(d.obj, mOutputReady)
	}
	for ch := range d.outs {
		for half := range 2 {
			clear(unsafe.Slice(ptr[byte](infos[ch].buffers[half]), size*bytes))
		}
	}
	d.info = Info{Driver: c.Name, Rate: int(math.Round(rate)), Buffer: size, MinBuffer: int(lo), MaxBuffer: int(hi),
		PreferredBuffer: int(pref), Latency: int(outLat), Bits: bits, Float: d.typ.float(), Outputs: int(outs)}
	if r := d.call(mStart); r != codeOK {
		return d.fail("starting to play", r)
	}
	d.started = true
	return nil
}

// rate returns the rate the driver is at, or 0.
func (d *Device) rate() float64 {
	var r float64
	if d.call(mGetSampleRate, uintptr(unsafe.Pointer(&r))) != codeOK {
		return 0
	}
	return r
}

// release stops the driver and lets it go. It runs on the device's
// thread.
func (d *Device) release() {
	if d.obj == 0 {
		return
	}
	if d.started {
		_ = d.call(mStop)
		d.started = false
	}
	if d.made {
		_ = d.call(mDisposeBuffers)
		d.made = false
	}
	playing.CompareAndSwap(d, nil)
	d.call(mRelease)
	d.obj = 0
}

// fail returns the error r, as the driver tells of it.
func (d *Device) fail(op string, r int32) error { return &Error{Op: op, Code: r, Message: d.message()} }

// message returns what the driver says of its last error.
func (d *Device) message() string {
	var b [128]byte
	_ = d.call(mGetErrorMessage, uintptr(unsafe.Pointer(&b[0])))
	n := 0
	for n < len(b) && b[n] != 0 {
		n++
	}
	return strings.TrimSpace(string(b[:n]))
}

// call calls method m of the driver, with integer arguments, and
// returns what it returns: a long, ASIO's 32 bits.
//
//go:uintptrescapes
func (d *Device) call(m int, args ...uintptr) int32 {
	r, _, _ := syscall.SyscallN(method(d.obj, m), append([]uintptr{d.obj}, args...)...)
	return int32(uint32(r))
}

// method returns the function at place m of obj's table.
func method(obj uintptr, m int) uintptr {
	vtbl := *ptr[unsafe.Pointer](obj)
	return *(*uintptr)(unsafe.Add(vtbl, uintptr(m)*unsafe.Sizeof(uintptr(0))))
}

// ptr returns p, an address in the driver's memory, as a pointer.
func ptr[T any](p uintptr) *T {
	return *(**T)(unsafe.Pointer(&p))
}

// Info says how the device plays.
func (d *Device) Info() Info { return d.info }

// ControlPanel opens the driver's own settings, and returns once they
// close. A change there that needs the device opened again calls
// Config.Reset.
func (d *Device) ControlPanel() error {
	var r int32
	d.t.run(func() {
		if d.obj != 0 {
			r = d.call(mControlPanel)
		}
	})
	if r != codeOK {
		return &Error{Op: "opening the control panel", Code: r}
	}
	return nil
}

// Close stops the driver and lets it go.
func (d *Device) Close() error {
	d.closeOnce.Do(func() {
		openMu.Lock()
		defer openMu.Unlock()
		d.t.run(d.release)
		d.t.stop()
	})
	return nil
}

// switchTo fills the buffers of half, 0 or 1, with what plays next.
// It runs on the driver's thread.
func (d *Device) switchTo(half int32) {
	if half != 0 && half != 1 {
		return
	}
	n := d.info.Buffer
	frames := d.buf[:2*n]
	d.fill(frames)
	if d.outs == 1 {
		mono(frames)
	}
	bytes, _ := d.typ.size()
	for ch := range d.outs {
		dst := unsafe.Slice(ptr[byte](infos[ch].buffers[half]), n*bytes)
		write(dst, frames, ch, d.typ)
	}
	if d.ready != 0 {
		_, _, _ = syscall.SyscallN(d.ready, d.obj)
	}
}

// askReset tells the device's user, once, that the driver wants to be
// opened again.
func (d *Device) askReset() {
	d.resetOnce.Do(func() {
		if d.reset != nil {
			go d.reset()
		}
	})
}

// makeCallbacks makes the functions the driver calls. A long, 32 bits,
// comes in a register of 64, the top half of which is left as it was:
// each is cut to its 32 bits.
func makeCallbacks() {
	cbs = callbacks{
		bufferSwitch: syscall.NewCallback(func(half, _ uintptr) uintptr {
			if d := playing.Load(); d != nil {
				d.switchTo(int32(uint32(half)))
			}
			return 0
		}),
		// The new rate comes as a double, in a register a callback of
		// Go's reads none of; the device is opened again to find it.
		sampleRateDidChange: syscall.NewCallback(func(uintptr) uintptr {
			if d := playing.Load(); d != nil {
				d.askReset()
			}
			return 0
		}),
		asioMessage: syscall.NewCallback(func(selector, value, _, _ uintptr) uintptr {
			switch int32(uint32(selector)) {
			case msgSelectorSupported:
				switch int32(uint32(value)) {
				case msgEngineVersion, msgResetRequest, msgBufferSizeChange, msgResyncRequest,
					msgLatenciesChanged, msgSupportsTimeInfo:
					return 1
				}
				return 0
			case msgEngineVersion:
				return 2
			case msgResetRequest, msgBufferSizeChange, msgLatenciesChanged:
				if d := playing.Load(); d != nil {
					d.askReset()
				}
				return 1
			case msgResyncRequest, msgSupportsTimeInfo:
				return 1
			}
			return 0
		}),
		bufferSwitchTimeInfo: syscall.NewCallback(func(params, half, _ uintptr) uintptr {
			if d := playing.Load(); d != nil {
				d.switchTo(int32(uint32(half)))
			}
			return params
		}),
	}
}

// sta is a thread of its own, in a single-threaded COM apartment, as
// ASIO drivers expect, that runs calls one at a time and passes on the
// window messages a driver's control panel needs between them.
type sta struct {
	calls chan func()
	wake  windows.Handle
	done  chan struct{}
}

func newSTA() (*sta, error) {
	wake, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return nil, err
	}
	s := &sta{calls: make(chan func(), 1), wake: wake, done: make(chan struct{})}
	errc := make(chan error)
	go func() {
		defer close(s.done)
		// The thread ends with the goroutine, its COM state with it.
		runtime.LockOSThread()
		if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil && !errors.Is(err, syscall.Errno(windows.S_FALSE)) {
			errc <- err
			return
		}
		defer windows.CoUninitialize()
		errc <- nil
		s.loop()
	}()
	if err := <-errc; err != nil {
		_ = windows.CloseHandle(wake)
		return nil, err
	}
	return s, nil
}

// msg is a window message, as MSG lays it out.
type msg struct {
	hwnd     uintptr
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       [2]int32
	lPrivate uint32
}

func (s *sta) loop() {
	const (
		qsAllInput         = 0x04ff
		mwmoInputAvailable = 0x0004
		pmRemove           = 0x0001
	)
	for {
		for drained := false; !drained; {
			select {
			case f, ok := <-s.calls:
				if !ok {
					return
				}
				f()
			default:
				drained = true
			}
		}
		_, _, _ = procMsgWaitForMultipleObjEx.Call(1, uintptr(unsafe.Pointer(&s.wake)), uintptr(windows.INFINITE),
			qsAllInput, mwmoInputAvailable)
		var m msg
		for {
			if r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove); r == 0 {
				break
			}
			_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
	}
}

// run calls f on the thread and waits for it.
func (s *sta) run(f func()) {
	done := make(chan struct{})
	s.calls <- func() {
		defer close(done)
		f()
	}
	_ = windows.SetEvent(s.wake)
	<-done
}

// stop ends the thread.
func (s *sta) stop() {
	close(s.calls)
	_ = windows.SetEvent(s.wake)
	<-s.done
	_ = windows.CloseHandle(s.wake)
}
