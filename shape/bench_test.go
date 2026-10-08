package shape

import (
	"strings"
	"testing"
)

// dense is a path of 20000 lines, all on one diagonal, there and back: the work of a drawing with many strokes.
func dense() string {
	return "M0 0" + strings.Repeat("L100 100L0 0", 10000)
}

func BenchmarkStrokeOfADensePath(b *testing.B) {
	p, err := NewPath(dense())
	if err != nil {
		b.Fatal(err)
	}
	s := p.Stroke(1)
	for b.Loop() {
		s.Coverage(256, 256)
	}
}

func BenchmarkFillOfADensePath(b *testing.B) {
	p, err := NewPath(dense())
	if err != nil {
		b.Fatal(err)
	}
	f := p.Fill()
	for b.Loop() {
		f.Coverage(256, 256)
	}
}
