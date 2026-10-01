//go:build android

package android

/*
#cgo LDFLAGS: -landroid -lEGL -llog
#include <stdlib.h>
#include "glue.h"
*/
import "C"

import (
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/marrasen/gunim/input"
)

// The functions Java calls, through glue.c, on its UI thread. Each
// hands its news to the driver and returns quickly, as the UI thread
// has a frame of its own to keep.

//export goStart
func goStart() { theDriver.start() }

//export goSurfaceChanged
func goSurfaceChanged(w *C.ANativeWindow, width, height C.int) {
	theDriver.surfaceChanged(w, int(width), int(height))
}

//export goSurfaceDestroyed
func goSurfaceDestroyed() { theDriver.surfaceDestroyed() }

//export goMetrics
func goMetrics(density, rate C.float) { theDriver.metrics(float32(density), float64(rate)) }

//export goTouch
func goTouch(action C.int, x, y C.float, ms C.longlong) {
	theDriver.touch(int(action), float32(x), float32(y), time.Now())
}

//export goKey
func goKey(down C.uchar, code, meta, ch, repeat C.int) {
	theDriver.key(down != 0, int(code), int(meta), rune(ch), repeat > 0)
}

//export goFocus
func goFocus(focused C.uchar) { theDriver.windowFocus(focused != 0) }

//export goEdit
func goEdit(with *C.uint16_t, n, r0, r1, s0, s1, c0, c1 C.int, seq C.longlong) {
	theDriver.edit(input.TextEdit{
		Replace:   [2]int{int(r0), int(r1)},
		With:      goString(with, n),
		Selection: [2]int{int(s0), int(s1)},
		Composing: [2]int{int(c0), int(c1)},
		Seq:       uint64(seq),
		Time:      time.Now(),
	})
}

//export goText
func goText(s *C.uint16_t, n C.int) {
	theDriver.typed(input.TextInput{Text: goString(s, n), Time: time.Now()})
}

//export goComposing
func goComposing(s *C.uint16_t, n, selA, selB C.int) {
	theDriver.typed(input.Composing{Text: goString(s, n), Selected: [2]int{int(selA), int(selB)}})
}

// goString turns n UTF-16 units from Java into a Go string.
func goString(s *C.uint16_t, n C.int) string {
	if n <= 0 {
		return ""
	}
	return string(utf16.Decode(unsafe.Slice((*uint16)(unsafe.Pointer(s)), int(n))))
}

// The calls into Java, from any thread.

func showKeyboard(show bool) {
	v := 0
	if show {
		v = 1
	}
	C.gunim_show_keyboard(C.int(v))
}

// sendTextState hands Java the focused text: the text in UTF-16, and
// where the selection and the composition fall in it, in UTF-16 units.
func sendTextState(s *input.TextState, seq uint64) {
	if s == nil {
		C.gunim_clear_text_state()
		return
	}
	u := utf16.Encode([]rune(s.Text))
	at := func(b int) C.int { return C.int(utf16Len(s.Text[:max(0, min(b-s.Start, len(s.Text)))])) }
	var p *C.uint16_t
	if len(u) > 0 {
		p = (*C.uint16_t)(unsafe.Pointer(&u[0]))
	}
	C.gunim_text_state(p, C.int(len(u)), C.int(s.Start),
		at(s.Selection[0]), at(s.Selection[1]), at(s.Composing[0]), at(s.Composing[1]),
		cBool(s.Multiline), cBool(s.Secret), C.longlong(seq))
}

func cBool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// utf16Len returns how many UTF-16 units s takes.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

func clipboard() string {
	var n C.int
	p := C.gunim_get_clipboard(&n)
	if p == nil {
		return ""
	}
	defer C.free(unsafe.Pointer(p))
	return goString(p, n)
}

func setClipboard(s string) {
	u := utf16.Encode([]rune(s))
	var p *C.uint16_t
	if len(u) > 0 {
		p = (*C.uint16_t)(unsafe.Pointer(&u[0]))
	}
	C.gunim_set_clipboard(p, C.int(len(u)))
}

func finish() { C.gunim_finish() }
