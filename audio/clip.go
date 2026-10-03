package audio

import (
	"errors"
	"io"
)

// A Clip is a sound held in memory, ready to play at once and any
// number of times over: a sound for what the user does, as a click.
type Clip struct {
	frames []float32
}

// NewClip returns a clip of samples: frames of left then right, at
// [SampleRate]. The clip keeps samples, which must not change after.
func NewClip(samples []float32) *Clip {
	return &Clip{frames: samples[:len(samples)/2*2]}
}

// Load reads a whole sound into a clip, decoding it as [Decode] does.
func Load(r io.ReadSeeker) (*Clip, error) {
	s, err := Decode(r)
	if err != nil {
		return nil, err
	}
	return ReadClip(s)
}

// ReadClip reads src to its end into a clip.
func ReadClip(src Source) (*Clip, error) {
	var out []float32
	buf := make([]float32, 2*4096)
	for {
		n, err := src.Read(buf)
		out = append(out, buf[:2*n]...)
		if errors.Is(err, io.EOF) {
			return NewClip(out), nil
		}
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, io.ErrNoProgress
		}
	}
}

// Len returns the clip's length in frames.
func (c *Clip) Len() int64 { return int64(len(c.frames) / 2) }

// Samples returns the clip's frames, left then right. They must not be
// changed.
func (c *Clip) Samples() []float32 { return c.frames }

// Source returns a new source playing the clip from its start. Each
// voice playing a clip needs a source of its own.
func (c *Clip) Source() Seeker { return &clipSource{c: c} }

type clipSource struct {
	c  *Clip
	at int
}

// Read implements [Source].
func (s *clipSource) Read(dst []float32) (int, error) {
	n := copy(dst[:len(dst)/2*2], s.c.frames[2*s.at:]) / 2
	s.at += n
	if s.at >= len(s.c.frames)/2 {
		return n, io.EOF
	}
	return n, nil
}

// SeekFrame implements [Seeker].
func (s *clipSource) SeekFrame(f int64) error {
	s.at = int(max(0, min(f, s.c.Len())))
	return nil
}

// Len implements [Seeker].
func (s *clipSource) Len() int64 { return s.c.Len() }
