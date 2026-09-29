// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

package glfw

import (
	"encoding/binary"
	"image/color"
	"testing"
)

// dib builds a bitmap w by h with a 40 byte header, bits a pixel, bottom up unless topDown, from px, which gives each
// pixel's bytes as the file keeps them.
func dib(w, h, bits int, topDown bool, px func(x, y int) []byte) []byte {
	le := binary.LittleEndian
	stride := (w*bits + 31) / 32 * 4
	b := make([]byte, 40+stride*h)
	le.PutUint32(b[0:], 40)
	le.PutUint32(b[4:], uint32(w))
	height := int32(h)
	if topDown {
		height = -height
	}
	le.PutUint32(b[8:], uint32(height))
	le.PutUint16(b[12:], 1)
	le.PutUint16(b[14:], uint16(bits))
	for row := range h {
		y := h - 1 - row
		if topDown {
			y = row
		}
		for x := range w {
			copy(b[40+row*stride+x*bits/8:], px(x, y))
		}
	}
	return b
}

func TestDecodeDIBReads24BitsBottomUp(t *testing.T) {
	b := dib(3, 2, 24, false, func(x, y int) []byte { return []byte{byte(x), byte(y), 0xaa} })
	img, err := decodeDIB(b)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := img.NRGBAAt(2, 1), (color.NRGBA{R: 0xaa, G: 1, B: 2, A: 0xff}); got != want {
		t.Fatalf("pixel 2,1 is %v, want %v", got, want)
	}
}

func TestDecodeDIBMakesAZeroAlphaOpaque(t *testing.T) {
	b := dib(2, 2, 32, true, func(x, y int) []byte { return []byte{1, 2, 3, 0} })
	img, err := decodeDIB(b)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.NRGBAAt(1, 1); got != (color.NRGBA{R: 3, G: 2, B: 1, A: 0xff}) {
		t.Fatalf("pixel is %v, want opaque", got)
	}
}

func TestDecodeDIBRefusesWhatItCannotRead(t *testing.T) {
	b := dib(2, 2, 8, false, func(int, int) []byte { return []byte{0} })
	if _, err := decodeDIB(b); err == nil {
		t.Fatal("an 8 bit bitmap decoded, want an error")
	}
	if _, err := decodeDIB(b[:20]); err == nil {
		t.Fatal("a short bitmap decoded, want an error")
	}
}
