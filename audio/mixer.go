package audio

import (
	"errors"
	"io"
	"sync"
	"time"

	"github.com/marrasen/gunim/anim"
)

// block is how many frames the mixer steps volumes and pans by. Within
// a block each moves in a straight line, so a fade has no steps to hear.
const block = 128

// historyFrames is how much of what it played the mixer keeps for an
// [Analyzer]: a power of two, more than the speakers hold back.
const historyFrames = 1 << 15

// declick is how long a pause, a resume and a stop with no fade of
// their own take to fade, so the sound never clicks off.
const declick = 6 * time.Millisecond

// A Mixer sums the sounds playing into one stream. Its methods, and
// its voices', are safe to call from any goroutine.
type Mixer struct {
	mu     sync.Mutex
	voices []*Voice
	// played counts the frames mixed so far.
	played int64
	// latency returns how many of the frames mixed the speakers have
	// still to play.
	latency func() int64
	// history holds the last frames mixed, as mono, at played modulo
	// its length.
	history []float32
	buf     []float32
	// out is Read's buffer; Read runs on one goroutine at a time.
	out []float32
}

// NewMixer returns a mixer playing nothing.
func NewMixer() *Mixer {
	return &Mixer{history: make([]float32, historyFrames), buf: make([]float32, 2*block)}
}

// Options says how a sound starts playing.
type Options struct {
	// Volume scales the sound, 1 playing it as recorded. Zero, the
	// default, plays it at 1 too: a sound starts silent with FadeIn.
	Volume float32
	// Pan places the sound from -1, the left speaker only, to 1, the
	// right only.
	Pan float32
	// FadeIn rises the sound from silence over this long.
	FadeIn time.Duration
	// Loop starts the sound over as it ends. The source must be a
	// [Seeker].
	Loop bool
	// Paused readies the sound without playing it, until
	// [Voice.Resume].
	Paused bool
}

// Play starts src playing and returns its voice.
func (m *Mixer) Play(src Source, o Options) *Voice {
	vol := o.Volume
	if vol == 0 {
		vol = 1
	}
	v := &Voice{
		m:      m,
		src:    src,
		vol:    anim.NewFloat(vol),
		pan:    anim.NewFloat(o.Pan),
		gate:   anim.NewFloat(1),
		loop:   o.Loop,
		paused: o.Paused,
		done:   make(chan struct{}),
	}
	if o.FadeIn > 0 {
		v.gate.Jump(0)
		v.gate.Animate(1, anim.Tween{Duration: o.FadeIn, Ease: anim.Linear})
	}
	m.mu.Lock()
	v.marks[0] = mark{mixed: m.played}
	v.nmarks = 1
	m.voices = append(m.voices, v)
	m.mu.Unlock()
	return v
}

// Playing returns how many voices are playing or paused.
func (m *Mixer) Playing() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.voices)
}

// SetLatency tells the mixer how many of the frames it mixed are still
// to be heard, as the speakers hold them back. Package audio/speaker
// sets it; a voice's position and an [Analyzer] count with it.
func (m *Mixer) SetLatency(f func() int64) {
	m.mu.Lock()
	m.latency = f
	m.mu.Unlock()
}

// Mixed returns how many frames the mixer has mixed.
func (m *Mixer) Mixed() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.played
}

// heardLocked returns how many frames have been heard. It runs with mu
// held.
func (m *Mixer) heardLocked() int64 {
	if m.latency == nil {
		return m.played
	}
	return max(0, m.played-m.latency())
}

// Mix fills dst, len(dst)/2 frames, with the sounds playing, and
// silence where none plays.
func (m *Mixer) Mix(dst []float32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for len(dst) >= 2 {
		n := min(len(dst)/2, block)
		m.mixBlock(dst[:2*n])
		dst = dst[2*n:]
	}
}

// Read implements [io.Reader] for a device that takes the mix as
// little-endian float32 samples, as package audio/speaker does. It
// always fills whole frames.
func (m *Mixer) Read(p []byte) (int, error) {
	frames := len(p) / 8
	if frames == 0 {
		return 0, io.ErrShortBuffer
	}
	if m.out == nil {
		m.out = make([]float32, 2*1024)
	}
	done := 0
	for done < frames {
		n := min(frames-done, len(m.out)/2)
		m.Mix(m.out[:2*n])
		putFloats(p[8*done:], m.out[:2*n])
		done += n
	}
	return 8 * frames, nil
}

