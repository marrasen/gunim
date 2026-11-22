package plugtest

import (
	"slices"
	"sync"
	"time"

	"github.com/marrasen/gunim/audio/plug"
)

// Probe is a synth for tests that keeps what it hears. It takes notes on two inputs: the main one, and "side". Its
// left channel sounds how many notes ring on the main input, and its right channel how many ring on "side".
// Register it with plug.Register(plugtest.Probe) where a host's test finds plugins by ID.
var Probe = plug.Descriptor{
	ID:         "gunim.plugtest.probe",
	Name:       "Probe",
	Vendor:     "gunim",
	Version:    "1.0.0",
	Kind:       "Instrument|Synth",
	Notes:      true,
	LookAhead:  100 * time.Millisecond,
	ChordAhead: time.Second,
	Past:       true,
	NotesIn: []plug.NotesInput{
		{Name: "Notes", Description: "Notes the probe counts on its left channel"},
		{Key: "side", Name: "Side", Description: "Notes the probe counts on its right channel"},
	},
	New: func(plug.Host) plug.Plugin { return &ProbeSynth{} },
}

// ProbeSynth is what [Probe] makes: a plugin and its own processor.
type ProbeSynth struct {
	mu       sync.Mutex
	events   []plug.Event
	upcoming []plug.Upcoming
	chords   []plug.ChordNews
	notes    []probeNote
	pedals   []probePedal
}

// probeNote is a note ringing, and whether its key is still down.
type probeNote struct {
	id, port, group int32
	down            bool
}

// probePedal is the pedal of one port and group.
type probePedal struct {
	port, group int32
	down        bool
}

// probeRoom is how many events, news and notes the probe keeps room for before it plays.
const probeRoom = 4096

// Processor returns p itself.
func (p *ProbeSynth) Processor() plug.Processor { return p }

// State returns nil: the probe keeps nothing beyond what it hears.
func (p *ProbeSynth) State() []byte { return nil }

// SetState takes any state, and sets nothing.
func (p *ProbeSynth) SetState([]byte) error { return nil }

// Close does nothing.
func (p *ProbeSynth) Close() {}

// Latency is 0.
func (p *ProbeSynth) Latency() int { return 0 }

// Tail is 0.
func (p *ProbeSynth) Tail() int { return 0 }

// Prepare forgets what the probe heard, and makes room for what it will hear.
func (p *ProbeSynth) Prepare(plug.Setup) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = make([]plug.Event, 0, probeRoom)
	p.upcoming = make([]plug.Upcoming, 0, probeRoom)
	p.chords = make([]plug.ChordNews, 0, probeRoom)
	p.notes = make([]probeNote, 0, probeRoom)
	p.pedals = make([]probePedal, 0, probeRoom)
}

// Reset ends every note and lifts every pedal. The probe keeps what it heard.
func (p *ProbeSynth) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.notes = p.notes[:0]
	p.pedals = p.pedals[:0]
}

// Heard returns the events the probe heard, each with its Frame counted from the start of playing.
func (p *ProbeSynth) Heard() []plug.Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.events)
}

// Upcoming returns the news of notes the probe heard, in the order it came.
func (p *ProbeSynth) Upcoming() []plug.Upcoming {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.upcoming)
}

// Chords returns the news of chords the probe heard, in the order it came.
func (p *ProbeSynth) Chords() []plug.ChordNews {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.chords)
}

// Process keeps the block's events and news, and sounds the notes ringing on each input.
func (p *ProbeSynth) Process(b *plug.Block) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.upcoming = append(p.upcoming, b.Upcoming...)
	p.chords = append(p.chords, b.Chords...)
	next := 0
	for f := range b.Frames {
		for ; next < len(b.Events) && b.Events[next].Frame <= f; next++ {
			e := b.Events[next]
			p.play(e)
			e.Frame += int(b.Position.Frame)
			p.events = append(p.events, e)
		}
		var ringing [2]float32
		for _, n := range p.notes {
			if n.port >= 0 && int(n.port) < len(ringing) {
				ringing[n.port]++
			}
		}
		for c, ch := range b.Out {
			ch[f] = 0
			if c < len(ringing) {
				ch[f] = ringing[c]
			}
		}
	}
}

// play starts, lets go or holds notes as e says.
func (p *ProbeSynth) play(e plug.Event) {
	switch e.Kind {
	case plug.NoteOn:
		p.notes = append(p.notes, probeNote{id: e.Note, port: e.Port, group: e.Group, down: true})
	case plug.NoteOff:
		for i, n := range p.notes {
			if n.id == e.Note && n.down {
				if p.held(n.port, n.group) {
					p.notes[i].down = false
				} else {
					p.notes = slices.Delete(p.notes, i, i+1)
				}
				return
			}
		}
	case plug.Pedal:
		down := e.Value >= 0.5
		i := slices.IndexFunc(p.pedals, func(q probePedal) bool { return q.port == e.Port && q.group == e.Group })
		if i < 0 {
			p.pedals = append(p.pedals, probePedal{port: e.Port, group: e.Group, down: down})
		} else {
			p.pedals[i].down = down
		}
		if !down {
			p.notes = slices.DeleteFunc(p.notes, func(n probeNote) bool {
				return !n.down && n.port == e.Port && n.group == e.Group
			})
		}
	case plug.Expression, plug.PastStart, plug.PastNote:
	}
}

// held reports whether the pedal of port and group is down.
func (p *ProbeSynth) held(port, group int32) bool {
	for _, q := range p.pedals {
		if q.port == port && q.group == group {
			return q.down
		}
	}
	return false
}
