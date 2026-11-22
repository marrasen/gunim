package plugtest

import "github.com/marrasen/gunim/audio/plug"

// Sender is an effect for tests that sends one note on its notes output, "notes", and passes its audio through. The
// note, [SenderNote], starts at the first frame it plays after it is prepared or reset, and ends [SenderFrames]
// frames later.
var Sender = plug.Descriptor{
	ID:       "gunim.plugtest.sender",
	Name:     "Sender",
	Vendor:   "gunim",
	Version:  "1.0.0",
	Kind:     "Fx|Tools",
	NotesOut: []plug.NotesOutput{{Key: "notes", Name: "Notes", Description: "The one note the sender sends"}},
	New:      func(plug.Host) plug.Plugin { return &sender{} },
}

// SenderNote is the NoteOn [Sender] sends, at frame 0 of the block it starts in.
var SenderNote = plug.Event{Kind: plug.NoteOn, Note: 1, Pitch: 60, Velocity: 1}

// SenderFrames is how many frames [Sender]'s note lasts.
const SenderFrames = 100

// sender is what [Sender] makes: a plugin and its own processor.
type sender struct {
	// at is how many frames have passed since the note started.
	at             int
	started, ended bool
}

// Processor returns p itself.
func (p *sender) Processor() plug.Processor { return p }

// State returns nil: the sender keeps no state.
func (p *sender) State() []byte { return nil }

// SetState takes any state, and sets nothing.
func (p *sender) SetState([]byte) error { return nil }

// Close does nothing.
func (p *sender) Close() {}

// Latency is 0.
func (p *sender) Latency() int { return 0 }

// Tail is 0.
func (p *sender) Tail() int { return 0 }

// Prepare readies the note to send again.
func (p *sender) Prepare(plug.Setup) { *p = sender{} }

// Reset readies the note to send again.
func (p *sender) Reset() { *p = sender{} }

// Process passes the audio through, and sends the note's start and end where they fall in the block.
func (p *sender) Process(b *plug.Block) {
	for c, ch := range b.Out {
		if c < len(b.In) {
			copy(ch[:b.Frames], b.In[c])
		} else {
			clear(ch[:b.Frames])
		}
	}
	w := b.NotesOutput(0)
	if !p.started {
		w.Write(SenderNote)
		p.started = true
	}
	if end := SenderFrames - p.at; !p.ended && end < b.Frames {
		w.Write(plug.Event{Frame: end, Kind: plug.NoteOff, Note: SenderNote.Note})
		p.ended = true
	}
	p.at += b.Frames
}
