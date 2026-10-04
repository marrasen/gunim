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
	"log"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/marrasen/oto/v3"

	"github.com/marrasen/gunim/audio"
)

// Options says how to open the speakers.
type Options struct {
	// Name names the application to the system's volume controls.
	Name string
	// Latency is how far ahead of the speakers the mixer starts out
	// working: how long a sound takes to be heard after it starts. Zero
	// means 30 milliseconds. Where the sound runs dry at that, as where
	// the system asks for it in larger pieces, the speaker works
	// further ahead, 10 milliseconds at a time, as far as it must.
	Latency time.Duration
}

// device is the device's own buffer, as the speaker counts it where the
// system reports no latency of its own. The player's buffer, which the
// mixer fills, makes up the rest of the latency.
const device = 10 * time.Millisecond

// maxLatency is as far ahead as the speaker grows to work, the device's
// buffer and the player's.
const maxLatency = 150 * time.Millisecond

// chunk is the most the mixer hands the player at once. The player
// reads a buffer's worth whenever its buffer has room, so a read of a
// whole buffer could leave nearly two waiting; small reads keep it to
// one.
const chunk = 5 * time.Millisecond

// A Speaker plays a mixer.
type Speaker struct {
	ctx    *oto.Context
	player *oto.Player
	m      *audio.Mixer
	// buffer is the player's buffer, in frames.
	buffer atomic.Int64
	// dry counts the reads that found the player's buffer empty: the
	// sound ran out before the mixer filled it again. reads counts all
	// of them, so the first, into an empty buffer, are let be.
	dry, reads atomic.Int64
	// suspended stops the watch growing the buffer while nothing plays.
	suspended atomic.Bool
	quit      chan struct{}
	debug     bool
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
		lat = 30 * time.Millisecond
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
	s := &Speaker{ctx: ctx, m: m, quit: make(chan struct{}), debug: os.Getenv("GUNIM_DEBUG_AUDIO") != ""}
	s.player = ctx.NewPlayer(feed{s})
	s.setBuffer(audio.Frames(max(lat-device, device)))
	s.logf("working %v ahead", s.Latency())
	m.SetLatency(s.held)
	s.player.Play()
	go s.watch()
	return s, nil
}

// feed hands the mixer's sound to the player a chunk at a time, and
// counts the times the player ran dry.
type feed struct{ s *Speaker }

func (f feed) Read(p []byte) (int, error) {
	s := f.s
	if s.reads.Add(1) > 20 && s.player.BufferedSize() == 0 {
		s.dry.Add(1)
	}
	n := min(len(p), int(audio.Frames(chunk))*8)
	return s.m.Read(p[:n/8*8])
}

func (s *Speaker) logf(format string, args ...any) {
	if s.debug {
		log.Printf("speaker: "+format, args...)
	}
}

func (s *Speaker) setBuffer(frames int64) {
	s.buffer.Store(frames)
	s.player.SetBufferSize(int(frames) * 8)
}

// held returns how many frames the mixer gave that are still to be
// heard: those in the player's buffer, and those past it.
func (s *Speaker) held() int64 {
	return int64(s.player.BufferedSize()/8) + audio.Frames(s.output())
}

// output returns how long the sound past the player's buffer takes to
// be heard: as the system reports it, where it does, which on Android
// counts a Bluetooth headset's own delay; or device.
func (s *Speaker) output() time.Duration {
	if d, ok := s.ctx.OutputLatency(); ok {
		return d
	}
	return device
}

// Latency returns how long a sound takes to be heard after it starts,
// as the speaker works now.
func (s *Speaker) Latency() time.Duration {
	return s.output() + audio.Duration(s.buffer.Load())
}

// watch grows the player's buffer while the sound runs dry: the
// system takes more at a time than the buffer holds, or the mixer is
// late to fill it, and the sound breaks up.
func (s *Speaker) watch() {
	const every = 250 * time.Millisecond
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-s.quit:
			return
		case <-t.C:
			dry := s.dry.Swap(0)
			if dry > 0 {
				s.logf("ran dry %d times in %v", dry, every)
			}
			// Once might be a hiccup; more is the buffer too small.
			if dry >= 2 && !s.suspended.Load() && device+audio.Duration(s.buffer.Load()) < maxLatency {
				s.setBuffer(min(s.buffer.Load()+audio.Frames(10*time.Millisecond), audio.Frames(maxLatency-device)))
				s.logf("working %v ahead", s.Latency())
			}
		}
	}
}

// SetLatency sets how far ahead of the speakers the mixer works, as
// Options.Latency does: further while nothing needs the sound to answer
// at once, as while a player plays on with its screen off, so the sound
// rides out a busy moment; nearer again when it does. The speaker still
// grows it, as it runs dry.
func (s *Speaker) SetLatency(d time.Duration) {
	s.setBuffer(audio.Frames(max(d-device, device)))
	s.logf("working %v ahead", s.Latency())
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