// mixBlock mixes one block, at most block frames, into dst. It runs
// with mu held.
func (m *Mixer) mixBlock(dst []float32) {
	clear(dst)
	frames := len(dst) / 2
	dt := Duration(int64(frames))
	live := m.voices[:0]
	for _, v := range m.voices {
		if v.mix(dst, m.buf[:2*frames], dt) {
			live = append(live, v)
		} else {
			v.finish()
		}
	}
	clear(m.voices[len(live):])
	m.voices = live
	for i := range frames {
		l, r := clip(dst[2*i]), clip(dst[2*i+1])
		dst[2*i], dst[2*i+1] = l, r
		m.history[(m.played+int64(i))&(historyFrames-1)] = (l + r) / 2
	}
	m.played += int64(frames)
}

// clip keeps a sample within -1 to 1.
func clip(s float32) float32 { return max(-1, min(1, s)) }

// A Voice is one sound playing in a [Mixer].
type Voice struct {
	m    *Mixer
	src  Source
	loop bool
	done chan struct{}

	// The fields below are guarded by the mixer's mu.

	// vol and pan are the voice's own; gate fades it for a pause, a
	// resume, a stop and FadeIn.
	vol, pan, gate *anim.Float
	paused         bool
	// pausing and stopping say the gate is closing for a pause or a
	// stop.
	pausing, stopping bool
	// at is the frame of the source the voice plays next.
	at    int64
	ended bool
	err   error
	// marks says which frames of the source the last blocks mixed, so
	// Position can tell which the speakers are playing.
	marks  [markCount]mark
	nmarks int
	next   int
}

// markCount is how many blocks a voice remembers: more than the
// speakers hold back.
const markCount = 512

// A mark records that the block mixed at mixer frame mixed took n
// frames of the source from frame at.
type mark struct {
	mixed, at int64
	n         int
}

// markLocked records a block. It runs with the mixer's mu held.
func (v *Voice) markLocked(k mark) {
	v.marks[v.next] = k
	v.next = (v.next + 1) % markCount
	v.nmarks = min(v.nmarks+1, markCount)
}

// mix adds the voice's next frames into dst, using buf, and reports
// whether it plays on. It runs with the mixer's mu held.
func (v *Voice) mix(dst, buf []float32, dt time.Duration) bool {
	if v.ended {
		return false
	}
	frames := len(dst) / 2
	if v.paused {
		return true
	}
	at := v.at
	l0, r0 := v.gains()
	v.vol.Step(dt)
	v.pan.Step(dt)
	v.gate.Step(dt)
	l1, r1 := v.gains()
	n := v.fill(buf)
	if v.at >= at {
		v.markLocked(mark{mixed: v.m.played, at: at, n: n})
	} else {
		// It looped: the block's frames run from the start.
		v.markLocked(mark{mixed: v.m.played, at: v.at - int64(n), n: n})
	}
	for i := range n {
		t := float32(i) / float32(frames)
		dst[2*i] += buf[2*i] * (l0 + (l1-l0)*t)
		dst[2*i+1] += buf[2*i+1] * (r0 + (r1-r0)*t)
	}
	if v.gate.Value() == 0 && !v.gate.Active() {
		switch {
		case v.stopping:
			return false
		case v.pausing:
			v.paused, v.pausing = true, false
		}
	}
	return !v.ended
}

// gains returns the left and right gains the voice is at.
func (v *Voice) gains() (l, r float32) {
	g := max(0, v.vol.Value()) * max(0, min(1, v.gate.Value()))
	p := max(-1, min(1, v.pan.Value()))
	return g * min(1, 1-p), g * min(1, 1+p)
}

// fill reads the voice's next frames into buf, starting over as a
// looping voice ends, and returns how many it read. It marks the voice
// ended once its source is.
func (v *Voice) fill(buf []float32) int {
	want := len(buf) / 2
	n := 0
	restarted := false
	for n < want {
		got, err := v.src.Read(buf[2*n : 2*want])
		n += got
		v.at += int64(got)
		if got > 0 {
			restarted = false
		}
		if err == nil {
			if got == 0 {
				break
			}
			continue
		}
		if errors.Is(err, io.EOF) && v.loop && !restarted {
			// A looping source with nothing in it ends, rather than
			// start over without end.
			if s, ok := v.src.(Seeker); ok && s.SeekFrame(0) == nil {
				v.at, restarted = 0, true
				continue
			}
		}
		if !errors.Is(err, io.EOF) {
			v.err = err
		}
		v.ended = true
		break
	}
	clear(buf[2*n:])
	return n
}

