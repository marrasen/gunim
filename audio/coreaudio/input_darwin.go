//go:build darwin && !ios

package coreaudio

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// The inputs record by an IOProc of their own, as the output plays: the
// device's thread hands each buffer on as it fills. Each input is a
// slot of inputs, which the one callback, made once, finds by the
// client data Core Audio hands it back.

var (
	// createInputProcID is AudioDeviceCreateIOProcID, its client data a
	// slot's number.
	createInputProcID func(dev uint32, proc uintptr, client uintptr, id *uintptr) int32
	// recordProc is the callback the inputs record by.
	recordProc uintptr
)

// maxInputs is how many inputs record at once, at most.
const maxInputs = 64

// inputs are the inputs recording, by slot; slot n is client data n+1.
// openMu guards the slots taken and let go.
var inputs [maxInputs]atomic.Pointer[Input]

// ErrLost is the error of an input whose device went away, stopped
// recording, or moved to another rate or another layout.
var ErrLost = errors.New("coreaudio: the input went away")

// lost is how long a device may hand on nothing before the input counts
// it gone.
const lost = 2 * time.Second

// audioTimeStamp is an AudioTimeStamp.
type audioTimeStamp struct {
	sampleTime    float64
	hostTime      uint64
	rateScalar    float64
	wordClockTime uint64
	smpte         [24]byte
	flags         uint32
	reserved      uint32
}

const timeStampSampleTimeValid = 1 << 0

// An Input is a device recording.
type Input struct {
	c    InputConfig
	dev  uint32
	proc uintptr
	slot int
	info InputInfo
	// chans is how many channels each of the device's buffers holds, and
	// frames the room the buffers are interleaved into, as large as the
	// device's largest buffer.
	chans  []int
	frames []float32
	// latency is the device's own and its stream's, in frames: a sound
	// reached the input that long before the sample time it is stamped
	// with.
	latency int
	// last is when the device last handed on a buffer, in nanoseconds
	// of the Unix clock.
	last    atomic.Int64
	err     atomic.Pointer[error]
	quit    chan struct{}
	done    chan struct{}
	watched chan struct{}
	ending  sync.Once
	closing sync.Once
}

// inputFollowed are the devices, and the rates asked of them, whose
// rate moved under an input: OpenInput records them at their own rate
// in place of setting it again, so it fights no other application.
// openMu guards it.
var inputFollowed = map[uint32]int{}

// Inputs lists the devices that record.
func Inputs() ([]InputDevice, error) {
	if err := load(); err != nil {
		return nil, fmt.Errorf("coreaudio: %w", err)
	}
	var def uint32
	_ = get(systemObject, propDefaultInputDevice, scopeGlobal, &def)
	var out []InputDevice
	for _, dev := range devices() {
		ch := sum(bufferChannels(dev, scopeInput))
		if ch == 0 {
			continue
		}
		d := InputDevice{UID: stringOf(dev, propDeviceUID, scopeGlobal, elementMain), Name: name(dev), Channels: ch,
			Default: dev == def}
		var rate float64
		if get(dev, propNominalSampleRate, scopeGlobal, &rate) {
			d.Rate = int(math.Round(rate))
		}
		d.ChannelNames = channelNames(dev, ch)
		out = append(out, d)
	}
	return out, nil
}

// devices returns the system's devices.
func devices() []uint32 {
	b := getBytes(systemObject, propDevices, scopeGlobal)
	out := make([]uint32, len(b)/4)
	for i := range out {
		out[i] = *(*uint32)(unsafe.Pointer(&b[4*i]))
	}
	return out
}

// channelNames returns what device dev calls its ch input channels, or
// nil where it names none.
func channelNames(dev uint32, ch int) []string {
	names := make([]string, ch)
	named := false
	for i := range names {
		names[i] = stringOf(dev, propElementName, scopeInput, uint32(i+1))
		named = named || names[i] != ""
	}
	if !named {
		return nil
	}
	return names
}

func sum(xs []int) int {
	n := 0
	for _, x := range xs {
		n += x
	}
	return n
}

