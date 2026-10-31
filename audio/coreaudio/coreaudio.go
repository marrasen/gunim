// Package coreaudio plays through macOS's default output device by Core
// Audio's own callback: the device asks for each buffer as it takes it,
// on its own thread, as an ASIO driver does on Windows. Nothing queues
// between the two, so a sound is heard a buffer and the device's own
// latency after it is made: under five milliseconds at 128 frames.
//
// The buffer is the process's own: other applications playing through
// the same device keep theirs. On other systems [Open] returns
// [ErrUnsupported].
//
// It records too: [Inputs] lists the devices that record, and
// [OpenInput] records one the same way, its thread handing on each
// buffer as it comes in.
package coreaudio

import "errors"

// ErrUnsupported is returned by [Open] and [OpenInput] where Core Audio
// is not.
var ErrUnsupported = errors.New("coreaudio: not supported on this system")

// ErrDenied is returned by [OpenInput] where the system lets the
// application record nothing. Recording would hand on silence.
var ErrDenied = errors.New("coreaudio: this application may not record: allow it in System Settings, " +
	"under Privacy & Security, Microphone")

// Config says how to open the output.
type Config struct {
	// Rate is the rate to play at, in frames a second. The device is set
	// to it, as a DAW sets it, which the other applications playing
	// through it are converted to. A rate the device refuses plays at
	// the device's own: [Info].Rate tells. Zero keeps the device's. Where
	// the device's rate then moves, as another application sets it, Reset
	// is called and Open plays at the device's new rate while asked the
	// same rate again.
	Rate int
	// Buffer is how many frames the device takes at a time, brought into
	// the range it allows. Zero keeps the size it has.
	Buffer int
	// Fill fills frames, stereo frames of interleaved float32 samples
	// from -1 to 1, with what plays next. The device calls it on its own
	// thread, each time it takes a buffer, so it must be quick and
	// return at once. A device of one channel plays the pair averaged.
	Fill func(frames []float32)
	// Reset, where set, is called as the output should open again: the
	// default output device changed, or the device's rate or buffer
	// changed under it, or it went away. It runs on a thread of Core
	// Audio's, and must return at once.
	Reset func()
	// Log, where set, tells what the output does.
	Log func(format string, args ...any)
}

// Info says how the output plays.
type Info struct {
	// Device is the device's name.
	Device string
	// Rate is the rate it plays at, in frames a second.
	Rate int
	// Buffer is how many frames it takes at a time; MinBuffer and
	// MaxBuffer the sizes it allows.
	Buffer, MinBuffer, MaxBuffer int
	// Latency is how many frames pass from a buffer filled to its first
	// frame heard: the buffer, and the device's and its stream's own
	// latency and safety offset.
	Latency int
	// Outputs is how many channels the device plays.
	Outputs int
}

// An InputDevice is a device that records.
type InputDevice struct {
	// UID names the device to [InputConfig].Device. It stays the same
	// as the device is plugged in again, and the computer starts again.
	UID string
	// Name is what people call it.
	Name string
	// Channels is how many channels it records, and ChannelNames what
	// the device calls them, where it names any.
	Channels     int
	ChannelNames []string
	// Rate is the rate it runs at, in frames a second.
	Rate int
	// Default marks the system's default input.
	Default bool
}

// InputConfig says how to open an input.
type InputConfig struct {
	// Device is the device to record, by its UID. Empty records the
	// system's default input as it opens.
	Device string
	// Rate is the rate to record at. The device is set to it, as for the
	// output, where it is not the device the output plays through: that
	// one keeps the rate the output gave it. [InputInfo].Rate tells the
	// rate it records at, which a rate the device refuses leaves at the
	// device's own. Zero keeps the device's.
	Rate int
	// Buffer is how many frames the device hands on at a time, brought
	// into the range it allows. A device the output plays through keeps
	// the output's buffer, as a process has one buffer of each device.
	// Zero keeps the size it has.
	Buffer int
	// Record takes each buffer as it comes in: frames of
	// [InputInfo].Channels interleaved float32 samples, and how many
	// frames, at the device's rate, came in since the first of them
	// reached the input. The device calls it on its own thread, so it
	// must be quick, and must not wait or allocate. frames is its own
	// only during the call.
	Record func(frames []float32, ago int)
	// Log, where set, tells what the input does.
	Log func(format string, args ...any)
}

// InputInfo says how an input records.
type InputInfo struct {
	// UID and Device are the device's UID and name.
	UID, Device string
	// Rate is the rate it records at, in frames a second, and Channels
	// how many channels each frame holds.
	Rate, Channels int
	// Buffer is how many frames it hands on at a time, about.
	Buffer int
	// Latency is how many frames pass from a sound reaching the input
	// to the buffer that holds it handed on: the buffer, and the
	// device's and its stream's own latency and safety offset.
	Latency int
}
