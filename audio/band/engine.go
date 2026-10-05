package band

import (
	"math"
	"sync"
	"sync/atomic"

	"github.com/marrasen/gunim/audio"
)

// timing is a song's time: its tempo, and its bars and phrases.
type timing struct {
	bpm        float64
	beats      int
	phraseBars int
}

// newTiming returns the timing of a song of bpm, beats to a bar and
// phraseBars to a phrase, zero meaning 4 and defaultPhrase.
func newTiming(bpm float64, beats, phraseBars, defaultPhrase int) timing {
	if beats == 0 {
		beats = 4
	}
	if phraseBars == 0 {
		phraseBars = defaultPhrase
	}
	return timing{bpm: bpm, beats: beats, phraseBars: phraseBars}
}

// barAt returns the frame bar b starts at, rounded as the pieces are
// cut.
func (t timing) barAt(b int) int64 {
	return int64(float64(b)*audio.SampleRate*60*float64(t.beats)/t.bpm + 0.5)
}

// phraseAt returns the frame phrase p starts at.
func (t timing) phraseAt(p int) int64 { return t.barAt(p * t.phraseBars) }

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

// carryOn returns the plan for a layer that goes on as it does: an
// intro into its loop, a loop round again while it has loops left and
// into its outro after, an outro to rest, and a solo on while it lasts.
func (l *layer) carryOn() plan {
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
	return n
}

// engine plays layers in phrases: each piece starts exactly where its
// phrase does, and choose says what each layer plays in the phrase
// after.
type engine struct {
	t      timing
	layers []*layer
	// at is the song's frame, and phrase the phrase it is in. played is
	// at, for Watch.
	at     int64
	phrase int
	played atomic.Int64
	// choose sets each layer's next for phrase q.
	choose func(q int)
	// stale, when set, reports that the coming phrase needs choosing
	// again, as the tier asked for has changed.
	stale func() bool
	// control, when set, returns layer i's control, for Watch.
	control func(i int) PartControl
	buf     []float32
	// mu guards status, what the layers play, for Watch.
	mu     sync.Mutex
	status []PartStatus
}

// start plans the first phrase and starts it.
func (e *engine) start() {
	e.plan(0)
	e.turn()
}

// Read implements [audio.Source]: the parts playing, summed, a piece
// changing exactly where its phrase does.
func (e *engine) Read(dst []float32) (int, error) {
	n := len(dst) / 2
	clear(dst[:2*n])
	if e.stale != nil && e.stale() {
		// The pieces opened for the old plan go unplayed.
		e.plan(e.phrase + 1)
	}
	for done := 0; done < n; {
		k := int(min(int64(n-done), e.t.phraseAt(e.phrase+1)-e.at))
		for _, l := range e.layers {
			e.buf = l.mix(dst[2*done:2*(done+k)], e.buf)
		}
		done += k
		e.at += int64(k)
		if e.at == e.t.phraseAt(e.phrase+1) {
			e.phrase++
			e.turn()
		}
	}
	e.played.Store(e.at)
	return n, nil
}

// turn starts the phrase planned, and plans the next.
func (e *engine) turn() {
	phrase := e.t.phraseAt(1)
	status := make([]PartStatus, len(e.layers))
	for i, l := range e.layers {
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
		status[i] = PartStatus{Name: l.p.Name, Tier: l.p.Tier, Playing: l.state != resting,
			Piece: [...]Piece{Intro, Intro, Loop, Outro, Solo}[l.state]}
	}
	e.mu.Lock()
	e.status = status
	e.mu.Unlock()
	e.plan(e.phrase + 1)
}

// plan chooses what each layer plays in phrase q, and starts the pieces
// they need opening.
func (e *engine) plan(q int) {
	e.choose(q)
	for _, l := range e.layers {
		if l.next.opens {
			l.next.pieces = openAhead(l.p, l.next.piece, l.next.seam)
		}
	}
}

// Watch implements [Watcher].
func (e *engine) Watch() Status {
	at := e.played.Load()
	bar := int(float64(at) * e.t.bpm / (audio.SampleRate * 60 * float64(e.t.beats)))
	e.mu.Lock()
	parts := append([]PartStatus(nil), e.status...)
	e.mu.Unlock()
	if e.control != nil {
		for i := range parts {
			parts[i].Control = e.control(i)
		}
	}
	return Status{Bar: bar, PhraseBars: e.t.phraseBars, Parts: parts}
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