// findInput returns the device of UID uid, or the default input for "".
func findInput(uid string) (uint32, error) {
	if uid == "" {
		var dev uint32
		if !get(systemObject, propDefaultInputDevice, scopeGlobal, &dev) || dev == 0 {
			return 0, errors.New("coreaudio: there is no input device")
		}
		return dev, nil
	}
	for _, dev := range devices() {
		if stringOf(dev, propDeviceUID, scopeGlobal, elementMain) == uid {
			return dev, nil
		}
	}
	return 0, fmt.Errorf("coreaudio: there is no input %q", uid)
}

// OpenInput opens a device that records, and starts it, as c says.
// Several inputs record at once, of one device or of several.
func OpenInput(c InputConfig) (*Input, error) {
	if c.Record == nil {
		return nil, errors.New("coreaudio: no Record")
	}
	if err := load(); err != nil {
		return nil, fmt.Errorf("coreaudio: %w", err)
	}
	if err := mayRecord(); err != nil {
		return nil, err
	}
	openMu.Lock()
	defer openMu.Unlock()
	dev, err := findInput(c.Device)
	if err != nil {
		return nil, err
	}
	in := &Input{c: c, dev: dev, slot: -1, quit: make(chan struct{}), done: make(chan struct{}), watched: make(chan struct{})}
	in.info.UID, in.info.Device = stringOf(dev, propDeviceUID, scopeGlobal, elementMain), name(dev)
	for i := range inputs {
		if inputs[i].Load() == nil {
			in.slot = i
			break
		}
	}
	if in.slot < 0 {
		return nil, fmt.Errorf("coreaudio: %d inputs record already", maxInputs)
	}

	var f streamDescription
	if !get(dev, propStreamFormat, scopeInput, &f) {
		return nil, fmt.Errorf("coreaudio: %s reports no input format", in.info.Device)
	}
	if f.formatID != formatLinearPCM || f.formatFlags&formatIsFloat == 0 || f.bitsPerChannel != 32 {
		return nil, fmt.Errorf("coreaudio: %s records no 32-bit float samples", in.info.Device)
	}
	in.chans = bufferChannels(dev, scopeInput)
	in.info.Channels = sum(in.chans)
	if in.info.Channels == 0 {
		return nil, fmt.Errorf("coreaudio: %s has no inputs", in.info.Device)
	}

	// The device the output plays through keeps its rate and its buffer:
	// either set here would open the output again.
	out := current.Load()
	playing := out != nil && out.dev == dev
	if c.Rate > 0 && !playing && inputFollowed[dev] != c.Rate {
		delete(inputFollowed, dev)
		setRate(dev, c.Rate, in.info.Device, in.logf)
	}
	var rate float64
	if !get(dev, propNominalSampleRate, scopeGlobal, &rate) || rate <= 0 {
		return nil, fmt.Errorf("coreaudio: %s reports no rate", in.info.Device)
	}
	in.info.Rate = int(math.Round(rate))

	var r valueRange
	_ = get(dev, propBufferFrameRange, scopeGlobal, &r)
	if c.Buffer > 0 && !playing {
		want := c.Buffer
		if r.max > 0 {
			want = min(max(want, int(r.min)), int(r.max))
		}
		if st := set(dev, propBufferFrameSize, scopeGlobal, uint32(want)); st != noErr {
			in.logf("%s refuses a buffer of %d frames: %d", in.info.Device, want, st)
		}
	}
	var size uint32
	if !get(dev, propBufferFrameSize, scopeGlobal, &size) || size == 0 {
		return nil, fmt.Errorf("coreaudio: %s reports no buffer size", in.info.Device)
	}
	in.info.Buffer = int(size)
	var lat, safety uint32
	if !get(dev, propLatency, scopeInput, &lat) || !get(dev, propSafetyOffset, scopeInput, &safety) {
		return nil, fmt.Errorf("coreaudio: %s reports no input latency", in.info.Device)
	}
	stream, _ := firstStreamLatency(dev, scopeInput)
	in.latency = int(lat) + stream
	in.info.Latency = in.info.Buffer + in.latency + int(safety)
	// The buffer may grow as it plays, as the output sets its own, to
	// the most the device allows.
	in.frames = make([]float32, in.info.Channels*max(in.info.Buffer, int(r.max), 4096))

	inputs[in.slot].Store(in)
	if st := createInputProcID(dev, recordProc, uintptr(in.slot+1), &in.proc); st != noErr {
		inputs[in.slot].Store(nil)
		return nil, fmt.Errorf("coreaudio: AudioDeviceCreateIOProcID: %d", st)
	}
	in.last.Store(time.Now().UnixNano())
	if st := deviceStart(dev, in.proc); st != noErr {
		err := fmt.Errorf("coreaudio: AudioDeviceStart: %d", st)
		if st := destroyIOProcID(dev, in.proc); st != noErr {
			err = errors.Join(err, fmt.Errorf("coreaudio: AudioDeviceDestroyIOProcID: %d", st))
		}
		inputs[in.slot].Store(nil)
		return nil, err
	}
	go in.watch()
	in.logf("%s records %d channels at %d Hz, %d frames at a time, %d frames of latency",
		in.info.Device, in.info.Channels, in.info.Rate, in.info.Buffer, in.info.Latency)
	return in, nil
}

