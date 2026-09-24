package paint

import (
	"encoding/json"
	"image"
	"image/color"
	"testing"
)

func TestImageIsPremultiplied(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 200, A: 128})
	m := NewImage(src)
	if w, h := m.Size(); w != 2 || h != 1 {
		t.Fatalf("size %dx%d, want 2x1", w, h)
	}
	if r, a := m.Pix()[0], m.Pix()[3]; a != 128 || r < 99 || r > 101 {
		t.Fatalf("pixel r=%d a=%d, want r about 100 and a 128", r, a)
	}
}

func TestImageSurvivesJSON(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i := range src.Pix {
		src.Pix[i] = uint8(i * 20)
	}
	for i := 3; i < len(src.Pix); i += 4 {
		src.Pix[i] = 255
	}
	data, err := json.Marshal(NewImage(src))
	if err != nil {
		t.Fatal(err)
	}
	var back Image
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if string(back.Pix()) != string(src.Pix) {
		t.Fatalf("pixels changed crossing JSON:\n got %v\nwant %v", back.Pix(), src.Pix)
	}
}
