// Package audio plays sound for gunim applications: short clips for
// what the user does, and music streamed from files.
//
// Everything plays through a [Mixer], which sums the sounds playing
// into one stream of stereo samples at [SampleRate]. Package
// audio/speaker sends that stream to the computer's speakers; this
// package holds no device code, so an application that plays nothing
// links none.
//
//	mix := audio.NewMixer()
//	spk, err := speaker.Open(mix)
//	...
//	song, err := audio.Decode(file)
//	v := mix.Play(song, audio.Options{FadeIn: time.Second})
//	v.SetVolume(0.5, anim.Spring{Response: 0.4, Damping: 1})
//
// Where the sounds playing add up past full scale, the mixer brings its
// gain down for as long as they do, so many sounds at once grow loud
// without clipping.
//
// A [Voice]'s volume and pan move with the motions of package anim,
// stepped in time with the sound itself, so a fade is as smooth as an
// animation on screen.
//
// The mixer keeps what it played last, and knows how long the speakers
// take to play it, so an [Analyzer] can show the sound being heard at
// each frame of the display.
package audio

import "time"

// SampleRate is the rate every [Source] gives frames at, in frames a
// second.
const SampleRate = 48000

// A Source gives sound as frames of two float32 samples, left then
// right, at [SampleRate], each from -1 to 1.
type Source interface {
	// Read fills dst with up to len(dst)/2 frames and returns how many
	// it filled. It returns io.EOF once the sound has ended, with or
	// after the last frames.
	Read(dst []float32) (frames int, err error)
}

// A Seeker is a [Source] of known length that can move to any frame.
type Seeker interface {
	Source
	// SeekFrame moves to frame, counted from the start.
	SeekFrame(frame int64) error
	// Len returns the sound's length in frames, or -1 when it is
	// unknown.
	Len() int64
}

// Frames returns how many frames last d.
func Frames(d time.Duration) int64 {
	return int64(d) * SampleRate / int64(time.Second)
}

// Duration returns how long n frames last.
func Duration(n int64) time.Duration {
	return time.Duration(n * int64(time.Second) / SampleRate)
}