// record hands the device's buffers on, on the device's thread.
func record(dev uint32, now, data, inTime, out, outTime unsafe.Pointer, client uintptr) int32 {
	if client == 0 || client > maxInputs || data == nil {
		return noErr
	}
	in := inputs[client-1].Load()
	if in == nil || in.dev != dev {
		return noErr
	}
	n := int(*(*uint32)(data))
	bufs := unsafe.Slice((*audioBuffer)(unsafe.Add(data, bufferListHeader)), n)
	frames := -1
	ch := 0
	for _, b := range bufs {
		if b.channels == 0 {
			continue
		}
		k := int(b.bytes) / 4 / int(b.channels)
		if frames < 0 || k < frames {
			frames = k
		}
		ch += int(b.channels)
	}
	if frames <= 0 || ch != in.info.Channels || frames*ch > len(in.frames) {
		return noErr
	}
	in.last.Store(time.Now().UnixNano())
	dst := in.frames[:frames*ch]
	// The buffers' channels, one after another, interleaved into one
	// frame; a buffer with no data records silence.
	at := 0
	for _, b := range bufs {
		k := int(b.channels)
		if k == 0 {
			continue
		}
		if b.data == nil {
			for i := range frames {
				clear(dst[i*ch+at : i*ch+at+k])
			}
		} else {
			src := unsafe.Slice((*float32)(b.data), int(b.bytes)/4)
			if k == ch {
				copy(dst, src[:frames*ch])
			} else {
				for i := range frames {
					copy(dst[i*ch+at:i*ch+at+k], src[i*k:i*k+k])
				}
			}
		}
		at += k
	}
	// The first frame was stamped with its sample time as the device
	// took it in, the device's and its stream's latency after the sound
	// reached the input; now is the device's time as it handed it on.
	ago := in.latency + in.info.Buffer + frames
	if now != nil && inTime != nil {
		t, nt := (*audioTimeStamp)(inTime), (*audioTimeStamp)(now)
		if t.flags&nt.flags&timeStampSampleTimeValid != 0 {
			ago = in.latency + int(math.Round(nt.sampleTime-t.sampleTime))
		}
	}
	in.c.Record(dst, ago)
	return noErr
}

// watch ends the input as its device goes away, stops handing on
// buffers, or moves under it, until Close.
func (in *Input) watch() {
	defer close(in.watched)
	t := time.NewTicker(250 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-in.quit:
			return
		case <-t.C:
		}
		var alive uint32
		var rate float64
		switch {
		case !get(in.dev, propDeviceIsAlive, scopeGlobal, &alive) || alive == 0:
			in.lose("went away")
		case !get(in.dev, propNominalSampleRate, scopeGlobal, &rate) || int(math.Round(rate)) != in.info.Rate:
			if in.c.Rate > 0 {
				openMu.Lock()
				inputFollowed[in.dev] = in.c.Rate
				openMu.Unlock()
			}
			in.lose(fmt.Sprintf("moved to %v Hz", rate))
		case !slices.Equal(bufferChannels(in.dev, scopeInput), in.chans):
			in.lose("records other channels now")
		case time.Since(time.Unix(0, in.last.Load())) > lost:
			in.lose("hands on nothing")
		default:
			continue
		}
		return
	}
}

