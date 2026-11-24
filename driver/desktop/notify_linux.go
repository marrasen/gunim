//go:build linux

package desktop

import (
	"fmt"

	"github.com/godbus/dbus/v5"

	"github.com/marrasen/gunim/driver"
)

// On Linux a message from the tray goes to the desktop's notification server over the session bus, through
// org.freedesktop.Notifications. The server hands back an ID for each message, and names that ID when the message
// is clicked or closed, so a click finds the tag of the message clicked. The tag stays in the process, kept by the
// ID; the server sees the title and body alone.

const (
	notifyName = "org.freedesktop.Notifications"
	notifyPath = dbus.ObjectPath("/org/freedesktop/Notifications")
	// notifyKept is how many of the last messages' tags the tray keeps for a click. A server may keep a message
	// waiting in its list long after it leaves the screen, and says nothing of it then, so the tags go oldest first.
	notifyKept = 64
)

// notices are the messages the tray showed, by the server's ID.
type notices struct {
	// watching says the tray hears the server's signals.
	watching bool
	// ids are the IDs kept, oldest first, and tags their tags.
	ids  []uint32
	tags map[uint32]string
	// last is the ID of the message shown last.
	last uint32
}

// TrayNotify implements [driver.TrayNotifier], through the desktop's notification server. It fails where no icon
// is up.
func (d *Driver) TrayNotify(title, body string) error { return d.TrayNotifyTagged(title, body, "") }

// TrayNotifyTagged implements [driver.TagNotifier], through the desktop's notification server. It fails where no
// icon is up.
func (d *Driver) TrayNotifyTagged(title, body, tag string) error {
	if err := driver.CheckNotifyTag(tag); err != nil {
		return err
	}
	tray.mu.Lock()
	s := tray.s
	tray.mu.Unlock()
	if s == nil {
		return fmt.Errorf("%w: there is no tray icon to show a message from", driver.ErrNoTray)
	}
	return s.notify(title, body, tag)
}

// notify shows a message, and keeps its tag by the ID the server gives it. It holds nmu until the tag is kept, so a
// click the server reports at once still finds it.
func (s *sniTray) notify(title, body, tag string) error {
	s.nmu.Lock()
	defer s.nmu.Unlock()
	if err := s.watchNotices(); err != nil {
		return fmt.Errorf("%w: %w", driver.ErrNoTray, err)
	}
	// The "default" action is what a click on the message's body does.
	var id uint32
	err := s.conn.Object(notifyName, notifyPath).Call(notifyName+".Notify", 0, programID(), uint32(0), "", title, body,
		[]string{"default", ""}, map[string]dbus.Variant{}, int32(-1)).Store(&id)
	if err != nil {
		return fmt.Errorf("%w: %w", driver.ErrNoTray, err)
	}
	n := &s.notices
	if n.tags == nil {
		n.tags = map[uint32]string{}
	}
	if _, ok := n.tags[id]; !ok {
		n.ids = append(n.ids, id)
	}
	n.tags[id] = tag
	n.last = id
	if len(n.ids) > notifyKept {
		delete(n.tags, n.ids[0])
		n.ids = n.ids[1:]
	}
	return nil
}

// watchNotices has the tray hear the server's signals, once. It runs with nmu held.
func (s *sniTray) watchNotices() error {
	if s.notices.watching {
		return nil
	}
	for _, member := range []string{"ActionInvoked", "NotificationClosed"} {
		if err := s.conn.AddMatchSignal(dbus.WithMatchSender(notifyName), dbus.WithMatchObjectPath(notifyPath),
			dbus.WithMatchInterface(notifyName), dbus.WithMatchMember(member)); err != nil {
			return err
		}
	}
	signals := make(chan *dbus.Signal, 16)
	s.conn.Signal(signals)
	go func() {
		for sig := range signals {
			if sig.Path == notifyPath {
				s.heard(sig)
			}
		}
	}()
	s.notices.watching = true
	return nil
}

// heard takes one signal from the server: a click on a message of the tray's runs OnNotifyClick and OnNotifyTag,
// and the message shown last leaving runs OnNotifyGone, each on a goroutine of its own.
func (s *sniTray) heard(sig *dbus.Signal) {
	if len(sig.Body) < 2 {
		return
	}
	id, ok := sig.Body[0].(uint32)
	if !ok {
		return
	}
	s.nmu.Lock()
	tag, ours := s.notices.tags[id]
	last := id == s.notices.last
	s.nmu.Unlock()
	if !ours {
		return
	}
	s.mu.Lock()
	t := s.t
	s.mu.Unlock()
	switch sig.Name {
	case notifyName + ".ActionInvoked":
		if action, _ := sig.Body[1].(string); action != "default" {
			return
		}
		click, tagged := t.OnNotifyClick, t.OnNotifyTag
		if click == nil && tagged == nil {
			return
		}
		go func() {
			if click != nil {
				click()
			}
			if tagged != nil {
				tagged(tag)
			}
		}()
	case notifyName + ".NotificationClosed":
		if gone := t.OnNotifyGone; last && gone != nil {
			go gone()
		}
	}
}
