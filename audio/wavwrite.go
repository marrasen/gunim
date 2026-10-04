package audio

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/rand/v2"
)

// A WAVWriter writes stereo sound to a WAV file: 16 or 24 bit integer
// samples, or 32 bit float. Dither, for integer samples, adds noise of
// a triangular distribution an LSB either way before each sample is
// rounded, so a quiet fade's last bits fade into a soft hiss rather
// than break into distortion, as rounding alone does.
type WAVWriter struct {
	w      io.WriteSeeker
	rate   int
	bits   int
	dither bool
	frames int64
	buf    []byte
	rng    *rand.Rand
	// Clipped counts the samples written past full scale, which were
	// held to it.
	Clipped int64
}

// NewWAVWriter starts a WAV file of sound at rate, in samples of bits:
// 16, 24, or 32 for float. Close finishes it.
func NewWAVWriter(w io.WriteSeeker, rate, bits int, dither bool) (*WAVWriter, error) {
	if bits != 16 && bits != 24 && bits != 32 {
		return nil, errors.New("audio: wav: 16, 24 or 32 bits")
	}
	ww := &WAVWriter{w: w, rate: rate, bits: bits, dither: dither && bits != 32,
		rng: rand.New(rand.NewPCG(0x6775, 0x6e69))}
	if err := ww.header(); err != nil {
		return nil, err
	}
	return ww, nil
}

// header writes the file's header, with the sizes of the frames so far.
func (w *WAVWriter) header() error {
	if _, err := w.w.Seek(0, io.SeekStart); err != nil {
		return err
	}
	bytes := w.bits / 8
	data := uint32(w.frames * int64(2*bytes))
	format := uint16(1)
	if w.bits == 32 {
		format = 3
	}
	var h [44]byte
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], 36+data)
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], format)
	binary.LittleEndian.PutUint16(h[22:], 2)
	binary.LittleEndian.PutUint32(h[24:], uint32(w.rate))
	binary.LittleEndian.PutUint32(h[28:], uint32(w.rate*2*bytes))
	binary.LittleEndian.PutUint16(h[32:], uint16(2*bytes))
	binary.LittleEndian.PutUint16(h[34:], uint16(w.bits))
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], data)
	_, err := w.w.Write(h[:])
	return err
}

// Write writes frames, interleaved stereo, at the file's rate.
func (w *WAVWriter) Write(frames []float32) error {
	bytes := w.bits / 8
	n := len(frames) / 2 * 2
	w.buf = w.buf[:0]
	// Full scale is 2^(bits-1), as readers divide by, held to one under
	// at the top.
	scale := float64(int64(1) << (w.bits - 1))
	full := scale - 1
	for _, s := range frames[:n] {
		if w.bits == 32 {
			w.buf = binary.LittleEndian.AppendUint32(w.buf, math.Float32bits(s))
			continue
		}
		v := float64(s) * scale
		if w.dither {
			v += w.rng.Float64() - w.rng.Float64()
		}
		v = math.Round(v)
		if v > full || v < -full-1 {
			w.Clipped++
			v = max(-full-1, min(v, full))
		}
		x := int32(v)
		switch bytes {
		case 2:
			w.buf = binary.LittleEndian.AppendUint16(w.buf, uint16(int16(x)))
		case 3:
			w.buf = append(w.buf, byte(x), byte(x>>8), byte(x>>16))
		}
	}
	if _, err := w.w.Write(w.buf); err != nil {
		return err
	}
	w.frames += int64(n / 2)
	return nil
}

// Close writes the file's sizes into its header. It leaves the file
// open.
func (w *WAVWriter) Close() error {
	end, err := w.w.Seek(0, io.SeekCurrent)
	if err != nil {
		return err
	}
	if headErr := w.header(); headErr != nil {
		return headErr
	}
	_, err = w.w.Seek(end, io.SeekStart)
	return err
}
