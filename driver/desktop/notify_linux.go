//go:build linux

package desktop

import (
	"context"
	"fmt"
	"image"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/marrasen/gunim/driver"
)

// On Linux a message from the tray goes to the desktop's notification server over the session bus, through
// org.freedesktop.Notifications, on a connection of its own, apart from the icon's: a desktop whose panel shows no
// icons still shows the messages. The server hands back an ID for each message, and names that ID when the message
// is clicked or closed, so a click finds the tag of the message clicked. The tag stays in the process, kept by the
// ID; the server sees the title, the body, the application's name and the icon's picture.
//
// A signal reaches the connection by the match rules, from the server alone, or sent to it directly, by any program
// on the bus. So each signal's sender is checked against the server's unique name, which the connection asks the bus
// for and follows as it changes.

const (
	notifyName = "org.freedesktop.Notifications"
	notifyPath = dbus.ObjectPath("/org/freedesktop/Notifications")
	busName    = "org.freedesktop.DBus"
	// notifyKept is how many of the last messages' tags the tray keeps for a click. A server may keep a message
	// waiting in its list long after it leaves the screen, and says nothing of it then, so the tags go oldest first.
	notifyKept = 64
	// notifyEarly is how many signals for IDs not known yet are kept, for a click the server reports before the
	// reply that names the message's ID has been read.
	notifyEarly = 16
	// notifyTimeout is how long a call to the bus may take.
	notifyTimeout = 2 * time.Second
)

// notices is the tray's messages, one for the process.
var notices struct {
	mu sync.Mutex
	// tray is the tray set, for its calls, or nil with no icon set.
	tray *driver.Tray
	// n is the connection to the server, made at the first message.
	n *notifier
}

// setNoticeTray keeps t for the messages' calls, and with no icon closes the connection to the server.
func setNoticeTray(t driver.Tray) {
	notices.mu.Lock()
	defer notices.mu.Unlock()
	if len(t.Icon) == 0 {
		notices.tray = nil
		if notices.n != nil {
			notices.n.close()
			notices.n = nil
		}
		return
	}
	notices.tray = &t
}

// noticeTray is the tray set, or nil.
func noticeTray() *driver.Tray {
	notices.mu.Lock()
	defer notices.mu.Unlock()
	return notices.tray
}

// notifier is a connection to the notification server, with the messages shown through it.
type notifier struct {
	conn *dbus.Conn

	mu sync.Mutex
	// owner is the server's unique name on the bus, or "" while none is known.
	owner string
	// ids are the IDs kept, oldest first, tags their tags, and clicked those clicked. last is the ID of the message
	// shown last.
	ids     []uint32
	tags    map[uint32]string
	clicked map[uint32]bool
	last    uint32
	// early are signals from the server for IDs not known yet.
	early []*dbus.Signal
}

// TrayNotify implements [driver.TrayNotifier], through the desktop's notification server. It fails where no icon
// is set.
func (d *Driver) TrayNotify(title, body string) error { return d.TrayNotifyTagged(title, body, "") }

// TrayNotifyTagged implements [driver.TagNotifier], through the desktop's notification server. It fails where no
// icon is set, and where the session bus is out of reach.
func (d *Driver) TrayNotifyTagged(title, body, tag string) error {
	if err := driver.CheckNotifyTag(tag); err != nil {
		return err
	}
	notices.mu.Lock()
	t, n := notices.tray, notices.n
	if t == nil {
		notices.mu.Unlock()
		return fmt.Errorf("%w: there is no tray icon to show a message from", driver.ErrNoTray)
	}
	if n == nil {
		var err error
		if n, err = newNotifier(); err != nil {
			notices.mu.Unlock()
			return fmt.Errorf("%w: %w", driver.ErrNoTray, err)
		}
		notices.n = n
	}
	notices.mu.Unlock()
	return n.notify(t, title, body, tag)
}

