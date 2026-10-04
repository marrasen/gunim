package main

import (
	"bytes"
	"embed"
	"errors"
	"math"
	"math/rand/v2"
	"path"
	"strings"
	"time"

	"github.com/marrasen/gunim/audio"
)

// The music is a band of synths playing one song, made in Reason at
// 136 BPM. Each synth comes in with its intro, plays its loop a few
// times and leaves with its outro, in 16-bar phrases, while how many
// play wanders from one to all of them. So the song changes as it goes,
// and seldom plays the same way twice. The brass is a solo, played
// through now and then.

// musicFiles are the synths' parts, made by music/encode.sh: each
// synth's intro, loop and outro, or its solo.
//
//go:embed music/*.ogg
var musicFiles embed.FS

// The song's time.
const (
	songBPM = 136
	// phraseBars is a phrase's length: an intro's, and a loop's.
	phraseBars = 16
	// seamFrames is how long a loop come round again crossfades from
	// the outro it would have gone on into: its last bar ringing on into
	// its first would click.
	seamFrames = 960
)

// barAt returns the frame bar b of the song starts at, rounded as the
// parts are cut.
func barAt(b int) int64 { return int64(float64(b)*audio.SampleRate*240/songBPM + 0.5) }

// phraseAt returns the frame phrase p starts at.
func phraseAt(p int) int64 { return barAt(p * phraseBars) }

// A synth is one of the band: its parts open by name, "intro", "loop"
// and "outro", or "solo".
type synth struct {
	name string
	// solo says it plays its solo once through, now and then, rather
	// than loop.
	solo bool
	open func(part string) (audio.Source, error)
}

// bandSynths returns the synths of the parts in musicFiles. A part's
// file is the synth's name, a dash, and the part.
func bandSynths() []*synth {
	entries, err := musicFiles.ReadDir("music")
	if err != nil {
		return nil
	}
	data := map[string][]byte{}
	var out []*synth
	byName := map[string]*synth{}
	for _, e := range entries {
		file := e.Name()
		base := strings.TrimSuffix(file, ".ogg")
		dash := strings.LastIndex(base, "-")
		if dash < 0 {
			continue
		}
		name, part := base[:dash], base[dash+1:]
		b, err := musicFiles.ReadFile(path.Join("music", file))
		if err != nil {
			continue
		}
		data[file] = b
		s := byName[name]
		if s == nil {
			s = &synth{name: name, open: func(part string) (audio.Source, error) {
				b, ok := data[name+"-"+part+".ogg"]
				if !ok {
					return nil, errors.New("sudoku: no part " + part + " of " + name)
				}
				return audio.Decode(bytes.NewReader(b))
			}}
			byName[name] = s
			out = append(out, s)
		}
		if part == "solo" {
			s.solo = true
		}
	}
	return out
}

// What a layer of the band is playing.
const (
	resting = iota
	inIntro
	inLoop
	inOutro
	inSolo
)

// A layer is a synth in the band, and what it plays.
type layer struct {
	s     *synth
	state int
	// loops is how many more times the loop plays after this one, or
	// the solo how many more phrases.
	loops int
	// cur is the part playing.
	cur audio.Source
	// seam is the outro's start, fading out under the loop come round
	// again, seamAt frames in; seamAt is -1 between seams.
	seam   audio.Source
	seamAt int
	// next is what the layer plays from the next phrase on.
	next plan
}

// A plan is what a layer plays from a phrase on: its state then, and
// the part it starts, if it starts one, which opens ahead of time on a
// goroutine of its own. Opening a part takes milliseconds, which the
// mixer, waiting on the band, cannot spare.
type plan struct {
	state, loops int
	part         string
	// seam says the part is the loop come round again, to fade in over
	// the outro's start.
	seam  bool
	parts chan [2]audio.Source
}

// band is the music: the synths, the song's place, and the conductor
// choosing who plays. It is an [audio.Source] without end, read only
// by the mixer.
type band struct {
	layers []*layer
	// at is the song's frame, and phrase the phrase it is in.
	at     int64
	phrase int
	// want is how many synths the conductor wants playing, wandering a
	// step a phrase.
	want int
	// soloAt is the phrase the solo last started at.
	soloAt int
	rng    *rand.Rand
	buf    []float32
}

