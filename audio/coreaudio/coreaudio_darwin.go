//go:build darwin && !ios

package coreaudio

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	noErr = 0

	systemObject = 1

	scopeGlobal = 0x676c6f62 // 'glob'
	scopeOutput = 0x6f757470 // 'outp'
	elementMain = 0

	propDefaultOutputDevice = 0x644f7574 // 'dOut'
	propName                = 0x6c6e616d // 'lnam'
	propDeviceIsAlive       = 0x6c69766e // 'livn'
	propBufferFrameSize     = 0x6673697a // 'fsiz'
	propBufferFrameRange    = 0x66737a23 // 'fsz#'
	propNominalSampleRate   = 0x6e737274 // 'nsrt'
	propLatency             = 0x6c746e63 // 'ltnc', of a device and of a stream
	propSafetyOffset        = 0x73616674 // 'saft'
	propStreams             = 0x73746d23 // 'stm#'
	propStreamConfiguration = 0x736c6179 // 'slay'
	propStreamFormat        = 0x73666d74 // 'sfmt'

	formatLinearPCM = 0x6c70636d // 'lpcm'
	formatIsFloat   = 1 << 0

	cfStringEncodingUTF8 = 0x08000100
)

type propertyAddress struct {
	selector, scope, element uint32
}

type valueRange struct {
	min, max float64
}

type streamDescription struct {
	sampleRate       float64
	formatID         uint32
	formatFlags      uint32
	bytesPerPacket   uint32
	framesPerPacket  uint32
	bytesPerFrame    uint32
	channelsPerFrame uint32
	bitsPerChannel   uint32
	reserved         uint32
}

// audioBuffer is an AudioBuffer: an AudioBufferList holds a count of
// them, then the buffers, from the next 8-byte boundary.
type audioBuffer struct {
	channels uint32
	bytes    uint32
	data     unsafe.Pointer
}

const bufferListHeader = 8

var (
	getPropertyData     func(id uint32, addr *propertyAddress, qualSize uint32, qual unsafe.Pointer, size *uint32, out unsafe.Pointer) int32
	getPropertyDataSize func(id uint32, addr *propertyAddress, qualSize uint32, qual unsafe.Pointer, size *uint32) int32
	setPropertyData     func(id uint32, addr *propertyAddress, qualSize uint32, qual unsafe.Pointer, size uint32, data unsafe.Pointer) int32
	addListener         func(id uint32, addr *propertyAddress, proc uintptr, client unsafe.Pointer) int32
	removeListener      func(id uint32, addr *propertyAddress, proc uintptr, client unsafe.Pointer) int32
	createIOProcID      func(dev uint32, proc uintptr, client unsafe.Pointer, id *uintptr) int32
	destroyIOProcID     func(dev uint32, id uintptr) int32
	deviceStart         func(dev uint32, id uintptr) int32
	deviceStop          func(dev uint32, id uintptr) int32
	cfStringGetCString  func(s uintptr, buf *byte, size int, encoding uint32) bool
	cfRelease           func(ref uintptr)

	// ioProc and listener are the callbacks Core Audio calls, made once:
	// purego keeps a callback for good. They play to current. Each call
	// through purego's callback path locks a mutex and allocates.
	ioProc, listener uintptr
)

var (
	loadOnce sync.Once
	loadErr  error
)

func load() error {
	loadOnce.Do(func() {
		ca, err := purego.Dlopen("/System/Library/Frameworks/CoreAudio.framework/CoreAudio", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			loadErr = err
			return
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			loadErr = err
			return
		}
		purego.RegisterLibFunc(&getPropertyData, ca, "AudioObjectGetPropertyData")
		purego.RegisterLibFunc(&getPropertyDataSize, ca, "AudioObjectGetPropertyDataSize")
		purego.RegisterLibFunc(&setPropertyData, ca, "AudioObjectSetPropertyData")
		purego.RegisterLibFunc(&addListener, ca, "AudioObjectAddPropertyListener")
		purego.RegisterLibFunc(&removeListener, ca, "AudioObjectRemovePropertyListener")
		purego.RegisterLibFunc(&createIOProcID, ca, "AudioDeviceCreateIOProcID")
		purego.RegisterLibFunc(&destroyIOProcID, ca, "AudioDeviceDestroyIOProcID")
		purego.RegisterLibFunc(&deviceStart, ca, "AudioDeviceStart")
		purego.RegisterLibFunc(&deviceStop, ca, "AudioDeviceStop")
		purego.RegisterLibFunc(&cfStringGetCString, cf, "CFStringGetCString")
		purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
		ioProc = purego.NewCallback(render)
		listener = purego.NewCallback(changed)
	})
	return loadErr
}

