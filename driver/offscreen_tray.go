package driver

import (
	"fmt"
	"sync"
)

// OffscreenTray is a tray icon kept in memory, for a test: it implements [Trayer], [TrayNotifier] and [TagNotifier],
// records each message shown, and plays the user's click on one with [OffscreenTray.Click]. A test's [Driver] embeds
// it to give the application, run with gunim's MainWith, a tray. The zero value is ready, with no icon up.
type OffscreenTray struct {
	mu      sync.Mutex
	tray    Tray
	notices []Notice
}

// Notice is a message an [OffscreenTray] showed.
type Notice struct {
	Title, Body, Tag string
}

// SetTray implements [Trayer]: the tray keeps t, and a Tray with no Icon takes the icon away.
func (o *OffscreenTray) SetTray(t Tray) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.tray = t
	return nil
}

// Tray returns the tray set last.
func (o *OffscreenTray) Tray() Tray {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.tray
}

// TrayNotify implements [TrayNotifier]: it records the message, with the tag "".
func (o *OffscreenTray) TrayNotify(title, body string) error {
	return o.TrayNotifyTagged(title, body, "")
}

// TrayNotifyTagged implements [TagNotifier]: it records the message, and fails as a real tray does, with
// [ErrNotifyTag] for a tag too long and [ErrNoTray] where no icon is up.
func (o *OffscreenTray) TrayNotifyTagged(title, body, tag string) error {
	if err := CheckNotifyTag(tag); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.tray.Icon) == 0 {
		return fmt.Errorf("%w: there is no tray icon to show a message from", ErrNoTray)
	}
	o.notices = append(o.notices, Notice{Title: title, Body: body, Tag: tag})
	return nil
}

// Notices returns the messages shown so far, oldest first.
func (o *OffscreenTray) Notices() []Notice {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]Notice(nil), o.notices...)
}

// Click plays a click on the message [OffscreenTray.Notices] lists at i: it runs the tray's OnNotifyClick and then
// OnNotifyTag with the message's tag, on the caller's goroutine, and returns once both are done. It fails for an i
// past the messages shown.
func (o *OffscreenTray) Click(i int) error {
	o.mu.Lock()
	if i < 0 || i >= len(o.notices) {
		n := len(o.notices)
		o.mu.Unlock()
		return fmt.Errorf("driver: there is no message %d of the %d shown", i, n)
	}
	tag, t := o.notices[i].Tag, o.tray
	o.mu.Unlock()
	if t.OnNotifyClick != nil {
		t.OnNotifyClick()
	}
	if t.OnNotifyTag != nil {
		t.OnNotifyTag(tag)
	}
	return nil
}

// TrayNotifyWay implements [NotifyWayer]: a click names its message, once an icon is up.
func (o *OffscreenTray) TrayNotifyWay() NotifyWay {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.tray.Icon) == 0 {
		return NotifyNone
	}
	return NotifyExact
}
