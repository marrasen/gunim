//go:build windows

package desktop

import (
	"errors"
	"fmt"
	"image/color"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/internal/glfw"
)

var (
	procSystemParametersInfoW = user32.NewProc("SystemParametersInfoW")
	procGetSysColor           = user32.NewProc("GetSysColor")
)

const (
	spiGetHighContrast        = 0x0042
	spiGetClientAreaAnimation = 0x1042
	hcfHighContrastOn         = 0x1

	// GetSysColor's indexes.
	colorWindow        = 5
	colorWindowText    = 8
	colorHighlight     = 13
	colorHighlightText = 14
	colorBtnFace       = 15
	colorGrayText      = 17
	colorBtnText       = 18
	colorHotLight      = 26
)

// highContrast is HIGHCONTRASTW.
type highContrast struct {
	cbSize        uint32
	dwFlags       uint32
	defaultScheme *uint16
}

// SystemLook implements [driver.LookReader].
func (d *Driver) SystemLook() (driver.SystemLook, error) { return readLook() }

// WatchSystemLook implements [driver.LookReader]. It reads the settings again on each change Windows announces, and
// calls fn only when they differ from the last ones read.
func (d *Driver) WatchSystemLook(fn func(driver.SystemLook, error)) (func(), error) {
	last, err := lookNow()
	if err != nil {
		return func() {}, err
	}
	kick := make(chan struct{}, 1)
	done := make(chan struct{})
	type result struct {
		id  uintptr
		err error
	}
	res := make(chan result, 1)
	if !d.post(func() {
		id, err := glfw.WatchSettings(func() {
			select {
			case kick <- struct{}{}:
			default:
			}
		})
		res <- result{id, err}
	}) {
		return func() {}, errStopped
	}
	r := <-res
	if r.err != nil {
		return func() {}, fmt.Errorf("desktop: watch the settings: %w", r.err)
	}
	go func() {
		for {
			select {
			case <-done:
				return
			case <-kick:
			}
			l, err := lookNow()
			if err != nil {
				fn(driver.SystemLook{}, err)
				continue
			}
			if !sameLook(l, last) {
				last = l
				fn(l, nil)
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			d.post(func() { glfw.UnwatchSettings(r.id) })
		})
	}, nil
}

// sameLook reports whether a and b say the same.
func sameLook(a, b driver.SystemLook) bool {
	if (a.Contrast == nil) != (b.Contrast == nil) {
		return false
	}
	if a.Contrast != nil && *a.Contrast != *b.Contrast {
		return false
	}
	return a.Dark == b.Dark && a.LessMotion == b.LessMotion && a.TextScale == b.TextScale
}

// lookNow reads the settings for a watch; a test puts settings of its own in its place.
var lookNow = readLook

// readLook reads the four settings.
func readLook() (driver.SystemLook, error) {
	var l driver.SystemLook
	light, err := readDWORD(`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "AppsUseLightTheme", 1)
	if err != nil {
		return l, err
	}
	l.Dark = light == 0
	var anim int32
	if ok, _, err := procSystemParametersInfoW.Call(spiGetClientAreaAnimation, 0, uintptr(unsafe.Pointer(&anim)), 0); ok == 0 {
		return l, fmt.Errorf("desktop: SystemParametersInfo(SPI_GETCLIENTAREAANIMATION): %w", err)
	}
	l.LessMotion = anim == 0
	scale, err := readDWORD(`Software\Microsoft\Accessibility`, "TextScaleFactor", 100)
	if err != nil {
		return l, err
	}
	l.TextScale = float32(scale) / 100
	hc := highContrast{cbSize: uint32(unsafe.Sizeof(highContrast{}))}
	if ok, _, err := procSystemParametersInfoW.Call(spiGetHighContrast, uintptr(hc.cbSize), uintptr(unsafe.Pointer(&hc)), 0); ok == 0 {
		return l, fmt.Errorf("desktop: SystemParametersInfo(SPI_GETHIGHCONTRAST): %w", err)
	}
	if hc.dwFlags&hcfHighContrastOn != 0 {
		l.Contrast = &driver.ContrastColors{
			Window: sysColor(colorWindow), WindowText: sysColor(colorWindowText),
			Highlight: sysColor(colorHighlight), HighlightText: sysColor(colorHighlightText),
			ButtonFace: sysColor(colorBtnFace), ButtonText: sysColor(colorBtnText),
			GrayText: sysColor(colorGrayText), HotLight: sysColor(colorHotLight),
		}
	}
	return l, nil
}

// readDWORD reads a value under HKEY_CURRENT_USER, or gives def where the user never set it.
func readDWORD(path, name string, def uint64) (uint64, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return def, nil
	}
	if err != nil {
		return 0, fmt.Errorf("desktop: open %s: %w", path, err)
	}
	defer func() { _ = k.Close() }()
	v, _, err := k.GetIntegerValue(name)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return def, nil
	}
	if err != nil {
		return 0, fmt.Errorf("desktop: read %s\\%s: %w", path, name, err)
	}
	return v, nil
}

// sysColor is the colour GetSysColor gives for index i.
func sysColor(i uintptr) color.NRGBA {
	c, _, _ := procGetSysColor.Call(i)
	return color.NRGBA{R: uint8(c), G: uint8(c >> 8), B: uint8(c >> 16), A: 0xff}
}