// finish marks the voice done, once, as the mixer drops it.
func (v *Voice) finish() {
	select {
	case <-v.done:
	default:
		close(v.done)
	}
}

// Done is closed when the voice has ended or stopped.
func (v *Voice) Done() <-chan struct{} { return v.done }

// Err returns the error that ended the voice early, if any.
func (v *Voice) Err() error {
	v.m.mu.Lock()
	defer v.m.mu.Unlock()
	return v.err
}

// Stop fades the voice out over fade and ends it. A fade of zero still
// takes a few milliseconds, so the sound never clicks off.
func (v *Voice) Stop(fade time.Duration) {
	v.m.mu.Lock()
	defer v.m.mu.Unlock()
	if v.paused {
		v.ended = true
		return
	}
	v.stopping = true
	v.gate.Animate(0, anim.Tween{Duration: max(fade, declick), Ease: anim.Linear})
}

// Pause holds the voice where it is, after a short fade.
func (v *Voice) Pause() {
	v.m.mu.Lock()
	defer v.m.mu.Unlock()
	if v.paused || v.stopping {
		return
	}
	v.pausing = true
	v.gate.Animate(0, anim.Tween{Duration: declick, Ease: anim.Linear})
}

// Resume plays a paused voice on from where it paused.
func (v *Voice) Resume() {
	v.m.mu.Lock()
	defer v.m.mu.Unlock()
	if v.stopping {
		return
	}
	v.paused, v.pausing = false, false
	v.gate.Animate(1, anim.Tween{Duration: declick, Ease: anim.Linear})
}

// Paused reports whether the voice is paused, or pausing.
func (v *Voice) Paused() bool {
	v.m.mu.Lock()
	defer v.m.mu.Unlock()
	return v.paused || v.pausing
}

// SetVolume moves the voice's volume to vol with motion m, or jumps
// there with a nil m.
func (v *Voice) SetVolume(vol float32, m anim.Motion) {
	v.m.mu.Lock()
	defer v.m.mu.Unlock()
	set(v.vol, vol, m)
}

// Volume returns the volume the voice is heading for.
func (v *Voice) Volume() float32 {
	v.m.mu.Lock()
	defer v.m.mu.Unlock()
	return v.vol.Target()
}

// SetPan moves the voice's pan to p, from -1, left, to 1, right, with
// motion m, or jumps there with a nil m.
func (v *Voice) SetPan(p float32, m anim.Motion) {
	v.m.mu.Lock()
	defer v.m.mu.Unlock()
	set(v.pan, p, m)
}

func set(a *anim.Float, to float32, m anim.Motion) {
	if m == nil {
		a.Jump(to)
		return
	}
	a.Animate(to, m)
}

// ErrNotSeekable is returned by [Voice.Seek] for a source that is not a
// [Seeker].
var ErrNotSeekable = errors.New("audio: the source cannot seek")

// Seek moves the voice to d from the start of its sound.
func (v *Voice) Seek(d time.Duration) error {
	v.m.mu.Lock()
	defer v.m.mu.Unlock()
	s, ok := v.src.(Seeker)
	if !ok {
		return ErrNotSeekable
	}
	f := max(0, Frames(d))
	if l := s.Len(); l >= 0 {
		f = min(f, l)
	}
	if err := s.SeekFrame(f); err != nil {
		return err
	}
	v.at = f
	v.ended = false
	// Until the speakers play what is mixed next, the voice is heard
	// where it moved to.
	v.nmarks, v.next = 0, 0
	v.markLocked(mark{mixed: v.m.played, at: f})
	return nil
}

// Position returns how far into its sound the voice is, as heard: the
// frames the speakers hold back are not counted yet.
func (v *Voice) Position() time.Duration {
	v.m.mu.Lock()
	defer v.m.mu.Unlock()
	heard := v.m.heardLocked()
	// The latest block mixed at or before the frame being heard says
	// where in the source that is.
	var k mark
	for i := 1; i <= v.nmarks; i++ {
		k = v.marks[(v.next-i+markCount)%markCount]
		if k.mixed <= heard {
			return Duration(k.at + min(heard-k.mixed, int64(k.n)))
		}
	}
	return Duration(k.at)
}

// Len returns the length of the voice's sound, or -1 when it is
// unknown.
func (v *Voice) Len() time.Duration {
	s, ok := v.src.(Seeker)
	if !ok {
		return -1
	}
	v.m.mu.Lock()
	defer v.m.mu.Unlock()
	l := s.Len()
	if l < 0 {
		return -1
	}
	return Duration(l)
}
