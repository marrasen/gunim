package camera

import (
	"errors"
	"fmt"
	"slices"
	"testing"
)

// frame returns a w by h frame whose pixel at (x, y) is as bright as 10*y+x, with strides wider than it and colour planes
// laid through each other as phones do, and grey.
func frame(w, h int) *Frame {
	f := &Frame{Width: w, Height: h, YStride: w + 3, UVStride: w + w%2 + 5, UVPixel: 2}
	f.Y = make([]byte, f.YStride*(h-1)+w)
	for y := range h {
		for x := range w {
			f.Y[y*f.YStride+x] = byte(10*y + x)
		}
	}
	cw, ch := (w+1)/2, (h+1)/2
	uv := make([]byte, f.UVStride*(ch-1)+2*(cw-1)+2)
	for i := range uv {
		uv[i] = 128
	}
	f.U, f.V = uv[:len(uv)-1], uv[1:]
	return f
}

// reds returns the red of each pixel of a picture, row by row.
func reds(pix []byte, w, h int) [][]byte {
	out := make([][]byte, h)
	for y := range h {
		for x := range w {
			out[y] = append(out[y], pix[4*(y*w+x)])
		}
	}
	return out
}

// Every turn and mirroring shows the frame's corners where they belong, and loses no pixel.
func TestEachTurnStandsUpright(t *testing.T) {
	// 3 wide and 2 high:  0  1  2
	//                    10 11 12
	for name, c := range map[string]struct {
		turn   int
		mirror bool
		want   [][]byte
	}{
		"0":          {0, false, [][]byte{{0, 1, 2}, {10, 11, 12}}},
		"90":         {90, false, [][]byte{{10, 0}, {11, 1}, {12, 2}}},
		"180":        {180, false, [][]byte{{12, 11, 10}, {2, 1, 0}}},
		"270":        {270, false, [][]byte{{2, 12}, {1, 11}, {0, 10}}},
		"0 mirror":   {0, true, [][]byte{{2, 1, 0}, {12, 11, 10}}},
		"90 mirror":  {90, true, [][]byte{{0, 10}, {1, 11}, {2, 12}}},
		"180 mirror": {180, true, [][]byte{{10, 11, 12}, {0, 1, 2}}},
		"270 mirror": {270, true, [][]byte{{12, 2}, {11, 1}, {10, 0}}},
	} {
		f := frame(3, 2)
		f.Turn, f.Mirror = c.turn, c.mirror
		pix, w, h, err := f.Picture(0)
		if err != nil {
			t.Fatal(err)
		}
		if got := reds(pix, w, h); fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s: %v, want %v", name, got, c.want)
		}
	}
}

// A frame larger than asked for is made smaller by whole steps, for each turn, also a column or a row alone.
func TestALargeFrameIsMadeSmaller(t *testing.T) {
	for _, c := range []struct{ w, h, most, pw, ph int }{
		{8, 4, 8, 8, 4}, {8, 4, 4, 4, 2}, {8, 4, 7, 4, 2}, {9, 5, 4, 4, 2}, {1, 9, 3, 1, 3}, {9, 1, 3, 3, 1}, {1, 1, 1, 1, 1},
	} {
		for _, turn := range []int{0, 90, 180, 270} {
			for _, mirror := range []bool{false, true} {
				f := frame(c.w, c.h)
				f.Turn, f.Mirror = turn, mirror
				pix, w, h, err := f.Picture(c.most)
				if turn%180 != 0 {
					w, h = h, w
				}
				if err != nil || w != c.pw || h != c.ph || len(pix) != 4*w*h {
					t.Errorf("%dx%d within %d, turned %d: %dx%d, %v; want %dx%d", c.w, c.h, c.most, turn, w, h, err, c.pw, c.ph)
				}
				for i := 3; i < len(pix); i += 4 {
					if pix[i] != 0xff {
						t.Fatalf("a pixel of %+v turned %d, mirrored %v was left out", c, turn, mirror)
					}
				}
			}
		}
	}
}

func TestColours(t *testing.T) {
	for _, c := range []struct{ y, u, v, r, g, b byte }{
		{0, 128, 128, 0, 0, 0}, {255, 128, 128, 255, 255, 255}, {128, 128, 128, 128, 128, 128},
		{76, 85, 255, 254, 0, 0}, {150, 44, 21, 0, 255, 1}, {29, 255, 107, 0, 0, 254},
	} {
		r, g, b := rgb(c.y, c.u, c.v)
		for i, d := range []int{int(r) - int(c.r), int(g) - int(c.g), int(b) - int(c.b)} {
			if d < -2 || d > 2 {
				t.Errorf("%v is %d, %d, %d: channel %d is %d off", c, r, g, b, i, d)
			}
		}
	}
}

func TestLumaIsTheFrameWithNoStride(t *testing.T) {
	f := frame(5, 3)
	got, err := f.Luma(nil)
	if want := []byte{0, 1, 2, 3, 4, 10, 11, 12, 13, 14, 20, 21, 22, 23, 24}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("%v, %v", got, err)
	}
	again, err := f.Luma(got)
	if err != nil || &again[0] != &got[0] {
		t.Fatal("a buffer large enough was not used again")
	}
}

