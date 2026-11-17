package audio

import (
	"math"
	"math/cmplx"
	"math/rand/v2"
	"strconv"
	"testing"
)

func TestAnFFTFindsATonesPitchAsTheAnalyzersDoes(t *testing.T) {
	const n = 4096
	f := NewFFT(n)
	x, y := make([]complex128, n), make([]complex128, n)
	for i := range x {
		v := math.Sin(2*math.Pi*100*float64(i)/n) + 0.25*math.Cos(2*math.Pi*700*float64(i)/n)
		x[i], y[i] = complex(v, 0), complex(v, 0)
	}
	f.Transform(x)
	fft(y)
	for k := range x {
		if cmplx.Abs(x[k]-y[k]) > 1e-6*n {
			t.Fatalf("bin %d: %v, the analyzer's %v", k, x[k], y[k])
		}
	}
	if m := cmplx.Abs(x[100]); math.Abs(m-n/2) > 1e-6*n {
		t.Fatalf("the tone's bin is %.1f, want %d", m, n/2)
	}
}

// transformBefore is the plain radix-2 FFT that Transform must match to the bit.
func transformBefore(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	tw := make([]complex128, n/2)
	for k := range tw {
		tw[k] = cmplx.Exp(complex(0, -2*math.Pi*float64(k)/float64(n)))
	}
	for size := 2; size <= n; size <<= 1 {
		step := n / size
		for start := 0; start < n; start += size {
			for k := range size / 2 {
				u, t := x[start+k], tw[k*step]*x[start+k+size/2]
				x[start+k], x[start+k+size/2] = u+t, u-t
			}
		}
	}
}

func TestAnFFTTransformsToTheBitAsThePlainFFTDoes(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 11))
	inputs := map[string]func(i, n int) complex128{
		"noise":   func(int, int) complex128 { return complex(2*r.Float64()-1, 2*r.Float64()-1) },
		"real":    func(int, int) complex128 { return complex(r.NormFloat64(), 0) },
		"tone":    func(i, n int) complex128 { return complex(math.Sin(2*math.Pi*13.3*float64(i)/float64(n)), 0) },
		"impulse": func(i, _ int) complex128 { return complex(float64(1-min(i, 1)), 0) },
		"zeros": func(i, _ int) complex128 {
			return complex(math.Copysign(0, float64(i%2)-0.5), math.Copysign(0, float64(i%3)-1.5))
		},
		"wide": func(int, int) complex128 {
			return complex(math.Ldexp(r.Float64(), r.IntN(1200)-600), math.Ldexp(-r.Float64(), r.IntN(1200)-600))
		},
		"specials": func(i, _ int) complex128 {
			switch i % 97 {
			case 5:
				return complex(math.Inf(1), 0)
			case 50:
				return complex(0, math.NaN())
			}
			return complex(r.Float64(), 0)
		},
	}
	for bits := 1; bits <= 16; bits++ {
		n := 1 << bits
		f := NewFFT(n)
		for name, in := range inputs {
			for round := range 3 {
				x, y := make([]complex128, n), make([]complex128, n)
				for i := range x {
					x[i] = in(i, n)
				}
				copy(y, x)
				f.Transform(x)
				transformBefore(y)
				for k := range x {
					if !sameBits(real(x[k]), real(y[k])) || !sameBits(imag(x[k]), imag(y[k])) {
						t.Fatalf("%s %d, round %d, bin %d: %v, the plain FFT's %v", name, n, round, k, x[k], y[k])
					}
				}
			}
		}
	}
}

// BenchmarkFFT times a transform of each size.
func BenchmarkFFT(b *testing.B) {
	for _, n := range []int{256, 2048, 4096, 65536} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			f := NewFFT(n)
			in, x := make([]complex128, n), make([]complex128, n)
			for i := range in {
				in[i] = complex(math.Sin(float64(i)), 0)
			}
			for b.Loop() {
				copy(x, in)
				f.Transform(x)
			}
		})
	}
}

// sameBits reports whether a and b are the same to the bit, or both NaN, whose bits follow the operand order the
// compiler picks.
func sameBits(a, b float64) bool {
	return math.Float64bits(a) == math.Float64bits(b) || math.IsNaN(a) && math.IsNaN(b)
}
