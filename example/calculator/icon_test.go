package main

import (
	"testing"
	"time"
)

// The icon is drawn at every size, with a tile that fills its middle
// and leaves its corners clear, soon enough to open the window with.
func TestTheIconIsDrawnAtEverySize(t *testing.T) {
	began := time.Now()
	imgs := icons()
	took := time.Since(began)
	if len(imgs) != len(iconSizes) {
		t.Fatalf("drew %d icons", len(imgs))
	}
	for i, img := range imgs {
		n := iconSizes[i]
		if b := img.Bounds(); b.Dx() != n || b.Dy() != n {
			t.Fatalf("the %d icon is %v", n, b)
		}
		if _, _, _, a := img.At(n/2, n/2).RGBA(); a != 0xffff {
			t.Fatalf("the %d icon's middle is not solid", n)
		}
		if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
			t.Fatalf("the %d icon's corner is not clear", n)
		}
	}
	t.Logf("drawn in %v", took)
	if took > 150*time.Millisecond {
		t.Fatalf("drawing the icons took %v", took)
	}
}
