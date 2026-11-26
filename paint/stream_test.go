package paint

import (
	"image"
	"testing"
)

func TestAStreamsPicturesAreImagesOfTheirOwn(t *testing.T) {
	var s Stream
	a, b := s.Image(1, 2, make([]byte, 8)), s.Image(1, 2, make([]byte, 8))
	if a == b || a.Stream() != &s || b.Stream() != &s {
		t.Fatal("two pictures are not two of the stream's")
	}
	if w, h := a.Size(); w != 1 || h != 2 || len(a.Pix()) != 8 {
		t.Fatalf("a picture is %d by %d", w, h)
	}
	if NewImage(image.NewRGBA(image.Rect(0, 0, 1, 1))).Stream() != nil {
		t.Fatal("an image of no stream has one")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("a picture too short was taken")
		}
	}()
	s.Image(2, 2, make([]byte, 8))
}
