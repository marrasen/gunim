//go:build linux

package desktop

import (
	"context"
	"errors"
	"fmt"
	"image"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/marrasen/gunim/driver"
	"github.com/marrasen/gunim/internal/toast"
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
	// notifyTimeout is how long connecting, or a message's caller, waits for the bus.
	notifyTimeout = 2 * time.Second
	// notifyLate is how long the server's reply is waited for once the caller has gone on, for the message's ID.
	notifyLate = time.Minute
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
	// seq numbers the messages sent, and lastSeq is last's, as a late reply may name an older message's ID.
	seq, lastSeq uint64
	// inflight counts the messages whose reply is awaited, and early are the server's signals for IDs not known yet,
	// kept while there are any.
	inflight int
	early    []*dbus.Signal
	// gen counts the bus's reports of a new owner, so an answer asked for before one is dropped.
	gen uint64
	// markup says the server reads markup in a body, and asked that it has said so.
	markup, asked bool
}

// TrayNotify implements [driver.TrayNotifier], through the desktop's notification server. It fails where no icon
// is set.
func (d *Driver) TrayNotify(title, body string) error { return d.TrayNotifyTagged(title, body, "") }

// TrayNotifyTagged implements [driver.TagNotifier], through the desktop's notification server. It fails where no
// icon is set, where the session bus is out of reach, and with [driver.ErrNotifyTimeout] where the server is slow.
func (d *Driver) TrayNotifyTagged(title, body, tag string) error {
	if err := driver.CheckNotifyTag(tag); err != nil {
		return err
	}
	t, n, err := noticeServer()
	if err != nil {
		return err
	}
	return n.notify(t, title, body, tag)
}

// TrayNotifyWay implements [driver.NotifyWayer]: the server names the message clicked, once an icon is set and where
// a server is on the bus, or the bus can start one.
func (d *Driver) TrayNotifyWay() driver.NotifyWay {
	if _, n, err := noticeServer(); err == nil && n.reachable() {
		return driver.NotifyExact
	}
	return driver.NotifyNone
}

// noticeServer is the tray set, and the connection to the server, made where there is none or the bus closed it.
func noticeServer() (*driver.Tray, *notifier, error) {
	notices.mu.Lock()
	defer notices.mu.Unlock()
	if notices.tray == nil {
		return nil, nil, fmt.Errorf("%w: there is no tray icon to show a message from", driver.ErrNoTray)
	}
	if notices.n != nil && !notices.n.conn.Connected() {
		notices.n.close()
		notices.n = nil
	}
	if notices.n == nil {
		n, err := newNotifier()
		if err != nil {
			return nil, nil, fmt.Errorf("%w: %w", driver.ErrNoTray, err)
		}
		notices.n = n
	}
	return notices.tray, notices.n, nil
}

// newNotifier connects to the session bus, and hears the server's signals. All of it takes notifyTimeout at most: the
// timer closes a connection that is not up by then.
func newNotifier() (*notifier, error) {
	ctx, cancel := context.WithCancel(context.Background())
	timer := time.AfterFunc(notifyTimeout, cancel)
	// Signals in the order they came, and no goroutine for each: anyone on the bus may send any number.
	conn, err := dbus.SessionBusPrivateNoAutoStartup(dbus.WithContext(ctx),
		dbus.WithSignalHandler(dbus.NewSequentialSignalHandler()))
	if err != nil {
		timer.Stop()
		cancel()
		return nil, err
	}
	n := &notifier{conn: conn, tags: map[uint32]string{}, clicked: map[uint32]bool{}}
	if err := n.start(ctx); err != nil {
		timer.Stop()
		n.close()
		return nil, err
	}
	if !timer.Stop() {
		n.close()
		return nil, context.DeadlineExceeded
	}
	return n, nil
}

