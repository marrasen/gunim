package audio

import (
	"math"
	"math/bits"
	"math/cmplx"
)

// An FFT transforms blocks of one size, a power of two, from time to
// frequency, its factors worked out once, for sound transformed block
// after block, as a spectrogram is.
type FFT struct {
	n   int
	rev []int
	// tw holds each stage's factors side by side: the stage of butterflies half apart reads tw[half : 2*half].
	tw []complex128
}

// NewFFT returns an FFT of blocks of n, a power of two.
func NewFFT(n int) *FFT {
	if n < 2 || n&(n-1) != 0 {
		panic("audio: FFT of a size that is no power of two")
	}
	f := &FFT{n: n, rev: make([]int, n), tw: make([]complex128, n)}
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		f.rev[i] = j
	}
	base := make([]complex128, n/2)
	for k := range base {
		base[k] = cmplx.Exp(complex(0, -2*math.Pi*float64(k)/float64(n)))
	}
	for half := 1; half < n; half <<= 1 {
		step := n / (2 * half)
		for k := range half {
			f.tw[half+k] = base[k*step]
		}
	}
	return f
}

// Transform transforms x, of the FFT's size, in place.
func (f *FFT) Transform(x []complex128) {
	n := f.n
	x = x[:n]
	for i, j := range f.rev {
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	half := 1
	if bits.TrailingZeros(uint(n))%2 == 1 {
		w0 := f.tw[1]
		for i := 0; i+1 < n; i += 2 {
			u, t := x[i], w0*x[i+1]
			x[i], x[i+1] = u+t, u-t
		}
		half = 2
	}
	for ; half < n; half <<= 2 {
		f.stages(x, half)
	}
}

// stages runs the butterflies half apart and then those twice as far apart, a group of four at a time.
func (f *FFT) stages(x []complex128, half int) {
	wa, wb, wc := f.tw[half:2*half], f.tw[2*half:3*half], f.tw[3*half:4*half]
	for start := 0; start < len(x); start += 4 * half {
		x0, x1 := x[start:start+half], x[start+half:start+2*half]
		x2, x3 := x[start+2*half:start+3*half], x[start+3*half:start+4*half]
		x0, x1, x2, x3 = x0[:len(wa)], x1[:len(wa)], x2[:len(wa)], x3[:len(wa)]
		wb, wc := wb[:len(wa)], wc[:len(wa)]
		for k, w := range wa {
			u, t := x0[k], w*x1[k]
			a0, a1 := u+t, u-t
			u, t = x2[k], w*x3[k]
			a2, a3 := u+t, u-t
			t = wb[k] * a2
			x0[k], x2[k] = a0+t, a0-t
			t = wc[k] * a3
			x1[k], x3[k] = a1+t, a1-t
		}
	}
}
