package audio

import (
	"math"
	"math/cmplx"
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