// The conductor's limits.
const (
	// maxLoops is the most times a synth loops before it leaves.
	maxLoops = 4
	// soloRest is how many phrases go by between solos at least, and
	// soloOdds the chance of one at each phrase after.
	soloRest = 6
	soloOdds = 0.3
)

// newBand starts a band of synths, seeded by seed, with two of them
// coming in.
func newBand(synths []*synth, seed uint64) *band {
	b := &band{rng: rand.New(rand.NewPCG(seed, 0x5eed)), want: 2, soloAt: -soloRest}
	for _, s := range synths {
		b.layers = append(b.layers, &layer{s: s, seamAt: -1})
	}
	b.plan(0)
	b.turn()
	return b
}

// Read implements [audio.Source]: the synths playing, summed, a part
// changing exactly where its phrase does.
func (b *band) Read(dst []float32) (int, error) {
	n := len(dst) / 2
	clear(dst[:2*n])
	for done := 0; done < n; {
		k := int(min(int64(n-done), phraseAt(b.phrase+1)-b.at))
		for _, l := range b.layers {
			b.buf = l.mix(dst[2*done:2*(done+k)], b.buf)
		}
		done += k
		b.at += int64(k)
		if b.at == phraseAt(b.phrase+1) {
			b.phrase++
			b.turn()
		}
	}
	return n, nil
}

// turn starts the phrase planned, and plans the next.
func (b *band) turn() {
	for _, l := range b.layers {
		n := l.next
		l.state, l.loops = n.state, n.loops
		if l.state == resting {
			l.cur = nil
		}
		if n.parts != nil {
			ps := <-n.parts
			l.cur = ps[0]
			if ps[1] != nil {
				l.seam, l.seamAt = ps[1], 0
			}
		}
		l.next = plan{}
	}
	b.plan(b.phrase + 1)
}

// plan chooses what each synth plays in phrase q, from what it plays
// in the phrase before: who comes in, who loops again and who leaves.
// It starts the parts they need opening.
func (b *band) plan(q int) {
	playing, looping := 0, 0
	for _, l := range b.layers {
		n := plan{state: l.state, loops: l.loops}
		switch l.state {
		case inIntro:
			n.state, n.part = inLoop, "loop"
		case inLoop:
			if l.loops > 0 {
				n.loops--
				n.part, n.seam = "loop", true
			} else {
				n.state, n.part = inOutro, "outro"
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
	// The mood wanders, from one synth to all.
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
			l.next = plan{state: inOutro, part: "outro"}
			playing--
			looping--
		}
	}
	// Too few: synths that rested all the phrase before come in.
	for _, i := range b.rng.Perm(len(b.layers)) {
		l := b.layers[i]
		if playing >= b.want {
			break
		}
		if !l.s.solo && l.state == resting && l.next.state == resting {
			l.next = plan{state: inIntro, loops: b.rng.IntN(maxLoops), part: "intro"}
			playing++
		}
	}
	// Now and then, over a few synths looping, the solo: its 20 bars
	// run 4 into the phrase after.
	if looping >= 2 && q-b.soloAt >= soloRest && b.rng.Float64() < soloOdds {
		for _, l := range b.layers {
			if l.s.solo && l.state == resting && l.next.state == resting {
				l.next = plan{state: inSolo, loops: 1, part: "solo"}
				b.soloAt = q
				break
			}
		}
	}
	for _, l := range b.layers {
		if l.next.part != "" {
			l.next.parts = openAhead(l.s, l.next.part, l.next.seam)
		}
	}
}

// openAhead opens synth s's part on a goroutine of its own, and with
// seam its outro too, and hands them over on the channel it returns.
func openAhead(s *synth, part string, seam bool) chan [2]audio.Source {
	ch := make(chan [2]audio.Source, 1)
	go func() {
		var ps [2]audio.Source
		ps[0] = open(s, part)
		if seam {
			ps[1] = open(s, "outro")
		}
		ch <- ps
	}()
	return ch
}

// open opens synth s's part, or returns nil, for silence, where it will
// not open.
func open(s *synth, part string) audio.Source {
	src, err := s.open(part)
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
var _ audio.Source = (*band)(nil)

// bandSeed seeds a band from the time, so each game's song is its own.
func bandSeed() uint64 { return uint64(time.Now().UnixNano()) }
