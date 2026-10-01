//go:build windows && (amd64 || arm64)

package filemanager

import (
	"errors"
	"fmt"
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	gdi32            = windows.NewLazySystemDLL("gdi32.dll")
	user32           = windows.NewLazySystemDLL("user32.dll")
	procGetObjectW   = gdi32.NewProc("GetObjectW")
	procGetDIBits    = gdi32.NewProc("GetDIBits")
	procDeleteObject = gdi32.NewProc("DeleteObject")
	procGetDC        = user32.NewProc("GetDC")
	procReleaseDC    = user32.NewProc("ReleaseDC")

	iidShellItemImageFactory = windows.GUID{Data1: 0xBCC18B79, Data2: 0xBA16, Data3: 0x442F,
		Data4: [8]byte{0x80, 0xC4, 0x8A, 0x59, 0xC3, 0x0C, 0x46, 0x3B}}
)

// The flags of IShellItemImageFactory.GetImage that keep it to a thumbnail the system already has.
const (
	siigbfThumbnailOnly = 0x08
	siigbfInCacheOnly   = 0x10
	vtGetImage          = 3
	// hrNotCached is what GetImage answers for a thumbnail not in the cache, which is no failure here.
	hrNotCached = 0x8004B200
)

// cachedShellThumb returns the thumbnail Windows already keeps of the file at path, at most size pixels across, or
// nil where it keeps none. It never reads the file, so it never downloads one kept online.
func cachedShellThumb(path string, size int) (image.Image, error) {
	var img image.Image
	err := onCOM(func() error {
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		var factory unsafe.Pointer
		hr, _, _ := procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(name)), 0,
			uintptr(unsafe.Pointer(&iidShellItemImageFactory)), uintptr(unsafe.Pointer(&factory)))
		if failed(uint32(hr)) {
			return fmt.Errorf("asking Windows for the thumbnail of %s: HRESULT %#x", path, uint32(hr))
		}
		defer release(factory)
		// SIZE travels by value, both halves in one register
		sz := uintptr(uint32(size)) | uintptr(uint32(size))<<32
		var bmp uintptr
		r := comCall(factory, vtGetImage, sz, siigbfThumbnailOnly|siigbfInCacheOnly, uintptr(unsafe.Pointer(&bmp)))
		switch {
		case r == hrNotCached || (failed(r) && bmp == 0):
			return nil
		case failed(r):
			return fmt.Errorf("asking Windows for the thumbnail of %s: HRESULT %#x", path, r)
		}
		defer func() { _, _, _ = procDeleteObject.Call(bmp) }()
		img, err = bitmapImage(bmp)
		return err
	})
	return img, err
}

// bitmapImage copies the pixels of the bitmap bmp out.
func bitmapImage(bmp uintptr) (image.Image, error) {
	var info struct {
		typ, width, height, widthBytes int32
		planes, bitsPixel              uint16
		bits                           uintptr
	}
	if n, _, _ := procGetObjectW.Call(bmp, unsafe.Sizeof(info), uintptr(unsafe.Pointer(&info))); n == 0 {
		return nil, errors.New("reading a thumbnail from Windows: GetObject failed")
	}
	w, h := int(info.width), int(info.height)
	if h < 0 {
		h = -h
	}
	if w <= 0 || h <= 0 {
		return nil, nil
	}
	header := struct {
		size                 uint32
		width, height        int32
		planes, bitCount     uint16
		compression, sizeImg uint32
		xPels, yPels         int32
		used, important      uint32
	}{width: int32(w), height: -int32(h), planes: 1, bitCount: 32}
	header.size = uint32(unsafe.Sizeof(header))
	px := make([]byte, 4*w*h)
	dc, _, _ := procGetDC.Call(0)
	if dc == 0 {
		return nil, errors.New("reading a thumbnail from Windows: GetDC failed")
	}
	lines, _, _ := procGetDIBits.Call(dc, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&px[0])),
		uintptr(unsafe.Pointer(&header)), 0)
	_, _, _ = procReleaseDC.Call(0, dc)
	if int(lines) != h {
		return nil, fmt.Errorf("reading a thumbnail from Windows: GetDIBits read %d of %d lines", lines, h)
	}
	// Blue, green, red and alpha, premultiplied; a picture with no alpha at all is opaque
	opaque := true
	for i := 3; i < len(px); i += 4 {
		if px[i] != 0 {
			opaque = false
			break
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(px); i += 4 {
		a := px[i+3]
		if opaque {
			a = 0xff
		}
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = px[i+2], px[i+1], px[i], a
	}
	return img, nil
}
