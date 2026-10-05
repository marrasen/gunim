// Package band plays a song made of looping parts, as a game's music:
// each part comes in with its intro, plays its loop a few times and
// leaves with its outro, in phrases of a fixed number of bars, while how
// many parts play wanders from one to all of them. So the song changes
// as it goes, and seldom plays the same way twice. A part can be a solo
// instead, played through now and then over the others.
//
// The parts come from audio files cut at the song's bars, which [Load]
// reads from a folder:
//
//	parts, err := band.Load(files, "music")
//	...
//	b := band.New(band.Song{BPM: 136, Parts: parts}, band.Options{Seed: seed})
//	mix.Play(b, audio.Options{Volume: 0.3, FadeIn: 2 * time.Second})
//
// github.com/marrasen/gunim-music holds a song of ten synths for it,
// ready to play.
package band

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"math/rand/v2"
	"path"
	"strings"

	"github.com/marrasen/gunim/audio"
)

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

// A Part is one instrument of the song.
type Part struct {
	// Name names the part.
	Name string
	// Solo says the part plays its [Solo] once through, now and then,
	// over at least two parts looping. The rest play [Intro], [Loop]
	// and [Outro].
	Solo bool
	// Open opens one of the part's pieces, at [audio.SampleRate]. The
	// band opens each piece ahead of the phrase it starts in, on a
	// goroutine of its own. A solo that tells its length, as an
	// [audio.Seeker] does, holds its part for as many phrases as it
	// lasts, and any other for one.
	Open func(p Piece) (audio.Source, error)
}

// Song is a song for a [Band]: its time and its parts.
type Song struct {
	// BPM is the tempo, in beats a minute.
	BPM float64
	// BeatsPerBar is how many beats a bar holds; zero means 4.
	BeatsPerBar int
	// PhraseBars is a phrase's length in bars: an intro's, and a
	// loop's; zero means 16.
	PhraseBars int
	Parts      []Part
}

// BarAt returns the frame bar b of the song starts at, counted from 0 at
// [audio.SampleRate] and rounded, as the pieces should be cut.
func (s *Song) BarAt(b int) int64 {
	beats := s.BeatsPerBar
	if beats == 0 {
		beats = 4
	}
	return int64(float64(b)*audio.SampleRate*60*float64(beats)/s.BPM + 0.5)
}

// phraseBars returns PhraseBars, or its default.
func (s *Song) phraseBars() int {
	if s.PhraseBars == 0 {
		return 16
	}
	return s.PhraseBars
}

// phraseAt returns the frame phrase p starts at.
func (s *Song) phraseAt(p int) int64 { return s.BarAt(p * s.phraseBars()) }

