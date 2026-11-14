package plug

import "time"

// MaxHitAhead is the longest [ControlPort.Ahead]: as far as a plugin
// hears of chords, by [MaxChordAhead]. It bounds the news a host keeps
// room for.
const MaxHitAhead = MaxChordAhead

// HitNews is news of the hits coming on a trigger input, for a port
// with a [ControlPort.Ahead]: a plugin that sounds ahead of a hit, as a
// riser that lands on it.
//
// The host tells of each hit it knows ahead once, with Coming, as the
// hit comes within Ahead, and with Moved or Removed as an edit changes
// it. The hit's pulse still comes on the port, on the frame the news
// named. Like a chord's, a Coming or a Moved may land in the very block
// it comes in. As playing starts or jumps, as a loop runs on from its
// start, and after a [Processor.Reset], the host tells of the hits
// coming within Ahead from there. The host tells of a loop's hits at
// most one pass ahead, whatever Ahead asks.
//
// A hit the host cannot know ahead comes as its pulse alone, with no
// news: one played live, or one another plugin makes. A plugin takes a
// pulse on a frame where no hit told of lands as such a hit.
//
// The news comes in the order the host sends it: by the frame it is
// sent at. It holds no pointer: a host fills a block's news, and a
// plugin keeps it, with nothing allocated.
type HitNews struct {
	Kind HitKind
	// Port is the trigger input the hit comes on, by
	// [Descriptor.ControlIn].
	Port int
	// ID is the hit's, from its Coming on: a Moved or a Removed names
	// the hit by it. The same hit, played again, as a loop repeats,
	// comes with an ID of its own.
	ID int32
	// Frame is where the hit lands, counted from the block's start: in
	// this block or a later one. A Removed's is 0. A host may skip
	// blocks while a plugin sleeps, so a plugin that waits for a later
	// block adds [Position.Frame] as the news comes, and compares with
	// it from then on.
	Frame int
	// Velocity is how high the hit's pulse comes, from 0 to 1.
	Velocity float64
	// Beat is where the hit lands in the music, in quarter notes from
	// the start of the track, as [Position.Beat] counts them, and
	// BarBeats how many quarter notes a bar of the meter holds there, as
	// a [ChordNews] has them. A plugin that times itself to the hit, as
	// a riser some bars long into it, measures in beats, and follows the
	// beat of each block on the way. A Removed's are 0.
	Beat     float64
	BarBeats float64
}

// HitKind is what a [HitNews] tells.
type HitKind uint8

// The kinds of hit news.
const (
	// HitComing tells of a hit that lands at Frame.
	HitComing HitKind = iota
	// HitMoved tells that a hit told of lands at Frame now, at
	// Velocity, as an edit moved or changed it.
	HitMoved
	// HitRemoved takes back a hit told of, as an edit took it out of
	// the music: its pulse is called off. A plugin passes over a Removed
	// for a hit it never heard of.
	HitRemoved
)

// HitAhead returns the longest [ControlPort.Ahead] of d's trigger
// inputs, or 0 where none asks to hear of its hits.
func (d *Descriptor) HitAhead() time.Duration {
	var far time.Duration
	for _, p := range d.ControlIn {
		far = max(far, p.Ahead)
	}
	return far
}