// lose ends the input, as its device did why.
func (in *Input) lose(why string) {
	in.ending.Do(func() {
		in.logf("%s %s", in.info.Device, why)
		err := ErrLost
		in.err.Store(&err)
		close(in.done)
	})
}

func (in *Input) logf(format string, args ...any) {
	if in.c.Log != nil {
		in.c.Log("coreaudio: "+format, args...)
	}
}

// Info says how the input records.
func (in *Input) Info() InputInfo { return in.info }

// Done returns a channel closed once the input stops: closed, or its
// device lost. [Input.Err] tells which.
func (in *Input) Done() <-chan struct{} { return in.done }

// Err returns [ErrLost] once the input stopped on its own, and nil
// before then or after Close.
func (in *Input) Err() error {
	if e := in.err.Load(); e != nil {
		return *e
	}
	return nil
}

// Close stops the input and lets the device go. Record is not called
// once it returns.
func (in *Input) Close() error {
	var err error
	in.closing.Do(func() {
		close(in.quit)
		<-in.watched
		openMu.Lock()
		defer openMu.Unlock()
		// Stopped from outside its thread, the device stops before
		// AudioDeviceStop returns.
		var errs []error
		if st := deviceStop(in.dev, in.proc); st != noErr {
			errs = append(errs, fmt.Errorf("coreaudio: AudioDeviceStop: %d", st))
		}
		if st := destroyIOProcID(in.dev, in.proc); st != noErr {
			errs = append(errs, fmt.Errorf("coreaudio: AudioDeviceDestroyIOProcID: %d", st))
		}
		inputs[in.slot].Store(nil)
		in.ending.Do(func() { close(in.done) })
		err = errors.Join(errs...)
	})
	return err
}

// The application's leave to record, as the system keeps it: what
// AVCaptureDevice's authorizationStatusForMediaType: returns.
const (
	authNotDetermined = 0
	authRestricted    = 1
	authDenied        = 2
)

var (
	authOnce sync.Once
	// avCaptureDevice is the class, and avMediaTypeAudio the media type
	// asked of it; zero where AVFoundation would not load.
	avCaptureDevice  objc.ID
	avMediaTypeAudio objc.ID
)

// mayRecord returns ErrDenied where the system lets the application
// record nothing; Core Audio would hand on silence. Where it has not
// been asked yet, it asks, and the system shows its question as the
// input opens: what comes in is silent until it is answered.
func mayRecord() error {
	authOnce.Do(func() {
		av, err := purego.Dlopen("/System/Library/Frameworks/AVFoundation.framework/AVFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		sym, err := purego.Dlsym(av, "AVMediaTypeAudio")
		if err != nil || sym == 0 {
			return
		}
		avMediaTypeAudio = **(**objc.ID)(unsafe.Pointer(&sym))
		avCaptureDevice = objc.ID(objc.GetClass("AVCaptureDevice"))
	})
	if avCaptureDevice == 0 || avMediaTypeAudio == 0 {
		return nil
	}
	switch objc.Send[int](avCaptureDevice, objc.RegisterName("authorizationStatusForMediaType:"), avMediaTypeAudio) {
	case authRestricted, authDenied:
		return ErrDenied
	case authNotDetermined:
		ask()
	}
	return nil
}

var asked atomic.Bool

// ask asks the system for leave to record, once, and answers nothing:
// a recording made before the answer is silent.
func ask() {
	if asked.Swap(true) {
		return
	}
	answered := objc.NewBlock(func(objc.Block, bool) {})
	avCaptureDevice.Send(objc.RegisterName("requestAccessForMediaType:completionHandler:"), avMediaTypeAudio, answered)
}
