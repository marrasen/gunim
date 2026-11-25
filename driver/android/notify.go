//go:build android

package android

/*
#include <stdlib.h>
#include "glue.h"
*/
import "C"

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"runtime"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/image/draw"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/driver/internal/news"
	"github.com/marrasen/gunim/driver/internal/pace"
)

// errNotify is returned for a message Android does not take.
var errNotify = errors.New("android: the notification was not shown")

// notices posts the program's messages, no faster than Android takes them.
var notices = pace.Pace[driver.Notification]{Post: func(_ string, n driver.Notification) error { return postNotice(n) }}

// taps are the tags of the messages the user tapped.
var taps = news.Queue[string]{Max: 32}

// Notify implements [driver.Notifier].
func (d *Driver) Notify(n driver.Notification) error {
	if err := driver.CheckNotification(n); err != nil {
		return err
	}
	if n.Progress != nil {
		// The program may go on counting in it.
		p := *n.Progress
		n.Progress = &p
	}
	// One that waits for its turn would have no one to tell.
	u, up := utf16Of(n.Channel.ID)
	on := C.gunim_notify_on(up, C.int(len(u)))
	runtime.KeepAlive(u)
	if on == 0 {
		return driver.ErrNotifyOff
	}
	return notices.Update(n.Tag, n)
}

// EndNotify implements [driver.Notifier].
func (d *Driver) EndNotify(tag string) error {
	return notices.End(tag, func() error {
		u, up := utf16Of(tag)
		C.gunim_notify_end(up, C.int(len(u)))
		runtime.KeepAlive(u)
		return nil
	})
}

// OnNotification implements [driver.Notifier].
func (d *Driver) OnNotification(fn func(tag string)) { taps.Hear(fn) }

//export goTapped
func goTapped(tag *C.uint16_t, n C.int) { taps.Add(goString(tag, n)) }

// postNotice hands n to Android.
func postNotice(n driver.Notification) error {
	words := [8]string{n.Tag, n.Channel.ID, n.Channel.Name, n.Channel.Description, n.Title, n.Body, n.PublicTitle, n.PublicBody}
	var ps [8]*C.uint16_t
	var ns [8]C.int
	for i, w := range words {
		u := utf16.Encode([]rune(w))
		// One more, as C's malloc may return NULL for none.
		p := (*C.uint16_t)(C.malloc(C.size_t(2 * (len(u) + 1))))
		defer C.free(unsafe.Pointer(p))
		copy(unsafe.Slice((*uint16)(unsafe.Pointer(p)), len(u)), u)
		ps[i], ns[i] = p, C.int(len(u))
	}
	parts := -2
	if n.Progress != nil {
		parts = -1
		if p, ok := n.Progress.Parts(); ok {
			parts = p
		}
	}
	icon := iconPNG(n.Icon)
	var ip unsafe.Pointer
	if len(icon) > 0 {
		ip = unsafe.Pointer(&icon[0])
	}
	r := C.gunim_notify(&ps[0], &ns[0], C.int(n.Channel.Importance), C.int(n.Lock), C.int(parts), cBool(n.Ongoing), ip,
		C.int(len(icon)))
	runtime.KeepAlive(icon)
	switch r {
	case 0:
		return nil
	case 1:
		return driver.ErrNotifyOff
	}
	return errNotify
}

// iconSide is how wide and high a message's icon is, in pixels: 24 units
// on the sharpest screens.
const iconSide = 96

// iconPNG returns img as a PNG of an icon's size, or nil for none.
func iconPNG(img image.Image) []byte {
	if img == nil || img.Bounds().Empty() {
		return nil
	}
	b := img.Bounds()
	// It keeps its shape, in the middle.
	s := min(float64(iconSide)/float64(b.Dx()), float64(iconSide)/float64(b.Dy()))
	w, h := max(1, int(float64(b.Dx())*s)), max(1, int(float64(b.Dy())*s))
	at := image.Pt((iconSide-w)/2, (iconSide-h)/2)
	to := image.NewNRGBA(image.Rect(0, 0, iconSide, iconSide))
	draw.CatmullRom.Scale(to, image.Rectangle{Min: at, Max: at.Add(image.Pt(w, h))}, img, b, draw.Src, nil)
	var out bytes.Buffer
	if png.Encode(&out, to) != nil {
		return nil
	}
	return out.Bytes()
}
