// Package band plays songs made for programs, as a game's music: a
// [Song] starts a [Player], which plays without end.
//
// Each kind of song is a type of its own, and each is made of looping
// parts: a part comes in with its intro, plays its loop and leaves with
// its outro, in phrases of a fixed number of bars, the loop crossfading
// into itself as it comes round.
//
//   - [Wander] lets how many parts play wander from one to all of them,
//     so the song changes as it goes, and seldom plays the same way
//     twice. A part can be a solo, played through now and then.
//   - [Tiers] plays its parts in tiers, as a game's music grows with
//     its combo: tier 1 always, and each tier above adds its parts.
//
// A Player may do more than play, as one whose tier a program sets
// does; each such feature is an interface of its own, as [Tiered] is,
// which a program checks for, as gunim's drivers' features are.
//
// The parts come from audio files cut at the song's bars, which [Load]
// reads from a folder:
//
//	parts, err := band.Load(files, "music")
//	...
//	song := &band.Wander{Title: "Greek Themes", BPM: 136, Parts: parts}
//	mix.Play(song.Play(seed), audio.Options{Volume: 0.3, FadeIn: 2 * time.Second})
//
// github.com/marrasen/gunim-game-audio holds songs for it, ready to play.
package band

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/marrasen/gunim/audio"
)

// A Song is music a program plays, as a game does.
type Song interface {
	// Info describes the song.
	Info() Info
	// Play starts the song, its choices seeded by seed, so the same
	// seed plays it the same way.
	Play(seed uint64) Player
}

// Info describes a [Song].
type Info struct {
	// Title and Artist name the song and who made it, to credit.
	Title, Artist string
	// BPM is the tempo, in beats a minute.
	BPM float64
}

// A Player plays a [Song] without end, read by one goroutine, as a
// mixer's. A song's features beyond playing are interfaces a Player
// also implements.
type Player interface {
	audio.Source
}

// Piece is one of a part's sounds.
type Piece int

// The pieces of a part.
const (
	// Intro brings the part in. It lasts a phrase, and runs on into
	// the loop as one recording does.
	Intro Piece = iota
	// Loop is the part playing on. It lasts a phrase, and runs on
	// into the outro, or into itself as it comes round again.
	Loop
	// Outro takes the part out. It may last less than a phrase, or
	// ring on into the next.
	Outro
	// Solo is the whole of a solo part, of any length.
	Solo
)

// String returns the piece's name, as [Load] finds it in a file's name:
// intro, loop, outro or solo.
func (p Piece) String() string {
	switch p {
	case Intro:
		return "intro"
	case Loop:
		return "loop"
	case Outro:
		return "outro"
	case Solo:
		return "solo"
	}
	return fmt.Sprintf("Piece(%d)", int(p))
}

// A Part is one instrument of a song.
type Part struct {
	// Name names the part.
	Name string
	// Solo says the part plays its [Solo] once through, now and then,
	// over at least two parts looping. The rest play [Intro], [Loop]
	// and [Outro].
	Solo bool
	// Tier is the tier the part plays from, in a [Tiers]: 1 always.
	Tier int
	// Loops, in a [Tiers], is how many times the part plays its loop
	// before it leaves with its outro, to come in again with its intro
	// at the phrase after, as a solo does now and then. Zero loops it
	// for as long as its tier plays.
	Loops int
	// Open opens one of the part's pieces, at [audio.SampleRate]. The
	// band opens each piece ahead of the phrase it starts in, on a
	// goroutine of its own. A piece may be a recording, or sound made
	// in code, or either through effects. A solo that tells its
	// length, as an [audio.Seeker] does, holds its part for as many
	// phrases as it lasts, and any other for one.
	Open func(p Piece) (audio.Source, error)
}

// A Tiered is a [Player] that plays in tiers, as a game's music grows
// with its combo: tier 1 plays always, and each tier above adds parts.
// SetTier changes the tier from the next phrase on, the parts above it
// leaving with their outros and those up to it coming in with their
// intros; it may be called from any goroutine.
type Tiered interface {
	Player
	// Tiers returns how many tiers there are.
	Tiers() int
	// Tier returns the tier SetTier last set.
	Tier() int
	// SetTier sets the tier, from 1 to Tiers.
	SetTier(n int)
}

// A Triggered is a [Player] whose parts a program also starts and stops
// one by one, as a player at a mixing desk does. SetPart changes a part
// from the next phrase on: [PartOn] brings it in with its intro and
// plays it as if its tier played, [PartOff] takes it out with its
// outro, and [PartAuto] hands it back to the song, as its tier says. A
// part keeps its control until SetPart changes it, through changes of
// tier. SetPart may be called from any goroutine.
type Triggered interface {
	Player
	// SetPart sets the control of the part named name, and returns
	// [ErrNoPart] for a name the song has no part of.
	SetPart(name string, c PartControl) error
	// Part returns the control of the part named name.
	Part(name string) PartControl
}

// PartControl says who chooses whether a part plays.
type PartControl int

// The controls of a part.
const (
	// PartAuto leaves the part to the song, as its tier says.
	PartAuto PartControl = iota
	// PartOn plays the part.
	PartOn
	// PartOff rests the part.
	PartOff
)

// ErrNoPart is returned for a part a song has none of.
var ErrNoPart = errors.New("band: no such part")

// A Watcher is a [Player] that tells what it plays, as a tool showing a
// song's parts does. Watch may be called from any goroutine.
type Watcher interface {
	Player
	Watch() Status
}

// Status is what a [Watcher] plays.
type Status struct {
	// Bar is how many bars have played, and PhraseBars a phrase's
	// length in bars.
	Bar, PhraseBars int
	// Parts are the song's parts, in order.
	Parts []PartStatus
}

// PartStatus is what a part plays.
type PartStatus struct {
	Name string
	// Tier is the part's [Part.Tier].
	Tier int
	// Playing says the part plays Piece in this phrase.
	Playing bool
	Piece   Piece
	// Control is the part's control, [PartAuto] in a song of no
	// [Triggered] player.
	Control PartControl
}

// Load finds the parts in folder dir of fsys. A piece's file is named
// for its part, a dash and the piece, and any extension, in a format
// [audio.Decode] reads: blade-intro.ogg, blade-loop.ogg and
// blade-outro.ogg make the part blade, and brass-solo.ogg the solo
// part brass. Each piece reads its file and decodes it as it opens.
func Load(fsys fs.FS, dir string) ([]Part, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	files := map[string]string{}
	var parts []Part
	at := map[string]int{}
	for _, e := range entries {
		file := e.Name()
		base := strings.TrimSuffix(file, path.Ext(file))
		dash := strings.LastIndex(base, "-")
		if e.IsDir() || dash < 0 {
			continue
		}
		name, piece := base[:dash], base[dash+1:]
		if piece != "intro" && piece != "loop" && piece != "outro" && piece != "solo" {
			continue
		}
		files[base] = path.Join(dir, file)
		i, ok := at[name]
		if !ok {
			i = len(parts)
			at[name] = i
			parts = append(parts, Part{Name: name, Open: func(p Piece) (audio.Source, error) {
				file, ok := files[name+"-"+p.String()]
				if !ok {
					return nil, errors.New("band: no " + p.String() + " of " + name)
				}
				b, err := fs.ReadFile(fsys, file)
				if err != nil {
					return nil, err
				}
				return audio.Decode(bytes.NewReader(b))
			}})
		}
		if piece == "solo" {
			parts[i].Solo = true
		}
	}
	return parts, nil
}
