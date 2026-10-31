// Package coreaudio plays through macOS's default output device by Core
// Audio's own callback: the device asks for each buffer as it takes it,
// on its own thread, as an ASIO driver does on Windows. Nothing queues
// between the two, so a sound is heard a buffer and the device's own
// latency after it is made: under five milliseconds at 128 frames.
//
// The buffer is the process's own: other applications playing through
// the same device keep theirs. On other systems [Open] returns
// [ErrUnsupported].
package coreaudio

import "errors"

// ErrUnsupported is returned by [Open] where Core Audio is not.
var ErrUnsupported = errors.New("coreaudio: not supported on this system")

// Config says how to open the output.
type Config struct {
	// Rate is the rate to play at, in frames a second. The device is set
	// to it, as a DAW sets it, which the other applications playing
	// through it are converted to. A rate the device refuses plays at
	// the device's own: [Info].Rate tells. Zero keeps the device's.
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
