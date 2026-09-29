// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

// gunim change: this file is gunim's.

package glfw

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
)

// Bitmap compressions a clipboard's device independent bitmap comes in.
const (
	biRGB       = 0
	biBitfields = 3
)

// decodeDIB reads a device independent bitmap, as Windows keeps a picture on the clipboard: a BITMAPINFOHEADER or
// one of its longer forms, then the pixels. It reads 24 and 32 bits a pixel, uncompressed or with bit masks. A 32
// bit picture whose alpha is zero throughout is opaque, as the clipboard often leaves alpha unset.
func decodeDIB(b []byte) (*image.NRGBA, error) {
	le := binary.LittleEndian
	if len(b) < 40 {
		return nil, errors.New("glfw: clipboard bitmap is too short")
	}
	size := int(le.Uint32(b[0:]))
	w, h := int(int32(le.Uint32(b[4:]))), int(int32(le.Uint32(b[8:])))
	bits, compression := int(le.Uint16(b[14:])), le.Uint32(b[16:])
	if size < 40 || size > len(b) || w <= 0 || h == 0 {
		return nil, fmt.Errorf("glfw: clipboard bitmap header is malformed: size %d, %d by %d", size, w, h)
	}
	topDown := h < 0
	if topDown {
		h = -h
	}
	masks := [4]uint32{0xff0000, 0xff00, 0xff, 0xff000000}
	at := size
	switch {
	case compression == biBitfields && size >= 56:
		masks = [4]uint32{le.Uint32(b[40:]), le.Uint32(b[44:]), le.Uint32(b[48:]), le.Uint32(b[52:])}
	case compression == biBitfields:
		// A short header has its three colour masks after it.
		if len(b) < at+12 {
			return nil, errors.New("glfw: clipboard bitmap masks are missing")
		}
		masks = [4]uint32{le.Uint32(b[at:]), le.Uint32(b[at+4:]), le.Uint32(b[at+8:]), 0}
		at += 12
	case compression != biRGB:
		return nil, fmt.Errorf("glfw: clipboard bitmap compression %d is not supported", compression)
	}
	if bits != 24 && bits != 32 {
		return nil, fmt.Errorf("glfw: clipboard bitmaps of %d bits a pixel are not supported", bits)
	}
	if bits == 24 {
		masks[3] = 0
	}
	stride := (w*bits + 31) / 32 * 4
	if len(b) < at+stride*h {
		return nil, errors.New("glfw: clipboard bitmap pixels are cut short")
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	anyAlpha := false
	for y := range h {
		row := b[at+y*stride:]
		out := y
		if !topDown {
			out = h - 1 - y
		}
		for x := range w {
			var px uint32
			if bits == 32 {
				px = le.Uint32(row[x*4:])
			} else {
				px = uint32(row[x*3]) | uint32(row[x*3+1])<<8 | uint32(row[x*3+2])<<16
			}
			o := img.PixOffset(x, out)
			img.Pix[o], img.Pix[o+1], img.Pix[o+2] = channel(px, masks[0]), channel(px, masks[1]), channel(px, masks[2])
			a := uint8(0xff)
			if masks[3] != 0 {
				a = channel(px, masks[3])
				anyAlpha = anyAlpha || a != 0
			}
			img.Pix[o+3] = a
		}
	}
	if masks[3] != 0 && !anyAlpha {
		for i := 3; i < len(img.Pix); i += 4 {
			img.Pix[i] = 0xff
		}
	}
	return img, nil
}

// channel returns the part of px mask selects, scaled to eight bits.
func channel(px, mask uint32) uint8 {
	if mask == 0 {
		return 0
	}
	shift := 0
	for mask&1 == 0 {
		mask >>= 1
		shift++
	}
	v := (px >> shift) & mask
	if mask == 0xff {
		return uint8(v)
	}
	return uint8(v * 255 / mask)
}
