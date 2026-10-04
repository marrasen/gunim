package audio

import (
	"math"
	"math/cmplx"
)

// An FFT transforms blocks of one size, a power of two, from time to
// frequency, its factors worked out once, for sound transformed block
// after block, as a spectrogram is.
type FFT struct {
	n   int
	rev []int
	tw  []complex128
}

// NewFFT returns an FFT of blocks of n, a power of two.
func NewFFT(n int) *FFT {
	if n < 2 || n&(n-1) != 0 {
		panic("audio: FFT of a size that is no power of two")
	}
	f := &FFT{n: n, rev: make([]int, n), tw: make([]complex128, n/2)}
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		f.rev[i] = j
	}
	for k := range f.tw {
		f.tw[k] = cmplx.Exp(complex(0, -2*math.Pi*float64(k)/float64(n)))
	}
	return f
}

// Transform transforms x, of the FFT's size, in place.
func (f *FFT) Transform(x []complex128) {
	n := f.n
	for i, j := range f.rev {
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		step := n / size
		for start := 0; start < n; start += size {
			for k := range size / 2 {
				u, t := x[start+k], f.tw[k*step]*x[start+k+size/2]
				x[start+k], x[start+k+size/2] = u+t, u-t
			}
		}
	}
}