// start says hello to the bus, and asks it for the server's signals and its unique name.
func (n *notifier) start(ctx context.Context) error {
	if err := n.conn.Auth(nil); err != nil {
		return err
	}
	if err := n.conn.Hello(); err != nil {
		return err
	}
	// Before the rules, as a connection that has closed takes the channel and never closes it.
	signals := make(chan *dbus.Signal, 16)
	n.conn.Signal(signals)
	go func() {
		for sig := range signals {
			n.heard(sig)
		}
	}()
	rules := [][]dbus.MatchOption{
		{dbus.WithMatchSender(notifyName), dbus.WithMatchObjectPath(notifyPath), dbus.WithMatchInterface(notifyName),
			dbus.WithMatchMember("ActionInvoked")},
		{dbus.WithMatchSender(notifyName), dbus.WithMatchObjectPath(notifyPath), dbus.WithMatchInterface(notifyName),
			dbus.WithMatchMember("NotificationClosed")},
		{dbus.WithMatchSender(busName), dbus.WithMatchInterface(busName), dbus.WithMatchMember("NameOwnerChanged"),
			dbus.WithMatchArg(0, notifyName)},
	}
	for _, r := range rules {
		if err := n.conn.AddMatchSignalContext(ctx, r...); err != nil {
			return err
		}
	}
	n.learnOwner(ctx)
	return nil
}

// close closes the connection, which ends the goroutine that hears its signals. It does not wait: closing waits for
// a message being written, and that never ends on a bus that has stopped reading.
func (n *notifier) close() { go func() { _ = n.conn.Close() }() }

// learnOwner asks the bus for the server's unique name, and keeps it, unless the bus reported a new one meanwhile. It
// is "" while no server is up, as one may start only for the first message. A question that failed changes nothing.
func (n *notifier) learnOwner(ctx context.Context) string {
	n.mu.Lock()
	gen := n.gen
	n.mu.Unlock()
	var owner string
	err := n.conn.BusObject().CallWithContext(ctx, busName+".GetNameOwner", 0, notifyName).Store(&owner)
	if err != nil {
		var none dbus.Error
		if !errors.As(err, &none) || none.Name != busName+".Error.NameHasNoOwner" {
			n.mu.Lock()
			defer n.mu.Unlock()
			return n.owner
		}
		owner = ""
	}
	return n.keepOwner(owner, gen)
}

// keepOwner keeps owner, the bus's answer to a question asked at gen, unless the bus reported a new owner since, and
// returns the owner known.
func (n *notifier) keepOwner(owner string, gen uint64) string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if gen == n.gen {
		n.setOwner(owner)
	}
	return n.owner
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
	n.owner, n.asked = owner, false
}

// reachable reports whether a server is on the bus, or the bus can start one.
func (n *notifier) reachable() bool {
	n.mu.Lock()
	owner := n.owner
	n.mu.Unlock()
	if owner != "" {
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), notifyTimeout)
	defer cancel()
	if n.learnOwner(ctx) != "" {
		return true
	}
	var names []string
	err := n.conn.BusObject().CallWithContext(ctx, busName+".ListActivatableNames", 0).Store(&names)
	return err == nil && slices.Contains(names, notifyName)
}

// notify shows a message, and keeps its tag by the ID the server gives it. A server slow to answer has the caller go
// on with ErrNotifyTimeout, and the ID is kept once it comes.
func (n *notifier) notify(t *driver.Tray, title, body, tag string) error {
	app := toast.Clean(t.AppName)
	if app == "" {
		app = toast.Clean(programID())
	}
	hints := map[string]dbus.Variant{}
	if img, ok := imageData(t.Icon); ok {
		hints["image-data"] = dbus.MakeVariant(img)
	}
	n.mu.Lock()
	n.seq++
	seq := n.seq
	n.inflight++
	n.mu.Unlock()
	done := make(chan error, 1)
	go func() {
		id, err := n.send(app, toast.Clean(title), toast.Clean(body), hints)
		n.sent(seq, id, tag, err)
		done <- err
	}()
	timer := time.NewTimer(notifyTimeout)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%w: %w", driver.ErrNoTray, err)
		}
		return nil
	case <-timer.C:
		return driver.ErrNotifyTimeout
	}
}

