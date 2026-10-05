package band

import (
	"slices"
	"sync/atomic"
)

// Tiers is a [Song] whose parts play in tiers, as a game's music grows
// with its combo. Each part has its [Part.Tier]: at tier 1 only the
// parts of tier 1 play, and each tier above adds its own. A part comes
// in with its intro and plays its loop for as long as its tier plays,
// or [Part.Loops] times before it takes its outro and comes in again,
// as a solo does. Its player is a [Tiered], and a [Triggered], whose
// parts a program also starts and stops one by one.
type Tiers struct {
	// Title and Artist name the song and who made it.
	Title, Artist string
	// BPM is the tempo, in beats a minute.
	BPM float64
	// BeatsPerBar is how many beats a bar holds; zero means 4.
	BeatsPerBar int
	// PhraseBars is a phrase's length in bars: an intro's, and a
	// loop's; zero means 8.
	PhraseBars int
	Parts      []Part
	// Start is the tier the song starts at; zero means 1.
	Start int
}

// Info implements [Song].
func (s *Tiers) Info() Info { return Info{Title: s.Title, Artist: s.Artist, BPM: s.BPM} }

// Play implements [Song]: it returns a [*TierBand]. The song plays the
// same way each time, so seed goes unused.
func (s *Tiers) Play(seed uint64) Player { return NewTiers(s) }

// BarAt returns the frame bar b of the song starts at, counted from 0 at
// [audio.SampleRate] and rounded, as the pieces should be cut.
func (s *Tiers) BarAt(b int) int64 { return s.timing().barAt(b) }

// timing returns the song's timing, a phrase 8 bars where unset.
func (s *Tiers) timing() timing { return newTiming(s.BPM, s.BeatsPerBar, s.PhraseBars, 8) }

// Tiers returns how many tiers the song has: its parts' highest.
func (s *Tiers) Tiers() int {
	n := 1
	for _, p := range s.Parts {
		n = max(n, p.Tier)
	}
	return n
}

// A TierBand plays a [Tiers]. It is a [Tiered], a [Triggered] and a
// [Watcher].
type TierBand struct {
	engine
	song Tiers
	// tier is the tier SetTier set, and controls the parts' controls,
	// as SetPart set them. changes counts the calls to either, and
	// planned is the count the coming phrase was planned at.
	tier     atomic.Int32
	controls []atomic.Int32
	changes  atomic.Uint64
	planned  uint64
}

// NewTiers starts a band playing s at s.Start. The band keeps a copy of
// s.
func NewTiers(s *Tiers) *TierBand {
	b := &TierBand{song: *s}
	b.t = s.timing()
	for i := range b.song.Parts {
		b.layers = append(b.layers, &layer{p: &b.song.Parts[i], seamAt: -1})
	}
	b.controls = make([]atomic.Int32, len(b.layers))
	b.SetTier(max(1, s.Start))
	b.choose = b.plan
	b.stale = func() bool { return b.changes.Load() != b.planned }
	b.control = func(i int) PartControl { return PartControl(b.controls[i].Load()) }
	b.start()
	return b
}

// Tiers implements [Tiered].
func (b *TierBand) Tiers() int { return b.song.Tiers() }

// Tier implements [Tiered].
func (b *TierBand) Tier() int { return int(b.tier.Load()) }

// SetTier implements [Tiered].
func (b *TierBand) SetTier(n int) {
	b.tier.Store(int32(max(1, min(n, b.Tiers()))))
	b.changes.Add(1)
}

// SetPart implements [Triggered].
func (b *TierBand) SetPart(name string, c PartControl) error {
	i := b.index(name)
	if i < 0 {
		return ErrNoPart
	}
	b.controls[i].Store(int32(c))
	b.changes.Add(1)
	return nil
}

// Part implements [Triggered].
func (b *TierBand) Part(name string) PartControl {
	if i := b.index(name); i >= 0 {
		return PartControl(b.controls[i].Load())
	}
	return PartAuto
}

// index returns the index of the part named name, or -1.
func (b *TierBand) index(name string) int {
	return slices.IndexFunc(b.song.Parts, func(p Part) bool { return p.Name == name })
}

// plan chooses what each part plays in phrase q: the parts of the tier
// and below, and those turned on, come in or play on, and the rest
// leave.
func (b *TierBand) plan(int) {
	b.planned = b.changes.Load()
	tier := b.Tier()
	for i, l := range b.layers {
		on := max(1, l.p.Tier) <= tier
		switch PartControl(b.controls[i].Load()) {
		case PartOn:
			on = true
		case PartOff:
			on = false
		case PartAuto:
		}
		in := plan{state: inIntro, loops: l.p.Loops - 1, piece: Intro, opens: true}
		out := plan{state: inOutro, piece: Outro, opens: true}
		switch l.state {
		case resting, inOutro:
			// A part that left comes in again at once.
			l.next = plan{}
			if on {
				l.next = in
			}
		case inIntro, inLoop:
			switch {
			case !on:
				l.next = out
			case l.p.Loops == 0:
				l.next = plan{state: inLoop, piece: Loop, opens: true, seam: l.state == inLoop}
			default:
				l.next = l.carryOn()
			}
		default:
			l.next = l.carryOn()
		}
	}
}

var (
	_ Tiered    = (*TierBand)(nil)
	_ Triggered = (*TierBand)(nil)
	_ Watcher   = (*TierBand)(nil)
	_ Song      = (*Tiers)(nil)
)