// TrayNotifyWay implements [driver.NotifyWayer]: the server names the message clicked, once an icon is set.
func (d *Driver) TrayNotifyWay() driver.NotifyWay {
	if noticeTray() == nil {
		return driver.NotifyNone
	}
	return driver.NotifyExact
}

// newNotifier connects to the session bus, and hears the server's signals.
func newNotifier() (*notifier, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	n := &notifier{conn: conn, tags: map[uint32]string{}, clicked: map[uint32]bool{}}
	rules := [][]dbus.MatchOption{
		{dbus.WithMatchSender(notifyName), dbus.WithMatchObjectPath(notifyPath), dbus.WithMatchInterface(notifyName),
			dbus.WithMatchMember("ActionInvoked")},
		{dbus.WithMatchSender(notifyName), dbus.WithMatchObjectPath(notifyPath), dbus.WithMatchInterface(notifyName),
			dbus.WithMatchMember("NotificationClosed")},
		{dbus.WithMatchSender(busName), dbus.WithMatchInterface(busName), dbus.WithMatchMember("NameOwnerChanged"),
			dbus.WithMatchArg(0, notifyName)},
	}
	for _, r := range rules {
		if err := conn.AddMatchSignal(r...); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	signals := make(chan *dbus.Signal, 16)
	conn.Signal(signals)
	go func() {
		for sig := range signals {
			n.heard(sig)
		}
	}()
	n.learnOwner()
	return n, nil
}

// close closes the connection, which ends the goroutine that hears its signals.
func (n *notifier) close() { _ = n.conn.Close() }

// learnOwner asks the bus for the server's unique name, and keeps it; it is "" while no server is up, as one may
// start only for the first message.
func (n *notifier) learnOwner() string {
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()
	var owner string
	if err := n.conn.BusObject().CallWithContext(ctx, busName+".GetNameOwner", 0, notifyName).Store(&owner); err != nil {
		owner = ""
	}
	n.mu.Lock()
	n.setOwner(owner)
	n.mu.Unlock()
	return owner
}

// setOwner takes owner as the server's unique name. A new server numbers its messages afresh, so the tags kept go.
// It runs with mu held.
func (n *notifier) setOwner(owner string) {
	if owner == n.owner {
		return
	}
	if n.owner != "" {
		n.ids, n.tags, n.clicked, n.last, n.early = nil, map[uint32]string{}, map[uint32]bool{}, 0, nil
	}
	n.owner = owner
}

// notify shows a message, and keeps its tag by the ID the server gives it. A click the server reported before the
// ID was kept is heard then.
func (n *notifier) notify(t *driver.Tray, title, body, tag string) error {
	app := t.AppName
	if app == "" {
		app = programID()
	}
	hints := map[string]dbus.Variant{}
	if img, ok := imageData(t.Icon); ok {
		hints["image-data"] = dbus.MakeVariant(img)
	}
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()
	// The "default" action is what a click on the message's body does.
	var id uint32
	err := n.conn.Object(notifyName, notifyPath).CallWithContext(ctx, notifyName+".Notify", 0, app, uint32(0), "",
		title, body, []string{"default", ""}, hints, int32(-1)).Store(&id)
	if err != nil {
		return fmt.Errorf("%w: %w", driver.ErrNoTray, err)
	}
	n.mu.Lock()
	owner := n.owner
	n.mu.Unlock()
	if owner == "" {
		n.learnOwner()
	}
	n.mu.Lock()
	if _, ok := n.tags[id]; !ok {
		n.ids = append(n.ids, id)
	}
	n.tags[id] = tag
	delete(n.clicked, id)
	n.last = id
	if len(n.ids) > notifyKept {
		delete(n.tags, n.ids[0])
		delete(n.clicked, n.ids[0])
		n.ids = n.ids[1:]
	}
	var replay []*dbus.Signal
	kept := n.early[:0]
	for _, sig := range n.early {
		if sigID(sig) == id {
			replay = append(replay, sig)
		} else {
			kept = append(kept, sig)
		}
	}
	n.early = kept
	n.mu.Unlock()
	for _, sig := range replay {
		n.heard(sig)
	}
	return nil
}

// sigID is the message ID a server's signal names, or 0.
func sigID(sig *dbus.Signal) uint32 {
	if len(sig.Body) < 2 {
		return 0
	}
	id, _ := sig.Body[0].(uint32)
	return id
}

// heard takes one signal: the server's new unique name from the bus, or from the server a click on a message of
// the tray's, which runs OnNotifyClick and OnNotifyTag, or the message shown last leaving, which runs
// OnNotifyGone, each on a goroutine of its own. A signal from anyone else is dropped.
func (n *notifier) heard(sig *dbus.Signal) {
	if sig.Name == busName+".NameOwnerChanged" {
		if sig.Sender != busName || len(sig.Body) < 3 {
			return
		}
		if name, _ := sig.Body[0].(string); name == notifyName {
			owner, _ := sig.Body[2].(string)
			n.mu.Lock()
			n.setOwner(owner)
			n.mu.Unlock()
		}
		return
	}
	click := sig.Name == notifyName+".ActionInvoked"
	if sig.Path != notifyPath || (!click && sig.Name != notifyName+".NotificationClosed") {
		return
	}
	id := sigID(sig)
	if id == 0 {
		return
	}
	n.mu.Lock()
	owner := n.owner
	n.mu.Unlock()
	if owner == "" || sig.Sender != owner {
		// The server's name may have changed unheard: ask again, once.
		if owner = n.learnOwner(); owner == "" || sig.Sender != owner {
			return
		}
	}
	n.mu.Lock()
	tag, ours := n.tags[id]
	if !ours {
		// Other programs' messages are named here too, so the oldest go.
		n.early = append(n.early, sig)
		if len(n.early) > notifyEarly {
			n.early = n.early[1:]
		}
		n.mu.Unlock()
		return
	}
	gone := false
	if click {
		if action, _ := sig.Body[1].(string); action != "default" {
			n.mu.Unlock()
			return
		}
		n.clicked[id] = true
	} else {
		// A message clicked leaves too; the click said all, as on Windows.
		gone = id == n.last && !n.clicked[id]
	}
	n.mu.Unlock()
	t := noticeTray()
	if t == nil {
		return
	}
	if click {
		onClick, onTag := t.OnNotifyClick, t.OnNotifyTag
		if onClick == nil && onTag == nil {
			return
		}
		go func() {
			if onClick != nil {
				onClick()
			}
			if onTag != nil {
				onTag(tag)
			}
		}()
	} else if gone && t.OnNotifyGone != nil {
		go t.OnNotifyGone()
	}
}

// notifyImage is the image-data hint: a picture's size, row length, alpha, bits per sample, samples per pixel and
// RGBA bytes, as the notification specification has it.
type notifyImage struct {
	W, H, Stride   int32
	Alpha          bool
	Bits, Channels int32
	Data           []byte
}

// imageData is the largest of the icon's pictures as the image-data hint, and false where there is none.
func imageData(icon []image.Image) (notifyImage, bool) {
	var best image.Image
	for _, m := range icon {
		if best == nil || m.Bounds().Dx() > best.Bounds().Dx() {
			best = m
		}
	}
	if best == nil || best.Bounds().Empty() {
		return notifyImage{}, false
	}
	b := best.Bounds()
	px := make([]byte, 0, 4*b.Dx()*b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := best.At(x, y).RGBA()
			// Straight rather than premultiplied, as the specification wants.
			if a > 0 {
				r, g, bl = r*0xffff/a, g*0xffff/a, bl*0xffff/a
			}
			px = append(px, byte(r>>8), byte(g>>8), byte(bl>>8), byte(a>>8))
		}
	}
	return notifyImage{W: int32(b.Dx()), H: int32(b.Dy()), Stride: int32(4 * b.Dx()), Alpha: true, Bits: 8, Channels: 4,
		Data: px}, true
}
