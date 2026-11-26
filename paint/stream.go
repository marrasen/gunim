package paint

import "fmt"

// A Stream is a picture that changes often, as a camera's or a video's. Each of its pictures is an [Image] of its own, and
// a driver keeps one texture for the stream, which the picture drawn last is in. The zero value is ready.
type Stream struct{ _ byte }

// Image returns the stream's next picture, w by h pixels of premultiplied RGBA row by row from the top. It keeps pix, which
// must not be changed. It panics where pix is not as long as that takes.
func (s *Stream) Image(w, h int, pix []byte) *Image {
	if w < 0 || h < 0 || len(pix) != 4*w*h {
		panic(fmt.Sprintf("paint: %d bytes for a picture of %d by %d", len(pix), w, h))
	}
	return &Image{w: w, h: h, pix: pix, stream: s}
}

// Stream returns the stream m is a picture of, or nil.
func (m *Image) Stream() *Stream { return m.stream }
