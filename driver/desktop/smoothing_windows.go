package desktop

import (
	"fmt"
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
func systemText() (text.Rendering, error) {
	tr := text.Rendering{Smoothing: text.Greyscale, Hinting: text.HintingLight, Gamma: 1.8}
	on, err := systemParam(spiGetFontSmoothing, "SPI_GETFONTSMOOTHING")
	if err != nil {
		return tr, err
	}
	if on != 0 {
		kind, kerr := systemParam(spiGetFontSmoothingType, "SPI_GETFONTSMOOTHINGTYPE")
		if kerr != nil {
			return tr, kerr
		}
		if kind == feFontSmoothingClearType {
			o, oerr := systemParam(spiGetFontSmoothingOrientation, "SPI_GETFONTSMOOTHINGORIENTATION")
			if oerr != nil {
				return tr, oerr
			}
			tr.Smoothing = text.SubpixelRGB
			if o == feFontSmoothingOrientationBGR {
				tr.Smoothing = text.SubpixelBGR
			}
		}
	}
	c, err := systemParam(spiGetFontSmoothingContrast, "SPI_GETFONTSMOOTHINGCONTRAST")
	if err != nil {
		return tr, err
	}
	if c >= 1000 && c <= 2200 {
		tr.Gamma = float32(c) / 1000
	}
	return tr, nil
}

// systemParam reads a numeric system parameter, named name in errors.
func systemParam(action uint32, name string) (uint32, error) {
	var v uint32
	ok, _, err := procSystemParametersInfo.Call(uintptr(action), 0, uintptr(unsafe.Pointer(&v)), 0)
	if ok == 0 {
		return 0, fmt.Errorf("desktop: reading how Windows draws text: SystemParametersInfoW(%s): %w", name, err)
	}
	return v, nil
}
