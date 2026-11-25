package driver

import (
	"fmt"
	"slices"
	"sync"
)

// OffscreenNotifier shows messages in memory, for a test: it implements [Notifier], and a test's [Driver] embeds it. The
// zero value is ready.
type OffscreenNotifier struct {
	mu    sync.Mutex
	shown []Notification
	off   bool
	fn    func(tag string)
	taps  []string
}

// SetNotifyOff has Notify fail as where the user has the messages off.
func (o *OffscreenNotifier) SetNotifyOff(off bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.off = off
}

// Notify implements [Notifier].
func (o *OffscreenNotifier) Notify(n Notification) error {
	if err := CheckNotification(n); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.off {
		return ErrNotifyOff
	}
	if i := o.at(n.Tag); i >= 0 {
		o.shown[i] = n
	} else {
		o.shown = append(o.shown, n)
	}
	return nil
}

// EndNotify implements [Notifier].
func (o *OffscreenNotifier) EndNotify(tag string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if i := o.at(tag); i >= 0 {
		o.shown = slices.Delete(o.shown, i, i+1)
	}
	return nil
}

// OnNotification implements [Notifier].
func (o *OffscreenNotifier) OnNotification(fn func(tag string)) {
	o.mu.Lock()
	o.fn = fn
	var taps []string
	if fn != nil {
		taps, o.taps = o.taps, nil
	}
	o.mu.Unlock()
	for _, t := range taps {
		fn(t)
	}
}

// Shown returns the messages that show, the oldest first.
func (o *OffscreenNotifier) Shown() []Notification {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.shown)
}

// Tap plays the user's tap on the message with tag, which takes it away unless it is ongoing. The function of
// OnNotification hears it on the caller's goroutine, or as it is set.
func (o *OffscreenNotifier) Tap(tag string) error {
	o.mu.Lock()
	i := o.at(tag)
	if i < 0 {
		o.mu.Unlock()
		return fmt.Errorf("driver: no message %q shows", tag)
	}
	if !o.shown[i].Ongoing {
		o.shown = slices.Delete(o.shown, i, i+1)
	}
	fn := o.fn
	if fn == nil {
		o.taps = append(o.taps, tag)
	}
	o.mu.Unlock()
	if fn != nil {
		fn(tag)
	}
	return nil
}

// at returns where the message with tag is in shown, or -1. It runs with mu held.
func (o *OffscreenNotifier) at(tag string) int {
	return slices.IndexFunc(o.shown, func(n Notification) bool { return n.Tag == tag })
}
