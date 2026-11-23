package plug

// NotesOutput is a notes output of a plugin's, as [Descriptor.NotesOut] lists them.
type NotesOutput struct {
	// Key names the output in a project, for good. It is the output's own among the plugin's notes outputs, and
	// empty is a key like any other: it does not fall back to Name.
	Key string
	// Name labels the output's port: "Pad", or "Bass".
	Name string
	// Description is the port's tooltip.
	Description string
}

// NotesOutRoom is the least room for events, and for news, a host gives each notes output in a block.
const NotesOutRoom = 1024

// NotesWriter takes the events a plugin sends on one notes output in a block, as [Block.NotesOut] holds them.
//
// A plugin writes NoteOn, NoteOff, Expression and Pedal events, never Live ones, at frames within the block, in the
// order of their frames. Every NoteOn gets its NoteOff, in this block or a later one, and a note's ID is its own on
// the output from its NoteOn to its NoteOff. The host sets each event's Group and Port. A plugin sends its notes as
// the music has them, not as late as its own sound: the host lines them up with that sound.
//
// The writer checks none of this, so the host checks what it reads: it drops Live events and frames outside the
// block, and sorts by frame. A plugin that is reset, or whose output goes nil, ends nothing on its own: the host ends
// every note it sent that still sounds. Once the writer is full, it refuses every event until Clear. The host then
// ends every note it took that still sounds, and lifts a pedal that is down, at the block's end. The host ignores a
// NoteOff or Expression for a note that isn't sounding, as for a refused NoteOn.
//
// A plugin may also tell of the notes it will start in later blocks, with [NotesWriter.Tell], so an instrument that
// looks ahead hears of them as it hears of the music's ([Descriptor.LookAhead], [Block.Upcoming]).
type NotesWriter struct {
	events []Event
	news   []Upcoming
	full   bool
	ahead  int
}

// NewNotesWriter returns a writer with room for room events and room news a block.
func NewNotesWriter(room int) *NotesWriter {
	return &NotesWriter{events: make([]Event, 0, room), news: make([]Upcoming, 0, room)}
}

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

// Tell sends news of a note the plugin starts on the output in a later block, and reports whether the writer took it.
// It follows [Upcoming]: a Coming once for each note, a Moved as the plugin changes its mind on where the note lands or
// how it plays, and a Removed as it calls the note off. Frame counts from the block's start, at Frames or more, and a
// Removed's is 0. The NoteOn that starts the note carries the same Note, and the ID is the note's own on the output from
// its Coming to its NoteOff. The host sets Group and Port.
//
// A nil writer, or one with no room left in the block, takes nothing: a note whose Coming is refused comes
// unannounced, and a plugin sends a refused Moved or Removed again. The host takes back the news of a note whose NoteOn
// hasn't come by the frame it was told of, and of every note still to start when the plugin is reset. Tell allocates
// nothing.
func (w *NotesWriter) Tell(u Upcoming) bool {
	if w == nil || len(w.news) == cap(w.news) {
		return false
	}
	w.news = append(w.news, u)
	return true
}

// Ahead returns how many frames ahead the instruments the output feeds ask to hear of notes, the most of their
// look-aheads, as the host sets it. Zero says none asks, and the plugin need tell of nothing. A nil writer's is 0.
func (w *NotesWriter) Ahead() int {
	if w == nil {
		return 0
	}
	return w.ahead
}

// SetAhead sets what Ahead returns, for the host to call as it wires the output. It does nothing to a nil writer.
func (w *NotesWriter) SetAhead(frames int) {
	if w != nil {
		w.ahead = max(frames, 0)
	}
}

// News returns the news told in the block, for the host to read after Process. A nil writer has none.
func (w *NotesWriter) News() []Upcoming {
	if w == nil {
		return nil
	}
	return w.news
}

// Events returns the events written in the block, for the host to read after Process. A nil writer has none.
func (w *NotesWriter) Events() []Event {
	if w == nil {
		return nil
	}
	return w.events
}

// Full reports whether Write refused an event in the block. A nil writer is never full.
func (w *NotesWriter) Full() bool { return w != nil && w.full }

// Clear forgets the block's events and news, for the host to call before each block. It keeps Ahead, and does nothing
// to a nil writer.
func (w *NotesWriter) Clear() {
	if w == nil {
		return
	}
	w.events = w.events[:0]
	w.news = w.news[:0]
	w.full = false
}
