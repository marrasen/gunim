// Package cues plays sounds for gunim's cues: what the user does with
// a widget, as a button pressed or a switch flipped.
//
// The sounds are made in code, quiet and short, so they sit under what
// the user is doing:
//
//	mix := audio.NewMixer()
//	spk, err := speaker.Open(mix, speaker.Options{Name: "My app"})
//	...
//	app.SetCues(cues.New(mix))
package cues

import (
	"sync"
	"time"

	"github.com/marrasen/gunim"
	"github.com/marrasen/gunim/audio"
)

// again is how soon a cue can sound again: a slider dragged fast
// passes steps quicker than they are worth hearing.
const again = 30 * time.Millisecond

// A Player plays cues through a mixer. It implements
// [gunim.CuePlayer].
type Player struct {
	m *audio.Mixer

	mu     sync.Mutex
	sounds map[gunim.Cue]*audio.Clip
	volume float32
	last   map[gunim.Cue]time.Time
}

// New returns a player of the [Sounds] through m.
func New(m *audio.Mixer) *Player {
	return &Player{m: m, sounds: Sounds(), volume: 1, last: map[gunim.Cue]time.Time{}}
}

// Set plays clip for c, or nothing with a nil clip.
func (p *Player) Set(c gunim.Cue, clip *audio.Clip) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if clip == nil {
		delete(p.sounds, c)
		return
	}
	p.sounds[c] = clip
}

// SetVolume scales every cue, 1 as made and 0 silent.
func (p *Player) SetVolume(v float32) {
	p.mu.Lock()
	p.volume = max(0, v)
	p.mu.Unlock()
}

// PlayCue implements [gunim.CuePlayer].
func (p *Player) PlayCue(c gunim.Cue, pan float32) {
	p.mu.Lock()
	clip, vol := p.sounds[c], p.volume
	now := time.Now()
	soon := now.Sub(p.last[c]) < again
	if clip != nil && !soon {
		p.last[c] = now
	}
	p.mu.Unlock()
	if clip == nil || soon || vol == 0 {
		return
	}
	p.m.Play(clip.Source(), audio.Options{Volume: vol, Pan: pan})
}

var _ gunim.CuePlayer = (*Player)(nil)