// bodyEscaper escapes a body for a server that reads markup in it.
var bodyEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// send hands the server a message, and returns its ID.
func (n *notifier) send(app, title, body string, hints map[string]dbus.Variant) (uint32, error) {
	ctx, cancel := context.WithTimeout(context.Background(), notifyLate)
	defer cancel()
	if n.readsMarkup(ctx) {
		body = bodyEscaper.Replace(body)
	}
	// The "default" action is what a click on the message's body does.
	var id uint32
	err := n.conn.Object(notifyName, notifyPath).CallWithContext(ctx, notifyName+".Notify", 0, app, uint32(0), "",
		title, body, []string{"default", ""}, hints, int32(-1)).Store(&id)
	if err != nil {
		return 0, err
	}
	n.mu.Lock()
	owner := n.owner
	n.mu.Unlock()
	if owner == "" {
		n.learnOwner(ctx)
	}
	return id, nil
}

// readsMarkup reports whether the server reads markup in a message's body, as it says of itself, asked once for each
// server. One slow to say is taken to read it: an "&amp;" shown is better than a link made of someone's text.
func (n *notifier) readsMarkup(ctx context.Context) bool {
	n.mu.Lock()
	markup, asked, gen := n.markup, n.asked, n.gen
	n.mu.Unlock()
	if asked {
		return markup
	}
	ctx, cancel := context.WithTimeout(ctx, notifyTimeout/2)
	defer cancel()
	var caps []string
	err := n.conn.Object(notifyName, notifyPath).CallWithContext(ctx, notifyName+".GetCapabilities", 0).Store(&caps)
	if err != nil {
		return errors.Is(err, context.DeadlineExceeded)
	}
	markup = slices.Contains(caps, "body-markup")
	n.mu.Lock()
	if gen == n.gen {
		n.markup, n.asked = markup, true
	}
	n.mu.Unlock()
	return markup
}

// sent takes the server's answer to the message seq: its ID, whose tag is kept, or err. A click the server reported
// before that is heard then.
func (n *notifier) sent(seq uint64, id uint32, tag string, err error) {
	n.mu.Lock()
	n.inflight--
	var replay []*dbus.Signal
	if err == nil {
		if _, ok := n.tags[id]; !ok {
			n.ids = append(n.ids, id)
		}
		n.tags[id] = tag
		delete(n.clicked, id)
		if seq > n.lastSeq {
			n.last, n.lastSeq = id, seq
		}
		if len(n.ids) > notifyKept {
			delete(n.tags, n.ids[0])
			delete(n.clicked, n.ids[0])
			n.ids = n.ids[1:]
		}
		var kept []*dbus.Signal
		for _, sig := range n.early {
			if sigID(sig) == id {
				replay = append(replay, sig)
				if sig.Sender == n.owner && isClick(sig) {
					// Now, as the message may be heard leaving before the click is replayed.
					n.clicked[id] = true
				}
			} else {
				kept = append(kept, sig)
			}
		}
		n.early = kept
	}
	if n.inflight == 0 {
		n.early = nil
	}
	n.mu.Unlock()
	for _, sig := range replay {
		n.heard(sig)
	}
}

// isClick reports whether a server's signal is a click on a message's body.
func isClick(sig *dbus.Signal) bool {
	if sig.Name != notifyName+".ActionInvoked" || len(sig.Body) < 2 {
		return false
	}
	action, _ := sig.Body[1].(string)
	return action == "default"
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
			n.gen++
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
	if n.owner == "" || sig.Sender != n.owner {
		n.mu.Unlock()
		return
	}
	tag, ours := n.tags[id]
	if !ours {
		if n.inflight > 0 {
			// Other programs' messages are named here too, so the oldest go.
			n.early = append(n.early, sig)
			if len(n.early) > notifyEarly {
				n.early = n.early[1:]
			}
		}
		n.mu.Unlock()
		return
	}
	gone := false
	if click {
		if !isClick(sig) {
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
