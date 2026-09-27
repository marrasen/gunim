package desktop

import (
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/marrasen/gunim/text"
)

var procSystemParametersInfo = windows.NewLazySystemDLL("user32.dll").NewProc("SystemParametersInfoW")

// SystemParametersInfo's font smoothing actions and values.
const (
	spiGetFontSmoothing            = 0x004A
	spiGetFontSmoothingType        = 0x200A
	spiGetFontSmoothingContrast    = 0x200C
	spiGetFontSmoothingOrientation = 0x2012
	feFontSmoothingClearType       = 2
	feFontSmoothingOrientationBGR  = 0
)

// systemText returns how Windows draws text: on the panel's subpixels
// where ClearType is on and greyscale otherwise, hinted, with
// ClearType's contrast as the gamma.
func systemText() text.Rendering {
	tr := text.Rendering{Smoothing: text.Greyscale, Hinting: text.HintingLight, Gamma: 1.8}
	if on, ok := systemParam(spiGetFontSmoothing); ok && on != 0 {
		if kind, ok := systemParam(spiGetFontSmoothingType); ok && kind == feFontSmoothingClearType {
			tr.Smoothing = text.SubpixelRGB
			if o, ok := systemParam(spiGetFontSmoothingOrientation); ok && o == feFontSmoothingOrientationBGR {
				tr.Smoothing = text.SubpixelBGR
			}
		}
	}
	if c, ok := systemParam(spiGetFontSmoothingContrast); ok && c >= 1000 && c <= 2200 {
		tr.Gamma = float32(c) / 1000
	}
	return tr
}

// systemParam reads a numeric system parameter.
func systemParam(action uint32) (uint32, bool) {
	var v uint32
	ok, _, _ := procSystemParametersInfo.Call(uintptr(action), 0, uintptr(unsafe.Pointer(&v)), 0)
	return v, ok != 0
}
