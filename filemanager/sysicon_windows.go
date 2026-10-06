//go:build windows && (amd64 || arm64)

package filemanager

import (
	"errors"
	"fmt"
	"image"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// systemIconsHere says Windows can give the icons it shows for files.
const systemIconsHere = true

var (
	procSHGetFileInfoW = shell32.NewProc("SHGetFileInfoW")
	procSHGetImageList = shell32.NewProc("SHGetImageList")
	procGetIconInfo    = user32.NewProc("GetIconInfo")
	procDestroyIcon    = user32.NewProc("DestroyIcon")

	iidImageList = windows.GUID{Data1: 0x46EB5926, Data2: 0x582E, Data3: 0x4017,
		Data4: [8]byte{0x9F, 0xDF, 0xE8, 0x99, 0x8D, 0xAA, 0x09, 0x50}}
)

// What SHGetFileInfo and the system's image lists are asked.
const (
	shgfiSysIconIndex      = 0x4000
	shgfiUseFileAttributes = 0x10
	fileAttributeNormal    = 0x80
	fileAttributeDirectory = 0x10
	shilLarge              = 0 // 32 pixels
	shilExtraLarge         = 2 // 48 pixels
	shilJumbo              = 4 // 256 pixels
	ildTransparent         = 0x1
	vtImageListGetIcon     = 10
)

// shFileInfo is Windows' SHFILEINFOW.
type shFileInfo struct {
	icon     uintptr
	index    int32
	attrs    uint32
	display  [260]uint16
	typeName [80]uint16
}

// systemIcon reads the icon Windows shows for an item, large, for a tile, and small, for a row of the details. With
// path set, it is that file's own, as a program's is; otherwise, a folder's with dir, or a file's with the
// extension ext. Only the file named reads its icon from the file itself.
func systemIcon(path, ext string, dir bool) (large, small image.Image, err error) {
	err = onCOM(func() error {
		index, ierr := iconIndex(path, ext, dir)
		if ierr != nil {
			return ierr
		}
		var lerr error
		if large, lerr = listIcon(shilJumbo, index); lerr != nil {
			return lerr
		}
		// An icon with no picture that large comes small in the corner of the jumbo one: the extra large one is
		// the same icon at its own size.
		if b := opaqueBounds(large); b.Dx() <= 64 && b.Dy() <= 64 {
			if xl, xerr := listIcon(shilExtraLarge, index); xerr == nil {
				large = xl
			}
		}
		var serr error
		small, serr = listIcon(shilLarge, index)
		return serr
	})
	return large, small, err
}

// iconIndex is the item's icon in the system's image lists.
func iconIndex(path, ext string, dir bool) (int32, error) {
	name, attrs, flags := path, uint32(0), uint32(shgfiSysIconIndex)
	if path == "" {
		// By its kind alone, so nothing is read, and a file kept online is not downloaded.
		name, attrs, flags = "x"+ext, fileAttributeNormal, flags|shgfiUseFileAttributes
		if dir {
			name, attrs = "folder", fileAttributeDirectory
		}
	}
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}
	var info shFileInfo
	if r, _, _ := procSHGetFileInfoW.Call(uintptr(unsafe.Pointer(p)), uintptr(attrs), uintptr(unsafe.Pointer(&info)),
		unsafe.Sizeof(info), uintptr(flags)); r == 0 {
		return 0, fmt.Errorf("asking Windows for the icon of %s: SHGetFileInfo failed", name)
	}
	return info.index, nil
}

// listIcon is icon index of the system's image list list, as a picture.
func listIcon(list int, index int32) (image.Image, error) {
	var il unsafe.Pointer
	hr, _, _ := procSHGetImageList.Call(uintptr(list), uintptr(unsafe.Pointer(&iidImageList)), uintptr(unsafe.Pointer(&il)))
	if failed(uint32(hr)) || il == nil {
		return nil, fmt.Errorf("asking Windows for its icons: HRESULT %#x", uint32(hr))
	}
	defer release(il)
	var icon uintptr
	if r := comCall(il, vtImageListGetIcon, uintptr(index), ildTransparent, uintptr(unsafe.Pointer(&icon))); failed(r) || icon == 0 {
		return nil, fmt.Errorf("asking Windows for icon %d: HRESULT %#x", index, r)
	}
	defer func() { _, _, _ = procDestroyIcon.Call(icon) }()
	return iconImage(icon)
}

