// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The gunim Authors

// gunim change: this file is gunim's. It keeps a chromeless window's
// border: the colours and width the application asked for, and the
// colour and width the border is drawn in now, which ease towards them
// as they change and as the window is activated.

package glfw

import (
	"errors"
	"fmt"
	"image/color"
	"math"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

const (
	// _DWMWA_BORDER_COLOR sets the colour of a window's thin border on Windows 11; _DWMWA_COLOR_NONE draws none, and
	// _DWMWA_COLOR_DEFAULT the system's.
	_DWMWA_BORDER_COLOR  = 34
	_DWMWA_COLOR_NONE    = 0xfffffffe
	_DWMWA_COLOR_DEFAULT = 0xffffffff

	// borderTimer is the main window's timer that steps a change of the border's colour or width.
	borderTimer = 0x67756e6a
	borderTick  = 16
)

// systemBorder and systemInactiveBorder are the colours Windows 11 draws a window's thin border in while it is active
// and inactive, measured over magenta.
var (
	systemBorder         = color.NRGBA{R: 0x5e, G: 0x5e, B: 0x5e, A: 0x80}
	systemInactiveBorder = color.NRGBA{R: 0x5f, G: 0x5f, B: 0x5f, A: 0x7d}
)

// borderLook is a border's colour, as straight red, green, blue and alpha from 0 to 255, and its width in device
// pixels.
type borderLook struct {
	color [4]float64
	width float64
}

// windowBorder is a chromeless window's border.
type windowBorder struct {
	// active and inactive are the colours asked for, zero for the system's, and width the width in logical pixels,
	// zero for one. none asks for no border, and transition how long a change takes.
	active, inactive color.NRGBA
	width            float32
	none             bool
	transition       time.Duration

	// accent is the accent colour, and accentShown whether the user shows it on window borders.
	accent      color.NRGBA
	accentShown bool

	// now is the look drawn, which eases from from to to over the transition, started at start.
	from, to, now borderLook
	start         time.Time
	running       bool
	// reported is the width last given to widthCallback, and shown the edge the window last showed for the border.
	reported      float64
	shown         float64
	widthCallback func(w *Window, px float32)
}

// drawnWidth is the width a drawn border is drawn at: as wide as it is now, and no narrower than the edge the window
// last showed, so a shrinking border leaves no gap before the window draws its narrower edge.
func (b *windowBorder) drawnWidth() float64 { return max(b.now.width, b.shown) }

// SetBorder sets a chromeless window's border: its colours while the window is active and inactive, a zero colour
// being the system's, its width in logical pixels, zero being one, or with none, no border. A change after the first
// eases in over transition. Where the border is drawn with a drawn shadow, it takes any colour, opacity and width, and
// the window leaves its edge clear for it; where Windows draws it, it is one pixel wide and opaque. It is a gunim
// addition, on Windows alone.
func (w *Window) SetBorder(active, inactive color.NRGBA, width float32, none bool, transition time.Duration) error {
	if !_glfw.initialized {
		return NotInitialized
	}
	first := w.platform.border == nil
	if first {
		w.platform.border = &windowBorder{}
	}
	b := w.platform.border
	b.active, b.inactive, b.width, b.none, b.transition = active, inactive, width, none, transition
	if err := w.readAccent(); err != nil {
		return err
	}
	return w.retargetBorder(!first)
}

// SetBorderWidthCallback sets the function told the width, in device pixels, the border is drawn at, as it changes.
func (w *Window) SetBorderWidthCallback(f func(w *Window, px float32)) {
	if w.platform.border == nil {
		w.platform.border = &windowBorder{}
	}
	w.platform.border.widthCallback = f
}

// SetBorderShown says how wide, in device pixels, the edge the window last showed for the border is.
func (w *Window) SetBorderShown(px float32) error {
	if !_glfw.initialized {
		return NotInitialized
	}
	b := w.platform.border
	if b == nil || b.shown == float64(px) {
		return nil
	}
	was := b.drawnWidth()
	b.shown = float64(px)
	if s := w.platform.shadow; s != nil && b.drawnWidth() != was {
		s.restyled = true
		return w.placeShadow()
	}
	return nil
}

// readAccent reads the accent colour, where the border is drawn with the shadow; where Windows draws it, it follows
// the accent by itself.
func (w *Window) readAccent() error {
	b := w.platform.border
	if w.platform.shadow == nil {
		return nil
	}
	c, shown, err := accentBorder()
	if err != nil {
		return err
	}
	b.accent, b.accentShown = c, shown
	return nil
}

// borderTarget is the look the border takes for whether the window is active.
func (w *Window) borderTarget() borderLook {
	b := w.platform.border
	c, system := b.inactive, systemInactiveBorder
	if w.platform.ncActive {
		c, system = b.active, systemBorder
		if b.accentShown {
			system = b.accent
		}
	}
	if c == (color.NRGBA{}) {
		c = system
	}
	t := borderLook{color: [4]float64{float64(c.R), float64(c.G), float64(c.B), float64(c.A)}}
	if !b.none {
		lw := float64(b.width)
		if lw <= 0 {
			lw = 1
		}
		// At rest the border is whole device pixels wide, as the system's is
		t.width = math.Ceil(lw * float64(_GetDpiForWindow(w.platform.handle)) / 96)
	}
	return t
}

// retargetBorder moves the border towards the look it should have now, easing there with ease, or at once.
func (w *Window) retargetBorder(ease bool) error {
	b := w.platform.border
	t := w.borderTarget()
	if t == b.to && (b.running || b.now == t) {
		return nil
	}
	b.to = t
	if !ease || b.transition <= 0 {
		b.running = false
		b.now = t
		procKillTimer.Call(uintptr(w.platform.handle), borderTimer)
		return w.drawBorder()
	}
	b.from, b.start, b.running = b.now, time.Now(), true
	procSetTimer.Call(uintptr(w.platform.handle), borderTimer, borderTick, 0)
	return w.drawBorder()
}

// stepBorder moves a change of the border one step on, eased out.
func (w *Window) stepBorder() error {
	b := w.platform.border
	if b == nil || !b.running {
		procKillTimer.Call(uintptr(w.platform.handle), borderTimer)
		return nil
	}
	p := float64(time.Since(b.start)) / float64(b.transition)
	if p >= 1 {
		b.now, b.running = b.to, false
		procKillTimer.Call(uintptr(w.platform.handle), borderTimer)
		return w.drawBorder()
	}
	e := 1 - math.Pow(1-p, 3)
	for i := range b.now.color {
		b.now.color[i] = b.from.color[i] + (b.to.color[i]-b.from.color[i])*e
	}
	b.now.width = b.from.width + (b.to.width-b.from.width)*e
	return w.drawBorder()
}

// drawBorder shows the border as it looks now: in the drawn shadow, or through Windows.
func (w *Window) drawBorder() error {
	b := w.platform.border
	if b.now.width != b.reported && b.widthCallback != nil {
		b.reported = b.now.width
		b.widthCallback(w, float32(b.now.width))
	}
	if s := w.platform.shadow; s != nil {
		s.restyled = true
		return w.placeShadow()
	}
	if procDwmSetWindowAttribute.Find() != nil {
		return nil
	}
	asked := b.inactive
	if w.platform.ncActive {
		asked = b.active
	}
	v := uint32(_DWMWA_COLOR_DEFAULT)
	switch {
	case b.none:
		v = _DWMWA_COLOR_NONE
	case asked != (color.NRGBA{}):
		c := b.now.color
		v = uint32(c[0]+0.5) | uint32(c[1]+0.5)<<8 | uint32(c[2]+0.5)<<16
	}
	_, _, _ = procDwmSetWindowAttribute.Call(uintptr(w.platform.handle), _DWMWA_BORDER_COLOR,
		uintptr(unsafe.Pointer(&v)), unsafe.Sizeof(v))
	return nil
}

// borderMessage keeps the border with whether the window is active, the accent colour and the monitor's scale, and
// steps a change of it on.
func (w *Window) borderMessage(uMsg uint32, wParam _WPARAM) {
	if uMsg == _WM_NCACTIVATE {
		w.platform.ncActive = wParam != 0
	}
	b := w.platform.border
	if b == nil {
		return
	}
	var err error
	switch uMsg {
	case _WM_NCACTIVATE:
		err = w.retargetBorder(true)
	case _WM_SETTINGCHANGE, _WM_DWMCOLORIZATIONCOLORCHANGED:
		// The accent colour, or whether it shows on borders, may have changed
		if err = w.readAccent(); err == nil {
			err = w.retargetBorder(true)
		}
	case _WM_DPICHANGED:
		err = w.retargetBorder(false)
	case _WM_TIMER:
		if wParam == borderTimer {
			err = w.stepBorder()
		}
	}
	if err != nil {
		_glfw.errors = append(_glfw.errors, err)
	}
}

// accentBorder returns the accent colour, and true where the user shows it on title bars and window borders, as
// Windows' settings under Personalisation and Colours say. A setting that is not there, as on earlier versions, is
// off.
func accentBorder() (color.NRGBA, bool, error) {
	const at = `Software\Microsoft\Windows\DWM`
	k, err := registry.OpenKey(registry.CURRENT_USER, at, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return color.NRGBA{}, false, nil
	}
	if err != nil {
		return color.NRGBA{}, false, fmt.Errorf("glfw: read %s: %w", at, err)
	}
	defer func() { _ = k.Close() }()
	shown, _, err := k.GetIntegerValue("ColorPrevalence")
	if errors.Is(err, registry.ErrNotExist) {
		return color.NRGBA{}, false, nil
	}
	if err != nil {
		return color.NRGBA{}, false, fmt.Errorf("glfw: read ColorPrevalence in %s: %w", at, err)
	}
	if shown == 0 {
		return color.NRGBA{}, false, nil
	}
	// The accent colour is stored as 0xAABBGGRR
	c, _, err := k.GetIntegerValue("AccentColor")
	if errors.Is(err, registry.ErrNotExist) {
		return color.NRGBA{}, false, nil
	}
	if err != nil {
		return color.NRGBA{}, false, fmt.Errorf("glfw: read AccentColor in %s: %w", at, err)
	}
	return color.NRGBA{R: uint8(c), G: uint8(c >> 8), B: uint8(c >> 16), A: 0xff}, true, nil
}
