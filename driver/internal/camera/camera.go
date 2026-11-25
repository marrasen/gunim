// Package camera makes a picture to show, and brightness to read, of a camera's frame.
package camera

import "errors"

// Frame is a frame as a camera gives it, in three planes: brightness, and two of colour at half the size each way. The
// planes are read and not kept.
type Frame struct {
	Y, U, V []byte
	// Width and Height are the frame's size in pixels.
	Width, Height int
	// YStride is how many bytes a row of Y takes, UVStride how many one of U or V does, and UVPixel how many bytes a
	// pixel of U or V is from the next.
	YStride, UVStride, UVPixel int
	// Turn is how many degrees clockwise the frame is to be turned to stand upright on the screen: 0, 90, 180 or 270.
	// Mirror says that it is then shown as in a mirror, left for right.
	Turn   int
	Mirror bool
}

// maxSide is the largest frame taken, each way.
const maxSide = 1 << 14

// ErrFrame is returned for a frame whose planes do not hold what it says.
var ErrFrame = errors.New("camera: the frame's planes do not hold what it says")

// Check returns ErrFrame for a frame whose planes do not hold what it says, so that the rest reads within them.
func (f *Frame) Check() error {
	cw, ch := (f.Width+1)/2, (f.Height+1)/2
	switch {
	case f.Width <= 0, f.Height <= 0, f.Width > maxSide, f.Height > maxSide,
		f.YStride < f.Width, f.UVStride <= 0, f.UVPixel <= 0,
		f.YStride > 4*maxSide, f.UVStride > 4*maxSide, f.UVPixel > 4,
		f.UVStride <= f.UVPixel*(cw-1),
		len(f.Y) < f.YStride*(f.Height-1)+f.Width,
		len(f.U) <= f.UVStride*(ch-1)+f.UVPixel*(cw-1),
		len(f.V) <= f.UVStride*(ch-1)+f.UVPixel*(cw-1),
		f.Turn != 0 && f.Turn != 90 && f.Turn != 180 && f.Turn != 270:
		return ErrFrame
	}
	return nil
}

// Luma copies the frame's brightness, a byte a pixel, row by row as the camera gave it, into buf, which it grows where it
// must, and returns it.
func (f *Frame) Luma(buf []byte) ([]byte, error) {
	if err := f.Check(); err != nil {
		return nil, err
	}
	n := f.Width * f.Height
	if cap(buf) < n {
		buf = make([]byte, n)
	}
	buf = buf[:n]
	for y := range f.Height {
		copy(buf[y*f.Width:(y+1)*f.Width], f.Y[y*f.YStride:])
	}
	return buf, nil
}

// Picture returns the frame as it is to be shown, upright and mirrored where it is, in RGBA row by row from the top, and
// its size. It is as large as the frame, or as many whole times smaller as keeps it within most pixels each way.
func (f *Frame) Picture(most int) (pix []byte, w, h int, err error) {
	if err := f.Check(); err != nil {
		return nil, 0, 0, err
	}
	step := 1
	for most > 0 && max(f.Width, f.Height)/step > most {
		step++
	}
	sw, sh := max(f.Width/step, 1), max(f.Height/step, 1)
	w, h = sw, sh
	if f.Turn == 90 || f.Turn == 270 {
		w, h = sh, sw
	}
	pix = make([]byte, 4*w*h)
	// Where the picture has the frame's first pixel, and how far from one to the next it goes along the frame's rows and
	// down its columns: across, and down.
	var x0, y0 int
	var along, down [2]int
	switch f.Turn {
	case 0:
		along, down = [2]int{1, 0}, [2]int{0, 1}
	case 90:
		x0, along, down = w-1, [2]int{0, 1}, [2]int{-1, 0}
	case 180:
		x0, y0, along, down = w-1, h-1, [2]int{-1, 0}, [2]int{0, -1}
	case 270:
		y0, along, down = h-1, [2]int{0, -1}, [2]int{1, 0}
	}
	if f.Mirror {
		x0, along[0], down[0] = w-1-x0, -along[0], -down[0]
	}
	at, right, next := y0*w+x0, along[1]*w+along[0], down[1]*w+down[0]
	for y := range sh {
		fy := y * step
		row, crow := f.Y[fy*f.YStride:], fy/2*f.UVStride
		u, v := f.U[crow:], f.V[crow:]
		o := 4 * (at + y*next)
		for x := range sw {
			fx := x * step
			c := fx / 2 * f.UVPixel
			p := pix[o : o+4 : o+4]
			p[0], p[1], p[2] = rgb(row[fx], u[c], v[c])
			p[3] = 0xff
			o += 4 * right
		}
	}
	return pix, w, h, nil
}

// rgb is a colour of brightness y, and u and v, over the whole range of each, as cameras' JPEGs have it.
func rgb(y, u, v byte) (r, g, b byte) {
	l, cb, cr := int32(y)<<16+1<<15, int32(u)-128, int32(v)-128
	return clamp(l + 91881*cr), clamp(l - 22554*cb - 46802*cr), clamp(l + 116130*cb)
}

// clamp is v, with 16 bits under the point, as a byte.
func clamp(v int32) byte {
	switch v >>= 16; {
	case v < 0:
		return 0
	case v > 255:
		return 255
	}
	return byte(v)
}