// iconImage copies the pixels of the icon out, with its alpha, or for an icon that has none, its mask's.
func iconImage(icon uintptr) (image.Image, error) {
	var info struct {
		isIcon           int32
		hotX, hotY       uint32
		hbmMask, hbmColr uintptr
	}
	if r, _, _ := procGetIconInfo.Call(icon, uintptr(unsafe.Pointer(&info))); r == 0 {
		return nil, errors.New("reading an icon from Windows: GetIconInfo failed")
	}
	defer func() {
		_, _, _ = procDeleteObject.Call(info.hbmMask)
		_, _, _ = procDeleteObject.Call(info.hbmColr)
	}()
	if info.hbmColr == 0 {
		return nil, errors.New("reading an icon from Windows: it has no colours")
	}
	colour, w, h, err := dibPixels(info.hbmColr)
	if err != nil {
		return nil, err
	}
	hasAlpha := false
	for i := 3; i < len(colour); i += 4 {
		if colour[i] != 0 {
			hasAlpha = true
			break
		}
	}
	var mask []byte
	if !hasAlpha && info.hbmMask != 0 {
		if m, mw, mh, err := dibPixels(info.hbmMask); err == nil && mw == w && mh >= h {
			mask = m
		}
	}
	// Blue, green, red and alpha, not premultiplied; where the mask is white the icon is clear.
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < 4*w*h; i += 4 {
		a := colour[i+3]
		if !hasAlpha {
			a = 0xff
			if i < len(mask) && mask[i] != 0 {
				a = 0
			}
		}
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = colour[i+2], colour[i+1], colour[i], a
	}
	return img, nil
}

// dibPixels reads the bitmap bmp as 32-bit pixels, top line first.
func dibPixels(bmp uintptr) (px []byte, w, h int, err error) {
	var info struct {
		typ, width, height, widthBytes int32
		planes, bitsPixel              uint16
		bits                           uintptr
	}
	if n, _, _ := procGetObjectW.Call(bmp, unsafe.Sizeof(info), uintptr(unsafe.Pointer(&info))); n == 0 {
		return nil, 0, 0, errors.New("reading an icon from Windows: GetObject failed")
	}
	w, h = int(info.width), int(info.height)
	if h < 0 {
		h = -h
	}
	if w <= 0 || h <= 0 {
		return nil, 0, 0, errors.New("reading an icon from Windows: it is empty")
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
	px = make([]byte, 4*w*h)
	dc, _, _ := procGetDC.Call(0)
	if dc == 0 {
		return nil, 0, 0, errors.New("reading an icon from Windows: GetDC failed")
	}
	lines, _, _ := procGetDIBits.Call(dc, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&px[0])),
		uintptr(unsafe.Pointer(&header)), 0)
	_, _, _ = procReleaseDC.Call(0, dc)
	if int(lines) != h {
		return nil, 0, 0, fmt.Errorf("reading an icon from Windows: GetDIBits read %d of %d lines", lines, h)
	}
	return px, w, h, nil
}

// ownIcon reports whether a file with the extension ext carries an icon of its own, as a program or a shortcut does,
// rather than its kind's.
func ownIcon(ext string) bool {
	switch strings.ToLower(ext) {
	case ".exe", ".lnk", ".ico", ".url", ".scr", ".appref-ms":
		return true
	}
	return false
}

// opaqueBounds is the part of img that is not clear.
func opaqueBounds(img image.Image) image.Rectangle {
	b := img.Bounds()
	out := image.Rectangle{}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0x0fff {
				out = out.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return out
}
