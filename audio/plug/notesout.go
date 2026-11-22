package plug

// NotesOutput is a notes output of a plugin's, as [Descriptor.NotesOut] lists them.
type NotesOutput struct {
	// Key names the output in a project, for good, as [ControlPort.Key] does.
	Key string
	// Name labels the output's port: "Pad", or "Bass".
	Name string
	// Description is the port's tooltip.
	Description string
}

// NotesOutRoom is the least room for events a host gives each notes output in a block.
const NotesOutRoom = 1024

// NotesWriter takes the events a plugin sends on one notes output in a block, as [Block.NotesOut] holds them.
//
// A plugin writes NoteOn, NoteOff, Expression and Pedal events, never Live ones, at frames within the block, in the
// order of their frames. Every NoteOn gets its NoteOff, in this block or a later one, and a note's ID is its own on
// the output from its NoteOn to its NoteOff. The host sets each event's Group and Port. A plugin sends its notes as
// the music has them, not as late as its own sound: the host lines them up with that sound.
//
// A plugin that is reset, or whose output goes nil, ends nothing on its own: the host ends every note it sent that
// still sounds. Where the writer is full, Write keeps nothing more in the block, and the host ends those notes too, at
// the block's end, and ignores the NoteOffs that come for them later.
type NotesWriter struct {
	events []Event
	full   bool
}

// NewNotesWriter returns a writer with room for room events a block.
func NewNotesWriter(room int) *NotesWriter { return &NotesWriter{events: make([]Event, 0, room)} }

// Write sends e, and reports whether the writer took it. A nil writer, an output with no wire, takes nothing. Write
// allocates nothing.
func (w *NotesWriter) Write(e Event) bool {
	if w == nil {
		return false
	}
	if w.full || len(w.events) == cap(w.events) {
		w.full = true
		return false
	}
	w.events = append(w.events, e)
	return true
}

// Events returns the events written in the block, for the host to read after Process.
func (w *NotesWriter) Events() []Event { return w.events }

// Full reports whether Write refused an event in the block.
func (w *NotesWriter) Full() bool { return w.full }

// Clear forgets the block's events, for the host to call before each block.
func (w *NotesWriter) Clear() {
	w.events = w.events[:0]
	w.full = false
}
