// Package speaker plays a [audio.Mixer] through the computer's
// speakers: through PulseAudio or PipeWire, else ALSA, on Linux,
// WASAPI on Windows, Core Audio on macOS and Oboe on Android.
//
// A process opens the speakers once and plays everything through the
// one mixer:
//
//	mix := audio.NewMixer()
//	spk, err := speaker.Open(mix, speaker.Options{Name: "My app"})
package speaker

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ebitengine/oto/v3"

	"github.com/marrasen/gunim/audio"
)

// Options says how to open the speakers.
type Options struct {
	// Name names the application to the system's volume controls.
	Name string
	// Latency is how far ahead of the speakers the mixer starts out
	// working: how long a sound takes to be heard after it starts. Zero
	// means 40 milliseconds. Where the system asks for sound in larger
	// pieces than that holds, the speaker works further ahead, as far
	// as it must not to break up.
	Latency time.Duration
}

// device is the device's own buffer. The player's buffer, which the
// mixer fills, makes up the rest of the latency.
const device = 10 * time.Millisecond

// maxLatency is as far ahead as the speaker grows to work.
const maxLatency = 250 * time.Millisecond

// A Speaker plays a mixer.
type Speaker struct {
	ctx    *oto.Context
	player *oto.Player
	m      *audio.Mixer
	// buffer is the player's buffer, in frames.
	buffer atomic.Int64
	// suspended stops the watch growing the buffer while nothing plays.
	suspended atomic.Bool
	quit      chan struct{}
}

var (
	opened   sync.Mutex
	isOpened bool
)

// ErrOpen is returned by [Open] while the speakers are open already.
var ErrOpen = errors.New("speaker: the speakers are open already")

// Open starts the speakers playing m. A process can open them once.
func Open(m *audio.Mixer, o Options) (*Speaker, error) {
	opened.Lock()
	defer opened.Unlock()
	if isOpened {
		return nil, ErrOpen
	}
	lat := o.Latency
	if lat <= 0 {
		lat = 40 * time.Millisecond
	}
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:      audio.SampleRate,
		ChannelCount:    2,
		Format:          oto.FormatFloat32LE,
		BufferSize:      device,
		ApplicationName: o.Name,
	})
	if err != nil {
		return nil, err
	}
	<-ready
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	isOpened = true
	s := &Speaker{ctx: ctx, m: m, quit: make(chan struct{})}
	s.player = ctx.NewPlayer(m)
	s.setBuffer(audio.Frames(max(lat-device, device)))
	m.SetLatency(s.held)
	s.player.Play()
	go s.watch()
	return s, nil
}

func (s *Speaker) setBuffer(frames int64) {
	s.buffer.Store(frames)
	s.player.SetBufferSize(int(frames) * 8)
}

// held returns how many frames the mixer gave that are still to be
// heard: those in the player's buffer, and the device's.
func (s *Speaker) held() int64 {
	return int64(s.player.BufferedSize()/8) + audio.Frames(device)
}

// Latency returns how long a sound takes to be heard after it starts,
// as the speaker works now.
func (s *Speaker) Latency() time.Duration {
	return device + audio.Duration(s.buffer.Load())
}

// watch grows the player's buffer while the mixer is read slower than
// the sound plays: the system asks for more at a time than the buffer
// holds, and the sound breaks up between.
func (s *Speaker) watch() {
	const every = 250 * time.Millisecond
	t := time.NewTicker(every)
	defer t.Stop()
	last, lastAt := s.m.Mixed(), time.Now()
	for {
		select {
		case <-s.quit:
			return
		case now := <-t.C:
			mixed := s.m.Mixed()
			want := audio.Frames(now.Sub(lastAt))
			starved := float64(mixed-last) < 0.97*float64(want) && !s.suspended.Load()
			last, lastAt = mixed, now
			if starved && s.Latency() < maxLatency {
				s.setBuffer(min(s.buffer.Load()*3/2, audio.Frames(maxLatency-device)))
			}
		}
	}
}

// Err returns an error the speakers met while playing, if any.
func (s *Speaker) Err() error {
	if err := s.player.Err(); err != nil {
		return err
	}
	return s.ctx.Err()
}

// Suspend stops the device, as while the application is in the
// background on a phone. The mixer stops with it.
func (s *Speaker) Suspend() error {
	s.suspended.Store(true)
	return s.ctx.Suspend()
}

// Resume starts a suspended device again.
func (s *Speaker) Resume() error {
	err := s.ctx.Resume()
	s.suspended.Store(false)
	return err
}
