package audio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
)

// ErrFormat is returned by [Decode] for a sound in no format it reads.
var ErrFormat = errors.New("audio: unknown format")

// Decode returns a source playing the sound r holds: WAV, MP3, Ogg
// Vorbis or FLAC, told apart by how it starts. The source plays at
// [SampleRate], resampling a sound recorded at another, and in stereo:
// a mono sound plays in both speakers, and a sound of more channels
// plays its first two.
//
// The source reads r as it plays, so r must stay open until it is done
// with.
func Decode(r io.ReadSeeker) (Seeker, error) {
	var head [12]byte
	n, err := io.ReadFull(r, head[:])
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return nil, err
	}
	if _, err = r.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	var d decoder
	h := head[:n]
	switch {
	case bytes.HasPrefix(h, []byte("RIFF")) && len(h) >= 12 && string(h[8:12]) == "WAVE":
		d, err = newWAV(r)
	case bytes.HasPrefix(h, []byte("OggS")):
		d, err = newVorbis(r)
	case bytes.HasPrefix(h, []byte("fLaC")):
		d, err = newFLAC(r)
	case bytes.HasPrefix(h, []byte("ID3")) || len(h) >= 2 && h[0] == 0xff && h[1]&0xe0 == 0xe0:
		d, err = newMP3(r)
	default:
		return nil, ErrFormat
	}
	if err != nil {
		return nil, err
	}
	if d.rate() <= 0 {
		return nil, fmt.Errorf("audio: sample rate %d", d.rate())
	}
	if d.rate() == SampleRate {
		return direct{d}, nil
	}
	return newResampler(d), nil
}

// toStereo writes frames of ch channels from src, which holds whole
// frames, into dst as stereo, and returns how many frames it wrote.
func toStereo(dst, src []float32, ch int) int {
	n := min(len(dst)/2, len(src)/ch)
	for i := range n {
		if ch == 1 {
			dst[2*i], dst[2*i+1] = src[i], src[i]
		} else {
			dst[2*i], dst[2*i+1] = src[ch*i], src[ch*i+1]
		}
	}
	return n
}