// Load reads the parts in folder dir of fsys. A piece's file is named
// for its part, a dash and the piece, and any extension, in a format
// [audio.Decode] reads: blade-intro.ogg, blade-loop.ogg and
// blade-outro.ogg make the part blade, and brass-solo.ogg the solo
// part brass. Load reads the files into memory, and each piece decodes
// from there as it opens.
func Load(fsys fs.FS, dir string) ([]Part, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, err
	}
	data := map[string][]byte{}
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
		b, err := fs.ReadFile(fsys, path.Join(dir, file))
		if err != nil {
			return nil, err
		}
		data[base] = b
		i, ok := at[name]
		if !ok {
			i = len(parts)
			at[name] = i
			parts = append(parts, Part{Name: name, Open: func(p Piece) (audio.Source, error) {
				b, ok := data[name+"-"+p.String()]
				if !ok {
					return nil, errors.New("band: no " + p.String() + " of " + name)
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

// Options tune how a [Band] chooses what plays. The zero value is the
// defaults.
type Options struct {
	// Seed seeds the choices, so a band of the same seed plays the
	// same way.
	Seed uint64
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

// What a layer of the band is playing.
const (
	resting = iota
	inIntro
	inLoop
	inOutro
	inSolo
)

// seamFrames is how long a loop come round again crossfades from the
// outro it would have gone on into: its last bar ringing on into its
// first would click.
const seamFrames = 960

// A layer is a part in the band, and what it plays.
type layer struct {
	p     *Part
	state int
	// loops is how many more times the loop plays after this one, or
	// the solo how many more phrases.
	loops int
	// cur is the piece playing.
	cur audio.Source
	// seam is the outro's start, fading out under the loop come round
	// again, seamAt frames in; seamAt is -1 between seams.
	seam   audio.Source
	seamAt int
	// next is what the layer plays from the next phrase on.
	next plan
}

// A plan is what a layer plays from a phrase on: its state then, and
// the piece it starts, if it starts one, which opens ahead of time on a
// goroutine of its own. Opening a piece takes milliseconds, which the
// mixer, waiting on the band, cannot spare.
type plan struct {
	state, loops int
	piece        Piece
	opens        bool
	// seam says the piece is the loop come round again, to fade in over
	// the outro's start.
	seam   bool
	pieces chan [2]audio.Source
}

// A Band plays a [Song]: an [audio.Source] without end, which chooses
// who plays at each phrase. Only one goroutine reads it, as a mixer does.
type Band struct {
	song   Song
	opt    Options
	layers []*layer
	// at is the song's frame, and phrase the phrase it is in.
	at     int64
	phrase int
	// want is how many parts the band wants playing, wandering a step a
	// phrase.
	want int
	// soloAt is the phrase the solo last started at.
	soloAt int
	rng    *rand.Rand
	buf    []float32
}

// New starts a band playing s, choosing as o says, with o.Start parts
// coming in.
func New(s Song, o Options) *Band {
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
	b := &Band{song: s, opt: o, rng: rand.New(rand.NewPCG(o.Seed, 0x5eed)), want: o.Start, soloAt: -o.SoloRest}
	for i := range b.song.Parts {
		b.layers = append(b.layers, &layer{p: &b.song.Parts[i], seamAt: -1})
	}
	b.plan(0)
	b.turn()
	return b
}

// Read implements [audio.Source]: the parts playing, summed, a piece
// changing exactly where its phrase does.
func (b *Band) Read(dst []float32) (int, error) {
	n := len(dst) / 2
	clear(dst[:2*n])
	for done := 0; done < n; {
		k := int(min(int64(n-done), b.song.phraseAt(b.phrase+1)-b.at))
		for _, l := range b.layers {
			b.buf = l.mix(dst[2*done:2*(done+k)], b.buf)
		}
		done += k
		b.at += int64(k)
		if b.at == b.song.phraseAt(b.phrase+1) {
			b.phrase++
			b.turn()
		}
	}
	return n, nil
}

// turn starts the phrase planned, and plans the next.
func (b *Band) turn() {
	phrase := b.song.phraseAt(1)
	for _, l := range b.layers {
		n := l.next
		l.state, l.loops = n.state, n.loops
		if l.state == resting {
			l.cur = nil
		}
		if n.pieces != nil {
			ps := <-n.pieces
			l.cur = ps[0]
			if ps[1] != nil {
				l.seam, l.seamAt = ps[1], 0
			}
			// A solo holds its part for as many phrases as it lasts.
			if s, ok := l.cur.(audio.Seeker); ok && l.state == inSolo {
				l.loops = int((s.Len()+phrase-1)/phrase) - 1
			}
		}
		l.next = plan{}
	}
	b.plan(b.phrase + 1)
}

// plan chooses what each part plays in phrase q, from what it plays in
// the phrase before: who comes in, who loops again and who leaves. It
// starts the pieces they need opening.
func (b *Band) plan(q int) {
	playing, looping := 0, 0
	for _, l := range b.layers {
		n := plan{state: l.state, loops: l.loops}
		switch l.state {
		case inIntro:
			n.state, n.piece, n.opens = inLoop, Loop, true
		case inLoop:
			if l.loops > 0 {
				n.loops--
				n.piece, n.opens, n.seam = Loop, true, true
			} else {
				n.state, n.piece, n.opens = inOutro, Outro, true
			}
		case inOutro:
			n.state = resting
		case inSolo:
			if l.loops > 0 {
				n.loops--
			} else {
				n.state = resting
			}
		}
		l.next = n
		if n.state == inIntro || n.state == inLoop {
			playing++
		}
		if n.state == inLoop {
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
			l.next = plan{state: inIntro, loops: b.rng.IntN(b.opt.MaxLoops), piece: Intro, opens: true}
			playing++
		}
	}
	// Now and then, over a few parts looping, the solo.
	if looping >= 2 && q-b.soloAt >= b.opt.SoloRest && b.rng.Float64() < b.opt.SoloOdds {
		for _, l := range b.layers {
			if l.p.Solo && l.state == resting && l.next.state == resting {
				l.next = plan{state: inSolo, piece: Solo, opens: true}
				b.soloAt = q
				break
			}
		}
	}
	for _, l := range b.layers {
		if l.next.opens {
			l.next.pieces = openAhead(l.p, l.next.piece, l.next.seam)
		}
	}
}

// openAhead opens part p's piece on a goroutine of its own, and with
// seam its outro too, and hands them over on the channel it returns.
func openAhead(p *Part, piece Piece, seam bool) chan [2]audio.Source {
	ch := make(chan [2]audio.Source, 1)
	go func() {
		var ps [2]audio.Source
		ps[0] = open(p, piece)
		if seam {
			ps[1] = open(p, Outro)
		}
		ch <- ps
	}()
	return ch
}

// open opens part p's piece, or returns nil, for silence, where it will
// not open.
func open(p *Part, piece Piece) audio.Source {
	src, err := p.Open(piece)
	if err != nil {
		return nil
	}
	return src
}

// mix adds the layer's next len(dst)/2 frames to dst, reading into buf,
// which it grows as it needs and returns.
func (l *layer) mix(dst, buf []float32) []float32 {
	n := len(dst) / 2
	if cap(buf) < 2*n {
		buf = make([]float32, 2*n)
	}
	buf = buf[:2*n]
	if l.cur != nil {
		k, ended := readFull(l.cur, buf)
		if ended {
			l.cur = nil
		}
		for i := range k {
			g := float32(1)
			if f := l.seamAt + i; l.seamAt >= 0 && f < seamFrames {
				g = float32(math.Sin(math.Pi / 2 * float64(f) / seamFrames))
			}
			dst[2*i] += g * buf[2*i]
			dst[2*i+1] += g * buf[2*i+1]
		}
	}
	if l.seam != nil {
		k, _ := readFull(l.seam, buf[:2*min(n, seamFrames-l.seamAt)])
		for i := range k {
			g := float32(math.Cos(math.Pi / 2 * float64(l.seamAt+i) / seamFrames))
			dst[2*i] += g * buf[2*i]
			dst[2*i+1] += g * buf[2*i+1]
		}
	}
	if l.seamAt >= 0 {
		l.seamAt += n
		if l.seamAt >= seamFrames {
			l.seam, l.seamAt = nil, -1
		}
	}
	return buf
}

// readFull fills buf from src as far as it goes, and reports how many
// frames it read and whether src has ended.
func readFull(src audio.Source, buf []float32) (int, bool) {
	n := 0
	for n < len(buf)/2 {
		k, err := src.Read(buf[2*n:])
		n += k
		if err != nil {
			return n, true
		}
		if k == 0 {
			break
		}
	}
	return n, false
}

// The band plays without end, as one voice.
var _ audio.Source = (*Band)(nil)