func get[T any](id, selector, scope uint32, out *T) bool {
	addr := propertyAddress{selector, scope, elementMain}
	size := uint32(unsafe.Sizeof(*out))
	return getPropertyData(id, &addr, 0, nil, &size, unsafe.Pointer(out)) == noErr
}

func set[T any](id, selector, scope uint32, v T) int32 {
	addr := propertyAddress{selector, scope, elementMain}
	return setPropertyData(id, &addr, 0, nil, uint32(unsafe.Sizeof(v)), unsafe.Pointer(&v))
}

// getBytes reads a property of variable size.
func getBytes(id, selector, scope uint32) []byte {
	addr := propertyAddress{selector, scope, elementMain}
	var size uint32
	if getPropertyDataSize(id, &addr, 0, nil, &size) != noErr || size == 0 {
		return nil
	}
	// Of uint64s, for the pointers and float64s the property holds.
	buf := make([]uint64, (size+7)/8)
	if getPropertyData(id, &addr, 0, nil, &size, unsafe.Pointer(&buf[0])) != noErr {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&buf[0])), size)
}

// name returns the device's name.
func name(dev uint32) string {
	var ref uintptr
	if !get(dev, propName, scopeGlobal, &ref) || ref == 0 {
		return ""
	}
	defer cfRelease(ref)
	buf := make([]byte, 256)
	if !cfStringGetCString(ref, &buf[0], len(buf), cfStringEncodingUTF8) {
		return ""
	}
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

// outputChannels returns how many channels each of the device's output
// buffers holds, in order.
func outputChannels(dev uint32) []int {
	b := getBytes(dev, propStreamConfiguration, scopeOutput)
	if len(b) < bufferListHeader {
		return nil
	}
	n := int(*(*uint32)(unsafe.Pointer(&b[0])))
	size := int(unsafe.Sizeof(audioBuffer{}))
	var chans []int
	for i := range n {
		at := bufferListHeader + i*size
		if at+size > len(b) {
			break
		}
		chans = append(chans, int((*audioBuffer)(unsafe.Pointer(&b[at])).channels))
	}
	return chans
}

// streamLatency returns the latency of the device's first output stream,
// and false where the device reports none.
func streamLatency(dev uint32) (int, bool) {
	b := getBytes(dev, propStreams, scopeOutput)
	if len(b) < 4 {
		return 0, false
	}
	var lat uint32
	if !get(*(*uint32)(unsafe.Pointer(&b[0])), propLatency, scopeGlobal, &lat) {
		return 0, false
	}
	return int(lat), true
}

// A Device is the default output device, playing.
type Device struct {
	c    Config
	dev  uint32
	proc uintptr
	info Info
	// chans is how many channels each of the device's buffers holds,
	// and stereo is Fill's room, as large as the device's largest
	// buffer.
	chans  []int
	stereo []float32
	// reset says Reset was called, or Close, so Reset is called once.
	reset atomic.Bool
	// rateMoved says the device's rate moved from Info.Rate while it
	// played.
	rateMoved atomic.Bool
}

// current is the device playing: Core Audio's callbacks play to it.
var current atomic.Pointer[Device]

var openMu sync.Mutex

// followed is the device, and the rate asked of it, whose rate moved
// under it: Open plays it at its own rate in place of setting it again.
// openMu guards it.
var followed struct {
	dev  uint32
	rate int
}

// Open opens the default output device and starts it playing, as c
// says. One device plays at a time.
func Open(c Config) (*Device, error) {
	if c.Fill == nil {
		return nil, errors.New("coreaudio: no Fill")
	}
	if err := load(); err != nil {
		return nil, fmt.Errorf("coreaudio: %w", err)
	}
	openMu.Lock()
	defer openMu.Unlock()
	if current.Load() != nil {
		return nil, errors.New("coreaudio: a device is playing already")
	}
	var dev uint32
	if !get(systemObject, propDefaultOutputDevice, scopeGlobal, &dev) || dev == 0 {
		return nil, errors.New("coreaudio: there is no output device")
	}
	d := &Device{c: c, dev: dev}
	d.info.Device = name(dev)

	var f streamDescription
	if !get(dev, propStreamFormat, scopeOutput, &f) {
		return nil, fmt.Errorf("coreaudio: %s reports no format", d.info.Device)
	}
	if f.formatID != formatLinearPCM || f.formatFlags&formatIsFloat == 0 || f.bitsPerChannel != 32 {
		return nil, fmt.Errorf("coreaudio: %s takes no 32-bit float samples", d.info.Device)
	}
	d.chans = outputChannels(dev)
	for _, n := range d.chans {
		d.info.Outputs += n
	}
	if d.info.Outputs == 0 {
		return nil, fmt.Errorf("coreaudio: %s has no outputs", d.info.Device)
	}

	if c.Rate > 0 && (followed.dev != dev || followed.rate != c.Rate) {
		followed.dev, followed.rate = 0, 0
		d.setRate(c.Rate)
	}
	var rate float64
	if !get(dev, propNominalSampleRate, scopeGlobal, &rate) || rate <= 0 {
		return nil, fmt.Errorf("coreaudio: %s reports no rate", d.info.Device)
	}
	d.info.Rate = int(math.Round(rate))

	var r valueRange
	if get(dev, propBufferFrameRange, scopeGlobal, &r) {
		d.info.MinBuffer, d.info.MaxBuffer = int(r.min), int(r.max)
	}
	if c.Buffer > 0 {
		want := c.Buffer
		if d.info.MaxBuffer > 0 {
			want = min(max(want, d.info.MinBuffer), d.info.MaxBuffer)
		}
		if st := set(dev, propBufferFrameSize, scopeGlobal, uint32(want)); st != noErr {
			d.logf("%s refuses a buffer of %d frames: %d", d.info.Device, want, st)
		}
	}
	var size uint32
	if !get(dev, propBufferFrameSize, scopeGlobal, &size) || size == 0 {
		return nil, fmt.Errorf("coreaudio: %s reports no buffer size", d.info.Device)
	}
	d.info.Buffer = int(size)
	var lat, safety uint32
	if !get(dev, propLatency, scopeOutput, &lat) || !get(dev, propSafetyOffset, scopeOutput, &safety) {
		return nil, fmt.Errorf("coreaudio: %s reports no latency", d.info.Device)
	}
	stream, ok := streamLatency(dev)
	if !ok {
		return nil, fmt.Errorf("coreaudio: %s reports no stream latency", d.info.Device)
	}
	d.info.Latency = d.info.Buffer + int(lat) + int(safety) + stream
	d.stereo = make([]float32, 2*max(d.info.Buffer, d.info.MaxBuffer))

	current.Store(d)
	if st := createIOProcID(dev, ioProc, nil, &d.proc); st != noErr {
		current.Store(nil)
		return nil, fmt.Errorf("coreaudio: AudioDeviceCreateIOProcID: %d", st)
	}
	if st := deviceStart(dev, d.proc); st != noErr {
		err := fmt.Errorf("coreaudio: AudioDeviceStart: %d", st)
		if st := destroyIOProcID(dev, d.proc); st != noErr {
			err = errors.Join(err, fmt.Errorf("coreaudio: AudioDeviceDestroyIOProcID: %d", st))
		}
		current.Store(nil)
		return nil, err
	}
	if err := d.listen(); err != nil {
		return nil, errors.Join(err, d.stop())
	}
	d.logf("%s plays at %d Hz, %d frames at a time (it allows %d to %d), %d frames of latency",
		d.info.Device, d.info.Rate, d.info.Buffer, d.info.MinBuffer, d.info.MaxBuffer, d.info.Latency)
	return d, nil
}

// setRate sets the device to rate, where it is not at it already, and
// waits a moment for it to change, as the device changes in its own
// time.
func (d *Device) setRate(rate int) {
	var now float64
	if rate <= 0 || !get(d.dev, propNominalSampleRate, scopeGlobal, &now) || int(math.Round(now)) == rate {
		return
	}
	if st := set(d.dev, propNominalSampleRate, scopeGlobal, float64(rate)); st != noErr {
		d.logf("%s refuses %d Hz: %d", d.info.Device, rate, st)
		return
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if get(d.dev, propNominalSampleRate, scopeGlobal, &now) && int(math.Round(now)) == rate {
			return
		}
	}
	d.logf("%s stays at %v Hz", d.info.Device, now)
}

// A watched is a property a change of which opens the output again.
type watched struct {
	system   bool
	selector uint32
}

// listened are the properties watched.
var listened = []watched{
	{true, propDefaultOutputDevice},
	{false, propDeviceIsAlive},
	{false, propNominalSampleRate},
	{false, propBufferFrameSize},
}

// object returns the object w is a property of.
func (d *Device) object(w watched) uint32 {
	if w.system {
		return systemObject
	}
	return d.dev
}

// listen adds the listener to the properties listened to, removing it
// from those it was added to where one fails.
func (d *Device) listen() error {
	for i, w := range listened {
		addr := propertyAddress{w.selector, scopeGlobal, elementMain}
		if st := addListener(d.object(w), &addr, listener, nil); st != noErr {
			err := fmt.Errorf("coreaudio: AudioObjectAddPropertyListener: %d", st)
			return errors.Join(err, d.unlisten(listened[:i]))
		}
	}
	return nil
}

// unlisten removes the listener from ws.
func (d *Device) unlisten(ws []watched) error {
	var errs []error
	for _, w := range ws {
		addr := propertyAddress{w.selector, scopeGlobal, elementMain}
		if st := removeListener(d.object(w), &addr, listener, nil); st != noErr {
			errs = append(errs, fmt.Errorf("coreaudio: AudioObjectRemovePropertyListener: %d", st))
		}
	}
	return errors.Join(errs...)
}

// changed is told of a change of a property listened to, and asks for
// the output to open again where it no longer plays as it opened. The
// device's own settings, made as it opened, are told of too, a moment
// later.
func changed(id, n uint32, addrs, client unsafe.Pointer) int32 {
	d := current.Load()
	if d == nil || d.c.Reset == nil {
		return noErr
	}
	var dev uint32
	var rate float64
	var size, alive uint32
	same := get(systemObject, propDefaultOutputDevice, scopeGlobal, &dev) && dev == d.dev &&
		get(d.dev, propDeviceIsAlive, scopeGlobal, &alive) && alive != 0
	rated := get(d.dev, propNominalSampleRate, scopeGlobal, &rate)
	moved := same && rated && int(math.Round(rate)) != d.info.Rate
	if moved {
		d.rateMoved.Store(true)
	}
	ok := same && rated && !moved && get(d.dev, propBufferFrameSize, scopeGlobal, &size) && int(size) == d.info.Buffer
	if !ok && !d.reset.Swap(true) {
		d.logf("%s changed: opening the output again", d.info.Device)
		d.c.Reset()
	}
	return noErr
}

// render fills the device's buffers, on the device's thread.
func render(dev uint32, now, in, inTime, out, outTime, client unsafe.Pointer) int32 {
	n := int(*(*uint32)(out))
	bufs := unsafe.Slice((*audioBuffer)(unsafe.Add(out, bufferListHeader)), n)
	d := current.Load()
	if d == nil || d.dev != dev || n == 0 || bufs[0].channels == 0 {
		silence(bufs)
		return noErr
	}
	frames := int(bufs[0].bytes) / 4 / int(bufs[0].channels)
	if 2*frames > len(d.stereo) {
		silence(bufs)
		return noErr
	}
	st := d.stereo[:2*frames]
	d.c.Fill(st)
	// The pair goes to the device's first two channels, counted across
	// its buffers, or averaged onto its one; the rest play silence.
	ch := 0
	for _, b := range bufs {
		k := int(b.channels)
		if k == 0 || b.data == nil {
			continue
		}
		s := unsafe.Slice((*float32)(b.data), int(b.bytes)/4)
		clear(s)
		for c := range k {
			src := ch + c
			if src > 1 {
				break
			}
			for i := range min(frames, len(s)/k) {
				if d.info.Outputs == 1 {
					s[i*k+c] = (st[2*i] + st[2*i+1]) / 2
				} else {
					s[i*k+c] = st[2*i+src]
				}
			}
		}
		ch += k
	}
	return noErr
}

func silence(bufs []audioBuffer) {
	for _, b := range bufs {
		if b.data != nil {
			clear(unsafe.Slice((*byte)(b.data), b.bytes))
		}
	}
}

func (d *Device) logf(format string, args ...any) {
	if d.c.Log != nil {
		d.c.Log("coreaudio: "+format, args...)
	}
}

// Info says how the device plays.
func (d *Device) Info() Info { return d.info }

// ControlPanel opens Audio MIDI Setup, where the device's own settings
// are.
func (d *Device) ControlPanel() error {
	return exec.CommandContext(context.Background(), "open", "-b", "com.apple.audio.AudioMIDISetup").Run()
}

// Close stops the device and lets it go. Fill is not called once it
// returns.
func (d *Device) Close() error {
	openMu.Lock()
	defer openMu.Unlock()
	if current.Load() != d {
		return nil
	}
	d.reset.Store(true)
	if d.rateMoved.Load() {
		followed.dev, followed.rate = d.dev, d.c.Rate
	}
	return errors.Join(d.unlisten(listened), d.stop())
}

// stop stops the device, lets its IOProc go and clears current, each
// step taken where one before it fails.
func (d *Device) stop() error {
	// Stopped from outside its thread, the device stops before
	// AudioDeviceStop returns.
	var errs []error
	if st := deviceStop(d.dev, d.proc); st != noErr {
		errs = append(errs, fmt.Errorf("coreaudio: AudioDeviceStop: %d", st))
	}
	if st := destroyIOProcID(d.dev, d.proc); st != noErr {
		errs = append(errs, fmt.Errorf("coreaudio: AudioDeviceDestroyIOProcID: %d", st))
	}
	current.Store(nil)
	return errors.Join(errs...)
}
