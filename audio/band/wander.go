package band

import "math/rand/v2"

// Wander is a [Song] of looping parts that come and go: see the package
// doc. Its zero settings are the defaults.
type Wander struct {
	// Title and Artist name the song and who made it.
	Title, Artist string
	// BPM is the tempo, in beats a minute.
	BPM float64
	// BeatsPerBar is how many beats a bar holds; zero means 4.
	BeatsPerBar int
	// PhraseBars is a phrase's length in bars: an intro's, and a
	// loop's; zero means 16.
	PhraseBars int
	Parts      []Part
	// Start is how many parts come in at the start; zero means 2.
	Start int
	// MaxLoops is the most times a part plays its loop before it
	// leaves; zero means 4. Each time a part comes in, it picks from 1
	// to MaxLoops.
	MaxLoops int
	// SoloRest is how many phrases go by from one solo's start to the
	// next at least; zero means 6.
	SoloRest int
	// SoloOdds is the chance of a solo at each phrase after the rest;
	// zero means 0.3, and a negative one never plays a solo.
	SoloOdds float64
}

// Info implements [Song].
func (s *Wander) Info() Info { return Info{Title: s.Title, Artist: s.Artist, BPM: s.BPM} }

// Play implements [Song]: it returns a [*Band].
func (s *Wander) Play(seed uint64) Player { return New(s, seed) }

// BarAt returns the frame bar b of the song starts at, counted from 0 at
// [audio.SampleRate] and rounded, as the pieces should be cut.
func (s *Wander) BarAt(b int) int64 { return s.timing().barAt(b) }

// phraseAt returns the frame phrase p starts at.
func (s *Wander) phraseAt(p int) int64 { return s.timing().phraseAt(p) }

// timing returns the song's timing, a phrase 16 bars where unset.
func (s *Wander) timing() timing { return newTiming(s.BPM, s.BeatsPerBar, s.PhraseBars, 16) }

// A Band plays a [Wander]: a [Player] that chooses who plays at each
// phrase. It is a [Watcher] too.
type Band struct {
	engine
	song Wander
	// want is how many parts the band wants playing, wandering a step a
	// phrase.
	want int
	// soloAt is the phrase the solo last started at.
	soloAt int
	rng    *rand.Rand
}

// New starts a band playing s, its choices seeded by seed, with
// s.Start parts coming in. The band keeps a copy of s.
func New(s *Wander, seed uint64) *Band {
	o := *s
	if o.Start == 0 {
		o.Start = 2
	}
	if o.MaxLoops == 0 {
		o.MaxLoops = 4
	}
	if o.SoloRest == 0 {
		o.SoloRest = 6
	}
	if o.SoloOdds == 0 {
		o.SoloOdds = 0.3
	}
	b := &Band{song: o, rng: rand.New(rand.NewPCG(seed, 0x5eed)), want: o.Start, soloAt: -o.SoloRest}
	b.t = o.timing()
	for i := range b.song.Parts {
		b.layers = append(b.layers, &layer{p: &b.song.Parts[i], seamAt: -1})
	}
	b.choose = b.plan
	b.start()
	return b
}

// plan chooses what each part plays in phrase q, from what it plays in
// the phrase before: who comes in, who loops again and who leaves.
func (b *Band) plan(q int) {
	playing, looping := 0, 0
	for _, l := range b.layers {
		l.next = l.carryOn()
		if l.next.state == inIntro || l.next.state == inLoop {
			playing++
		}
		if l.next.state == inLoop {
			looping++
		}
	}
	// The mood wanders, from one part to all.
	if q > 0 {
		b.want = max(1, min(b.want+b.rng.IntN(3)-1, len(b.layers)))
	}
	// Too many: some of those looping leave, with their outro.
	for _, i := range b.rng.Perm(len(b.layers)) {
		l := b.layers[i]
		if playing <= b.want {
			break
		}
		if l.next.state == inLoop && l.next.seam {
			l.next = plan{state: inOutro, piece: Outro, opens: true}
			playing--
			looping--
		}
	}
	// Too few: parts that rested all the phrase before come in.
	for _, i := range b.rng.Perm(len(b.layers)) {
		l := b.layers[i]
		if playing >= b.want {
			break
		}
		if !l.p.Solo && l.state == resting && l.next.state == resting {
			l.next = plan{state: inIntro, loops: b.rng.IntN(b.song.MaxLoops), piece: Intro, opens: true}
			playing++
		}
	}
	// Now and then, over a few parts looping, the solo.
	if looping >= 2 && q-b.soloAt >= b.song.SoloRest && b.rng.Float64() < b.song.SoloOdds {
		for _, l := range b.layers {
			if l.p.Solo && l.state == resting && l.next.state == resting {
				l.next = plan{state: inSolo, piece: Solo, opens: true}
				b.soloAt = q
				break
			}
		}
	}
}

var (
	_ Watcher = (*Band)(nil)
	_ Song    = (*Wander)(nil)
)