// A frame whose planes are too short for what it says is refused, and one as short as phones give is not.
func TestAFrameThatLiesIsRefused(t *testing.T) {
	for i, c := range []func(f *Frame){
		func(f *Frame) { f.Y = f.Y[:len(f.Y)-1] }, func(f *Frame) { f.U = f.U[:len(f.U)-1] },
		func(f *Frame) { f.V = f.V[:len(f.V)-1] }, func(f *Frame) { f.Width = 0 }, func(f *Frame) { f.Height = -1 },
		func(f *Frame) { f.Width = 1 << 30 }, func(f *Frame) { f.YStride = 1 }, func(f *Frame) { f.UVStride = 0 },
		func(f *Frame) { f.UVStride = 1 }, func(f *Frame) { f.UVPixel = 0 }, func(f *Frame) { f.UVPixel = -1 },
		func(f *Frame) { f.UVPixel = 1 << 40 }, func(f *Frame) { f.YStride = 1 << 40 }, func(f *Frame) { f.Turn = 45 },
		func(f *Frame) { f.Turn = -90 }, func(f *Frame) { f.Y = nil },
	} {
		f := frame(6, 4)
		if err := f.Check(); err != nil {
			t.Fatal(err)
		}
		c(f)
		if _, _, _, err := f.Picture(0); !errors.Is(err, ErrFrame) {
			t.Errorf("%d: Picture returned %v", i, err)
		}
		if _, err := f.Luma(nil); !errors.Is(err, ErrFrame) {
			t.Errorf("%d: Luma returned %v", i, err)
		}
	}
}

func FuzzFrame(f *testing.F) {
	f.Add([]byte("0123456789"), []byte("0123"), []byte("0123"), 3, 3, 3, 2, 1, 90, true, 2)
	f.Fuzz(func(_ *testing.T, y, u, v []byte, w, h, ys, uvs, uvp, turn int, mirror bool, most int) {
		fr := &Frame{Y: y, U: u, V: v, Width: w, Height: h, YStride: ys, UVStride: uvs, UVPixel: uvp, Turn: turn, Mirror: mirror}
		_, _, _, _ = fr.Picture(most)
		_, _ = fr.Luma(nil)
	})
}

func BenchmarkPicture(b *testing.B) {
	f := frame(1280, 720)
	f.Turn = 90
	for b.Loop() {
		_, _, _, _ = f.Picture(720)
	}
}

// colourful returns a w by h frame whose every pixel has a brightness, and every two by two pixels a colour, of their
// own, with the colour planes apart, in rows wider than they are, as some phones give them.
func colourful(w, h, uvPixel int) *Frame {
	cw, ch := (w+1)/2, (h+1)/2
	f := &Frame{Width: w, Height: h, YStride: w + 1, UVStride: uvPixel*cw + 3, UVPixel: uvPixel}
	f.Y = make([]byte, f.YStride*h)
	for y := range h {
		for x := range w {
			f.Y[y*f.YStride+x] = byte(60 + 7*x + 13*y)
		}
	}
	f.U, f.V = make([]byte, f.UVStride*ch), make([]byte, f.UVStride*ch)
	for y := range ch {
		for x := range cw {
			f.U[y*f.UVStride+x*uvPixel] = byte(40 + 37*x + 59*y)
			f.V[y*f.UVStride+x*uvPixel] = byte(200 - 29*x - 43*y)
		}
	}
	return f
}

// Each pixel's colour is the one its two by two have, wherever it is turned, for each way of laying out the colour.
func TestEachPixelHasItsOwnColourForEachTurn(t *testing.T) {
	const w, h = 6, 4
	for _, uvPixel := range []int{1, 2} {
		for _, turn := range []int{0, 90, 180, 270} {
			for _, mirror := range []bool{false, true} {
				f := colourful(w, h, uvPixel)
				f.Turn, f.Mirror = turn, mirror
				pix, pw, ph, err := f.Picture(0)
				if err != nil {
					t.Fatal(err)
				}
				for y := range h {
					for x := range w {
						// Where (x, y) goes, worked out apart from Picture.
						px, py := x, y
						switch turn {
						case 90:
							px, py = h-1-y, x
						case 180:
							px, py = w-1-x, h-1-y
						case 270:
							px, py = y, w-1-x
						}
						if mirror {
							px = pw - 1 - px
						}
						c := (y/2)*f.UVStride + (x/2)*uvPixel
						r, g, b := rgb(f.Y[y*f.YStride+x], f.U[c], f.V[c])
						o := 4 * (py*pw + px)
						if py >= ph || [3]byte(pix[o:o+3]) != [3]byte{r, g, b} {
							t.Fatalf("laid out %d apart, turned %d, mirrored %v: (%d, %d) is %v at (%d, %d), want %v",
								uvPixel, turn, mirror, x, y, pix[o:o+3], px, py, [3]byte{r, g, b})
						}
					}
				}
			}
		}
	}
}

// A frame larger than any camera's, or laid out wider apart than one is, is refused, though its planes hold it.
func TestAFrameBeyondTheBoundsIsRefused(t *testing.T) {
	for name, set := range map[string]func(f *Frame){
		"colour five bytes apart": func(f *Frame) {
			*f = *colourful(4, 2, 5)
		},
		"one pixel wider than the most": func(f *Frame) {
			*f = *colourful(maxSide+1, 2, 2)
		},
		"one pixel higher than the most": func(f *Frame) {
			*f = *colourful(2, maxSide+1, 2)
		},
		"rows of brightness further apart than any": func(f *Frame) {
			*f = *colourful(4, 2, 2)
			f.YStride = 4*maxSide + 1
			f.Y = make([]byte, 2*f.YStride)
		},
		"rows of colour further apart than any": func(f *Frame) {
			*f = *colourful(4, 2, 2)
			f.UVStride = 4*maxSide + 1
			f.U, f.V = make([]byte, f.UVStride), make([]byte, f.UVStride)
		},
	} {
		f := &Frame{}
		set(f)
		if err := f.Check(); err == nil {
			t.Errorf("%s: taken", name)
		}
	}
	// As large as the most, it is taken.
	if err := colourful(maxSide, 2, 2).Check(); err != nil {
		t.Errorf("a frame as wide as the most is refused: %v", err)
	}
}
