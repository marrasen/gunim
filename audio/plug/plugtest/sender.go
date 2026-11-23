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

// Teller is an effect for tests that sends one note on its notes output, "notes", [TellerNote], at frame [TellerFrame]
// after it is prepared or reset, for [SenderFrames] frames, and tells of it ahead as the note comes within
// [plug.NotesWriter.Ahead]. It passes its audio through.
var Teller = plug.Descriptor{
	ID:       "gunim.plugtest.teller",
	Name:     "Teller",
	Vendor:   "gunim",
	Version:  "1.0.0",
	Kind:     "Fx|Tools",
	NotesOut: []plug.NotesOutput{{Key: "notes", Name: "Notes", Description: "The one note the teller tells of and sends"}},
	New:      func(plug.Host) plug.Plugin { return &teller{} },
}

// TellerNote is the NoteOn [Teller] sends, with its Frame counted from where it is prepared or reset.
var TellerNote = plug.Event{Frame: TellerFrame, Kind: plug.NoteOn, Note: 7, Pitch: 48, Velocity: 0.75, Length: SenderFrames}

// TellerFrame is how many frames after it is prepared or reset [Teller] starts its note.
const TellerFrame = 1000

// teller is what [Teller] makes: a plugin and its own processor.
type teller struct {
	at          int
	told, ended bool
}

// Processor returns p itself.
func (p *teller) Processor() plug.Processor { return p }

// State returns nil: the teller keeps no state.
func (p *teller) State() []byte { return nil }

// SetState takes any state, and sets nothing.
func (p *teller) SetState([]byte) error { return nil }

// Close does nothing.
func (p *teller) Close() {}

// Latency is 0.
func (p *teller) Latency() int { return 0 }

// Tail is 0.
func (p *teller) Tail() int { return 0 }

// Prepare readies the note to send again.
func (p *teller) Prepare(plug.Setup) { *p = teller{} }

// Reset readies the note to send again.
func (p *teller) Reset() { *p = teller{} }

// Process passes the audio through, tells of the note as it comes within the look-ahead, and sends its start and end
// where they fall in the block.
func (p *teller) Process(b *plug.Block) {
	for c, ch := range b.Out {
		if c < len(b.In) {
			copy(ch[:b.Frames], b.In[c])
		} else {
			clear(ch[:b.Frames])
		}
	}
	w := b.NotesOutput(0)
	on, off := TellerFrame-p.at, TellerFrame+SenderFrames-p.at
	if !p.told && on >= b.Frames && on < b.Frames+w.Ahead() {
		p.told = w.Tell(plug.Upcoming{Kind: plug.Coming, Frame: on, Note: TellerNote.Note, Pitch: TellerNote.Pitch,
			Velocity: TellerNote.Velocity, Length: TellerNote.Length})
	}
	if on >= 0 && on < b.Frames {
		e := TellerNote
		e.Frame = on
		w.Write(e)
	}
	if !p.ended && off >= 0 && off < b.Frames {
		w.Write(plug.Event{Frame: off, Kind: plug.NoteOff, Note: TellerNote.Note})
		p.ended = true
	}
	p.at += b.Frames
}
